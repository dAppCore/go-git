package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dappco.re/go/core"
)

// --- validatePath tests ---

func TestService_ValidatePath_Bad_RelativePath(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("relative/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be absolute")
}

func TestService_ValidatePath_Bad_OutsideWorkDir(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("/etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "outside of allowed WorkDir")
}

func TestService_ValidatePath_Bad_OutsideWorkDirPrefix(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("/home/repos2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "outside of allowed WorkDir")
}

func TestService_ValidatePath_Bad_WorkDirNotAbsolute(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "relative/workdir"}}
	err := svc.validatePath("/any/absolute/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WorkDir must be absolute")
}

func TestService_ValidatePath_Good_InsideWorkDir(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("/home/repos/my-project")
	assert.NoError(t, err)
}

func TestService_ValidatePath_Good_NoWorkDir(t *testing.T) {
	svc := &Service{opts: ServiceOptions{}}
	err := svc.validatePath("/any/absolute/path")
	assert.NoError(t, err)
}

// --- handleQuery path validation ---

func TestService_HandleQuery_Bad_InvalidPath(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}

	result := svc.handleQuery(c, QueryStatus{
		Paths: []string{"/outside/path"},
		Names: map[string]string{"/outside/path": "bad"},
	})
	assert.False(t, result.OK)
}

// --- handleTask path validation ---

func TestService_Action_Bad_PushInvalidPath(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}
	svc.OnStartup(context.Background())

	result := c.Action("git.push").Run(context.Background(), core.NewOptions(
		core.Option{Key: "path", Value: "relative/path"},
	))
	_ = svc
	assert.False(t, result.OK)
}

func TestService_Action_Bad_PullInvalidPath(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}
	svc.OnStartup(context.Background())

	result := c.Action("git.pull").Run(context.Background(), core.NewOptions(
		core.Option{Key: "path", Value: "/etc/passwd"},
	))
	_ = svc
	assert.False(t, result.OK)
}

func TestService_Action_Bad_PushMultipleInvalidPath(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}
	svc.OnStartup(context.Background())

	opts := core.NewOptions()
	opts.Set("paths", []string{"/home/repos/ok", "/etc/bad"})
	opts.Set("names", map[string]string{})
	result := c.Action("git.push-multiple").Run(context.Background(), opts)
	_ = svc
	assert.False(t, result.OK)
}

func TestService_Action_Bad_PullMultipleInvalidPath(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}
	svc.OnStartup(context.Background())

	opts := core.NewOptions()
	opts.Set("paths", []string{"/home/repos/ok", "/etc/bad"})
	opts.Set("names", map[string]string{})
	result := c.Action("git.pull-multiple").Run(context.Background(), opts)
	_ = svc
	assert.False(t, result.OK)
}

func TestNewService_Good(t *testing.T) {
	opts := ServiceOptions{WorkDir: t.TempDir()}
	factory := NewService(opts)
	assert.NotNil(t, factory)

	// Create a minimal Core to test the factory.
	c := core.New()

	svc, err := factory(c)
	require.NoError(t, err)
	assert.NotNil(t, svc)

	service, ok := svc.(*Service)
	require.True(t, ok)
	assert.NotNil(t, service)
}

func TestService_OnStartup_Good(t *testing.T) {
	c := core.New()

	opts := ServiceOptions{WorkDir: t.TempDir()}
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, opts),
		opts:           opts,
	}

	result := svc.OnStartup(context.Background())
	assert.True(t, result.OK)
}

func TestService_HandleQuery_Good_Status(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	// Call handleQuery directly.
	result := svc.handleQuery(c, QueryStatus{
		Paths: []string{dir},
		Names: map[string]string{dir: "test-repo"},
	})

	assert.True(t, result.OK)

	statuses, ok := result.Value.([]RepoStatus)
	require.True(t, ok)
	require.Len(t, statuses, 1)
	assert.Equal(t, "test-repo", statuses[0].Name)

	// Verify lastStatus was updated.
	assert.Len(t, svc.lastStatus, 1)
}

func TestService_HandleQuery_Good_DirtyRepos(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
		lastStatus: []RepoStatus{
			{Name: "clean"},
			{Name: "dirty", Modified: 1},
		},
	}

	result := svc.handleQuery(c, QueryDirtyRepos{})
	assert.True(t, result.OK)

	dirty, ok := result.Value.([]RepoStatus)
	require.True(t, ok)
	assert.Len(t, dirty, 1)
	assert.Equal(t, "dirty", dirty[0].Name)
}

