package git

import (
	core "dappco.re/go"
)

type T = core.T
type Option = core.Option

var (
	AssertContains    = core.AssertContains
	AssertEqual       = core.AssertEqual
	AssertFalse       = core.AssertFalse
	AssertLen         = core.AssertLen
	AssertNil         = core.AssertNil
	AssertNotNil      = core.AssertNotNil
	AssertTrue        = core.AssertTrue
	Background        = core.Background
	MkdirAll          = core.MkdirAll
	MkdirTemp         = core.MkdirTemp
	New               = core.New
	NewError          = core.NewError
	NewOptions        = core.NewOptions
	NewServiceRuntime = core.NewServiceRuntime[ServiceOptions]
	Path              = core.Path
	PathDir           = core.PathDir
	Println           = core.Println
	RemoveAll         = core.RemoveAll
	RequireTrue       = core.RequireTrue
	WriteFile         = core.WriteFile
)

func testTempDir(t *T) string {
	t.Helper()
	r := MkdirTemp("", "go-git-test-")
	RequireTrue(t, r.OK, r.Error())
	dir := r.Value.(string)
	t.Cleanup(func() {
		cleanup := RemoveAll(dir)
		if !cleanup.OK {
			t.Logf("cleanup failed: %v", cleanup.Value)
		}
	})
	return dir
}

func runTestGit(t *T, dir string, args ...string) string {
	t.Helper()
	out, err := gitCmd(dir, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s: %v", args, string(out), err)
	}
	return string(out)
}

func writeTestFile(t *T, filePath, content string) {
	t.Helper()
	RequireTrue(t, MkdirAll(PathDir(filePath), 0o755).OK)
	r := WriteFile(filePath, []byte(content), 0o644)
	RequireTrue(t, r.OK, r.Error())
}

func configureTestGit(t *T, dir string) {
	t.Helper()
	runTestGit(t, dir, "config", "user.email", "test@example.com")
	runTestGit(t, dir, "config", "user.name", "Test User")
}

func commitTestFile(t *T, dir, name, content, message string) {
	t.Helper()
	writeTestFile(t, Path(dir, name), content)
	runTestGit(t, dir, "add", name)
	runTestGit(t, dir, "commit", "-m", message)
}

func initTestRepo(t *T) string {
	t.Helper()
	dir := testTempDir(t)
	runTestGit(t, dir, "init")
	configureTestGit(t, dir)
	commitTestFile(t, dir, "README.md", "# Test\n", "initial commit")
	return dir
}

func initBareRemote(t *T) string {
	t.Helper()
	dir := testTempDir(t)
	runTestGit(t, dir, "init", "--bare")
	return dir
}

func cloneTestRepo(t *T, remote string) string {
	t.Helper()
	dir := testTempDir(t)
	runTestGit(t, "", "clone", remote, dir)
	configureTestGit(t, dir)
	return dir
}

func initRemoteRepo(t *T) (string, string) {
	t.Helper()
	remote := initBareRemote(t)
	clone := cloneTestRepo(t, remote)
	commitTestFile(t, clone, "file.txt", "v1\n", "initial")
	runTestGit(t, clone, "push", "-u", "origin", "HEAD")
	return remote, clone
}

func initPushableRepo(t *T) string {
	t.Helper()
	_, clone := initRemoteRepo(t)
	commitTestFile(t, clone, "file.txt", "v2\n", "local commit")
	return clone
}

func initPullableRepo(t *T) string {
	t.Helper()
	remote, upstream := initRemoteRepo(t)
	clone := cloneTestRepo(t, remote)
	commitTestFile(t, upstream, "file.txt", "v2\n", "remote commit")
	runTestGit(t, upstream, "push", "origin", "HEAD")
	return clone
}

func TestGit_RepoStatus_IsDirty_Good(t *T) {
	status := RepoStatus{Modified: 1}
	AssertTrue(t, status.IsDirty())
	AssertEqual(t, 1, status.Modified)
}

