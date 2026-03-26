package git

import (
	"context"
	"testing"

	"dappco.re/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		runGit(t, dir, args...)
	}

	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Test\n")

	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial commit"},
	} {
		runGit(t, dir, args...)
	}

	return dir
}

func TestGit_RepoStatus_IsDirty_Good(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{name: "clean repo", status: RepoStatus{}, expected: false},
		{name: "modified files", status: RepoStatus{Modified: 3}, expected: true},
		{name: "untracked files", status: RepoStatus{Untracked: 1}, expected: true},
		{name: "staged files", status: RepoStatus{Staged: 2}, expected: true},
		{name: "all types dirty", status: RepoStatus{Modified: 1, Untracked: 2, Staged: 3}, expected: true},
		{name: "only ahead is not dirty", status: RepoStatus{Ahead: 5}, expected: false},
		{name: "only behind is not dirty", status: RepoStatus{Behind: 2}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.IsDirty())
		})
	}
}

func TestGit_RepoStatus_HasUnpushed_Good(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{name: "no commits ahead", status: RepoStatus{Ahead: 0}, expected: false},
		{name: "commits ahead", status: RepoStatus{Ahead: 3}, expected: true},
		{name: "behind but not ahead", status: RepoStatus{Behind: 5}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.HasUnpushed())
		})
	}
}

func TestGit_RepoStatus_HasUnpulled_Good(t *testing.T) {
	tests := []struct {
		name     string
		status   RepoStatus
		expected bool
	}{
		{name: "no commits behind", status: RepoStatus{Behind: 0}, expected: false},
		{name: "commits behind", status: RepoStatus{Behind: 2}, expected: true},
		{name: "ahead but not behind", status: RepoStatus{Ahead: 3}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.HasUnpulled())
		})
	}
}

func TestGit_GitError_Error_Good(t *testing.T) {
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
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestGit_GitError_Unwrap_Good(t *testing.T) {
	inner := core.NewError("underlying error")
	gitErr := &GitError{Err: inner, Stderr: "stderr output"}

	assert.Equal(t, inner, gitErr.Unwrap())
	assert.True(t, core.Is(gitErr, inner))
}

func TestGit_IsNonFastForward_Good(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "non-fast-forward message", err: core.NewError("! [rejected] main -> main (non-fast-forward)"), expected: true},
		{name: "fetch first message", err: core.NewError("Updates were rejected because the remote contains work that you do not have locally. fetch first"), expected: true},
		{name: "tip behind message", err: core.NewError("Updates were rejected because the tip of your current branch is behind"), expected: true},
		{name: "unrelated error", err: core.NewError("connection refused"), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNonFastForward(tt.err))
		})
	}
}

func TestGit_GitCommand_Good(t *testing.T) {
	dir := initTestRepo(t)

	out, err := gitCommand(context.Background(), dir, "rev-parse", "--abbrev-ref", "HEAD")
	require.NoError(t, err)
	assert.NotEmpty(t, core.Trim(out))
}

func TestGit_GitCommand_InvalidDir_Bad(t *testing.T) {
	_, err := gitCommand(context.Background(), "/nonexistent/path", "status")
	require.Error(t, err)
}

func TestGit_GitCommand_NotARepo_Bad(t *testing.T) {
	dir := t.TempDir()

	_, err := gitCommand(context.Background(), dir, "status")
	require.Error(t, err)

	var gitErr *GitError
	if core.As(err, &gitErr) {
		assert.Contains(t, gitErr.Stderr, "not a git repository")
		assert.Equal(t, []string{"status"}, gitErr.Args)
	}
}

