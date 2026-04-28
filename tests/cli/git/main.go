// AX-10 CLI driver for go-git. It exercises the public Git status and
// push/pull helpers against local temporary repositories.
//
//	task -d tests/cli/git test
//	go run ./tests/cli/git
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	gitlib "dappco.re/go/git"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	if err := verifyStatus(ctx); err != nil {
		return fmt.Errorf("status: %w", err)
	}
	if err := verifyPushPull(ctx); err != nil {
		return fmt.Errorf("push/pull: %w", err)
	}
	if err := verifyErrors(ctx); err != nil {
		return fmt.Errorf("errors: %w", err)
	}

	return nil
}

func verifyStatus(ctx context.Context) error {
	clean, err := initRepo()
	if err != nil {
		return err
	}
	defer cleanupTempDir(clean)

	dirty, err := initRepo()
	if err != nil {
		return err
	}
	defer cleanupTempDir(dirty)

	if err := os.WriteFile(filepath.Join(dirty, "README.md"), []byte("# Changed\n"), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dirty, "staged.txt"), []byte("staged\n"), 0644); err != nil {
		return err
	}
	if err := runGit(dirty, "add", "staged.txt"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dirty, "untracked.txt"), []byte("untracked\n"), 0644); err != nil {
		return err
	}

	opts := gitlib.StatusOptions{
		Paths: []string{clean, dirty},
		Names: map[string]string{
			clean: "clean-repo",
			dirty: "dirty-repo",
		},
	}

	statuses := gitlib.Status(ctx, opts)
	if err := verifyStatusResults(statuses, clean, dirty); err != nil {
		return err
	}

	iterStatuses := slices.Collect(gitlib.StatusIter(ctx, opts))
	if err := verifyStatusResults(iterStatuses, clean, dirty); err != nil {
		return fmt.Errorf("iterator: %w", err)
	}

	return nil
}

func verifyStatusResults(statuses []gitlib.RepoStatus, clean, dirty string) error {
	if len(statuses) != 2 {
		return fmt.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	cleanStatus := statuses[0]
	if cleanStatus.Name != "clean-repo" {
		return fmt.Errorf("clean name = %q", cleanStatus.Name)
	}
	if cleanStatus.Path != clean {
		return fmt.Errorf("clean path = %q", cleanStatus.Path)
	}
	if cleanStatus.Error != nil {
		return cleanStatus.Error
	}
	if cleanStatus.Branch == "" {
		return errors.New("clean branch should not be empty")
	}
	if cleanStatus.IsDirty() {
		return errors.New("clean repo reported dirty")
	}

	dirtyStatus := statuses[1]
	if dirtyStatus.Name != "dirty-repo" {
		return fmt.Errorf("dirty name = %q", dirtyStatus.Name)
	}
	if dirtyStatus.Path != dirty {
		return fmt.Errorf("dirty path = %q", dirtyStatus.Path)
	}
	if dirtyStatus.Error != nil {
		return dirtyStatus.Error
	}
	if !dirtyStatus.IsDirty() {
		return errors.New("dirty repo reported clean")
	}
	if dirtyStatus.Modified != 1 || dirtyStatus.Staged != 1 || dirtyStatus.Untracked != 1 {
		return fmt.Errorf("dirty counts = modified:%d staged:%d untracked:%d", dirtyStatus.Modified, dirtyStatus.Staged, dirtyStatus.Untracked)
	}

	return nil
}

func verifyPushPull(ctx context.Context) error {
	root, err := os.MkdirTemp("", "go-git-ax10-")
	if err != nil {
		return err
	}
	defer cleanupTempDir(root)

	remote := filepath.Join(root, "remote.git")
	pushClone := filepath.Join(root, "push")
	pullClone := filepath.Join(root, "pull")

	if err := runGit(root, "init", "--bare", remote); err != nil {
		return err
	}
	if err := runGit(root, "clone", remote, pushClone); err != nil {
		return err
	}
	if err := configureUser(pushClone); err != nil {
		return err
	}
	if err := commitFile(pushClone, "file.txt", "v1\n", "initial commit"); err != nil {
		return err
	}
	if err := runGit(pushClone, "push", "-u", "origin", "HEAD"); err != nil {
		return err
	}

	if err := runGit(root, "clone", remote, pullClone); err != nil {
		return err
	}
	if err := configureUser(pullClone); err != nil {
		return err
	}

	if err := commitFile(pushClone, "file.txt", "v2\n", "local commit"); err != nil {
		return err
	}
	statuses := gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pushClone}, Names: map[string]string{pushClone: "push"}})
	if err := expectSingleStatus(statuses, "push", 1, 0); err != nil {
		return err
	}

	if err := gitlib.Push(ctx, pushClone); err != nil {
		return err
	}
	statuses = gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pushClone}, Names: map[string]string{pushClone: "push"}})
	if err := expectSingleStatus(statuses, "push", 0, 0); err != nil {
		return err
	}

	if err := runGit(pullClone, "fetch", "origin"); err != nil {
		return err
	}
	statuses = gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pullClone}, Names: map[string]string{pullClone: "pull"}})
	if err := expectSingleStatus(statuses, "pull", 0, 1); err != nil {
		return err
	}

	if err := gitlib.Pull(ctx, pullClone); err != nil {
		return err
	}
	statuses = gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pullClone}, Names: map[string]string{pullClone: "pull"}})
	if err := expectSingleStatus(statuses, "pull", 0, 0); err != nil {
		return err
	}

	pushResults, err := gitlib.PushMultiple(ctx, []string{pushClone}, map[string]string{pushClone: "push"})
	if err != nil {
		return err
	}
	if len(pushResults) != 1 || !pushResults[0].Success || pushResults[0].Name != "push" {
		return fmt.Errorf("unexpected push multiple results: %+v", pushResults)
	}

	pullResults, err := gitlib.PullMultiple(ctx, []string{pullClone}, map[string]string{pullClone: "pull"})
	if err != nil {
		return err
	}
	if len(pullResults) != 1 || !pullResults[0].Success || pullResults[0].Name != "pull" {
		return fmt.Errorf("unexpected pull multiple results: %+v", pullResults)
	}

	return nil
}

