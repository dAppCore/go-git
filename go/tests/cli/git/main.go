// AX-10 CLI driver for go-git. It exercises the public Git status and
// push/pull helpers against local temporary repositories.
//
//	task -d tests/cli/git test
//	go run ./tests/cli/git
package main

import (
	core "dappco.re/go"
	gitlib "dappco.re/go/git"
)

func main() {
	r := run()
	if !r.OK {
		core.Print(core.Stderr(), "%v", r.Value)
		core.Exit(1)
	}
}

func run() core.Result {
	ctx := core.Background()

	if r := verifyStatus(ctx); !r.OK {
		return r
	}
	if r := verifyPushPull(ctx); !r.OK {
		return r
	}
	if r := verifyErrors(ctx); !r.OK {
		return r
	}

	return core.Ok(nil)
}

func verifyStatus(ctx core.Context) core.Result {
	clean := initRepo()
	if !clean.OK {
		return clean
	}
	cleanPath := clean.Value.(string)
	defer cleanupTempDir(cleanPath)

	dirty := initRepo()
	if !dirty.OK {
		return dirty
	}
	dirtyPath := dirty.Value.(string)
	defer cleanupTempDir(dirtyPath)

	if r := core.WriteFile(core.Path(dirtyPath, "README.md"), []byte("# Changed\n"), 0o644); !r.OK {
		return r
	}
	if r := core.WriteFile(core.Path(dirtyPath, "staged.txt"), []byte("staged\n"), 0o644); !r.OK {
		return r
	}
	if r := runGit(dirtyPath, "add", "staged.txt"); !r.OK {
		return r
	}
	if r := core.WriteFile(core.Path(dirtyPath, "untracked.txt"), []byte("untracked\n"), 0o644); !r.OK {
		return r
	}

	opts := gitlib.StatusOptions{
		Paths: []string{cleanPath, dirtyPath},
		Names: map[string]string{
			cleanPath: "clean-repo",
			dirtyPath: "dirty-repo",
		},
	}

	statuses := gitlib.Status(ctx, opts)
	if r := verifyStatusResults(statuses, cleanPath, dirtyPath); !r.OK {
		return r
	}

	var iterStatuses []gitlib.RepoStatus
	for status := range gitlib.StatusIter(ctx, opts) {
		iterStatuses = append(iterStatuses, status)
	}
	if r := verifyStatusResults(iterStatuses, cleanPath, dirtyPath); !r.OK {
		return r
	}

	return core.Ok(nil)
}

func verifyStatusResults(statuses []gitlib.RepoStatus, clean, dirty string) core.Result {
	if len(statuses) != 2 {
		return core.Fail(core.Errorf("expected 2 statuses, got %d", len(statuses)))
	}

	cleanStatus := statuses[0]
	if cleanStatus.Name != "clean-repo" {
		return core.Fail(core.Errorf("clean name = %q", cleanStatus.Name))
	}
	if cleanStatus.Path != clean {
		return core.Fail(core.Errorf("clean file = %q", cleanStatus.Path))
	}
	if cleanStatus.Error != nil {
		return core.Fail(cleanStatus.Error)
	}
	if cleanStatus.Branch == "" {
		return core.Fail(core.NewError("clean branch should not be empty"))
	}
	if cleanStatus.IsDirty() {
		return core.Fail(core.NewError("clean repo reported dirty"))
	}

	dirtyStatus := statuses[1]
	if dirtyStatus.Name != "dirty-repo" {
		return core.Fail(core.Errorf("dirty name = %q", dirtyStatus.Name))
	}
	if dirtyStatus.Path != dirty {
		return core.Fail(core.Errorf("dirty file = %q", dirtyStatus.Path))
	}
	if dirtyStatus.Error != nil {
		return core.Fail(dirtyStatus.Error)
	}
	if !dirtyStatus.IsDirty() {
		return core.Fail(core.NewError("dirty repo reported clean"))
	}
	if dirtyStatus.Modified != 1 || dirtyStatus.Staged != 1 || dirtyStatus.Untracked != 1 {
		return core.Fail(core.Errorf("dirty counts = modified:%d staged:%d untracked:%d", dirtyStatus.Modified, dirtyStatus.Staged, dirtyStatus.Untracked))
	}

	return core.Ok(nil)
}

