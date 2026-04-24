package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dappco.re/go/core"
)

// --- validatePath tests ---

func TestService_ValidatePath_Bad_RelativePath(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("relative/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "path must be absolute") {
		t.Fatalf("expected %v to contain %v", err.Error(), "path must be absolute")
	}
}

func TestService_ValidatePath_Bad_OutsideWorkDir(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("/etc/passwd")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "outside of allowed WorkDir") {
		t.Fatalf("expected %v to contain %v", err.Error(), "outside of allowed WorkDir")
	}
}

func TestService_ValidatePath_Bad_OutsideWorkDirPrefix(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("/home/repos2")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "outside of allowed WorkDir") {
		t.Fatalf("expected %v to contain %v", err.Error(), "outside of allowed WorkDir")
	}
}

func TestService_ValidatePath_Bad_WorkDirNotAbsolute(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "relative/workdir"}}
	err := svc.validatePath("/any/absolute/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "WorkDir must be absolute") {
		t.Fatalf("expected %v to contain %v", err.Error(), "WorkDir must be absolute")
	}
}

func TestService_ValidatePath_Good_InsideWorkDir(t *testing.T) {
	svc := &Service{opts: ServiceOptions{WorkDir: "/home/repos"}}
	err := svc.validatePath("/home/repos/my-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_ValidatePath_Good_NoWorkDir(t *testing.T) {
	svc := &Service{opts: ServiceOptions{}}
	err := svc.validatePath("/any/absolute/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if result.OK {
		t.Fatal("expected false")
	}
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
	if result.OK {
		t.Fatal("expected false")
	}
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
	if result.OK {
		t.Fatal("expected false")
	}
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
	if result.OK {
		t.Fatal("expected false")
	}
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
	if result.OK {
		t.Fatal("expected false")
	}
}

func TestNewService_Good(t *testing.T) {
	opts := ServiceOptions{WorkDir: t.TempDir()}
	factory := NewService(opts)
	if factory == nil {
		t.Fatal("expected non-nil")
	}

	// Create a minimal Core to test the factory.
	c := core.New()

	svc, err := factory(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil")
	}

	service, ok := svc.(*Service)
	if !ok {
		t.Fatal("expected true")
	}
	if service == nil {
		t.Fatal("expected non-nil")
	}
}

func TestService_OnStartup_Good(t *testing.T) {
	c := core.New()

	opts := ServiceOptions{WorkDir: t.TempDir()}
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, opts),
		opts:           opts,
	}

	result := svc.OnStartup(context.Background())
	if !result.OK {
		t.Fatal("expected true")
	}
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

	if !result.OK {
		t.Fatal("expected true")
	}

	statuses, ok := result.Value.([]RepoStatus)
	if !ok {
		t.Fatal("expected true")
	}
	if len(statuses) != 1 {
		t.Fatalf("want %v, got %v", 1, len(statuses))
	}
	if "test-repo" != statuses[0].Name {
		t.Fatalf("want %v, got %v", "test-repo", statuses[0].Name)
	}

	// Verify lastStatus was updated.
	if len(svc.lastStatus) != 1 {
		t.Fatalf("want %v, got %v", 1, len(svc.lastStatus))
	}
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
	if !result.OK {
		t.Fatal("expected true")
	}

	dirty, ok := result.Value.([]RepoStatus)
	if !ok {
		t.Fatal("expected true")
	}
	if len(dirty) != 1 {
		t.Fatalf("want %v, got %v", 1, len(dirty))
	}
	if "dirty" != dirty[0].Name {
		t.Fatalf("want %v, got %v", "dirty", dirty[0].Name)
	}
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
	if !result.OK {
		t.Fatal("expected true")
	}

	ahead, ok := result.Value.([]RepoStatus)
	if !ok {
		t.Fatal("expected true")
	}
	if len(ahead) != 1 {
		t.Fatalf("want %v, got %v", 1, len(ahead))
	}
	if "ahead" != ahead[0].Name {
		t.Fatalf("want %v, got %v", "ahead", ahead[0].Name)
	}
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
	if !result.OK {
		t.Fatal("expected true")
	}

	behind, ok := result.Value.([]RepoStatus)
	if !ok {
		t.Fatal("expected true")
	}
	if len(behind) != 1 {
		t.Fatalf("want %v, got %v", 1, len(behind))
	}
	if "behind" != behind[0].Name {
		t.Fatalf("want %v, got %v", "behind", behind[0].Name)
	}
}

func TestService_HandleTaskMessage_Good_TaskPush(t *testing.T) {
	bareDir, _ := filepath.Abs(t.TempDir())
	cloneDir, _ := filepath.Abs(t.TempDir())

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	if err := os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v2"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "second commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTaskMessage(c, TaskPush{Path: cloneDir})
	if !result.OK {
		t.Fatal("expected true")
	}
}

func TestService_HandleTaskMessage_Ignores_UnknownTask(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTaskMessage(c, struct{}{})
	if result.OK {
		t.Fatal("expected false")
	}
	if result.Value != nil {
		t.Fatalf("expected nil, got %v", result.Value)
	}
}

func TestService_HandleTask_Bad_UnknownTask(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleTask(c, struct{}{})
	if result.OK {
		t.Fatal("expected false")
	}
	err := result.Value.(error)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported task type") {
		t.Fatalf("expected %v to contain %v", err.Error(), "unsupported task type")
	}
}