func expectSingleStatus(statuses []gitlib.RepoStatus, name string, ahead, behind int) error {
	if len(statuses) != 1 {
		return fmt.Errorf("expected 1 status, got %d", len(statuses))
	}
	status := statuses[0]
	if status.Error != nil {
		return status.Error
	}
	if status.Name != name {
		return fmt.Errorf("status name = %q", status.Name)
	}
	if status.Ahead != ahead || status.Behind != behind {
		return fmt.Errorf("%s ahead/behind = %d/%d, want %d/%d", name, status.Ahead, status.Behind, ahead, behind)
	}
	if ahead > 0 && !status.HasUnpushed() {
		return fmt.Errorf("%s should report unpushed commits", name)
	}
	if behind > 0 && !status.HasUnpulled() {
		return fmt.Errorf("%s should report unpulled commits", name)
	}
	return nil
}

func verifyErrors(ctx context.Context) error {
	statuses := gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{"relative/path"}})
	if len(statuses) != 1 || statuses[0].Error == nil {
		return errors.New("relative status path should fail")
	}
	if !strings.Contains(statuses[0].Error.Error(), "path must be absolute") {
		return fmt.Errorf("relative status error = %v", statuses[0].Error)
	}

	if err := gitlib.Push(ctx, "relative/path"); err == nil {
		return errors.New("relative push path should fail")
	}
	if err := gitlib.Pull(ctx, "relative/path"); err == nil {
		return errors.New("relative pull path should fail")
	}
	if !gitlib.IsNonFastForward(errors.New("Updates were rejected: fetch first")) {
		return errors.New("non-fast-forward detection should match fetch first errors")
	}
	if gitlib.IsNonFastForward(errors.New("connection refused")) {
		return errors.New("non-fast-forward detection should ignore unrelated errors")
	}

	return nil
}

func initRepo() (string, error) {
	dir, err := os.MkdirTemp("", "go-git-ax10-repo-")
	if err != nil {
		return "", err
	}
	if err := runGit(dir, "init"); err != nil {
		cleanupTempDir(dir)
		return "", err
	}
	if err := configureUser(dir); err != nil {
		cleanupTempDir(dir)
		return "", err
	}
	if err := commitFile(dir, "README.md", "# Test\n", "initial commit"); err != nil {
		cleanupTempDir(dir)
		return "", err
	}
	return dir, nil
}

func cleanupTempDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup %s: %v\n", path, err)
	}
}

func configureUser(dir string) error {
	if err := runGit(dir, "config", "user.email", "test@example.com"); err != nil {
		return err
	}
	return runGit(dir, "config", "user.name", "Test User")
}

func commitFile(dir, name, content, message string) error {
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		return err
	}
	if err := runGit(dir, "add", name); err != nil {
		return err
	}
	return runGit(dir, "commit", "-m", message)
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
