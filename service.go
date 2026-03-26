package git

import (
	"context"
	"iter"
	"slices"
	"sync"

	"dappco.re/go/core"
)

// QueryStatus requests git status for paths.
// c.Query(git.QueryStatus{Paths: []string{"/srv/repos/core"}})
type QueryStatus struct {
	Paths []string
	Names map[string]string
}

// QueryDirtyRepos requests repos with uncommitted changes.
// c.Query(git.QueryDirtyRepos{})
type QueryDirtyRepos struct{}

// QueryAheadRepos requests repos with unpushed commits.
// c.Query(git.QueryAheadRepos{})
type QueryAheadRepos struct{}

// TaskPush requests git push for a path.
// c.Perform(git.TaskPush{Path: "/srv/repos/core", Name: "core"})
type TaskPush struct {
	Path string
	Name string
}

// TaskPull requests git pull for a path.
// c.Perform(git.TaskPull{Path: "/srv/repos/core", Name: "core"})
type TaskPull struct {
	Path string
	Name string
}

// TaskPushMultiple requests git push for multiple paths.
// c.Perform(git.TaskPushMultiple{Paths: []string{"/srv/repos/core"}})
type TaskPushMultiple struct {
	Paths []string
	Names map[string]string
}

// ServiceOptions configures the git service.
// opts := git.ServiceOptions{WorkDir: "/srv/repos"}
type ServiceOptions struct {
	WorkDir string
}

// Compile-time interface checks.
var _ core.Startable = (*Service)(nil)

// Service provides git operations as a Core service.
// factory := git.NewService(git.ServiceOptions{WorkDir: "/srv/repos"})
type Service struct {
	*core.ServiceRuntime[ServiceOptions]
	opts       ServiceOptions
	mu         sync.RWMutex
	lastStatus []RepoStatus
}

// NewService creates a git service factory.
// factory := git.NewService(git.ServiceOptions{WorkDir: "/srv/repos"})
func NewService(opts ServiceOptions) func(*core.Core) (any, error) {
	return func(c *core.Core) (any, error) {
		return &Service{
			ServiceRuntime: core.NewServiceRuntime(c, opts),
			opts:           opts,
		}, nil
	}
}

// OnStartup registers query and task handlers.
// err := svc.OnStartup(context.Background())
func (s *Service) OnStartup(ctx context.Context) error {
	s.Core().RegisterQuery(s.handleQuery)
	s.Core().RegisterTask(s.handleTask)
	return nil
}

func (s *Service) handleQuery(c *core.Core, q core.Query) core.Result {
	ctx := context.Background() // TODO: core should pass context to handlers

	switch m := q.(type) {
	case QueryStatus:
		for _, repoPath := range m.Paths {
			if err := s.validatePath(repoPath); err != nil {
				return c.LogError(err, "git.handleQuery", "path validation failed")
			}
		}

		statuses := Status(ctx, StatusOptions(m))

		s.mu.Lock()
		s.lastStatus = statuses
		s.mu.Unlock()

		return core.Result{Value: statuses, OK: true}

	case QueryDirtyRepos:
		return core.Result{Value: s.DirtyRepos(), OK: true}

	case QueryAheadRepos:
		return core.Result{Value: s.AheadRepos(), OK: true}
	}

	return core.Result{}
}

func (s *Service) handleTask(c *core.Core, t core.Task) core.Result {
	ctx := context.Background() // TODO: core should pass context to handlers

	switch m := t.(type) {
	case TaskPush:
		if err := s.validatePath(m.Path); err != nil {
			return c.LogError(err, "git.handleTask", "path validation failed")
		}
		if err := Push(ctx, m.Path); err != nil {
			return c.LogError(err, "git.handleTask", "push failed")
		}
		return core.Result{OK: true}

	case TaskPull:
		if err := s.validatePath(m.Path); err != nil {
			return c.LogError(err, "git.handleTask", "path validation failed")
		}
		if err := Pull(ctx, m.Path); err != nil {
			return c.LogError(err, "git.handleTask", "pull failed")
		}
		return core.Result{OK: true}

	case TaskPushMultiple:
		for _, repoPath := range m.Paths {
			if err := s.validatePath(repoPath); err != nil {
				return c.LogError(err, "git.handleTask", "path validation failed")
			}
		}

		results, err := PushMultiple(ctx, m.Paths, m.Names)
		if err != nil {
			_ = c.LogError(err, "git.handleTask", "push multiple had failures")
		}

		return core.Result{Value: results, OK: true}
	}

	return core.Result{}
}

func (s *Service) validatePath(repoPath string) error {
	if !isAbsolutePath(repoPath) {
		return core.E("git.validatePath", core.Concat("path must be absolute: ", repoPath), nil)
	}

	if s.opts.WorkDir != "" && !pathWithin(s.opts.WorkDir, repoPath) {
		return core.E(
			"git.validatePath",
			core.Concat("path ", repoPath, " is outside of allowed WorkDir ", s.opts.WorkDir),
			nil,
		)
	}

	return nil
}

// Status returns the last status result.
// statuses := svc.Status()
func (s *Service) Status() []RepoStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.lastStatus)
}

// All returns an iterator over all last known statuses.
// all := slices.Collect(svc.All())
func (s *Service) All() iter.Seq[RepoStatus] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Values(slices.Clone(s.lastStatus))
}

// Dirty returns an iterator over repos with uncommitted changes.
// dirty := slices.Collect(svc.Dirty())
func (s *Service) Dirty() iter.Seq[RepoStatus] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lastStatus := slices.Clone(s.lastStatus)

	return func(yield func(RepoStatus) bool) {
		for _, st := range lastStatus {
			if st.Error == nil && st.IsDirty() {
				if !yield(st) {
					return
				}
			}
		}
	}
}

// Ahead returns an iterator over repos with unpushed commits.
// ahead := slices.Collect(svc.Ahead())
func (s *Service) Ahead() iter.Seq[RepoStatus] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lastStatus := slices.Clone(s.lastStatus)

	return func(yield func(RepoStatus) bool) {
		for _, st := range lastStatus {
			if st.Error == nil && st.HasUnpushed() {
				if !yield(st) {
					return
				}
			}
		}
	}
}

// DirtyRepos returns repos with uncommitted changes.
// dirty := svc.DirtyRepos()
func (s *Service) DirtyRepos() []RepoStatus {
	return slices.Collect(s.Dirty())
}

// AheadRepos returns repos with unpushed commits.
// ahead := svc.AheadRepos()
func (s *Service) AheadRepos() []RepoStatus {
	return slices.Collect(s.Ahead())
}
