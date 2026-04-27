package git

import (
	"context"
	"errors"
	"os"
	"os/exec" // Note: test-only intrinsic - drives git CLI fixtures for repository setup.
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func deleteTestPath(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func configureTestGit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		runTestGit(t, dir, args...)
	}
}

func commitTestFile(t *testing.T, dir, path, content, message string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, path), content)
	runTestGit(t, dir, "add", path)
	runTestGit(t, dir, "commit", "-m", message)
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
	deleteTestPath(t, filepath.Join(dir, path))
	stageSymlink(t, dir, path, target)
	runTestGit(t, dir, "checkout-index", "-f", path)
}

func replaceWorkingTreeWithSymlink(t *testing.T, dir, path, target string) {
	t.Helper()
	checkoutSymlink(t, dir, path, target)
	runTestGit(t, dir, "reset", "--mixed", "HEAD")
}

// initTestRepo creates a temporary git repository with an initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runTestGit(t, dir, "init")
	configureTestGit(t, dir)
	commitTestFile(t, dir, "README.md", "# Test\n", "initial commit")

	return dir
}

func initBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, dir, "init", "--bare")
	return dir
}

func cloneTestRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, "", "clone", remote, dir)
	configureTestGit(t, dir)
	return dir
}

func initRemoteRepo(t *testing.T) (string, string) {
	t.Helper()
	remote := initBareRemote(t)
	clone := cloneTestRepo(t, remote)
	commitTestFile(t, clone, "file.txt", "v1", "initial")
	runTestGit(t, clone, "push", "origin", "HEAD")
	return remote, clone
}

func initPushableRepo(t *testing.T) string {
	t.Helper()
	_, clone := initRemoteRepo(t)
	commitTestFile(t, clone, "file.txt", "v2", "local commit")
	return clone
}

func initPullableRepo(t *testing.T) string {
	t.Helper()
	remote, upstream := initRemoteRepo(t)
	clone := cloneTestRepo(t, remote)
	commitTestFile(t, upstream, "file.txt", "v2", "remote commit")
	runTestGit(t, upstream, "push", "origin", "HEAD")
	return clone
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected %v to contain %v", err.Error(), want)
	}
}

func assertGitError(t *testing.T, err error, wantArg string) *GitError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("expected *GitError, got %T", err)
	}
	if wantArg != "" && !slices.Contains(gitErr.Args, wantArg) {
		t.Fatalf("expected args %v to contain %v", gitErr.Args, wantArg)
	}
	if strings.TrimSpace(gitErr.Stderr) == "" {
		t.Fatalf("expected non-empty stderr for %v", gitErr.Args)
	}
	return gitErr
}

func localSymlinkTarget(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "symlink-target")
	writeTestFile(t, target, "symlink target")

	probe := filepath.Join(t.TempDir(), "symlink-probe")
	if err := os.Symlink(target, probe); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return target
}

func TestGit_RepoStatusIsDirty_Good(t *testing.T) {
	tests := []struct {
		name   string
		status RepoStatus
	}{
		{name: "modified files", status: RepoStatus{Modified: 3}},
		{name: "untracked files", status: RepoStatus{Untracked: 1}},
		{name: "staged files", status: RepoStatus{Staged: 2}},
		{name: "all dirty counters", status: RepoStatus{Modified: 1, Untracked: 2, Staged: 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.status.IsDirty() {
				t.Fatal("expected true")
			}
		})
	}
}

func TestGit_RepoStatusIsDirty_Bad(t *testing.T) {
	status := RepoStatus{Modified: -1, Untracked: -1, Staged: -1}
	if status.IsDirty() {
		t.Fatal("negative counters should not mark a repo dirty")
	}
}