func verifyPushPull(ctx core.Context) core.Result {
	root := core.MkdirTemp("", "go-git-ax10-")
	if !root.OK {
		return root
	}
	rootPath := root.Value.(string)
	defer cleanupTempDir(rootPath)

	remote := core.Path(rootPath, "remote.git")
	pushClone := core.Path(rootPath, "push")
	pullClone := core.Path(rootPath, "pull")

	if r := runGit(rootPath, "init", "--bare", remote); !r.OK {
		return r
	}
	if r := runGit(rootPath, "clone", remote, pushClone); !r.OK {
		return r
	}
	if r := configureUser(pushClone); !r.OK {
		return r
	}
	if r := commitFile(pushClone, "file.txt", "v1\n", "initial commit"); !r.OK {
		return r
	}
	if r := runGit(pushClone, "push", "-u", "origin", "HEAD"); !r.OK {
		return r
	}

	if r := runGit(rootPath, "clone", remote, pullClone); !r.OK {
		return r
	}
	if r := configureUser(pullClone); !r.OK {
		return r
	}

	if r := commitFile(pushClone, "file.txt", "v2\n", "local commit"); !r.OK {
		return r
	}
	statuses := gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pushClone}, Names: map[string]string{pushClone: "push"}})
	if r := expectSingleStatus(statuses, "push", 1, 0); !r.OK {
		return r
	}

	if r := gitlib.Push(ctx, pushClone); !r.OK {
		return r
	}
	statuses = gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pushClone}, Names: map[string]string{pushClone: "push"}})
	if r := expectSingleStatus(statuses, "push", 0, 0); !r.OK {
		return r
	}

	if r := runGit(pullClone, "fetch", "origin"); !r.OK {
		return r
	}
	statuses = gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pullClone}, Names: map[string]string{pullClone: "pull"}})
	if r := expectSingleStatus(statuses, "pull", 0, 1); !r.OK {
		return r
	}

	if r := gitlib.Pull(ctx, pullClone); !r.OK {
		return r
	}
	statuses = gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{pullClone}, Names: map[string]string{pullClone: "pull"}})
	if r := expectSingleStatus(statuses, "pull", 0, 0); !r.OK {
		return r
	}

	pushMultiple := gitlib.PushMultiple(ctx, []string{pushClone}, map[string]string{pushClone: "push"})
	if !pushMultiple.OK {
		return pushMultiple
	}
	pushResults := pushMultiple.Value.([]gitlib.PushResult)
	if len(pushResults) != 1 || !pushResults[0].Success || pushResults[0].Name != "push" {
		return core.Fail(core.Errorf("unexpected push multiple results: %+v", pushResults))
	}

	pullMultiple := gitlib.PullMultiple(ctx, []string{pullClone}, map[string]string{pullClone: "pull"})
	if !pullMultiple.OK {
		return pullMultiple
	}
	pullResults := pullMultiple.Value.([]gitlib.PullResult)
	if len(pullResults) != 1 || !pullResults[0].Success || pullResults[0].Name != "pull" {
		return core.Fail(core.Errorf("unexpected pull multiple results: %+v", pullResults))
	}

	return core.Ok(nil)
}

func expectSingleStatus(statuses []gitlib.RepoStatus, name string, ahead, behind int) core.Result {
	if len(statuses) != 1 {
		return core.Fail(core.Errorf("expected 1 status, got %d", len(statuses)))
	}
	status := statuses[0]
	if status.Error != nil {
		return core.Fail(status.Error)
	}
	if status.Name != name {
		return core.Fail(core.Errorf("status name = %q", status.Name))
	}
	if status.Ahead != ahead || status.Behind != behind {
		return core.Fail(core.Errorf("%s ahead/behind = %d/%d, want %d/%d", name, status.Ahead, status.Behind, ahead, behind))
	}
	if ahead > 0 && !status.HasUnpushed() {
		return core.Fail(core.Errorf("%s should report unpushed commits", name))
	}
	if behind > 0 && !status.HasUnpulled() {
		return core.Fail(core.Errorf("%s should report unpulled commits", name))
	}
	return core.Ok(nil)
}

func verifyErrors(ctx core.Context) core.Result {
	statuses := gitlib.Status(ctx, gitlib.StatusOptions{Paths: []string{"relative/repo"}})
	if len(statuses) != 1 || statuses[0].Error == nil {
		return core.Fail(core.NewError("relative status should fail"))
	}
	if !core.Contains(statuses[0].Error.Error(), "path must be absolute") {
		return core.Fail(core.Errorf("relative status error = %v", statuses[0].Error))
	}

	if r := gitlib.Push(ctx, "relative/repo"); r.OK {
		return core.Fail(core.NewError("relative push should fail"))
	}
	if r := gitlib.Pull(ctx, "relative/repo"); r.OK {
		return core.Fail(core.NewError("relative pull should fail"))
	}
	if !gitlib.IsNonFastForward(core.NewError("Updates were rejected: fetch first")) {
		return core.Fail(core.NewError("non-fast-forward detection should match fetch first errors"))
	}
	if gitlib.IsNonFastForward(core.NewError("connection refused")) {
		return core.Fail(core.NewError("non-fast-forward detection should ignore unrelated errors"))
	}

	return core.Ok(nil)
}

func initRepo() core.Result {
	dir := core.MkdirTemp("", "go-git-ax10-repo-")
	if !dir.OK {
		return dir
	}
	dirPath := dir.Value.(string)
	if r := runGit(dirPath, "init"); !r.OK {
		cleanupTempDir(dirPath)
		return r
	}
	if r := configureUser(dirPath); !r.OK {
		cleanupTempDir(dirPath)
		return r
	}
	if r := commitFile(dirPath, "README.md", "# Test\n", "initial commit"); !r.OK {
		cleanupTempDir(dirPath)
		return r
	}
	return core.Ok(dirPath)
}

func cleanupTempDir(target string) {
	r := core.RemoveAll(target)
	if !r.OK {
		core.Print(core.Stderr(), "cleanup %s: %v", target, r.Value)
	}
}

func configureUser(dir string) core.Result {
	if r := runGit(dir, "config", "user.email", "test@example.com"); !r.OK {
		return r
	}
	return runGit(dir, "config", "user.name", "Test User")
}

func commitFile(dir, name, content, message string) core.Result {
	if r := core.WriteFile(core.Path(dir, name), []byte(content), 0o644); !r.OK {
		return r
	}
	if r := runGit(dir, "add", name); !r.OK {
		return r
	}
	return runGit(dir, "commit", "-m", message)
}

func runGit(dir string, args ...string) core.Result {
	cmdArgs := append([]string{"env", "git"}, args...)
	cmd := &core.Cmd{Path: "/usr/bin/env", Args: cmdArgs, Dir: dir}
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return core.Fail(core.Errorf("git %s: %s: %w", core.Join(" ", args...), core.Trim(string(out)), runErr))
	}
	return core.Ok(string(out))
}
