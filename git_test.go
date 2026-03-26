package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NoError(t, err, "failed to run %v: %s", args, string(out))
	}

	// Create a file and commit it so HEAD exists.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644))

	cmds = [][]string{
		{"git", "add", "README.md"},
		{"git", "commit", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "failed to run %v: %s", args, string(out))
	}

	return dir
}

// --- RepoStatus method tests ---

func TestRepoStatus_IsDirty_Good(t *testing.T) {
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
			assert.Equal(t, tt.expected, tt.status.IsDirty())
		})
	}
}

func TestRepoStatus_HasUnpushed_Good(t *testing.T) {
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
			assert.Equal(t, tt.expected, tt.status.HasUnpushed())
		})
	}
}

func TestRepoStatus_HasUnpulled_Good(t *testing.T) {
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
			assert.Equal(t, tt.expected, tt.status.HasUnpulled())
		})
	}
}

// --- GitError tests ---

func TestGitError_Error_Good(t *testing.T) {
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
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestGitError_Unwrap_Good(t *testing.T) {
	inner := errors.New("underlying error")
	gitErr := &GitError{Err: inner, Stderr: "stderr output"}
	assert.Equal(t, inner, gitErr.Unwrap())
	assert.True(t, errors.Is(gitErr, inner))
}

// --- IsNonFastForward tests ---

func TestIsNonFastForward_Good(t *testing.T) {
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
			assert.Equal(t, tt.expected, IsNonFastForward(tt.err))
		})
	}
}

// --- gitCommand tests with real git repos ---

func TestGitCommand_Good(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	out, err := gitCommand(context.Background(), dir, "rev-parse", "--abbrev-ref", "HEAD")
	require.NoError(t, err)
	// Default branch could be main or master depending on git config.
	branch := out
	assert.NotEmpty(t, branch)
}

func TestGitCommand_Bad_InvalidDir(t *testing.T) {
	_, err := gitCommand(context.Background(), "/nonexistent/path", "status")
	require.Error(t, err)
}

func TestGitCommand_Bad_NotARepo(t *testing.T) {
	dir, _ := filepath.Abs(t.TempDir())
	_, err := gitCommand(context.Background(), dir, "status")
	require.Error(t, err)

	// Should be a GitError with stderr.
	var gitErr *GitError
	if errors.As(err, &gitErr) {
		assert.Contains(t, gitErr.Stderr, "not a git repository")
		assert.Equal(t, []string{"status"}, gitErr.Args)
	}
}

// --- getStatus integration tests ---

func TestGetStatus_Good_CleanRepo(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	status := getStatus(context.Background(), dir, "test-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, "test-repo", status.Name)
	assert.Equal(t, dir, status.Path)
	assert.NotEmpty(t, status.Branch)
	assert.False(t, status.IsDirty())
}

func TestGetStatus_Good_ModifiedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Modify the existing tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified\n"), 0644))

	status := getStatus(context.Background(), dir, "modified-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Modified)
	assert.True(t, status.IsDirty())
}

func TestGetStatus_Good_UntrackedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Create a new untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("hello"), 0644))

	status := getStatus(context.Background(), dir, "untracked-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Untracked)
	assert.True(t, status.IsDirty())
}

func TestGetStatus_Good_StagedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Create and stage a new file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged"), 0644))
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	status := getStatus(context.Background(), dir, "staged-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Staged)
	assert.True(t, status.IsDirty())
}

func TestGetStatus_Good_MixedChanges(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Create untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new"), 0644))

	// Modify tracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Changed\n"), 0644))

	// Create and stage another file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged"), 0644))
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	status := getStatus(context.Background(), dir, "mixed-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Modified, "expected 1 modified file")
	assert.Equal(t, 1, status.Untracked, "expected 1 untracked file")
	assert.Equal(t, 1, status.Staged, "expected 1 staged file")
	assert.True(t, status.IsDirty())
}

func TestGetStatus_Good_DeletedTrackedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Delete the tracked file (unstaged deletion).
	require.NoError(t, os.Remove(filepath.Join(dir, "README.md")))

	status := getStatus(context.Background(), dir, "deleted-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Modified, "deletion in working tree counts as modified")
	assert.True(t, status.IsDirty())
}

func TestGetStatus_Good_StagedDeletion(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Stage a deletion.
	cmd := exec.Command("git", "rm", "README.md")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	status := getStatus(context.Background(), dir, "staged-delete-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Staged, "staged deletion counts as staged")
	assert.True(t, status.IsDirty())
}

func TestGetStatus_Bad_InvalidPath(t *testing.T) {
	status := getStatus(context.Background(), "/nonexistent/path", "bad-repo")
	assert.Error(t, status.Error)
	assert.Equal(t, "bad-repo", status.Name)
}

func TestGetStatus_Bad_RelativePath(t *testing.T) {
	status := getStatus(context.Background(), "relative/path", "rel-repo")
	assert.Error(t, status.Error)
	assert.Contains(t, status.Error.Error(), "path must be absolute")
	assert.Equal(t, "rel-repo", status.Name)
}

// --- Status (parallel multi-repo) tests ---

func TestStatus_Good_MultipleRepos(t *testing.T) {
	dir1, _ := filepath.Abs(initTestRepo(t))
	dir2, _ := filepath.Abs(initTestRepo(t))

	// Make dir2 dirty.
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "extra.txt"), []byte("extra"), 0644))

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