func TestGit_RepoStatusIsDirty_Ugly(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{name: "clean zero value", status: RepoStatus{}, expected: false},
		{name: "ahead and behind only", status: RepoStatus{Ahead: 9, Behind: 7}, expected: false},
		{name: "large dirty counters", status: RepoStatus{Modified: 1 << 30, Untracked: 1 << 30, Staged: 1 << 30}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsDirty(); got != tt.expected {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestGit_RepoStatusHasUnpushed_Good(t *testing.T) {
	status := RepoStatus{Ahead: 3}
	if !status.HasUnpushed() {
		t.Fatal("expected true")
	}
}

func TestGit_RepoStatusHasUnpushed_Bad(t *testing.T) {
	status := RepoStatus{Ahead: -1}
	if status.HasUnpushed() {
		t.Fatal("negative ahead count should not mark a repo unpushed")
	}
}

func TestGit_RepoStatusHasUnpushed_Ugly(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{name: "zero count", status: RepoStatus{}, expected: false},
		{name: "behind only", status: RepoStatus{Behind: 5}, expected: false},
		{name: "large ahead count", status: RepoStatus{Ahead: 1 << 30}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.HasUnpushed(); got != tt.expected {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestGit_RepoStatusHasUnpulled_Good(t *testing.T) {
	status := RepoStatus{Behind: 2}
	if !status.HasUnpulled() {
		t.Fatal("expected true")
	}
}

func TestGit_RepoStatusHasUnpulled_Bad(t *testing.T) {
	status := RepoStatus{Behind: -1}
	if status.HasUnpulled() {
		t.Fatal("negative behind count should not mark a repo unpulled")
	}
}

func TestGit_RepoStatusHasUnpulled_Ugly(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{name: "zero count", status: RepoStatus{}, expected: false},
		{name: "ahead only", status: RepoStatus{Ahead: 3}, expected: false},
		{name: "large behind count", status: RepoStatus{Behind: 1 << 30}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.HasUnpulled(); got != tt.expected {
				t.Fatalf("want %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestGit_GitErrorError_Good(t *testing.T) {
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

func TestGit_GitErrorError_Bad(t *testing.T) {
	err := &GitError{Args: []string{"status"}}
	expected := "git command \"git status\" failed"
	if got := err.Error(); got != expected {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func TestGit_GitErrorError_Ugly(t *testing.T) {
	err := &GitError{
		Args:   []string{"status", "--short"},
		Err:    errors.New("fallback"),
		Stderr: "\n\tfatal: spaced stderr\n\n",
	}
	expected := "git command \"git status --short\" failed: fatal: spaced stderr"
	if got := err.Error(); got != expected {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func TestGit_GitErrorUnwrap_Good(t *testing.T) {
	inner := errors.New("underlying error")
	gitErr := &GitError{Err: inner, Stderr: "stderr output"}
	if got := gitErr.Unwrap(); inner != got {
		t.Fatalf("want %v, got %v", inner, got)
	}
}

func TestGit_GitErrorUnwrap_Bad(t *testing.T) {
	gitErr := &GitError{}
	if got := gitErr.Unwrap(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestGit_GitErrorUnwrap_Ugly(t *testing.T) {
	inner := errors.New("underlying error")
	gitErr := &GitError{Err: inner, Stderr: "stderr output"}
	if !errors.Is(gitErr, inner) {
		t.Fatal("expected wrapped error to match")
	}
}

func TestGit_IsNonFastForward_Good(t *testing.T) {
	tests := []error{
		errors.New("! [rejected] main -> main (non-fast-forward)"),
		errors.New("Updates were rejected because the remote contains work that you do not have locally. fetch first"),
		errors.New("Updates were rejected because the tip of your current branch is behind"),
	}

	for _, err := range tests {
		if !IsNonFastForward(err) {
			t.Fatalf("expected true for %v", err)
		}
	}
}

func TestGit_IsNonFastForward_Bad(t *testing.T) {
	tests := []error{
		nil,
		errors.New("connection refused"),
		errors.New("authentication failed"),
	}

	for _, err := range tests {
		if IsNonFastForward(err) {
			t.Fatalf("expected false for %v", err)
		}
	}
}

func TestGit_IsNonFastForward_Ugly(t *testing.T) {
	err := &GitError{
		Args:   []string{"push"},
		Err:    errors.New("exit status 1"),
		Stderr: "UPDATES WERE REJECTED BECAUSE THE REMOTE CONTAINS WORK THAT YOU DO NOT HAVE LOCALLY. FETCH FIRST",
	}
	if !IsNonFastForward(err) {
		t.Fatal("expected mixed-case wrapped git error to match")
	}
}

func TestGit_IsStagedStatus_Good(t *testing.T) {
	for _, ch := range []byte{'A', 'C', 'D', 'R', 'M', 'T', 'U'} {
		if !isStagedStatus(ch) {
			t.Fatalf("expected %q to be staged", ch)
		}
	}
}

func TestGit_IsStagedStatus_Bad(t *testing.T) {
	for _, ch := range []byte{' ', '?', 'm', 'x'} {
		if isStagedStatus(ch) {
			t.Fatalf("expected %q not to be staged", ch)
		}
	}
}

func TestGit_IsStagedStatus_Ugly(t *testing.T) {
	for _, ch := range []byte{0, '\n', 255} {
		if isStagedStatus(ch) {
			t.Fatalf("expected boundary byte %d not to be staged", ch)
		}
	}
}

func TestGit_IsModifiedStatus_Good(t *testing.T) {
	for _, ch := range []byte{'M', 'D', 'T', 'U'} {
		if !isModifiedStatus(ch) {
			t.Fatalf("expected %q to be modified", ch)
		}
	}
}

func TestGit_IsModifiedStatus_Bad(t *testing.T) {
	for _, ch := range []byte{' ', '?', 'A', 'm'} {
		if isModifiedStatus(ch) {
			t.Fatalf("expected %q not to be modified", ch)
		}
	}
}

func TestGit_IsModifiedStatus_Ugly(t *testing.T) {
	for _, ch := range []byte{0, '\n', 255} {
		if isModifiedStatus(ch) {
			t.Fatalf("expected boundary byte %d not to be modified", ch)
		}
	}
}

func TestGit_IsNoUpstreamError_Good(t *testing.T) {
	err := errors.New("fatal: no upstream configured for branch")
	if !isNoUpstreamError(err) {
		t.Fatal("expected true")
	}
}

func TestGit_IsNoUpstreamError_Bad(t *testing.T) {
	tests := []error{
		nil,
		errors.New("fatal: not a git repository"),
	}

	for _, err := range tests {
		if isNoUpstreamError(err) {
			t.Fatalf("expected false for %v", err)
		}
	}
}

func TestGit_IsNoUpstreamError_Ugly(t *testing.T) {
	err := errors.New("\nNO UPSTREAM branch configured\n")
	if !isNoUpstreamError(err) {
		t.Fatal("expected trimmed mixed-case message to match")
	}
}

func TestGit_RequireAbsolutePath_Good(t *testing.T) {
	if err := requireAbsolutePath("git.test", t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGit_RequireAbsolutePath_Bad(t *testing.T) {
	err := requireAbsolutePath("git.test", "relative/path")
	assertErrorContains(t, err, "path must be absolute")
}

func TestGit_RequireAbsolutePath_Ugly(t *testing.T) {
	err := requireAbsolutePath("git.test", "")
	assertErrorContains(t, err, "path must be absolute")
}

func TestGit_ParseGitCount_Good(t *testing.T) {
	got, err := parseGitCount("ahead", "12\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 12 {
		t.Fatalf("want %v, got %v", 12, got)
	}
}

func TestGit_ParseGitCount_Bad(t *testing.T) {
	_, err := parseGitCount("ahead", "not-a-number")
	assertErrorContains(t, err, "failed to parse ahead count")
}

func TestGit_ParseGitCount_Ugly(t *testing.T) {
	got, err := parseGitCount("behind", "\t0\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("want %v, got %v", 0, got)
	}
}

func TestGit_GitCommand_Good(t *testing.T) {
	dir := initTestRepo(t)

	out, err := gitCommand(context.Background(), dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trim(out) != "true" {
		t.Fatalf("want %v, got %v", "true", trim(out))
	}
}

func TestGit_GitCommand_Bad(t *testing.T) {
	t.Run("invalid dir", func(t *testing.T) {
		_, err := gitCommand(context.Background(), filepath.Join(t.TempDir(), "missing"), "status")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("not a repo", func(t *testing.T) {
		dir := t.TempDir()
		_, err := gitCommand(context.Background(), dir, "status")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var gitErr *GitError
		if !errors.As(err, &gitErr) {
			t.Fatalf("expected GitError, got %T", err)
		}
		if !strings.Contains(gitErr.Stderr, "not a git repository") {
			t.Fatalf("expected %v to contain %v", gitErr.Stderr, "not a git repository")
		}
		if !slices.Equal([]string{"status"}, gitErr.Args) {
			t.Fatalf("want %v, got %v", []string{"status"}, gitErr.Args)
		}
	})

	t.Run("relative path", func(t *testing.T) {
		_, err := gitCommand(context.Background(), "relative/path", "status")
		assertErrorContains(t, err, "path must be absolute")
		assertGitError(t, err, "relative/path")
	})
}

func TestGit_GitCommand_Ugly(t *testing.T) {
	dir := initTestRepo(t)

	out, err := gitCommand(nil, dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("want empty porcelain output, got %q", out)
	}
}

func TestGit_Push_Good(t *testing.T) {
	dir := initPushableRepo(t)

	if err := Push(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ahead, behind, err := getAheadBehind(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Fatalf("want ahead/behind 0/0, got %d/%d", ahead, behind)
	}
}

func TestGit_Push_Bad(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		err := Push(context.Background(), "relative/path")
		assertErrorContains(t, err, "path must be absolute")
		assertGitError(t, err, "relative/path")
	})

	t.Run("no remote", func(t *testing.T) {
		dir := initTestRepo(t)
		err := Push(context.Background(), dir)
		if err == nil {
			t.Fatal("push without remote should fail: expected error, got nil")
		}
	})
}

func TestGit_Push_Ugly(t *testing.T) {
	dir := initPushableRepo(t)

	if err := Push(nil, dir); err != nil {
		t.Fatalf("unexpected error with nil context: %v", err)
	}
}

func TestGit_Pull_Good(t *testing.T) {
	dir := initPullableRepo(t)

	if err := Pull(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ahead, behind, err := getAheadBehind(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Fatalf("want ahead/behind 0/0, got %d/%d", ahead, behind)
	}
}

func TestGit_Pull_Bad(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		err := Pull(context.Background(), "relative/path")
		assertErrorContains(t, err, "path must be absolute")
		assertGitError(t, err, "relative/path")
	})

	t.Run("no remote", func(t *testing.T) {
		dir := initTestRepo(t)
		err := Pull(context.Background(), dir)
		if err == nil {
			t.Fatal("pull without remote should fail: expected error, got nil")
		}
	})
}

func TestGit_Pull_Ugly(t *testing.T) {
	dir := initPullableRepo(t)

	if err := Pull(nil, dir); err != nil {
		t.Fatalf("unexpected error with nil context: %v", err)
	}
}

func TestGit_GetStatus_Good(t *testing.T) {
	t.Run("clean repo", func(t *testing.T) {
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
	})

	t.Run("modified file", func(t *testing.T) {
		dir := initTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "README.md"), "# Modified\n")

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
	})

	t.Run("untracked file", func(t *testing.T) {
		dir := initTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "newfile.txt"), "hello")

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
	})

	t.Run("staged file", func(t *testing.T) {
		dir := initTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "staged.txt"), "staged")
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
	})

	t.Run("mixed changes", func(t *testing.T) {
		dir := initTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "untracked.txt"), "new")
		writeTestFile(t, filepath.Join(dir, "README.md"), "# Changed\n")
		writeTestFile(t, filepath.Join(dir, "staged.txt"), "staged")
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
	})

	t.Run("deleted tracked file", func(t *testing.T) {
		dir := initTestRepo(t)
		deleteTestPath(t, filepath.Join(dir, "README.md"))

		status := getStatus(context.Background(), dir, "deleted-repo")
		if status.Error != nil {
			t.Fatalf("unexpected error: %v", status.Error)
		}
		if 1 != status.Modified {
			t.Fatalf("deletion in working tree counts as modified: want %v, got %v", 1, status.Modified)
		}
	})

	t.Run("staged deletion", func(t *testing.T) {
		dir := initTestRepo(t)
		runTestGit(t, dir, "rm", "README.md")

		status := getStatus(context.Background(), dir, "staged-delete-repo")
		if status.Error != nil {
			t.Fatalf("unexpected error: %v", status.Error)
		}
		if 1 != status.Staged {
			t.Fatalf("staged deletion counts as staged: want %v, got %v", 1, status.Staged)
		}
	})
}

func TestGit_GetStatus_Bad(t *testing.T) {
	t.Run("invalid path", func(t *testing.T) {
		status := getStatus(context.Background(), filepath.Join(t.TempDir(), "missing"), "bad-repo")
		if status.Error == nil {
			t.Fatal("expected error, got nil")
		}
		if "bad-repo" != status.Name {
			t.Fatalf("want %v, got %v", "bad-repo", status.Name)
		}
	})

	t.Run("relative path", func(t *testing.T) {
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
	})

	t.Run("not a repo", func(t *testing.T) {
		dir := t.TempDir()
		status := getStatus(context.Background(), dir, "not-a-repo")
		if status.Error == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGit_GetStatus_Ugly(t *testing.T) {
	t.Run("merge conflict", func(t *testing.T) {
		dir := initTestRepo(t)

		runTestGit(t, dir, "checkout", "-b", "feature")
		commitTestFile(t, dir, "README.md", "# Feature\n", "feature change")
		runTestGit(t, dir, "checkout", "-")
		commitTestFile(t, dir, "README.md", "# Main\n", "main change")

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
	})

	t.Run("context cancellation", func(t *testing.T) {
		dir := initTestRepo(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		status := getStatus(ctx, dir, "cancelled-repo")
		if status.Error == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("renamed file", func(t *testing.T) {
		dir := initTestRepo(t)
		runTestGit(t, dir, "mv", "README.md", "GUIDE.md")

		status := getStatus(context.Background(), dir, "renamed-repo")
		if status.Error != nil {
			t.Fatalf("unexpected error: %v", status.Error)
		}
		if 1 != status.Staged {
			t.Fatalf("rename should count as staged: want %v, got %v", 1, status.Staged)
		}
	})

	t.Run("type changed file in working tree", func(t *testing.T) {
		dir := initTestRepo(t)
		replaceWorkingTreeWithSymlink(t, dir, "README.md", localSymlinkTarget(t))

		status := getStatus(context.Background(), dir, "typechanged-working-tree")
		if status.Error != nil {
			t.Fatalf("unexpected error: %v", status.Error)
		}
		if 1 != status.Modified {
			t.Fatalf("type change in working tree counts as modified: want %v, got %v", 1, status.Modified)
		}
	})

	t.Run("type changed file staged", func(t *testing.T) {
		dir := initTestRepo(t)
		checkoutSymlink(t, dir, "README.md", localSymlinkTarget(t))

		status := getStatus(context.Background(), dir, "typechanged-staged")
		if status.Error != nil {
			t.Fatalf("unexpected error: %v", status.Error)
		}
		if 1 != status.Staged {
			t.Fatalf("type change in the index counts as staged: want %v, got %v", 1, status.Staged)
		}
	})

	t.Run("ahead behind without upstream", func(t *testing.T) {
		dir := initTestRepo(t)

		status := getStatus(context.Background(), dir, "no-upstream")
		if status.Error != nil {
			t.Fatalf("unexpected error: %v", status.Error)
		}
		if status.Ahead != 0 || status.Behind != 0 {
			t.Fatalf("want ahead/behind 0/0, got %d/%d", status.Ahead, status.Behind)
		}
	})
}

func TestGit_Status_Good(t *testing.T) {
	dir1 := initTestRepo(t)
	dir2 := initTestRepo(t)
	writeTestFile(t, filepath.Join(dir2, "extra.txt"), "extra")

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

func TestGit_Status_Bad(t *testing.T) {
	validDir := initTestRepo(t)
	invalidDir := filepath.Join(t.TempDir(), "missing")

	results := Status(context.Background(), StatusOptions{
		Paths: []string{validDir, invalidDir, "relative/path"},
		Names: map[string]string{
			validDir:   "good",
			invalidDir: "missing",
		},
	})

	if len(results) != 3 {
		t.Fatalf("want %v, got %v", 3, len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if results[1].Error == nil {
		t.Fatal("expected error for missing path")
	}
	if results[2].Error == nil {
		t.Fatal("expected error for relative path")
	}
	assertGitError(t, results[2].Error, "relative/path")
}

func TestGit_Status_Ugly(t *testing.T) {
	t.Run("empty paths", func(t *testing.T) {
		results := Status(context.Background(), StatusOptions{Paths: []string{}})
		if len(results) != 0 {
			t.Fatalf("want %v, got %v", 0, len(results))
		}
	})

	t.Run("name fallback", func(t *testing.T) {
		dir := initTestRepo(t)

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
	})

	t.Run("nil context", func(t *testing.T) {
		dir := initTestRepo(t)

		results := Status(nil, StatusOptions{Paths: []string{dir}})
		if len(results) != 1 {
			t.Fatalf("want %v, got %v", 1, len(results))
		}
		if results[0].Error != nil {
			t.Fatalf("unexpected error: %v", results[0].Error)
		}
	})
}

func TestGit_StatusIter_Good(t *testing.T) {
	dir1 := initTestRepo(t)
	dir2 := initTestRepo(t)
	writeTestFile(t, filepath.Join(dir2, "extra.txt"), "extra")

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

func TestGit_StatusIter_Bad(t *testing.T) {
	statuses := slices.Collect(StatusIter(context.Background(), StatusOptions{
		Paths: []string{"relative/path"},
	}))

	if len(statuses) != 1 {
		t.Fatalf("want %v, got %v", 1, len(statuses))
	}
	if statuses[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
	assertGitError(t, statuses[0].Error, "relative/path")
}

func TestGit_StatusIter_Ugly(t *testing.T) {
	t.Run("empty paths", func(t *testing.T) {
		statuses := slices.Collect(StatusIter(context.Background(), StatusOptions{}))
		if len(statuses) != 0 {
			t.Fatalf("want %v, got %v", 0, len(statuses))
		}
	})

	t.Run("yield stops early", func(t *testing.T) {
		dir1 := initTestRepo(t)
		dir2 := initTestRepo(t)

		var statuses []RepoStatus
		StatusIter(context.Background(), StatusOptions{Paths: []string{dir1, dir2}})(func(st RepoStatus) bool {
			statuses = append(statuses, st)
			return false
		})

		if len(statuses) != 1 {
			t.Fatalf("want %v, got %v", 1, len(statuses))
		}
	})
}

func TestGit_GetAheadBehind_Good(t *testing.T) {
	t.Run("ahead with upstream", func(t *testing.T) {
		dir := initPushableRepo(t)

		ahead, behind, err := getAheadBehind(context.Background(), dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if 1 != ahead {
			t.Fatalf("should be 1 commit ahead: want %v, got %v", 1, ahead)
		}
		if 0 != behind {
			t.Fatalf("should not be behind: want %v, got %v", 0, behind)
		}
	})

	t.Run("behind with upstream", func(t *testing.T) {
		dir := initPullableRepo(t)
		runTestGit(t, dir, "fetch", "origin")

		ahead, behind, err := getAheadBehind(context.Background(), dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if 0 != ahead {
			t.Fatalf("should not be ahead: want %v, got %v", 0, ahead)
		}
		if 1 != behind {
			t.Fatalf("should be 1 commit behind: want %v, got %v", 1, behind)
		}
	})
}

func TestGit_GetAheadBehind_Bad(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		_, _, err := getAheadBehind(context.Background(), "relative/path")
		assertErrorContains(t, err, "path must be absolute")
		assertGitError(t, err, "relative/path")
	})

	t.Run("not a repo", func(t *testing.T) {
		_, _, err := getAheadBehind(context.Background(), t.TempDir())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGit_GetAheadBehind_Ugly(t *testing.T) {
	dir := initTestRepo(t)

	ahead, behind, err := getAheadBehind(nil, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Fatalf("repo without upstream should be 0/0, got %d/%d", ahead, behind)
	}
}

func TestGit_PushMultiple_Good(t *testing.T) {
	dir1 := initPushableRepo(t)
	dir2 := initPushableRepo(t)

	results, err := PushMultiple(context.Background(), []string{dir1, dir2}, map[string]string{
		dir1: "repo-1",
		dir2: "repo-2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want %v, got %v", 2, len(results))
	}
	for i, result := range results {
		if !result.Success {
			t.Fatalf("result %d should succeed: %+v", i, result)
		}
		if result.Error != nil {
			t.Fatalf("unexpected result error: %v", result.Error)
		}
	}
}

func TestGit_PushMultiple_Bad(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		results, err := PushMultiple(context.Background(), []string{"relative/repo"}, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(results) != 1 {
			t.Fatalf("want %v, got %v", 1, len(results))
		}
		assertErrorContains(t, results[0].Error, "path must be absolute")
		assertGitError(t, results[0].Error, "relative/repo")
		assertGitError(t, err, "relative/repo")
	})

	t.Run("no remote", func(t *testing.T) {
		dir := initTestRepo(t)
		results, err := PushMultiple(context.Background(), []string{dir}, map[string]string{dir: "test-repo"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(results) != 1 {
			t.Fatalf("want %v, got %v", 1, len(results))
		}
		if results[0].Success {
			t.Fatal("expected false")
		}
		if results[0].Error == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGit_PushMultiple_Ugly(t *testing.T) {
	t.Run("empty paths", func(t *testing.T) {
		results, err := PushMultiple(context.Background(), []string{}, map[string]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("want %v, got %v", 0, len(results))
		}
	})

	t.Run("name fallback", func(t *testing.T) {
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
	})
}

func TestGit_PullMultiple_Good(t *testing.T) {
	dir1 := initPullableRepo(t)
	dir2 := initPullableRepo(t)

	results, err := PullMultiple(context.Background(), []string{dir1, dir2}, map[string]string{
		dir1: "repo-1",
		dir2: "repo-2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want %v, got %v", 2, len(results))
	}
	for i, result := range results {
		if !result.Success {
			t.Fatalf("result %d should succeed: %+v", i, result)
		}
		if result.Error != nil {
			t.Fatalf("unexpected result error: %v", result.Error)
		}
	}
}

func TestGit_PullMultiple_Bad(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		results, err := PullMultiple(context.Background(), []string{"relative/repo"}, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(results) != 1 {
			t.Fatalf("want %v, got %v", 1, len(results))
		}
		assertErrorContains(t, results[0].Error, "path must be absolute")
		assertGitError(t, results[0].Error, "relative/repo")
		assertGitError(t, err, "relative/repo")
	})

	t.Run("no remote", func(t *testing.T) {
		dir := initTestRepo(t)
		results, err := PullMultiple(context.Background(), []string{dir}, map[string]string{dir: "test-repo"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(results) != 1 {
			t.Fatalf("want %v, got %v", 1, len(results))
		}
		if results[0].Success {
			t.Fatal("expected false")
		}
		if results[0].Error == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGit_PullMultiple_Ugly(t *testing.T) {
	t.Run("empty paths", func(t *testing.T) {
		results, err := PullMultiple(context.Background(), []string{}, map[string]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("want %v, got %v", 0, len(results))
		}
	})

	t.Run("name fallback", func(t *testing.T) {
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
	})
}

func TestGit_PushMultipleIter_Good(t *testing.T) {
	dir := initPushableRepo(t)

	results := slices.Collect(PushMultipleIter(context.Background(), []string{dir}, map[string]string{dir: "test-repo"}))

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if "test-repo" != results[0].Name {
		t.Fatalf("want %v, got %v", "test-repo", results[0].Name)
	}
	if !results[0].Success {
		t.Fatal("expected true")
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
}

func TestGit_PushMultipleIter_Bad(t *testing.T) {
	results := slices.Collect(PushMultipleIter(context.Background(), []string{"relative/repo"}, nil))

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	assertErrorContains(t, results[0].Error, "path must be absolute")
}

func TestGit_PushMultipleIter_Ugly(t *testing.T) {
	dir := initPushableRepo(t)

	var results []PushResult
	PushMultipleIter(context.Background(), []string{"relative/repo", dir}, nil)(func(result PushResult) bool {
		results = append(results, result)
		return false
	})

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if results[0].Path != "relative/repo" {
		t.Fatalf("want %v, got %v", "relative/repo", results[0].Path)
	}
}

func TestGit_PullMultipleIter_Good(t *testing.T) {
	dir := initPullableRepo(t)

	results := slices.Collect(PullMultipleIter(context.Background(), []string{dir}, map[string]string{dir: "test-repo"}))

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if "test-repo" != results[0].Name {
		t.Fatalf("want %v, got %v", "test-repo", results[0].Name)
	}
	if !results[0].Success {
		t.Fatal("expected true")
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
}

func TestGit_PullMultipleIter_Bad(t *testing.T) {
	results := slices.Collect(PullMultipleIter(context.Background(), []string{"relative/repo"}, nil))

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	assertErrorContains(t, results[0].Error, "path must be absolute")
}

func TestGit_PullMultipleIter_Ugly(t *testing.T) {
	dir := initPullableRepo(t)

	var results []PullResult
	PullMultipleIter(context.Background(), []string{"relative/repo", dir}, nil)(func(result PullResult) bool {
		results = append(results, result)
		return false
	})

	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if results[0].Path != "relative/repo" {
		t.Fatalf("want %v, got %v", "relative/repo", results[0].Path)
	}
}

func TestGit_Trim_Good(t *testing.T) {
	if got := trim(" value \n"); got != "value" {
		t.Fatalf("want %v, got %v", "value", got)
	}
}

func TestGit_Trim_Bad(t *testing.T) {
	if got := trim(""); got != "" {
		t.Fatalf("want empty string, got %v", got)
	}
}

func TestGit_Trim_Ugly(t *testing.T) {
	if got := trim("\n\t "); got != "" {
		t.Fatalf("want empty string, got %v", got)
	}
}
