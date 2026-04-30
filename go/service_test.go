package git

const (
	relativeWorkDir = "relative/workdir"
	statusFailed    = "status failed"
)

func repoNames(statuses []RepoStatus) []string {
	names := make([]string, 0, len(statuses))
	for _, st := range statuses {
		names = append(names, st.Name)
	}
	return names
}

func testService(statuses ...RepoStatus) *Service {
	return &Service{lastStatus: statuses}
}

func TestService_NewService_Good(t *T) {
	c := New()
	opts := ServiceOptions{WorkDir: testTempDir(t)}

	r := NewService(opts)(c)

	AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	AssertEqual(t, opts.WorkDir, svc.opts.WorkDir)
	AssertEqual(t, c, svc.Core())
}

func TestService_NewService_Bad(t *T) {
	c := New()
	opts := ServiceOptions{WorkDir: relativeWorkDir}

	r := NewService(opts)(c)

	AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	AssertEqual(t, relativeWorkDir, svc.opts.WorkDir)
}

func TestService_NewService_Ugly(t *T) {
	r := NewService(ServiceOptions{})(nil)

	AssertTrue(t, r.OK)
	svc := r.Value.(*Service)
	AssertNil(t, svc.Core())
	AssertEqual(t, "", svc.opts.WorkDir)
}

func TestService_Service_OnStartup_Good(t *T) {
	c := New()
	svc := &Service{ServiceRuntime: NewServiceRuntime(c, ServiceOptions{})}

	r := svc.OnStartup(Background())

	AssertTrue(t, r.OK)
	AssertTrue(t, c.Action(actionGitPush).Exists())
	AssertTrue(t, c.Action(actionGitPull).Exists())
}

func TestService_Service_OnStartup_Bad(t *T) {
	c := New()
	svc := &Service{ServiceRuntime: NewServiceRuntime(c, ServiceOptions{WorkDir: relativeWorkDir})}

	r := svc.OnStartup(Background())

	AssertTrue(t, r.OK)
	result := c.Action(actionGitPush).Run(Background(), NewOptions(Option{Key: actionPathKey, Value: relativeRepoPath}))
	AssertFalse(t, result.OK)
	AssertContains(t, result.Error(), pathMustBeAbsolute)
}

func TestService_Service_OnStartup_Ugly(t *T) {
	c := New()
	svc := &Service{ServiceRuntime: NewServiceRuntime(c, ServiceOptions{})}

	first := svc.OnStartup(Background())
	second := svc.OnStartup(Background())

	AssertTrue(t, first.OK)
	AssertTrue(t, second.OK)
	AssertTrue(t, c.Action(actionGitPullMultiple).Exists())
}

func TestService_Service_Status_Good(t *T) {
	expected := []RepoStatus{{Name: "repo1", Branch: "main"}, {Name: "repo2", Branch: "develop"}}
	svc := testService(expected...)

	got := svc.Status()

	AssertEqual(t, expected, got)
}

func TestService_Service_Status_Bad(t *T) {
	svc := testService(RepoStatus{Name: "repo1"})

	got := svc.Status()
	got[0].Name = "mutated"

	AssertEqual(t, "repo1", svc.lastStatus[0].Name)
}

func TestService_Service_Status_Ugly(t *T) {
	svc := testService()
	statuses := svc.Status()
	AssertNil(t, statuses)
	AssertLen(t, statuses, 0)
}

func TestService_Service_All_Good(t *T) {
	svc := testService(RepoStatus{Name: "clean"}, RepoStatus{Name: "dirty", Modified: 1})

	all := collectSeq(svc.All())

	AssertLen(t, all, 2)
	AssertEqual(t, []string{"clean", "dirty"}, repoNames(all))
}

func TestService_Service_All_Bad(t *T) {
	svc := testService()

	all := collectSeq(svc.All())

	AssertLen(t, all, 0)
}

func TestService_Service_All_Ugly(t *T) {
	svc := testService(RepoStatus{Name: "before"})
	iter := svc.All()
	svc.lastStatus[0].Name = "after"

	all := collectSeq(iter)

	AssertLen(t, all, 1)
	AssertEqual(t, "before", all[0].Name)
}

func TestService_Service_Dirty_Good(t *T) {
	svc := testService(RepoStatus{Name: "clean"}, RepoStatus{Name: "dirty", Modified: 1})

	dirty := collectSeq(svc.Dirty())

	AssertLen(t, dirty, 1)
	AssertEqual(t, "dirty", dirty[0].Name)
}

func TestService_Service_Dirty_Bad(t *T) {
	svc := testService(RepoStatus{Name: "errored", Modified: 1, Error: NewError(statusFailed)})

	dirty := collectSeq(svc.Dirty())

	AssertLen(t, dirty, 0)
}

func TestService_Service_Dirty_Ugly(t *T) {
	svc := testService(RepoStatus{Name: "dirty", Modified: 1})
	iter := svc.Dirty()
	svc.lastStatus[0].Modified = 0

	dirty := collectSeq(iter)

	AssertLen(t, dirty, 1)
	AssertEqual(t, "dirty", dirty[0].Name)
}

