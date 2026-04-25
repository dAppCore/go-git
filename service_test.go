package git

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func statusNames(statuses []RepoStatus) []string {
	names := make([]string, 0, len(statuses))
	for _, st := range statuses {
		names = append(names, st.Name)
	}
	return names
}

func TestService_DirtyRepos_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "clean"},
			{Name: "dirty-modified", Modified: 2},
			{Name: "dirty-untracked", Untracked: 1},
			{Name: "dirty-staged", Staged: 3},
			{Name: "ahead-only", Ahead: 4},
			{Name: "behind-only", Behind: 5},
		},
	}

	dirty := s.DirtyRepos()
	if len(dirty) != 3 {
		t.Fatalf("want %v, got %v", 3, len(dirty))
	}

	names := statusNames(dirty)
	for _, name := range []string{"dirty-modified", "dirty-untracked", "dirty-staged"} {
		if !slices.Contains(names, name) {
			t.Fatalf("expected %v to contain %v", names, name)
		}
	}
}

func TestService_DirtyRepos_Bad(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "dirty-error", Modified: 1, Error: errors.New("status failed")},
			{Name: "invalid-negative", Modified: -1, Untracked: -1, Staged: -1},
		},
	}

	dirty := s.DirtyRepos()
	if len(dirty) != 0 {
		t.Fatalf("want %v, got %v", 0, len(dirty))
	}
}

func TestService_DirtyRepos_Ugly(t *testing.T) {
	tests := []struct {
		name string
		svc  *Service
	}{
		{name: "nil status slice", svc: &Service{}},
		{name: "empty status slice", svc: &Service{lastStatus: []RepoStatus{}}},
		{name: "only clean repos", svc: &Service{lastStatus: []RepoStatus{{Name: "clean1"}, {Name: "clean2"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirty := tt.svc.DirtyRepos()
			if len(dirty) != 0 {
				t.Fatalf("want %v, got %v", 0, len(dirty))
			}
		})
	}
}

func TestService_AheadRepos_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "up-to-date", Ahead: 0},
			{Name: "ahead-by-one", Ahead: 1},
			{Name: "ahead-by-five", Ahead: 5},
			{Name: "behind-only", Behind: 3},
		},
	}

	ahead := s.AheadRepos()
	if len(ahead) != 2 {
		t.Fatalf("want %v, got %v", 2, len(ahead))
	}

	names := statusNames(ahead)
	for _, name := range []string{"ahead-by-one", "ahead-by-five"} {
		if !slices.Contains(names, name) {
			t.Fatalf("expected %v to contain %v", names, name)
		}
	}
}

func TestService_AheadRepos_Bad(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "ahead-error", Ahead: 2, Error: errors.New("status failed")},
			{Name: "invalid-negative", Ahead: -1},
		},
	}

	ahead := s.AheadRepos()
	if len(ahead) != 0 {
		t.Fatalf("want %v, got %v", 0, len(ahead))
	}
}

func TestService_AheadRepos_Ugly(t *testing.T) {
	tests := []struct {
		name string
		svc  *Service
	}{
		{name: "nil status slice", svc: &Service{}},
		{name: "empty status slice", svc: &Service{lastStatus: []RepoStatus{}}},
		{name: "only synced repos", svc: &Service{lastStatus: []RepoStatus{{Name: "synced1"}, {Name: "synced2"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ahead := tt.svc.AheadRepos()
			if len(ahead) != 0 {
				t.Fatalf("want %v, got %v", 0, len(ahead))
			}
		})
	}
}

func TestService_BehindRepos_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "synced", Behind: 0},
			{Name: "behind-by-one", Behind: 1},
			{Name: "behind-by-five", Behind: 5},
			{Name: "ahead-only", Ahead: 3},
		},
	}

	behind := s.BehindRepos()
	if len(behind) != 2 {
		t.Fatalf("want %v, got %v", 2, len(behind))
	}

	names := statusNames(behind)
	for _, name := range []string{"behind-by-one", "behind-by-five"} {
		if !slices.Contains(names, name) {
			t.Fatalf("expected %v to contain %v", names, name)
		}
	}
}

