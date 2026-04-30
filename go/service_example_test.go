package git

func ExampleNewService() {
	r := NewService(ServiceOptions{})(New())
	Println(r.OK)
	Println(r.Value.(*Service) != nil)
	// Output:
	// true
	// true
}

func ExampleService_OnStartup() {
	c := New()
	svc := &Service{ServiceRuntime: NewServiceRuntime(c, ServiceOptions{})}
	r := svc.OnStartup(Background())
	Println(r.OK)
	Println(c.Action(actionGitPush).Exists())
	// Output:
	// true
	// true
}

func ExampleService_Status() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "repo", Branch: "main"}}}
	Println(svc.Status()[0].Name)
	// Output: repo
}

func ExampleService_All() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "one"}, {Name: "two"}}}
	Println(len(collectSeq(svc.All())))
	// Output: 2
}

func ExampleService_Dirty() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "clean"}, {Name: "dirty", Modified: 1}}}
	Println(collectSeq(svc.Dirty())[0].Name)
	// Output: dirty
}

func ExampleService_Ahead() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "synced"}, {Name: "ahead", Ahead: 1}}}
	Println(collectSeq(svc.Ahead())[0].Name)
	// Output: ahead
}

func ExampleService_Behind() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "synced"}, {Name: "behind", Behind: 1}}}
	Println(collectSeq(svc.Behind())[0].Name)
	// Output: behind
}

func ExampleService_DirtyRepos() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "dirty", Modified: 1}}}
	Println(svc.DirtyRepos()[0].Name)
	// Output: dirty
}

func ExampleService_AheadRepos() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "ahead", Ahead: 1}}}
	Println(svc.AheadRepos()[0].Name)
	// Output: ahead
}

func ExampleService_BehindRepos() {
	svc := &Service{lastStatus: []RepoStatus{{Name: "behind", Behind: 1}}}
	Println(svc.BehindRepos()[0].Name)
	// Output: behind
}
