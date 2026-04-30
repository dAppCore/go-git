package git

func ExampleRepoStatus_IsDirty() {
	status := RepoStatus{Modified: 1}
	Println(status.IsDirty())
	// Output: true
}

func ExampleRepoStatus_HasUnpushed() {
	status := RepoStatus{Ahead: 2}
	Println(status.HasUnpushed())
	// Output: true
}

func ExampleRepoStatus_HasUnpulled() {
	status := RepoStatus{Behind: 2}
	Println(status.HasUnpulled())
	// Output: true
}

func ExampleStatus() {
	statuses := Status(Background(), StatusOptions{Paths: []string{relativeRepoPath}})
	Println(len(statuses))
	Println(statuses[0].Error != nil)
	// Output:
	// 1
	// true
}

func ExampleStatusIter() {
	count := 0
	for status := range StatusIter(Background(), StatusOptions{Paths: []string{relativeRepoPath}}) {
		if status.Error != nil {
			count++
		}
	}
	Println(count)
	// Output: 1
}

func ExamplePush() {
	r := Push(Background(), relativeRepoPath)
	Println(r.OK)
	// Output: false
}

func ExamplePull() {
	r := Pull(Background(), relativeRepoPath)
	Println(r.OK)
	// Output: false
}

func ExampleIsNonFastForward() {
	Println(IsNonFastForward(NewError("updates were rejected: fetch first")))
	// Output: true
}

func ExamplePushMultiple() {
	r := PushMultiple(Background(), []string{relativeRepoPath}, nil)
	Println(r.OK)
	Println(len(r.Value.([]PushResult)))
	// Output:
	// false
	// 1
}

func ExamplePushMultipleIter() {
	results := collectSeq(PushMultipleIter(Background(), []string{relativeRepoPath}, nil))
	Println(len(results))
	Println(results[0].Success)
	// Output:
	// 1
	// false
}

func ExamplePullMultiple() {
	r := PullMultiple(Background(), []string{relativeRepoPath}, nil)
	Println(r.OK)
	Println(len(r.Value.([]PullResult)))
	// Output:
	// false
	// 1
}

func ExamplePullMultipleIter() {
	results := collectSeq(PullMultipleIter(Background(), []string{relativeRepoPath}, nil))
	Println(len(results))
	Println(results[0].Success)
	// Output:
	// 1
	// false
}

func ExampleGitError_Error() {
	err := &GitError{Args: []string{"status"}, Stderr: "fatal: not a git repository"}
	Println(err.Error())
	// Output: git command "git status" failed: fatal: not a git repository
}
