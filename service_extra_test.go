package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"forge.lthn.ai/core/go/pkg/core"
)

func TestNewService_Good(t *testing.T) {
	opts := ServiceOptions{WorkDir: t.TempDir()}
	factory := NewService(opts)
	assert.NotNil(t, factory)

	// Create a minimal Core to test the factory.
	c, err := core.New()
	require.NoError(t, err)

	svc, err := factory(c)
	require.NoError(t, err)
	assert.NotNil(t, svc)

	service, ok := svc.(*Service)
	require.True(t, ok)
	assert.NotNil(t, service)
}

func TestService_OnStartup_Good(t *testing.T) {
	c, err := core.New()
	require.NoError(t, err)

	opts := ServiceOptions{WorkDir: t.TempDir()}
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, opts),
		opts:           opts,
	}

	err = svc.OnStartup(context.Background())
	assert.NoError(t, err)
}

func TestService_HandleQuery_Good_Status(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	// Call handleQuery directly.
	result, handled, err := svc.handleQuery(c, QueryStatus{
		Paths: []string{dir},
		Names: map[string]string{dir: "test-repo"},
	})

	require.NoError(t, err)
	assert.True(t, handled)

	statuses, ok := result.([]RepoStatus)
	require.True(t, ok)
	require.Len(t, statuses, 1)
	assert.Equal(t, "test-repo", statuses[0].Name)

	// Verify lastStatus was updated.
	assert.Len(t, svc.lastStatus, 1)
}

func TestService_HandleQuery_Good_DirtyRepos(t *testing.T) {
	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
		lastStatus: []RepoStatus{
			{Name: "clean"},
			{Name: "dirty", Modified: 1},
		},
	}

	result, handled, err := svc.handleQuery(c, QueryDirtyRepos{})
	require.NoError(t, err)
	assert.True(t, handled)

	dirty, ok := result.([]RepoStatus)
	require.True(t, ok)
	assert.Len(t, dirty, 1)
	assert.Equal(t, "dirty", dirty[0].Name)
}

func TestService_HandleQuery_Good_AheadRepos(t *testing.T) {
	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
		lastStatus: []RepoStatus{
			{Name: "synced"},
			{Name: "ahead", Ahead: 3},
		},
	}

	result, handled, err := svc.handleQuery(c, QueryAheadRepos{})
	require.NoError(t, err)
	assert.True(t, handled)

	ahead, ok := result.([]RepoStatus)
	require.True(t, ok)
	assert.Len(t, ahead, 1)
	assert.Equal(t, "ahead", ahead[0].Name)
}

func TestService_HandleQuery_Good_UnknownQuery(t *testing.T) {
	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result, handled, err := svc.handleQuery(c, "unknown query type")
	require.NoError(t, err)
	assert.False(t, handled)
	assert.Nil(t, result)
}

func TestService_HandleTask_Good_Push(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	// Push without a remote will fail, but handleTask should still handle it.
	_, handled, err := svc.handleTask(c, TaskPush{Path: dir, Name: "test"})
	assert.True(t, handled)
	assert.Error(t, err, "push without remote should fail")
}

func TestService_HandleTask_Good_Pull(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	_, handled, err := svc.handleTask(c, TaskPull{Path: dir, Name: "test"})
	assert.True(t, handled)
	assert.Error(t, err, "pull without remote should fail")
}

func TestService_HandleTask_Good_PushMultiple(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result, handled, err := svc.handleTask(c, TaskPushMultiple{
		Paths: []string{dir},
		Names: map[string]string{dir: "test"},
	})

	assert.True(t, handled)
	assert.Error(t, err) // PushMultiple returns errors directly

	results, ok := result.([]PushResult)
	require.True(t, ok)
	assert.Len(t, results, 1)
	assert.False(t, results[0].Success) // No remote
}

func TestService_HandleTask_Good_UnknownTask(t *testing.T) {
	c, err := core.New()
	require.NoError(t, err)

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result, handled, err := svc.handleTask(c, "unknown task")
	require.NoError(t, err)
	assert.False(t, handled)
	assert.Nil(t, result)
}

// --- Additional git operation tests ---

func TestGetStatus_Good_AheadBehindNoUpstream(t *testing.T) {
	// A repo without a tracking branch should return 0 ahead/behind.
	dir, _ := filepath.Abs(initTestRepo(t))

	status := getStatus(context.Background(), dir, "no-upstream")
	require.NoError(t, status.Error)
	assert.Equal(t, 0, status.Ahead)
	assert.Equal(t, 0, status.Behind)
}

func TestPushMultiple_Good_Empty(t *testing.T) {
	results, err := PushMultiple(context.Background(), []string{}, map[string]string{})
	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestPushMultiple_Good_MultiplePaths(t *testing.T) {
	dir1, _ := filepath.Abs(initTestRepo(t))
	dir2, _ := filepath.Abs(initTestRepo(t))

	results, err := PushMultiple(context.Background(), []string{dir1, dir2}, map[string]string{
		dir1: "repo-1",
		dir2: "repo-2",
	})
	assert.Error(t, err)

	require.Len(t, results, 2)
	assert.Equal(t, "repo-1", results[0].Name)
	assert.Equal(t, "repo-2", results[1].Name)
	// Both should fail (no remote).
	assert.False(t, results[0].Success)
	assert.False(t, results[1].Success)
}

func TestPush_Good_WithRemote(t *testing.T) {
	// Create a bare remote and a clone.
	bareDir, _ := filepath.Abs(t.TempDir())
	cloneDir, _ := filepath.Abs(t.TempDir())

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	require.NoError(t, cmd.Run())

	for _, args := range [][]string{
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		require.NoError(t, cmd.Run())
	}

	require.NoError(t, os.WriteFile(filepath.Join(cloneDir, "file.txt"), []byte("v1"), 0644))
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
		{"git", "push", "origin", "HEAD"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "failed: %v: %s", args, string(out))
	}

	// Make a local commit.
	require.NoError(t, os.WriteFile(filepath.Join(cloneDir, "file.txt"), []byte("v2"), 0644))
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "second commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		require.NoError(t, cmd.Run())
	}

	// Push should succeed.
	err := Push(context.Background(), cloneDir)
	assert.NoError(t, err)

	// Verify ahead count is now 0.
	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)
}