func TestStatus_Good_EmptyPaths(t *testing.T) {
	results := Status(context.Background(), StatusOptions{
		Paths: []string{},
	})
	assert.Empty(t, results)
}

func TestStatus_Good_NameFallback(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// No name mapping — path should be used as name.
	results := Status(context.Background(), StatusOptions{
		Paths: []string{dir},
		Names: map[string]string{},
	})

	require.Len(t, results, 1)
	assert.Equal(t, dir, results[0].Name, "name should fall back to path")
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

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Error)
	assert.Error(t, results[1].Error)
}

// --- PushMultiple tests ---

func TestPushMultiple_Bad_NoRemote(t *testing.T) {
	// Push without a remote will fail but we can test the result structure.
	dir, _ := filepath.Abs(initTestRepo(t))

	results, err := PushMultiple(context.Background(), []string{dir}, map[string]string{
		dir: "test-repo",
	})
	assert.Error(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "test-repo", results[0].Name)
	assert.Equal(t, dir, results[0].Path)
	// Push without remote should fail.
	assert.False(t, results[0].Success)
	assert.Error(t, results[0].Error)
}

func TestPushMultiple_Bad_NoRemote_NameFallback(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	results, err := PushMultiple(context.Background(), []string{dir}, map[string]string{})
	assert.Error(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, dir, results[0].Name, "name should fall back to path")
}

// --- Pull tests ---

func TestPull_Bad_NoRemote(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))
	err := Pull(context.Background(), dir)
	assert.Error(t, err, "pull without remote should fail")
}

// --- Push tests ---

func TestPush_Bad_NoRemote(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))
	err := Push(context.Background(), dir)
	assert.Error(t, err, "push without remote should fail")
}

// --- Context cancellation test ---

func TestGetStatus_Bad_ContextCancellation(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	status := getStatus(ctx, dir, "cancelled-repo")
	// With a cancelled context, the git commands should fail.
	assert.Error(t, status.Error)
}

// --- getAheadBehind with a tracking branch ---

func TestGetAheadBehind_Good_WithUpstream(t *testing.T) {
	// Create a bare remote and a clone to test ahead/behind counts.
	bareDir, _ := filepath.Abs(t.TempDir())
	cloneDir, _ := filepath.Abs(t.TempDir())

	// Initialise the bare repo.
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	require.NoError(t, cmd.Run())

	// Clone it.
	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	require.NoError(t, cmd.Run())

	// Configure user in clone.
	for _, args := range [][]string{
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		require.NoError(t, cmd.Run())
	}

	// Create initial commit and push.
	require.NoError(t, os.WriteFile(filepath.Join(cloneDir, "file.txt"), []byte("v1"), 0644))
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
		{"git", "push", "origin", "HEAD"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, string(out))
	}

	// Make a local commit without pushing (ahead by 1).
	require.NoError(t, os.WriteFile(filepath.Join(cloneDir, "file.txt"), []byte("v2"), 0644))
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "local commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		require.NoError(t, cmd.Run())
	}

	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, ahead, "should be 1 commit ahead")
	assert.Equal(t, 0, behind, "should not be behind")
}

// --- Renamed file detection ---

func TestGetStatus_Good_RenamedFile(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	// Rename via git mv (stages the rename).
	cmd := exec.Command("git", "mv", "README.md", "GUIDE.md")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	status := getStatus(context.Background(), dir, "renamed-repo")
	require.NoError(t, status.Error)
	assert.Equal(t, 1, status.Staged, "rename should count as staged")
	assert.True(t, status.IsDirty())
}