func TestGit_GetStatus_CleanRepo_Good(t *testing.T) {
	dir := initTestRepo(t)

	status := getStatus(context.Background(), dir, "test-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, "test-repo", status.Name)
	assert.Equal(t, dir, status.Path)
	assert.NotEmpty(t, status.Branch)
	assert.False(t, status.IsDirty())
}

func TestGit_GetStatus_ModifiedFile_Good(t *testing.T) {
	dir := initTestRepo(t)

	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Modified\n")

	status := getStatus(context.Background(), dir, "modified-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Modified)
	assert.True(t, status.IsDirty())
}

func TestGit_GetStatus_UntrackedFile_Good(t *testing.T) {
	dir := initTestRepo(t)

	writeTestFile(t, core.JoinPath(dir, "newfile.txt"), "hello")

	status := getStatus(context.Background(), dir, "untracked-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Untracked)
	assert.True(t, status.IsDirty())
}

func TestGit_GetStatus_StagedFile_Good(t *testing.T) {
	dir := initTestRepo(t)

	writeTestFile(t, core.JoinPath(dir, "staged.txt"), "staged")
	runGit(t, dir, "add", "staged.txt")

	status := getStatus(context.Background(), dir, "staged-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Staged)
	assert.True(t, status.IsDirty())
}

func TestGit_GetStatus_MixedChanges_Good(t *testing.T) {
	dir := initTestRepo(t)

	writeTestFile(t, core.JoinPath(dir, "untracked.txt"), "new")
	writeTestFile(t, core.JoinPath(dir, "README.md"), "# Changed\n")
	writeTestFile(t, core.JoinPath(dir, "staged.txt"), "staged")
	runGit(t, dir, "add", "staged.txt")

	status := getStatus(context.Background(), dir, "mixed-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Modified, "expected 1 modified file")
	assert.Equal(t, 1, status.Untracked, "expected 1 untracked file")
	assert.Equal(t, 1, status.Staged, "expected 1 staged file")
	assert.True(t, status.IsDirty())
}

func TestGit_GetStatus_DeletedTrackedFile_Good(t *testing.T) {
	dir := initTestRepo(t)

	deleteTestPath(t, core.JoinPath(dir, "README.md"))

	status := getStatus(context.Background(), dir, "deleted-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Modified, "deletion in working tree counts as modified")
	assert.True(t, status.IsDirty())
}

func TestGit_GetStatus_StagedDeletion_Good(t *testing.T) {
	dir := initTestRepo(t)

	runGit(t, dir, "rm", "README.md")

	status := getStatus(context.Background(), dir, "staged-delete-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Staged, "staged deletion counts as staged")
	assert.True(t, status.IsDirty())
}

func TestGit_GetStatus_InvalidPath_Bad(t *testing.T) {
	status := getStatus(context.Background(), "/nonexistent/path", "bad-repo")
	assert.Error(t, status.Error)
	assert.Equal(t, "bad-repo", status.Name)
}

func TestGit_GetStatus_RelativePath_Bad(t *testing.T) {
	status := getStatus(context.Background(), "relative/path", "rel-repo")
	assert.Error(t, status.Error)
	assert.Contains(t, status.Error.Error(), "path must be absolute")
	assert.Equal(t, "rel-repo", status.Name)
}

func TestGit_Status_MultipleRepos_Good(t *testing.T) {
	dir1 := initTestRepo(t)
	dir2 := initTestRepo(t)

	writeTestFile(t, core.JoinPath(dir2, "extra.txt"), "extra")

	results := Status(context.Background(), StatusOptions{
		Paths: []string{dir1, dir2},
		Names: map[string]string{
			dir1: "clean-repo",
			dir2: "dirty-repo",
		},
	})

	require.Len(t, results, 2)
	assert.Equal(t, "clean-repo", results[0].Name)
	assert.NoError(t, results[0].Error)
	assert.False(t, results[0].IsDirty())
	assert.Equal(t, "dirty-repo", results[1].Name)
	assert.NoError(t, results[1].Error)
	assert.True(t, results[1].IsDirty())
}

func TestGit_Status_EmptyPaths_Good(t *testing.T) {
	results := Status(context.Background(), StatusOptions{Paths: []string{}})
	assert.Empty(t, results)
}

func TestGit_Status_NameFallback_Good(t *testing.T) {
	dir := initTestRepo(t)

	results := Status(context.Background(), StatusOptions{
		Paths: []string{dir},
		Names: map[string]string{},
	})

	require.Len(t, results, 1)
	assert.Equal(t, dir, results[0].Name, "name should fall back to path")
}

func TestGit_Status_WithErrors_Good(t *testing.T) {
	validDir := initTestRepo(t)
	invalidDir := "/nonexistent/path"

	results := Status(context.Background(), StatusOptions{
		Paths: []string{validDir, invalidDir},
		Names: map[string]string{
			validDir:   "good",
			invalidDir: "bad",
		},
	})

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Error)
	assert.Error(t, results[1].Error)
}

func TestGit_PushMultiple_NoRemote_Bad(t *testing.T) {
	dir := initTestRepo(t)

	results, err := PushMultiple(context.Background(), []string{dir}, map[string]string{
		dir: "test-repo",
	})
	assert.Error(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "test-repo", results[0].Name)
	assert.Equal(t, dir, results[0].Path)
	assert.False(t, results[0].Success)
	assert.Error(t, results[0].Error)
}

func TestGit_PushMultiple_NameFallback_NoRemote_Bad(t *testing.T) {
	dir := initTestRepo(t)

	results, err := PushMultiple(context.Background(), []string{dir}, map[string]string{})
	assert.Error(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, dir, results[0].Name, "name should fall back to path")
}

func TestGit_Pull_NoRemote_Bad(t *testing.T) {
	dir := initTestRepo(t)
	err := Pull(context.Background(), dir)
	assert.Error(t, err, "pull without remote should fail")
}

func TestGit_Push_NoRemote_Bad(t *testing.T) {
	dir := initTestRepo(t)
	err := Push(context.Background(), dir)
	assert.Error(t, err, "push without remote should fail")
}

func TestGit_GetStatus_ContextCancellation_Bad(t *testing.T) {
	dir := initTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status := getStatus(ctx, dir, "cancelled-repo")
	assert.Error(t, status.Error)
}

func TestGit_GetAheadBehind_WithUpstream_Good(t *testing.T) {
	bareDir := t.TempDir()
	cloneDir := t.TempDir()

	runGit(t, bareDir, "init", "--bare")
	runGit(t, "", "clone", bareDir, cloneDir)

	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		runGit(t, cloneDir, args...)
	}

	writeTestFile(t, core.JoinPath(cloneDir, "file.txt"), "v1")
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "initial"},
		{"push", "origin", "HEAD"},
	} {
		runGit(t, cloneDir, args...)
	}

	writeTestFile(t, core.JoinPath(cloneDir, "file.txt"), "v2")
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "local commit"},
	} {
		runGit(t, cloneDir, args...)
	}

	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, ahead, "should be 1 commit ahead")
	assert.Equal(t, 0, behind, "should not be behind")
}

func TestGit_GetStatus_RenamedFile_Good(t *testing.T) {
	dir := initTestRepo(t)

	runGit(t, dir, "mv", "README.md", "GUIDE.md")

	status := getStatus(context.Background(), dir, "renamed-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Staged, "rename should count as staged")
	assert.True(t, status.IsDirty())
}