func TestGit_RepoStatus_IsDirty_Bad(t *T) {
	status := RepoStatus{Ahead: 2, Behind: 1}
	AssertFalse(t, status.IsDirty())
	AssertTrue(t, status.HasUnpushed())
}

func TestGit_RepoStatus_IsDirty_Ugly(t *T) {
	status := RepoStatus{Untracked: 1 << 20, Staged: 1 << 20}
	AssertTrue(t, status.IsDirty())
	AssertEqual(t, 1<<20, status.Untracked)
}

func TestGit_RepoStatus_HasUnpushed_Good(t *T) {
	status := RepoStatus{Ahead: 3}
	AssertTrue(t, status.HasUnpushed())
	AssertEqual(t, 3, status.Ahead)
}

func TestGit_RepoStatus_HasUnpushed_Bad(t *T) {
	status := RepoStatus{Ahead: -1}
	AssertFalse(t, status.HasUnpushed())
	AssertEqual(t, -1, status.Ahead)
}

func TestGit_RepoStatus_HasUnpushed_Ugly(t *T) {
	status := RepoStatus{}
	AssertFalse(t, status.HasUnpushed())
	AssertEqual(t, 0, status.Ahead)
}

func TestGit_RepoStatus_HasUnpulled_Good(t *T) {
	status := RepoStatus{Behind: 2}
	AssertTrue(t, status.HasUnpulled())
	AssertEqual(t, 2, status.Behind)
}

func TestGit_RepoStatus_HasUnpulled_Bad(t *T) {
	status := RepoStatus{Behind: -1}
	AssertFalse(t, status.HasUnpulled())
	AssertEqual(t, -1, status.Behind)
}

func TestGit_RepoStatus_HasUnpulled_Ugly(t *T) {
	status := RepoStatus{}
	AssertFalse(t, status.HasUnpulled())
	AssertEqual(t, 0, status.Behind)
}

func TestGit_Status_Good(t *T) {
	clean := initTestRepo(t)
	dirty := initTestRepo(t)
	writeTestFile(t, Path(dirty, "extra.txt"), "extra\n")

	statuses := Status(Background(), StatusOptions{
		Paths: []string{clean, dirty},
		Names: map[string]string{clean: "clean", dirty: "dirty"},
	})

	AssertLen(t, statuses, 2)
	AssertEqual(t, "clean", statuses[0].Name)
	AssertNil(t, statuses[0].Error)
	AssertFalse(t, statuses[0].IsDirty())
	AssertEqual(t, "dirty", statuses[1].Name)
	AssertNil(t, statuses[1].Error)
	AssertTrue(t, statuses[1].IsDirty())
}

func TestGit_Status_Bad(t *T) {
	statuses := Status(Background(), StatusOptions{Paths: []string{"relative/repo"}})
	AssertLen(t, statuses, 1)
	AssertNotNil(t, statuses[0].Error)
	AssertContains(t, statuses[0].Error.Error(), "path must be absolute")
}

func TestGit_Status_Ugly(t *T) {
	statuses := Status(nil, StatusOptions{})
	AssertLen(t, statuses, 0)
	AssertNil(t, statuses)
}

func TestGit_StatusIter_Good(t *T) {
	clean := initTestRepo(t)
	dirty := initTestRepo(t)
	writeTestFile(t, Path(dirty, "extra.txt"), "extra\n")

	statuses := collectSeq(StatusIter(Background(), StatusOptions{
		Paths: []string{clean, dirty},
		Names: map[string]string{clean: "clean", dirty: "dirty"},
	}))

	AssertLen(t, statuses, 2)
	AssertEqual(t, "clean", statuses[0].Name)
	AssertEqual(t, "dirty", statuses[1].Name)
	AssertTrue(t, statuses[1].IsDirty())
}

func TestGit_StatusIter_Bad(t *T) {
	statuses := collectSeq(StatusIter(Background(), StatusOptions{Paths: []string{"relative/repo"}}))
	AssertLen(t, statuses, 1)
	AssertNotNil(t, statuses[0].Error)
	AssertContains(t, statuses[0].Error.Error(), "path must be absolute")
}

