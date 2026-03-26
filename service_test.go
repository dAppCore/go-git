package git

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
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
			{Name: "errored", Modified: 5, Error: assert.AnError},
		},
	}

	dirty := s.DirtyRepos()
	assert.Len(t, dirty, 3)

	names := slices.Collect(func(yield func(string) bool) {
		for _, d := range dirty {
			if !yield(d.Name) {
				return
			}
		}
	})
	assert.Contains(t, names, "dirty-modified")
	assert.Contains(t, names, "dirty-untracked")
	assert.Contains(t, names, "dirty-staged")
}

func TestService_DirtyRepos_Good_NoneFound(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "clean1"},
			{Name: "clean2"},
		},
	}

	dirty := s.DirtyRepos()
	assert.Empty(t, dirty)
}

func TestService_DirtyRepos_Good_EmptyStatus(t *testing.T) {
	s := &Service{}
	dirty := s.DirtyRepos()
	assert.Empty(t, dirty)
}

func TestService_AheadRepos_Good(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "up-to-date", Ahead: 0},
			{Name: "ahead-by-one", Ahead: 1},
			{Name: "ahead-by-five", Ahead: 5},
			{Name: "behind-only", Behind: 3},
			{Name: "errored-ahead", Ahead: 2, Error: assert.AnError},
		},
	}

	ahead := s.AheadRepos()
	assert.Len(t, ahead, 2)

	names := slices.Collect(func(yield func(string) bool) {
		for _, a := range ahead {
			if !yield(a.Name) {
				return
			}
		}
	})
	assert.Contains(t, names, "ahead-by-one")
	assert.Contains(t, names, "ahead-by-five")
}

func TestService_AheadRepos_Good_NoneFound(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "synced1"},
			{Name: "synced2"},
		},
	}

	ahead := s.AheadRepos()
	assert.Empty(t, ahead)
}

func TestService_AheadRepos_Good_EmptyStatus(t *testing.T) {
	s := &Service{}
	ahead := s.AheadRepos()
	assert.Empty(t, ahead)
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
	assert.Len(t, all, 3)

	// Test Dirty()
	dirty := slices.Collect(s.Dirty())
	assert.Len(t, dirty, 1)
	assert.Equal(t, "dirty", dirty[0].Name)

	// Test Ahead()
	ahead := slices.Collect(s.Ahead())
	assert.Len(t, ahead, 1)
	assert.Equal(t, "ahead", ahead[0].Name)
}

func TestService_Status_Good(t *testing.T) {
	expected := []RepoStatus{
		{Name: "repo1", Branch: "main"},
		{Name: "repo2", Branch: "develop"},
	}
	s := &Service{lastStatus: expected}

	assert.Equal(t, expected, s.Status())
}

func TestService_Status_Good_NilSlice(t *testing.T) {
	s := &Service{}
	assert.Nil(t, s.Status())
}

// --- Query/Task type tests ---

func TestQueryStatus_Good_MapsToStatusOptions(t *testing.T) {
	q := QueryStatus{
		Paths: []string{"/path/a", "/path/b"},
		Names: map[string]string{"/path/a": "repo-a"},
	}

	// QueryStatus can be cast directly to StatusOptions.
	opts := StatusOptions(q)
	assert.Equal(t, q.Paths, opts.Paths)
	assert.Equal(t, q.Names, opts.Names)
}

func TestServiceOptions_Good_WorkDir(t *testing.T) {
	opts := ServiceOptions{WorkDir: "/home/claude/repos"}
	assert.Equal(t, "/home/claude/repos", opts.WorkDir)
}

// --- DirtyRepos excludes errored repos ---

func TestService_DirtyRepos_Good_ExcludesErrors(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "dirty-ok", Modified: 1},
			{Name: "dirty-error", Modified: 1, Error: assert.AnError},
		},
	}

	dirty := s.DirtyRepos()
	assert.Len(t, dirty, 1)
	assert.Equal(t, "dirty-ok", dirty[0].Name)
}

// --- AheadRepos excludes errored repos ---

func TestService_AheadRepos_Good_ExcludesErrors(t *testing.T) {
	s := &Service{
		lastStatus: []RepoStatus{
			{Name: "ahead-ok", Ahead: 2},
			{Name: "ahead-error", Ahead: 3, Error: assert.AnError},
		},
	}

	ahead := s.AheadRepos()
	assert.Len(t, ahead, 1)
	assert.Equal(t, "ahead-ok", ahead[0].Name)
}