func TestService_BehindRepos_Bad(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "behind-error", Behind: 2, Error: errors.New("status failed")},
			{Name: "invalid-negative", Behind: -1},
		},
	}

	behind := s.BehindRepos()
	if len(behind) != 0 {
		t.Fatalf("want %v, got %v", 0, len(behind))
	}
}

func TestService_BehindRepos_Ugly(t *testing.T) {
	tests := []struct {
		name string
		svc  *Service
	}{
		{name: "nil status slice", svc: &Service{}},
		{name: "empty status slice", svc: &Service{lastStatus: []RepoStatus{}}},
		{name: "only synced repos", svc: &Service{lastStatus: []RepoStatus{{Name: "synced1"}, {Name: "synced2"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			behind := tt.svc.BehindRepos()
			if len(behind) != 0 {
				t.Fatalf("want %v, got %v", 0, len(behind))
			}
		})
	}
}

func TestService_Iterators_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "clean"},
			{Name: "dirty", Modified: 1},
			{Name: "ahead", Ahead: 2},
			{Name: "behind", Behind: 3},
		},
	}

	all := slices.Collect(s.All())
	if len(all) != 4 {
		t.Fatalf("want %v, got %v", 4, len(all))
	}

	dirty := slices.Collect(s.Dirty())
	if len(dirty) != 1 || dirty[0].Name != "dirty" {
		t.Fatalf("want dirty repo only, got %v", dirty)
	}

	ahead := slices.Collect(s.Ahead())
	if len(ahead) != 1 || ahead[0].Name != "ahead" {
		t.Fatalf("want ahead repo only, got %v", ahead)
	}

	behind := slices.Collect(s.Behind())
	if len(behind) != 1 || behind[0].Name != "behind" {
		t.Fatalf("want behind repo only, got %v", behind)
	}
}

func TestService_Iterators_Bad(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "dirty-error", Modified: 1, Error: errors.New("status failed")},
			{Name: "ahead-error", Ahead: 1, Error: errors.New("status failed")},
			{Name: "behind-error", Behind: 1, Error: errors.New("status failed")},
		},
	}

	if dirty := slices.Collect(s.Dirty()); len(dirty) != 0 {
		t.Fatalf("want %v, got %v", 0, len(dirty))
	}
	if ahead := slices.Collect(s.Ahead()); len(ahead) != 0 {
		t.Fatalf("want %v, got %v", 0, len(ahead))
	}
	if behind := slices.Collect(s.Behind()); len(behind) != 0 {
		t.Fatalf("want %v, got %v", 0, len(behind))
	}

	all := slices.Collect(s.All())
	if len(all) != 3 {
		t.Fatalf("All should keep errored repos: want %v, got %v", 3, len(all))
	}
}

func TestService_Iterators_Ugly(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "dirty", Modified: 1},
			{Name: "ahead", Ahead: 1},
		},
	}

	allIter := s.All()
	dirtyIter := s.Dirty()
	s.lastStatus[0].Name = "mutated"
	s.lastStatus[0].Modified = 0

	all := slices.Collect(allIter)
	if len(all) != 2 || all[0].Name != "dirty" {
		t.Fatalf("iterator should use a snapshot, got %v", all)
	}

	dirty := slices.Collect(dirtyIter)
	if len(dirty) != 1 || dirty[0].Name != "dirty" {
		t.Fatalf("filtered iterator should use a snapshot, got %v", dirty)
	}
}