func TestGit_StatusIter_Ugly(t *T) {
	repoA := initTestRepo(t)
	repoB := initTestRepo(t)
	var statuses []RepoStatus
	StatusIter(Background(), StatusOptions{Paths: []string{repoA, repoB}})(func(st RepoStatus) bool {
		statuses = append(statuses, st)
		return false
	})
	AssertLen(t, statuses, 1)
}

func TestGit_Push_Good(t *T) {
	dir := initPushableRepo(t)

	r := Push(Background(), dir)

	AssertTrue(t, r.OK, r.Error())
	status := Status(Background(), StatusOptions{Paths: []string{dir}})[0]
	AssertEqual(t, 0, status.Ahead)
	AssertEqual(t, 0, status.Behind)
}

func TestGit_Push_Bad(t *T) {
	r := Push(Background(), "relative/repo")
	AssertFalse(t, r.OK)
	AssertContains(t, r.Error(), "path must be absolute")
}

func TestGit_Push_Ugly(t *T) {
	dir := initPushableRepo(t)
	r := Push(nil, dir)
	AssertTrue(t, r.OK, r.Error())
}

func TestGit_Pull_Good(t *T) {
	dir := initPullableRepo(t)

	r := Pull(Background(), dir)

	AssertTrue(t, r.OK, r.Error())
	status := Status(Background(), StatusOptions{Paths: []string{dir}})[0]
	AssertEqual(t, 0, status.Ahead)
	AssertEqual(t, 0, status.Behind)
}

func TestGit_Pull_Bad(t *T) {
	r := Pull(Background(), "relative/repo")
	AssertFalse(t, r.OK)
	AssertContains(t, r.Error(), "path must be absolute")
}

func TestGit_Pull_Ugly(t *T) {
	dir := initPullableRepo(t)
	r := Pull(nil, dir)
	AssertTrue(t, r.OK, r.Error())
}

func TestGit_IsNonFastForward_Good(t *T) {
	err := NewError("updates were rejected because the remote contains work you do not have locally: fetch first")
	AssertTrue(t, IsNonFastForward(err))
	AssertContains(t, err.Error(), "fetch first")
}

func TestGit_IsNonFastForward_Bad(t *T) {
	err := NewError("connection refused")
	AssertFalse(t, IsNonFastForward(err))
	AssertContains(t, err.Error(), "connection refused")
}

func TestGit_IsNonFastForward_Ugly(t *T) {
	err := &GitError{Stderr: "UPDATES WERE REJECTED BECAUSE THE TIP OF YOUR CURRENT BRANCH IS BEHIND"}
	AssertTrue(t, IsNonFastForward(err))
	AssertContains(t, err.Error(), "BRANCH IS BEHIND")
}

func TestGit_PushMultiple_Good(t *T) {
	first := initPushableRepo(t)
	second := initPushableRepo(t)

	r := PushMultiple(Background(), []string{first, second}, map[string]string{first: "first", second: "second"})

	AssertTrue(t, r.OK, r.Error())
	results := r.Value.([]PushResult)
	AssertLen(t, results, 2)
	AssertTrue(t, results[0].Success)
	AssertTrue(t, results[1].Success)
	AssertEqual(t, "first", results[0].Name)
	AssertEqual(t, "second", results[1].Name)
}

func TestGit_PushMultiple_Bad(t *T) {
	r := PushMultiple(Background(), []string{"relative/repo"}, nil)
	AssertFalse(t, r.OK)
	results := r.Value.([]PushResult)
	AssertLen(t, results, 1)
	AssertNotNil(t, results[0].Error)
	AssertContains(t, results[0].Error.Error(), "path must be absolute")
}

func TestGit_PushMultiple_Ugly(t *T) {
	r := PushMultiple(Background(), []string{}, map[string]string{})
	AssertTrue(t, r.OK, r.Error())
	AssertLen(t, r.Value.([]PushResult), 0)
}