func TestService_HandleQuery_Good_AheadRepos(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
		lastStatus: []RepoStatus{
			{Name: "synced"},
			{Name: "ahead", Ahead: 3},
		},
	}

	result := svc.handleQuery(c, QueryAheadRepos{})
	assert.True(t, result.OK)

	ahead, ok := result.Value.([]RepoStatus)
	require.True(t, ok)
	assert.Len(t, ahead, 1)
	assert.Equal(t, "ahead", ahead[0].Name)
}

func TestService_HandleQuery_Good_BehindRepos(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
		lastStatus: []RepoStatus{
			{Name: "synced"},
			{Name: "behind", Behind: 2},
		},
	}

	result := svc.handleQuery(c, QueryBehindRepos{})
	assert.True(t, result.OK)

	behind, ok := result.Value.([]RepoStatus)
	require.True(t, ok)
	assert.Len(t, behind, 1)
	assert.Equal(t, "behind", behind[0].Name)
}

func TestService_HandleTaskMessage_Good_TaskPush(t *testing.T) {
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

	require.NoError(t, os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v1"), 0644))
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

	require.NoError(t, os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v2"), 0644))
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "second commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		require.NoError(t, cmd.Run())
	}

	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTaskMessage(c, TaskPush{Path: cloneDir})
	assert.True(t, result.OK)
}

func TestService_Action_Good_TaskPush(t *testing.T) {
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

	require.NoError(t, os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v1"), 0644))
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

	require.NoError(t, os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v2"), 0644))
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "second commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		require.NoError(t, cmd.Run())
	}

	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}
	svc.OnStartup(context.Background())

	result := c.ACTION(TaskPush{Path: cloneDir})
	assert.True(t, result.OK)

	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	require.NoError(t, err)
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)
}

func TestService_HandleQuery_Good_UnknownQuery(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleQuery(c, "unknown query type")
	assert.False(t, result.OK)
	assert.Nil(t, result.Value)
}

func TestService_Action_Bad_PushNoRemote(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}
	svc.OnStartup(context.Background())

	result := c.Action("git.push").Run(context.Background(), core.NewOptions(
		core.Option{Key: "path", Value: dir},
	))
	assert.False(t, result.OK, "push without remote should fail")
}

func TestService_Action_Bad_PullNoRemote(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}
	svc.OnStartup(context.Background())

	result := c.Action("git.pull").Run(context.Background(), core.NewOptions(
		core.Option{Key: "path", Value: dir},
	))
	assert.False(t, result.OK, "pull without remote should fail")
}

func TestService_Action_Good_PushMultiple(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}
	svc.OnStartup(context.Background())

	opts := core.NewOptions()
	opts.Set("paths", []string{dir})
	opts.Set("names", map[string]string{dir: "test"})
	result := c.Action("git.push-multiple").Run(context.Background(), opts)
	_ = svc

	// PushMultiple returns results even when individual pushes fail.
	assert.True(t, result.OK)

	results, ok := result.Value.([]PushResult)
	require.True(t, ok)
	assert.Len(t, results, 1)
	assert.False(t, results[0].Success) // No remote
}

func TestService_Action_Good_PullMultiple(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}
	svc.OnStartup(context.Background())

	opts := core.NewOptions()
	opts.Set("paths", []string{dir})
	opts.Set("names", map[string]string{dir: "test"})
	result := c.Action("git.pull-multiple").Run(context.Background(), opts)
	_ = svc

	assert.True(t, result.OK)
	results, ok := result.Value.([]PullResult)
	require.True(t, ok)
	assert.Len(t, results, 1)
	assert.Equal(t, "test", results[0].Name)
	assert.False(t, results[0].Success)
	assert.Error(t, results[0].Error)
}

func TestService_HandleTask_Good_PushMultiple(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTask(c, TaskPushMultiple{
		Paths: []string{dir},
		Names: map[string]string{dir: "test"},
	})

	assert.True(t, result.OK)
	results, ok := result.Value.([]PushResult)
	require.True(t, ok)
	assert.Len(t, results, 1)
	assert.Equal(t, "test", results[0].Name)
	assert.False(t, results[0].Success)
	assert.Error(t, results[0].Error)
}

func TestService_HandleTask_Good_PullMultiple(t *testing.T) {
	dir, _ := filepath.Abs(initTestRepo(t))

	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTask(c, TaskPullMultiple{
		Paths: []string{dir},
		Names: map[string]string{dir: "test"},
	})

	assert.True(t, result.OK)
	results, ok := result.Value.([]PullResult)
	require.True(t, ok)
	assert.Len(t, results, 1)
	assert.Equal(t, "test", results[0].Name)
	assert.False(t, results[0].Success)
	assert.Error(t, results[0].Error)
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

	require.NoError(t, os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v1"), 0644))
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
	require.NoError(t, os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v2"), 0644))
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