func TestService_Action_Good_TaskPush(t *testing.T) {
	bareDir, _ := filepath.Abs(t.TempDir())
	cloneDir, _ := filepath.Abs(t.TempDir())

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	if err := os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v2"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "second commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	c := core.New()
	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}
	svc.OnStartup(context.Background())

	result := c.ACTION(TaskPush{Path: cloneDir})
	if !result.OK {
		t.Fatal("expected true")
	}

	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 0 != ahead {
		t.Fatalf("want %v, got %v", 0, ahead)
	}
	if 0 != behind {
		t.Fatalf("want %v, got %v", 0, behind)
	}
}

func TestService_HandleQuery_Ignores_UnknownQuery(t *testing.T) {
	c := core.New()

	svc := &Service{
		ServiceRuntime: core.NewServiceRuntime(c, ServiceOptions{}),
	}

	result := svc.handleQuery(c, "unknown query type")
	if result.OK {
		t.Fatal("expected false")
	}
	if result.Value != nil {
		t.Fatalf("expected nil, got %v", result.Value)
	}
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
	if result.OK {
		t.Fatal("push without remote should fail: expected false")
	}
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
	if result.OK {
		t.Fatal("pull without remote should fail: expected false")
	}
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

	// PushMultiple returns results even when individual pushes fail, but the
	// overall action should still report failure.
	if result.OK {
		t.Fatal("expected false")
	}

	results, ok := result.Value.([]PushResult)
	if !ok {
		t.Fatal("expected true")
	}
	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if results[0].Success { // No remote
		t.Fatal("expected false")
	}
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

	if result.OK {
		t.Fatal("expected false")
	}
	results, ok := result.Value.([]PullResult)
	if !ok {
		t.Fatal("expected true")
	}
	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if "test" != results[0].Name {
		t.Fatalf("want %v, got %v", "test", results[0].Name)
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
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

	if result.OK {
		t.Fatal("expected false")
	}
	results, ok := result.Value.([]PushResult)
	if !ok {
		t.Fatal("expected true")
	}
	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if "test" != results[0].Name {
		t.Fatalf("want %v, got %v", "test", results[0].Name)
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
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

	if result.OK {
		t.Fatal("expected false")
	}
	results, ok := result.Value.([]PullResult)
	if !ok {
		t.Fatal("expected true")
	}
	if len(results) != 1 {
		t.Fatalf("want %v, got %v", 1, len(results))
	}
	if "test" != results[0].Name {
		t.Fatalf("want %v, got %v", "test", results[0].Name)
	}
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[0].Error == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Additional git operation tests ---

func TestGetStatus_Good_AheadBehindNoUpstream(t *testing.T) {
	// A repo without a tracking branch should return 0 ahead/behind.
	dir, _ := filepath.Abs(initTestRepo(t))

	status := getStatus(context.Background(), dir, "no-upstream")
	if status.Error != nil {
		t.Fatalf("unexpected error: %v", status.Error)
	}
	if 0 != status.Ahead {
		t.Fatalf("want %v, got %v", 0, status.Ahead)
	}
	if 0 != status.Behind {
		t.Fatalf("want %v, got %v", 0, status.Behind)
	}
}

func TestPushMultiple_Good_Empty(t *testing.T) {
	results, err := PushMultiple(context.Background(), []string{}, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("want %v, got %v", 0, len(results))
	}
}

func TestPushMultiple_Good_MultiplePaths(t *testing.T) {
	dir1, _ := filepath.Abs(initTestRepo(t))
	dir2, _ := filepath.Abs(initTestRepo(t))

	results, err := PushMultiple(context.Background(), []string{dir1, dir2}, map[string]string{
		dir1: "repo-1",
		dir2: "repo-2",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(results) != 2 {
		t.Fatalf("want %v, got %v", 2, len(results))
	}
	if "repo-1" != results[0].Name {
		t.Fatalf("want %v, got %v", "repo-1", results[0].Name)
	}
	if "repo-2" != results[1].Name {
		t.Fatalf("want %v, got %v", "repo-2", results[1].Name)
	}
	// Both should fail (no remote).
	if results[0].Success {
		t.Fatal("expected false")
	}
	if results[1].Success {
		t.Fatal("expected false")
	}
}

func TestPush_Good_WithRemote(t *testing.T) {
	// Create a bare remote and a clone.
	bareDir, _ := filepath.Abs(t.TempDir())
	cloneDir, _ := filepath.Abs(t.TempDir())

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd = exec.Command("git", "clone", bareDir, cloneDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
			t.Fatalf("failed: %v: %s: %v", args, string(out), err)
		}
	}

	// Make a local commit.
	if err := os.WriteFile(core.JoinPath(cloneDir, "file.txt"), []byte("v2"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "second commit"},
	} {
		cmd = exec.Command(args[0], args[1:]...)
		cmd.Dir = cloneDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Push should succeed.
	err := Push(context.Background(), cloneDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ahead count is now 0.
	ahead, behind, err := getAheadBehind(context.Background(), cloneDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if 0 != ahead {
		t.Fatalf("want %v, got %v", 0, ahead)
	}
	if 0 != behind {
		t.Fatalf("want %v, got %v", 0, behind)
	}
}