func TestGit_PushMultipleIter_Good(t *T) {
	dir := initPushableRepo(t)
	results := collectSeq(PushMultipleIter(Background(), []string{dir}, map[string]string{dir: "repo"}))

	AssertLen(t, results, 1)
	AssertTrue(t, results[0].Success)
	AssertEqual(t, "repo", results[0].Name)
}

func TestGit_PushMultipleIter_Bad(t *T) {
	results := collectSeq(PushMultipleIter(Background(), []string{"relative/repo"}, nil))

	AssertLen(t, results, 1)
	AssertFalse(t, results[0].Success)
	AssertNotNil(t, results[0].Error)
}

func TestGit_PushMultipleIter_Ugly(t *T) {
	dir := initPushableRepo(t)
	var results []PushResult
	PushMultipleIter(Background(), []string{"relative/repo", dir}, nil)(func(result PushResult) bool {
		results = append(results, result)
		return false
	})
	AssertLen(t, results, 1)
	AssertEqual(t, "relative/repo", results[0].Path)
}

func TestGit_PullMultiple_Good(t *T) {
	first := initPullableRepo(t)
	second := initPullableRepo(t)

	r := PullMultiple(Background(), []string{first, second}, map[string]string{first: "first", second: "second"})

	AssertTrue(t, r.OK, r.Error())
	results := r.Value.([]PullResult)
	AssertLen(t, results, 2)
	AssertTrue(t, results[0].Success)
	AssertTrue(t, results[1].Success)
	AssertEqual(t, "first", results[0].Name)
	AssertEqual(t, "second", results[1].Name)
}

func TestGit_PullMultiple_Bad(t *T) {
	r := PullMultiple(Background(), []string{"relative/repo"}, nil)
	AssertFalse(t, r.OK)
	results := r.Value.([]PullResult)
	AssertLen(t, results, 1)
	AssertNotNil(t, results[0].Error)
	AssertContains(t, results[0].Error.Error(), "path must be absolute")
}

func TestGit_PullMultiple_Ugly(t *T) {
	r := PullMultiple(Background(), []string{}, map[string]string{})
	AssertTrue(t, r.OK, r.Error())
	AssertLen(t, r.Value.([]PullResult), 0)
}

func TestGit_PullMultipleIter_Good(t *T) {
	dir := initPullableRepo(t)
	results := collectSeq(PullMultipleIter(Background(), []string{dir}, map[string]string{dir: "repo"}))

	AssertLen(t, results, 1)
	AssertTrue(t, results[0].Success)
	AssertEqual(t, "repo", results[0].Name)
}

func TestGit_PullMultipleIter_Bad(t *T) {
	results := collectSeq(PullMultipleIter(Background(), []string{"relative/repo"}, nil))

	AssertLen(t, results, 1)
	AssertFalse(t, results[0].Success)
	AssertNotNil(t, results[0].Error)
}

func TestGit_PullMultipleIter_Ugly(t *T) {
	dir := initPullableRepo(t)
	var results []PullResult
	PullMultipleIter(Background(), []string{"relative/repo", dir}, nil)(func(result PullResult) bool {
		results = append(results, result)
		return false
	})
	AssertLen(t, results, 1)
	AssertEqual(t, "relative/repo", results[0].Path)
}

func TestGit_GitError_Error_Good(t *T) {
	err := &GitError{Args: []string{"status"}, Err: NewError("exit"), Stderr: "fatal: not a git repository"}
	AssertEqual(t, "git command \"git status\" failed: fatal: not a git repository", err.Error())
	AssertContains(t, err.Error(), "not a git repository")
}

func TestGit_GitError_Error_Bad(t *T) {
	err := &GitError{Args: []string{"status"}}
	AssertEqual(t, "git command \"git status\" failed", err.Error())
	AssertEqual(t, []string{"status"}, err.Args)
}

func TestGit_GitError_Error_Ugly(t *T) {
	err := &GitError{Args: []string{"status", "--short"}, Err: NewError("fallback"), Stderr: "\n\tfatal: spaced stderr\n\n"}
	AssertEqual(t, "git command \"git status --short\" failed: fatal: spaced stderr", err.Error())
	AssertContains(t, err.Error(), "spaced stderr")
}