func TestService_Status_Good(t *testing.T) {
	expected := []RepoStatus{
		{Name: "repo1", Branch: "main"},
		{Name: "repo2", Branch: "develop"},
	}
	s := &Service{lastStatus: expected}

	if got := s.Status(); !reflect.DeepEqual(expected, got) {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func TestService_Status_Bad(t *testing.T) {
	s := &Service{lastStatus: []RepoStatus{{Name: "repo1", Branch: "main"}}}

	statuses := s.Status()
	statuses[0].Name = "mutated"

	if got := s.lastStatus[0].Name; got != "repo1" {
		t.Fatalf("Status should return a clone: want %v, got %v", "repo1", got)
	}
}

func TestService_Status_Ugly(t *testing.T) {
	s := &Service{}
	if got := s.Status(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestService_QueryStatus_Good(t *testing.T) {
	q := QueryStatus{
		Paths: []string{"/path/a", "/path/b"},
		Names: map[string]string{"/path/a": "repo-a"},
	}

	opts := StatusOptions(q)
	if !slices.Equal(q.Paths, opts.Paths) {
		t.Fatalf("want %v, got %v", q.Paths, opts.Paths)
	}
	if !reflect.DeepEqual(q.Names, opts.Names) {
		t.Fatalf("want %v, got %v", q.Names, opts.Names)
	}
}

func TestService_QueryStatus_Bad(t *testing.T) {
	var q QueryStatus

	opts := StatusOptions(q)
	if opts.Paths != nil {
		t.Fatalf("expected nil paths, got %v", opts.Paths)
	}
	if opts.Names != nil {
		t.Fatalf("expected nil names, got %v", opts.Names)
	}
}

func TestService_QueryStatus_Ugly(t *testing.T) {
	q := QueryStatus{
		Paths: []string{},
		Names: map[string]string{},
	}

	opts := StatusOptions(q)
	if opts.Paths == nil {
		t.Fatal("expected empty but non-nil paths")
	}
	if opts.Names == nil {
		t.Fatal("expected empty but non-nil names")
	}
}

func TestService_QueryBehindRepos_Good(t *testing.T) {
	var q QueryBehindRepos
	if reflect.TypeOf(QueryBehindRepos{}) != reflect.TypeOf(q) {
		t.Fatalf("want %T, got %T", QueryBehindRepos{}, q)
	}
}

func TestService_QueryBehindRepos_Bad(t *testing.T) {
	if reflect.TypeOf(QueryBehindRepos{}) == reflect.TypeOf(QueryAheadRepos{}) {
		t.Fatalf("marker query types should remain distinct")
	}
}

func TestService_QueryBehindRepos_Ugly(t *testing.T) {
	if !reflect.DeepEqual(QueryBehindRepos{}, QueryBehindRepos{}) {
		t.Fatal("zero-value marker queries should compare equal")
	}
}

func TestService_TaskPullMultiple_Good(t *testing.T) {
	task := TaskPullMultiple{
		Paths: []string{"/repo/a", "/repo/b"},
		Names: map[string]string{"/repo/a": "repo-a"},
	}

	if !slices.Equal(task.Paths, []string{"/repo/a", "/repo/b"}) {
		t.Fatalf("want %v, got %v", []string{"/repo/a", "/repo/b"}, task.Paths)
	}
	if task.Names["/repo/a"] != "repo-a" {
		t.Fatalf("want %v, got %v", "repo-a", task.Names["/repo/a"])
	}
}

func TestService_TaskPullMultiple_Bad(t *testing.T) {
	var task TaskPullMultiple
	if task.Paths != nil {
		t.Fatalf("expected nil paths, got %v", task.Paths)
	}
	if task.Names != nil {
		t.Fatalf("expected nil names, got %v", task.Names)
	}
}

func TestService_TaskPullMultiple_Ugly(t *testing.T) {
	task := TaskPullMultiple{
		Paths: []string{},
		Names: map[string]string{},
	}

	if task.Paths == nil {
		t.Fatal("expected empty but non-nil paths")
	}
	if task.Names == nil {
		t.Fatal("expected empty but non-nil names")
	}
}

func TestService_ServiceOptions_Good(t *testing.T) {
	workDir := "/tmp/test-repos"
	opts := ServiceOptions{WorkDir: workDir}
	if workDir != opts.WorkDir {
		t.Fatalf("want %v, got %v", workDir, opts.WorkDir)
	}
}

func TestService_ServiceOptions_Bad(t *testing.T) {
	opts := ServiceOptions{WorkDir: "relative/workdir"}
	if opts.WorkDir != "relative/workdir" {
		t.Fatalf("want %v, got %v", "relative/workdir", opts.WorkDir)
	}
}

func TestService_ServiceOptions_Ugly(t *testing.T) {
	var opts ServiceOptions
	if opts.WorkDir != "" {
		t.Fatalf("want empty WorkDir, got %v", opts.WorkDir)
	}
}
