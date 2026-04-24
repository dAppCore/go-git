package git

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

// --- Service helper method tests ---
// These test DirtyRepos/AheadRepos filtering without needing the framework.

func TestService_DirtyRepos_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "clean", Modified: 0, Untracked: 0, Staged: 0},
			{Name: "dirty-modified", Modified: 2},
			{Name: "dirty-untracked", Untracked: 1},
			{Name: "dirty-staged", Staged: 3},
			{Name: "errored", Modified: 5, Error: errors.New("test error")},
		},
	}

	dirty := s.DirtyRepos()
	if len(dirty) != 3 {
		t.Fatalf("want %v, got %v", 3, len(dirty))
	}

	names := slices.Collect(func(yield func(string) bool) {
		for _, d := range dirty {
			if !yield(d.Name) {
				return
			}
		}
	})
	if !slices.Contains(names, "dirty-modified") {
		t.Fatalf("expected %v to contain %v", names, "dirty-modified")
	}
	if !slices.Contains(names, "dirty-untracked") {
		t.Fatalf("expected %v to contain %v", names, "dirty-untracked")
	}
	if !slices.Contains(names, "dirty-staged") {
		t.Fatalf("expected %v to contain %v", names, "dirty-staged")
	}
}

func TestService_DirtyRepos_Good_NoneFound(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "clean1"},
			{Name: "clean2"},
		},
	}

	dirty := s.DirtyRepos()
	if len(dirty) != 0 {
		t.Fatalf("want %v, got %v", 0, len(dirty))
	}
}

func TestService_DirtyRepos_Good_EmptyStatus(t *testing.T) {
	s := &Service{}
	dirty := s.DirtyRepos()
	if len(dirty) != 0 {
		t.Fatalf("want %v, got %v", 0, len(dirty))
	}
}

func TestService_AheadRepos_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "up-to-date", Ahead: 0},
			{Name: "ahead-by-one", Ahead: 1},
			{Name: "ahead-by-five", Ahead: 5},
			{Name: "behind-only", Behind: 3},
			{Name: "errored-ahead", Ahead: 2, Error: errors.New("test error")},
		},
	}

	ahead := s.AheadRepos()
	if len(ahead) != 2 {
		t.Fatalf("want %v, got %v", 2, len(ahead))
	}

	names := slices.Collect(func(yield func(string) bool) {
		for _, a := range ahead {
			if !yield(a.Name) {
				return
			}
		}
	})
	if !slices.Contains(names, "ahead-by-one") {
		t.Fatalf("expected %v to contain %v", names, "ahead-by-one")
	}
	if !slices.Contains(names, "ahead-by-five") {
		t.Fatalf("expected %v to contain %v", names, "ahead-by-five")
	}
}

func TestService_AheadRepos_Good_NoneFound(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "synced1"},
			{Name: "synced2"},
		},
	}

	ahead := s.AheadRepos()
	if len(ahead) != 0 {
		t.Fatalf("want %v, got %v", 0, len(ahead))
	}
}

func TestService_AheadRepos_Good_EmptyStatus(t *testing.T) {
	s := &Service{}
	ahead := s.AheadRepos()
	if len(ahead) != 0 {
		t.Fatalf("want %v, got %v", 0, len(ahead))
	}
}

func TestService_BehindRepos_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "synced", Behind: 0},
			{Name: "behind-by-one", Behind: 1},
			{Name: "behind-by-five", Behind: 5},
			{Name: "ahead-only", Ahead: 3},
			{Name: "errored-behind", Behind: 2, Error: errors.New("test error")},
		},
	}

	behind := s.BehindRepos()
	if len(behind) != 2 {
		t.Fatalf("want %v, got %v", 2, len(behind))
	}

	names := slices.Collect(func(yield func(string) bool) {
		for _, b := range behind {
			if !yield(b.Name) {
				return
			}
		}
	})
	if !slices.Contains(names, "behind-by-one") {
		t.Fatalf("expected %v to contain %v", names, "behind-by-one")
	}
	if !slices.Contains(names, "behind-by-five") {
		t.Fatalf("expected %v to contain %v", names, "behind-by-five")
	}
}

func TestService_BehindRepos_Good_NoneFound(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "synced1"},
			{Name: "synced2"},
		},
	}

	behind := s.BehindRepos()
	if len(behind) != 0 {
		t.Fatalf("want %v, got %v", 0, len(behind))
	}
}

func TestService_BehindRepos_Good_EmptyStatus(t *testing.T) {
	s := &Service{}
	behind := s.BehindRepos()
	if len(behind) != 0 {
		t.Fatalf("want %v, got %v", 0, len(behind))
	}
}

