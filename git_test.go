package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	core "dappco.re/go/core"
)

// initTestRepo creates a temporary git repository with an initial commit.
// Returns the path to the repository.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to run %v: %s: %v", args, string(out), err)
		}
	}

	// Create a file and commit it so HEAD exists.
	if err := os.WriteFile(core.JoinPath(dir, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmds = [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to run %v: %s: %v", args, string(out), err)
		}
	}

	return dir
}

// --- RepoStatus method tests ---

func TestRepoStatus_IsDirty(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{
			name:     "clean repo",
			status:   RepoStatus{},
			expected: false,
		},
		{
			name:     "modified files",
			status:   RepoStatus{Modified: 3},
			expected: true,
		},
		{
			name:     "untracked files",
			status:   RepoStatus{Untracked: 1},
			expected: true,
		},
		{
			name:     "staged files",
			status:   RepoStatus{Staged: 2},
			expected: true,
		},
		{
			name:     "all types dirty",
			status:   RepoStatus{Modified: 1, Untracked: 2, Staged: 3},
			expected: true,
		},
		{
			name:     "only ahead is not dirty",
			status:   RepoStatus{Ahead: 5},
			expected: false,
		},
		{
			name:     "only behind is not dirty",
			status:   RepoStatus{Behind: 2},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsDirty(); tt.expected != got {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestRepoStatus_HasUnpushed(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{
			name:     "no commits ahead",
			status:   RepoStatus{Ahead: 0},
			expected: false,
		},
		{
			name:     "commits ahead",
			status:   RepoStatus{Ahead: 3},
			expected: true,
		},
		{
			name:     "behind but not ahead",
			status:   RepoStatus{Behind: 5},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.HasUnpushed(); tt.expected != got {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestRepoStatus_HasUnpulled(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{
			name:     "no commits behind",
			status:   RepoStatus{Behind: 0},
			expected: false,
		},
		{
			name:     "commits behind",
			status:   RepoStatus{Behind: 2},
			expected: true,
		},
		{
			name:     "ahead but not behind",
			status:   RepoStatus{Ahead: 3},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.HasUnpulled(); tt.expected != got {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

// --- GitError tests ---

func TestGitError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *GitError
		expected string
	}{
		{
			name:     "stderr takes precedence",
			err:      &GitError{Args: []string{"status"}, Err: errors.New("exit 1"), Stderr: "fatal: not a git repository"},
			expected: "git command \"git status\" failed: fatal: not a git repository",
		},
		{
			name:     "falls back to underlying error",
			err:      &GitError{Args: []string{"status"}, Err: errors.New("exit status 128"), Stderr: ""},
			expected: "git command \"git status\" failed: exit status 128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); tt.expected != got {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestGitError_Unwrap(t *testing.T) {
	inner := errors.New("underlying error")
	gitErr := &GitError{Err: inner, Stderr: "stderr output"}
	if got := gitErr.Unwrap(); inner != got {
		t.Fatalf("want %v, got %v", inner, got)
	}
	if !core.Is(gitErr, inner) {
		t.Fatal("expected true")
	}
}

// --- IsNonFastForward tests ---

func TestIsNonFastForward(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-fast-forward message",
			err:      errors.New("! [rejected] main -> main (non-fast-forward)"),
			expected: true,
		},
		{
			name:     "fetch first message",
			err:      errors.New("Updates were rejected because the remote contains work that you do not have locally. fetch first"),
			expected: true,
		},
		{
			name:     "tip behind message",
			err:      errors.New("Updates were rejected because the tip of your current branch is behind"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("connection refused"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNonFastForward(tt.err); tt.expected != got {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

// --- gitCommand tests with real git repos ---

func TestGitCommand_Good(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	out, err := gitCommand(context.Background(), dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default branch could be main or master depending on git config.
	branch := out
	if branch == "" {
		t.Fatal("expected non-empty")
	}
}

func TestGitCommand_Bad_InvalidDir(t *testing.T) {
	_, err := gitCommand(context.Background(), "/nonexistent/path", "status")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGitCommand_Bad_NotARepo(t *testing.T) {
	dir, _ := filepath.Abs(t.TempDir())
	_, err := gitCommand(context.Background(), dir, "status")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be a GitError with stderr.
	var gitErr *GitError
	if core.As(err, &gitErr) {
		if !strings.Contains(gitErr.Stderr, "not a git repository") {
			t.Fatalf("expected %v to contain %v", gitErr.Stderr, "not a git repository")
		}
		if !slices.Equal([]string{"status"}, gitErr.Args) {
			t.Fatalf("want %v, got %v", []string{"status"}, gitErr.Args)
		}
	}
}

func TestGitCommand_Bad_RelativePath(t *testing.T) {
	_, err := gitCommand(context.Background(), "relative/path", "status")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPush_Bad_RelativePath(t *testing.T) {
	err := Push(context.Background(), "relative/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPull_Bad_RelativePath(t *testing.T) {
	err := Pull(context.Background(), "relative/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- getStatus integration tests ---

func TestGetStatus_Good_CleanRepo(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	status := getStatus(context.Background(), dir, "test-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if "test-repo" != status.Name {
		t.Fatalf("want %v, got %v", "test-repo", status.Name)
	}
	if dir != status.Path {
		t.Fatalf("want %v, got %v", dir, status.Path)
	}
	if status.Branch == "" {
		t.Fatal("expected non-empty")
	}
	if status.IsDirty() {
		t.Fatal("expected false")
	}
}

func TestGetStatus_Good_ModifiedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Modify the existing tracked file.
	if err := os.WriteFile(core.JoinPath(dir, "README.md"), []byte("# Modified\n"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "modified-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Modified {
		t.Fatalf("want %v, got %v", 1, status.Modified)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_UntrackedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Create a new untracked file.
	if err := os.WriteFile(core.JoinPath(dir, "newfile.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "untracked-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Untracked {
		t.Fatalf("want %v, got %v", 1, status.Untracked)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_StagedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Create and stage a new file.
	if err := os.WriteFile(core.JoinPath(dir, "staged.txt"), []byte("staged"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "staged-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Staged {
		t.Fatalf("want %v, got %v", 1, status.Staged)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_MixedChanges(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Create untracked file.
	if err := os.WriteFile(core.JoinPath(dir, "untracked.txt"), []byte("new"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Modify tracked file.
	if err := os.WriteFile(core.JoinPath(dir, "README.md"), []byte("# Changed\n"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create and stage another file.
	if err := os.WriteFile(core.JoinPath(dir, "staged.txt"), []byte("staged"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "mixed-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Modified {
		t.Fatalf("expected 1 modified file: want %v, got %v", 1, status.Modified)
	}
	if 1 != status.Untracked {
		t.Fatalf("expected 1 untracked file: want %v, got %v", 1, status.Untracked)
	}
	if 1 != status.Staged {
		t.Fatalf("expected 1 staged file: want %v, got %v", 1, status.Staged)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_DeletedTrackedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Delete the tracked file (unstaged deletion).
	if err := os.Remove(core.JoinPath(dir, "README.md")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "deleted-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Modified {
		t.Fatalf("deletion in working tree counts as modified: want %v, got %v", 1, status.Modified)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_StagedDeletion(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Stage a deletion.
	cmd := exec.Command("git", "rm", "README.md")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "staged-delete-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Staged {
		t.Fatalf("staged deletion counts as staged: want %v, got %v", 1, status.Staged)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_MergeConflict(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Create a conflicting change on a feature branch.
	cmd := exec.Command("git", "checkout", "-b", "feature")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := os.WriteFile(core.JoinPath(dir, "README.md"), []byte("# Feature\n"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "feature change"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to run %v: %s: %v", args, string(out), err)
		}
	}

	// Return to the original branch and create a divergent change.
	cmd = exec.Command("git", "checkout", "-")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := os.WriteFile(core.JoinPath(dir, "README.md"), []byte("# Main\n"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "main change"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to run %v: %s: %v", args, string(out), err)
		}
	}

	cmd = exec.Command("git", "merge", "feature")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected the merge to conflict: expected error, got nil")
	}
	if !strings.Contains(string(out), "CONFLICT") {
		t.Fatalf("expected %v to contain %v", string(out), "CONFLICT")
	}

	status := getStatus(context.Background(), dir, "conflicted-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Staged {
		t.Fatalf("unmerged paths count as staged: want %v, got %v", 1, status.Staged)
	}
	if 1 != status.Modified {
		t.Fatalf("unmerged paths count as modified: want %v, got %v", 1, status.Modified)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Bad_InvalidPath(t *testing.T) {
	status := getStatus(context.Background(), "/nonexistent/path", "bad-repo")
	if status.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if "bad-repo" != status.Name {
		t.Fatalf("want %v, got %v", "bad-repo", status.Name)
	}
}

func TestGetStatus_Bad_RelativePath(t *testing.T) {
	status := getStatus(context.Background(), "relative/path", "rel-repo")
	if status.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(status.Error.Error(), "path must be absolute") {
		t.Fatalf("expected %v to contain %v", status.Error.Error(), "path must be absolute")
	}
	if "rel-repo" != status.Name {
		t.Fatalf("want %v, got %v", "rel-repo", status.Name)
	}
}

// --- Status (parallel multi-repo) tests ---

func TestStatus_Good_MultipleRepos(t *testing.T) {
	dir1, _ := filepath.Abs(initTestRepo(t))
	dir2, _ := filepath.Abs(initTestRepo(t))

	// Make dir2 dirty.
	if err := os.WriteFile(core.JoinPath(dir2, "extra.txt"), []byte("extra"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results := Status(context.Background(), StatusOptions{
		Paths: []string{dir1, dir2},
		Names: map[string]string{
			dir1: "clean-repo",
			dir2: "dirty-repo",
		},
	})

	if len(results) != 2 {
		t.Fatalf("want %v, got %v", 2, len(results))
	}

	if "clean-repo" != results[0].Name {
		t.Fatalf("want %v, got %v", "clean-repo", results[0].Name)
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if results[0].IsDirty() {
		t.Fatal("expected false")
	}

	if "dirty-repo" != results[1].Name {
		t.Fatalf("want %v, got %v", "dirty-repo", results[1].Name)
	}
	if results[1].Error != nil {
		t.Fatalf("unexpected error: %v", results[1].Error)
	}
	if !results[1].IsDirty() {
		t.Fatal("expected true")
	}
}

func TestStatusIter_Good_MultipleRepos(t *testing.T) {
	dir1, _ := filepath.Abs(initTestRepo(t))
	dir2, _ := filepath.Abs(initTestRepo(t))

	if err := os.WriteFile(core.JoinPath(dir2, "extra.txt"), []byte("extra"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses := slices.Collect(StatusIter(context.Background(), StatusOptions{
		Paths: []string{dir1, dir2},
		Names: map[string]string{
			dir1: "clean-repo",
			dir2: "dirty-repo",
		},
	}))

	if len(statuses) != 2 {
		t.Fatalf("want %v, got %v", 2, len(statuses))
	}
	if "clean-repo" != statuses[0].Name {
		t.Fatalf("want %v, got %v", "clean-repo", statuses[0].Name)
	}
	if "dirty-repo" != statuses[1].Name {
		t.Fatalf("want %v, got %v", "dirty-repo", statuses[1].Name)
	}
	if statuses[0].IsDirty() {
		t.Fatal("expected false")
	}
	if !statuses[1].IsDirty() {
		t.Fatal("expected true")
	}
}

func TestStatus_Good_EmptyPaths(t *testing.T) {
	results := Status(context.Background(), StatusOptions{
		Paths: []string{},
	})
	if len(results) != 0 {
		t.Fatalf("want %v, got %v", 0, len(results))
	}
}

func TestStatus_Good_NameFallback(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// No name mapping — path should be used as name.
	results := Status(context.Background(), StatusOptions{
		Paths: []string{dir},
		Names: map[string]string{},
	})

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if dir != results[0].Name {
		t.Fatalf("name should fall back to path: want %v, got %v", dir, results[0].Name)
	}
}

func TestStatus_Good_WithErrors(t *testing.T) {
	validDir, _ := filepath.Abs(initTestRepo(t))
	invalidDir, _ := filepath.Abs("/nonexistent/path")

	results := Status(context.Background(), StatusOptions{
		Paths: []string{validDir, invalidDir},
		Names: map[string]string{
			validDir:   "good",
			invalidDir: "bad",
		},
	})

	if len(results) != 2 {
		t.Fatalf("want %v, got %v", 2, len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if results[1].Error == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- PushMultiple tests ---

func TestPushMultiple_Good_NoRemote(t *testing.T) {
	// Push without a remote will fail but we can test the result structure.
	dir, _ := filepath.Abs(initTestRepo(t))

	results, err := PushMultiple(context.Background(), []string{dir}, map[string]string{
		dir: "test-repo",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if "test-repo" != results[0].Name {
		t.Fatalf("want %v, got %v", "test-repo", results[0].Name)
	}
	if dir != results[0].Path {
		t.Fatalf("want %v, got %v", dir, results[0].Path)
	}
	// Push without remote should fail.
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPullMultiple_Good_NoRemote(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	results, err := PullMultiple(context.Background(), []string{dir}, map[string]string{
		dir: "test-repo",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if "test-repo" != results[0].Name {
		t.Fatalf("want %v, got %v", "test-repo", results[0].Name)
	}
	if dir != results[0].Path {
		t.Fatalf("want %v, got %v", dir, results[0].Path)
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPushMultiple_Good_NameFallback(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	results, err := PushMultiple(context.Background(), []string{dir}, map[string]string{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if dir != results[0].Name {
		t.Fatalf("name should fall back to path: want %v, got %v", dir, results[0].Name)
	}
}

func TestPullMultiple_Good_NameFallback(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	results, err := PullMultiple(context.Background(), []string{dir}, map[string]string{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if dir != results[0].Name {
		t.Fatalf("name should fall back to path: want %v, got %v", dir, results[0].Name)
	}
}

func TestPushMultipleIter_Good_NameFallback(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	results := slices.Collect(PushMultipleIter(context.Background(), []string{dir}, map[string]string{}))

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if dir != results[0].Name {
		t.Fatalf("name should fall back to path: want %v, got %v", dir, results[0].Name)
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPullMultipleIter_Good_NameFallback(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	results := slices.Collect(PullMultipleIter(context.Background(), []string{dir}, map[string]string{}))

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if dir != results[0].Name {
		t.Fatalf("name should fall back to path: want %v, got %v", dir, results[0].Name)
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPushMultiple_Bad_RelativePath(t *testing.T) {
	validDir, _ := filepath.Abs(initTestRepo(t))
	relativePath := "relative/repo"

	results, err := PushMultiple(context.Background(), []string{relativePath, validDir}, map[string]string{
		validDir: "valid-repo",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(results) != 2 {
		t.Fatalf("want %v, got %v", 2, len(results))
	}
	if relativePath != results[0].Path {
		t.Fatalf("want %v, got %v", relativePath, results[0].Path)
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(results[0].Error.Error(), "path must be absolute") {
		t.Fatalf("expected %v to contain %v", results[0].Error.Error(), "path must be absolute")
	}
	if validDir != results[1].Path {
		t.Fatalf("want %v, got %v", validDir, results[1].Path)
	}
}

func TestPullMultiple_Bad_RelativePath(t *testing.T) {
	validDir, _ := filepath.Abs(initTestRepo(t))
	relativePath := "relative/repo"

	results, err := PullMultiple(context.Background(), []string{relativePath, validDir}, map[string]string{
		validDir: "valid-repo",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(results) != 2 {
		t.Fatalf("want %v, got %v", 2, len(results))
	}
	if relativePath != results[0].Path {
		t.Fatalf("want %v, got %v", relativePath, results[0].Path)
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(results[0].Error.Error(), "path must be absolute") {
		t.Fatalf("expected %v to contain %v", results[0].Error.Error(), "path must be absolute")
	}
	if validDir != results[1].Path {
		t.Fatalf("want %v, got %v", validDir, results[1].Path)
	}
}

// --- Pull tests ---

func TestPull_Bad_NoRemote(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))
	err := Pull(context.Background(), dir)
	if err == nil {
		t.Fatal("pull without remote should fail: expected error, got nil")
	}
}

// --- Push tests ---

func TestPush_Bad_NoRemote(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))
	err := Push(context.Background(), dir)
	if err == nil {
		t.Fatal("push without remote should fail: expected error, got nil")
	}
}

// --- Context cancellation test ---

func TestGetStatus_Good_ContextCancellation(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	status := getStatus(ctx, dir, "cancelled-repo")
	// With a cancelled context, the git commands should fail.
	if status.Error == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- getAheadBehind with a tracking branch ---

func TestGetAheadBehind_Good_WithUpstream(t *testing.T) {
	// Create a bare remote and a clone to test ahead/behind counts.
	bareDir, _ := filepath.Abs(t.TempDir())
	cloneDir, _ := filepath.Abs(t.TempDir())

	// Initialise the bare repo.
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Clone it.
	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Configure user in clone.
	for _, args := range [][]string{
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Create initial commit and push.
	if err := os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v1"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
		{"git", "push", "origin", "HEAD"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("command %v failed: %s: %v", args, string(out), err)
		}
	}

	// Make a local commit without pushing (ahead by 1).
	if err := os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v2"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "local commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 1 != ahead {
		t.Fatalf("should be 1 commit ahead: want %v, got %v", 1, ahead)
	}
	if 0 != behind {
		t.Fatalf("should not be behind: want %v, got %v", 0, behind)
	}
}

// --- Renamed file detection ---

func TestGetStatus_Good_RenamedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Rename via git mv (stages the rename).
	cmd := exec.Command("git", "mv", "README.md", "GUIDE.md")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "renamed-repo")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Staged {
		t.Fatalf("rename should count as staged: want %v, got %v", 1, status.Staged)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_TypeChangedFile_WorkingTree(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Replace the tracked file with a symlink to trigger a working-tree type change.
	if err := os.Remove(core.JoinPath(dir, "README.md")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Symlink("/etc/hosts", core.JoinPath(dir, "README.md")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "typechanged-working-tree")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Modified {
		t.Fatalf("type change in working tree counts as modified: want %v, got %v", 1, status.Modified)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}

func TestGetStatus_Good_TypeChangedFile_Staged(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Stage a type change by replacing the tracked file with a symlink and adding it.
	if err := os.Remove(core.JoinPath(dir, "README.md")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Symlink("/etc/hosts", core.JoinPath(dir, "README.md")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := exec.Command("git", "add", "README.md")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := getStatus(context.Background(), dir, "typechanged-staged")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 1 != status.Staged {
		t.Fatalf("type change in the index counts as staged: want %v, got %v", 1, status.Staged)
	}
	if !status.IsDirty() {
		t.Fatal("expected true")
	}
}