func TestService_Service_Ahead_Good(t *T) {
	svc := testService(RepoStatus{Name: "synced"}, RepoStatus{Name: "ahead", Ahead: 1})

	ahead := collectSeq(svc.Ahead())

	AssertLen(t, ahead, 1)
	AssertEqual(t, "ahead", ahead[0].Name)
}

func TestService_Service_Ahead_Bad(t *T) {
	svc := testService(RepoStatus{Name: "errored", Ahead: 1, Error: NewError(statusFailed)})

	ahead := collectSeq(svc.Ahead())

	AssertLen(t, ahead, 0)
}

func TestService_Service_Ahead_Ugly(t *T) {
	svc := testService(RepoStatus{Name: "ahead", Ahead: 1})
	iter := svc.Ahead()
	svc.lastStatus[0].Ahead = 0

	ahead := collectSeq(iter)

	AssertLen(t, ahead, 1)
	AssertEqual(t, "ahead", ahead[0].Name)
}

func TestService_Service_Behind_Good(t *T) {
	svc := testService(RepoStatus{Name: "synced"}, RepoStatus{Name: "behind", Behind: 1})

	behind := collectSeq(svc.Behind())

	AssertLen(t, behind, 1)
	AssertEqual(t, "behind", behind[0].Name)
}

func TestService_Service_Behind_Bad(t *T) {
	svc := testService(RepoStatus{Name: "errored", Behind: 1, Error: NewError(statusFailed)})

	behind := collectSeq(svc.Behind())

	AssertLen(t, behind, 0)
}

func TestService_Service_Behind_Ugly(t *T) {
	svc := testService(RepoStatus{Name: "behind", Behind: 1})
	iter := svc.Behind()
	svc.lastStatus[0].Behind = 0

	behind := collectSeq(iter)

	AssertLen(t, behind, 1)
	AssertEqual(t, "behind", behind[0].Name)
}

func TestService_Service_DirtyRepos_Good(t *T) {
	svc := testService(
		RepoStatus{Name: "clean"},
		RepoStatus{Name: "dirty-modified", Modified: 2},
		RepoStatus{Name: "dirty-untracked", Untracked: 1},
		RepoStatus{Name: "dirty-staged", Staged: 3},
	)

	dirty := svc.DirtyRepos()

	AssertLen(t, dirty, 3)
	AssertContains(t, repoNames(dirty), "dirty-modified")
	AssertContains(t, repoNames(dirty), "dirty-untracked")
	AssertContains(t, repoNames(dirty), "dirty-staged")
}

func TestService_Service_DirtyRepos_Bad(t *T) {
	svc := testService(RepoStatus{Name: "dirty-error", Modified: 1, Error: NewError(statusFailed)})

	dirty := svc.DirtyRepos()

	AssertLen(t, dirty, 0)
}

func TestService_Service_DirtyRepos_Ugly(t *T) {
	svc := testService()
	dirty := svc.DirtyRepos()
	AssertLen(t, dirty, 0)
	AssertNil(t, dirty)
}

func TestService_Service_AheadRepos_Good(t *T) {
	svc := testService(RepoStatus{Name: "synced"}, RepoStatus{Name: "ahead-one", Ahead: 1}, RepoStatus{Name: "ahead-two", Ahead: 2})

	ahead := svc.AheadRepos()

	AssertLen(t, ahead, 2)
	AssertContains(t, repoNames(ahead), "ahead-one")
	AssertContains(t, repoNames(ahead), "ahead-two")
}

func TestService_Service_AheadRepos_Bad(t *T) {
	svc := testService(RepoStatus{Name: "ahead-error", Ahead: 1, Error: NewError(statusFailed)})

	ahead := svc.AheadRepos()

	AssertLen(t, ahead, 0)
}

func TestService_Service_AheadRepos_Ugly(t *T) {
	svc := testService()
	ahead := svc.AheadRepos()
	AssertLen(t, ahead, 0)
	AssertNil(t, ahead)
}

func TestService_Service_BehindRepos_Good(t *T) {
	svc := testService(RepoStatus{Name: "synced"}, RepoStatus{Name: "behind-one", Behind: 1}, RepoStatus{Name: "behind-two", Behind: 2})

	behind := svc.BehindRepos()

	AssertLen(t, behind, 2)
	AssertContains(t, repoNames(behind), "behind-one")
	AssertContains(t, repoNames(behind), "behind-two")
}

func TestService_Service_BehindRepos_Bad(t *T) {
	svc := testService(RepoStatus{Name: "behind-error", Behind: 1, Error: NewError(statusFailed)})

	behind := svc.BehindRepos()

	AssertLen(t, behind, 0)
}

func TestService_Service_BehindRepos_Ugly(t *T) {
	svc := testService()
	behind := svc.BehindRepos()
	AssertLen(t, behind, 0)
	AssertNil(t, behind)
}
