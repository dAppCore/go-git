package git

import (
	"context"
	"testing"

	"dappco.re/go/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceExtra_ValidatePath_RelativePath_Bad(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}

	err := svc.validatePath("relative/path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be absolute")
}

func TestServiceExtra_ValidatePath_OutsideWorkDir_Bad(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}

	err := svc.validatePath("/etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "outside of allowed WorkDir")
}

func TestServiceExtra_ValidatePath_InsideWorkDir_Good(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}

	err := svc.validatePath("/home/repos/my-project")
	assert.NoError(t, err)
}

func TestServiceExtra_ValidatePath_NoWorkDir_Good(t *testing.T) {
	svc := &Service{opts: ServiceOptions{}}

	err := svc.validatePath("/any/absolute/path")
	assert.NoError(t, err)
}

func TestServiceExtra_HandleQuery_InvalidPath_Bad(t *testing.T) {
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

func TestServiceExtra_HandleTask_PushInvalidPath_Bad(t *testing.T) {
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}

	result := svc.handleTask(c, TaskPush{Path: "relative/path"})
	assert.False(t, result.OK)
}

func TestServiceExtra_HandleTask_PullInvalidPath_Bad(t *testing.T) {
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}

	result := svc.handleTask(c, TaskPull{Path: "/etc/passwd"})
	assert.False(t, result.OK)
}

func TestServiceExtra_HandleTask_PushMultipleInvalidPath_Bad(t *testing.T) {
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{WorkDir: "/home/repos"}),
		opts:           ServiceOptions{WorkDir: "/home/repos"},
	}

	result := svc.handleTask(c, TaskPushMultiple{
		Paths: []string{"/home/repos/ok", "/etc/bad"},
		Names: map[string]string{},
	})
	assert.False(t, result.OK)
}

func TestServiceExtra_NewService_Good(t *testing.T) {
	opts := ServiceOptions{WorkDir: t.TempDir()}
	factory := NewService(opts)
	assert.NotNil(t, factory)

	c := core.New()
	svc, err := factory(c)
	require.NoError(t, err)
	assert.NotNil(t, svc)

	service, ok := svc.(*Service)
	require.True(t, ok)
	assert.NotNil(t, service)
}

func TestServiceExtra_OnStartup_Good(t *testing.T) {
	c := core.New()
	opts := ServiceOptions{WorkDir: t.TempDir()}
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, opts),
		opts:           opts,
	}

	err := svc.OnStartup(context.Background())
	assert.NoError(t, err)
}

func TestServiceExtra_HandleQuery_Status_Good(t *testing.T) {
	dir := initTestRepo(t)
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleQuery(c, QueryStatus{
		Paths: []string{dir},
		Names: map[string]string{dir: "test-repo"},
	})
	assert.True(t, result.OK)

	statuses, ok := result.Value.([]RepoStatus)
	require.True(t, ok)
	require.Len(t, statuses, 1)
	assert.Equal(t, "test-repo", statuses[0].Name)
	assert.Len(t, svc.lastStatus, 1)
}

func TestServiceExtra_HandleQuery_DirtyRepos_Good(t *testing.T) {
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

func TestServiceExtra_HandleQuery_AheadRepos_Good(t *testing.T) {
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

func TestServiceExtra_HandleQuery_UnknownQuery_Bad(t *testing.T) {
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleQuery(c, "unknown query type")
	assert.False(t, result.OK)
	assert.Nil(t, result.Value)
}

func TestServiceExtra_HandleTask_PushNoRemote_Bad(t *testing.T) {
	dir := initTestRepo(t)
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTask(c, TaskPush{Path: dir, Name: "test"})
	assert.False(t, result.OK, "push without remote should fail")
}

func TestServiceExtra_HandleTask_PullNoRemote_Bad(t *testing.T) {
	dir := initTestRepo(t)
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTask(c, TaskPull{Path: dir, Name: "test"})
	assert.False(t, result.OK, "pull without remote should fail")
}

func TestServiceExtra_HandleTask_PushMultiple_Good(t *testing.T) {
	dir := initTestRepo(t)
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
	assert.False(t, results[0].Success)
}

func TestServiceExtra_HandleTask_UnknownTask_Bad(t *testing.T) {
	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTask(c, "unknown task")
	assert.False(t, result.OK)
	assert.Nil(t, result.Value)
}

func TestServiceExtra_GetStatus_AheadBehindNoUpstream_Good(t *testing.T) {
	dir := initTestRepo(t)

	status := getStatus(context.Background(), dir, "no-upstream")
	require.NoError(t, status.Error)
	assert.Equal(t, 0, status.Ahead)
	assert.Equal(t, 0, status.Behind)
}

func TestServiceExtra_PushMultiple_Empty_Good(t *testing.T) {
	results, err := PushMultiple(context.Background(), []string{}, map[string]string{})
	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestServiceExtra_PushMultiple_NoRemote_MultiplePaths_Bad(t *testing.T) {
	dir1 := initTestRepo(t)
	dir2 := initTestRepo(t)

	results, err := PushMultiple(context.Background(), []string{dir1, dir2}, map[string]string{
		dir1: "repo-1",
		dir2: "repo-2",
	})
	assert.Error(t, err)

	require.Len(t, results, 2)
	assert.Equal(t, "repo-1", results[0].Name)
	assert.Equal(t, "repo-2", results[1].Name)
	assert.False(t, results[0].Success)
	assert.False(t, results[1].Success)
}

func TestServiceExtra_Push_WithRemote_Good(t *testing.T) {
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
		{"commit", "-m", "second commit"},
	} {
		runGit(t, cloneDir, args...)
	}

	err := Push(context.Background(), cloneDir)
	assert.NoError(t, err)

	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)
}
