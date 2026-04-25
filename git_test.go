package git

import (
	"context"
	"os/exec" // Note: test-only intrinsic - drives git CLI fixtures for repository setup.
	"slices"
	"strings"
	"testing"

	core "dappco.re/go/core"
)

func testFS() *core.Fs {
	return (&core.Fs{}).New("/")
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if r := testFS().Write(path, content); !r.OK {
		t.Fatalf("unexpected error: %v", r.Value)
	}
}

func deleteTestPath(t *testing.T, path string) {
	t.Helper()
	if r := testFS().Delete(path); !r.OK {
		t.Fatalf("unexpected error: %v", r.Value)
	}
}

func gitTestOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := gitTestOutput(dir, args...)
	if err != nil {
		t.Fatalf("failed to run git %v: %s: %v", args, string(out), err)
	}
}

func gitHashObject(t *testing.T, dir, content string) string {
	t.Helper()
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to hash git object: %s: %v", string(out), err)
	}
	return strings.TrimSpace(string(out))
}

func stageSymlink(t *testing.T, dir, path, target string) {
	t.Helper()
	blob := gitHashObject(t, dir, target)
	runTestGit(t, dir, "update-index", "--cacheinfo", "120000", blob, path)
}

func checkoutSymlink(t *testing.T, dir, path, target string) {
	t.Helper()
	deleteTestPath(t, core.JoinPath(dir, path))
	stageSymlink(t, dir, path, target)
	runTestGit(t, dir, "checkout-index", "-f", path)
}

func replaceWorkingTreeWithSymlink(t *testing.T, dir, path, target string) {
	t.Helper()
	checkoutSymlink(t, dir, path, target)
	runTestGit(t, dir, "reset", "--mixed", "HEAD")
}

// initTestRepo creates a temporary git repository with an initial commit.
// Returns the path to the repository.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		runTestGit(t, dir, args...)
	}

	// Create a file and commit it so HEAD exists.
	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Test\n")

	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial commit"},
	} {
		runTestGit(t, dir, args...)
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
			err:      &GitError{Args: []string{"status"}, Err: core.NewError("exit 1"), Stderr: "fatal: not a git repository"},
			expected: "git command \"git status\" failed: fatal: not a git repository",
		},
		{
			name:     "falls back to underlying error",
			err:      &GitError{Args: []string{"status"}, Err: core.NewError("exit status 128"), Stderr: ""},
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
	inner := core.NewError("underlying error")
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
			err:      core.NewError("! [rejected] main -> main (non-fast-forward)"),
			expected: true,
		},
		{
			name:     "fetch first message",
			err:      core.NewError("Updates were rejected because the remote contains work that you do not have locally. fetch first"),
			expected: true,
		},
		{
			name:     "tip behind message",
			err:      core.NewError("Updates were rejected because the tip of your current branch is behind"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      core.NewError("connection refused"),
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
	dir := initTestRepo(t)

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
	dir := t.TempDir()
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
	dir := initTestRepo(t)

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
	dir := initTestRepo(t)

	// Modify the existing tracked file.
	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Modified\n")

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
	dir := initTestRepo(t)

	// Create a new untracked file.
	writeTestFile(t, core.JoinPath(dir, "newfile.txt"), "hello")

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
	dir := initTestRepo(t)

	// Create and stage a new file.
	writeTestFile(t, core.JoinPath(dir, "staged.txt"), "staged")
	runTestGit(t, dir, "add", "staged.txt")

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
	dir := initTestRepo(t)

	// Create untracked file.
	writeTestFile(t, core.JoinPath(dir, "untracked.txt"), "new")

	// Modify tracked file.
	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Changed\n")

	// Create and stage another file.
	writeTestFile(t, core.JoinPath(dir, "staged.txt"), "staged")
	runTestGit(t, dir, "add", "staged.txt")

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
	dir := initTestRepo(t)

	// Delete the tracked file (unstaged deletion).
	deleteTestPath(t, core.JoinPath(dir, "README.md"))

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
	dir := initTestRepo(t)

	// Stage a deletion.
	runTestGit(t, dir, "rm", "README.md")

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
	dir := initTestRepo(t)

	// Create a conflicting change on a feature branch.
	runTestGit(t, dir, "checkout", "-b", "feature")

	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Feature\n")
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "feature change"},
	} {
		runTestGit(t, dir, args...)
	}

	// Return to the original branch and create a divergent change.
	runTestGit(t, dir, "checkout", "-")

	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Main\n")
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "main change"},
	} {
		runTestGit(t, dir, args...)
	}

	out, err := gitTestOutput(dir, "merge", "feature")
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
	dir1 := initTestRepo(t)
	dir2 := initTestRepo(t)

	// Make dir2 dirty.
	writeTestFile(t, core.JoinPath(dir2, "extra.txt"), "extra")

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
	dir1 := initTestRepo(t)
	dir2 := initTestRepo(t)

	writeTestFile(t, core.JoinPath(dir2, "extra.txt"), "extra")

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
	dir := initTestRepo(t)

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
	validDir := initTestRepo(t)
	invalidDir := "/nonexistent/path"

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
	dir := initTestRepo(t)

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
	dir := initTestRepo(t)

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
	dir := initTestRepo(t)

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
	dir := initTestRepo(t)

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
	dir := initTestRepo(t)

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
	dir := initTestRepo(t)

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
	validDir := initTestRepo(t)
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
	validDir := initTestRepo(t)
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
	dir := initTestRepo(t)
	err := Pull(context.Background(), dir)
	if err == nil {
		t.Fatal("pull without remote should fail: expected error, got nil")
	}
}

// --- Push tests ---

func TestPush_Bad_NoRemote(t *testing.T) {
	dir := initTestRepo(t)
	err := Push(context.Background(), dir)
	if err == nil {
		t.Fatal("push without remote should fail: expected error, got nil")
	}
}

// --- Context cancellation test ---

func TestGetStatus_Good_ContextCancellation(t *testing.T) {
	dir := initTestRepo(t)

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
	bareDir := t.TempDir()
	cloneDir := t.TempDir()

	// Initialise the bare repo.
	runTestGit(t, bareDir, "init", "--bare")

	// Clone it.
	runTestGit(t, "", "clone", bareDir, cloneDir)

	// Configure user in clone.
	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		runTestGit(t, cloneDir, args...)
	}

	// Create initial commit and push.
	writeTestFile(t, core.JoinPath(cloneDir, "file.txt"), "v1")
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "initial"},
		{"push", "origin", "HEAD"},
	} {
		runTestGit(t, cloneDir, args...)
	}

	// Make a local commit without pushing (ahead by 1).
	writeTestFile(t, core.JoinPath(cloneDir, "file.txt"), "v2")
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "local commit"},
	} {
		runTestGit(t, cloneDir, args...)
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
	dir := initTestRepo(t)

	// Rename via git mv (stages the rename).
	runTestGit(t, dir, "mv", "README.md", "GUIDE.md")

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
	dir := initTestRepo(t)

	// Replace the tracked file with a symlink to trigger a working-tree type change.
	replaceWorkingTreeWithSymlink(t, dir, "README.md", "/etc/hosts")

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
	dir := initTestRepo(t)

	// Stage a type change by replacing the tracked file with a git symlink entry.
	checkoutSymlink(t, dir, "README.md", "/etc/hosts")

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