func TestService_Iterators_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "clean"},
			{Name: "dirty", Modified: 1},
			{Name: "ahead", Ahead: 2},
		},
	}

	// Test All()
	all := slices.Collect(s.All())
	if len(all) != 3 {
		t.Fatalf("want %v, got %v", 3, len(all))
	}

	// Test Dirty()
	dirty := slices.Collect(s.Dirty())
	if len(dirty) != 1 {
		t.Fatalf("want %v, got %v", 1, len(dirty))
	}
	if "dirty" != dirty[0].Name {
		t.Fatalf("want %v, got %v", "dirty", dirty[0].Name)
	}

	// Test Ahead()
	ahead := slices.Collect(s.Ahead())
	if len(ahead) != 1 {
		t.Fatalf("want %v, got %v", 1, len(ahead))
	}
	if "ahead" != ahead[0].Name {
		t.Fatalf("want %v, got %v", "ahead", ahead[0].Name)
	}

	// Test Behind()
	behind := slices.Collect(s.Behind())
	if len(behind) != 0 {
		t.Fatalf("want %v, got %v", 0, len(behind))
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

func TestService_Status_Good_NilSlice(t *testing.T) {
	s := &Service{}
	if got := s.Status(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// --- Query/Task type tests ---

func TestQueryStatus_MapsToStatusOptions(t *testing.T) {
	q := QueryStatus{
		Paths: []string{"/path/a", "/path/b"},
		Names: map[string]string{"/path/a": "repo-a"},
	}

	// QueryStatus can be cast directly to StatusOptions.
	opts := StatusOptions(q)
	if !slices.Equal(q.Paths, opts.Paths) {
		t.Fatalf("want %v, got %v", q.Paths, opts.Paths)
	}
	if !reflect.DeepEqual(q.Names, opts.Names) {
		t.Fatalf("want %v, got %v", q.Names, opts.Names)
	}
}

func TestQueryBehindRepos_TypeExists(t *testing.T) {
	var q QueryBehindRepos
	if reflect.TypeOf(QueryBehindRepos{}) != reflect.TypeOf(q) {
		t.Fatalf("want %T, got %T", QueryBehindRepos{}, q)
	}
}

func TestTaskPullMultiple_TypeExists(t *testing.T) {
	var tpm TaskPullMultiple
	if reflect.TypeOf(TaskPullMultiple{}) != reflect.TypeOf(tpm) {
		t.Fatalf("want %T, got %T", TaskPullMultiple{}, tpm)
	}
}

func TestServiceOptions_WorkDir(t *testing.T) {
	opts := ServiceOptions{WorkDir: "/home/claude/repos"}
	if "/home/claude/repos" != opts.WorkDir {
		t.Fatalf("want %v, got %v", "/home/claude/repos", opts.WorkDir)
	}
}

// --- DirtyRepos excludes errored repos ---

func TestService_DirtyRepos_Good_ExcludesErrors(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "dirty-ok", Modified: 1},
			{Name: "dirty-error", Modified: 1, Error: errors.New("test error")},
		},
	}

	dirty := s.DirtyRepos()
	if len(dirty) != 1 {
		t.Fatalf("want %v, got %v", 1, len(dirty))
	}
	if "dirty-ok" != dirty[0].Name {
		t.Fatalf("want %v, got %v", "dirty-ok", dirty[0].Name)
	}
}

// --- AheadRepos excludes errored repos ---

func TestService_AheadRepos_Good_ExcludesErrors(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "ahead-ok", Ahead: 2},
			{Name: "ahead-error", Ahead: 3, Error: errors.New("test error")},
		},
	}

	ahead := s.AheadRepos()
	if len(ahead) != 1 {
		t.Fatalf("want %v, got %v", 1, len(ahead))
	}
	if "ahead-ok" != ahead[0].Name {
		t.Fatalf("want %v, got %v", "ahead-ok", ahead[0].Name)
	}
}

// --- BehindRepos excludes errored repos ---

func TestService_BehindRepos_Good_ExcludesErrors(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "behind-ok", Behind: 2},
			{Name: "behind-error", Behind: 3, Error: errors.New("test error")},
		},
	}

	behind := s.BehindRepos()
	if len(behind) != 1 {
		t.Fatalf("want %v, got %v", 1, len(behind))
	}
	if "behind-ok" != behind[0].Name {
		t.Fatalf("want %v, got %v", "behind-ok", behind[0].Name)
	}
}
