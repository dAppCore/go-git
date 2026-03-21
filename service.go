package git

import (
	"context"
	"iter"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"dappco.re/go/core"
	coreerr "forge.lthn.ai/core/go-log"
)

// Queries for git service

// QueryStatus requests git status for paths.
type QueryStatus struct {
	Paths []string
	Names map[string]string
}

// QueryDirtyRepos requests repos with uncommitted changes.
type QueryDirtyRepos struct{}

// QueryAheadRepos requests repos with unpushed commits.
type QueryAheadRepos struct{}

// Tasks for git service

// TaskPush requests git push for a path.
type TaskPush struct {
	Path string
	Name string
}

// TaskPull requests git pull for a path.
type TaskPull struct {
	Path string
	Name string
}

// TaskPushMultiple requests git push for multiple paths.
type TaskPushMultiple struct {
	Paths []string
	Names map[string]string
}

// ServiceOptions for configuring the git service.
type ServiceOptions struct {
	WorkDir string
}

// Compile-time interface checks.
var _ core.Startable = (*Service)(nil)

// Service provides git operations as a Core service.
type Service struct {
	*core.ServiceRuntime[ServiceOptions]
	opts       ServiceOptions
	mu         sync.RWMutex
	lastStatus []RepoStatus
}

// NewService creates a git service factory.
func NewService(opts ServiceOptions) func(*core.Core) (any, error) {
	return func(c *core.Core) (any, error) {
		return &Service{
			ServiceRuntime: core.NewServiceRuntime(c, opts),
			opts:           opts,
		}, nil
	}
}

// OnStartup registers query and task handlers.
func (s *Service) OnStartup(ctx context.Context) error {
	s.Core().RegisterQuery(s.handleQuery)
	s.Core().RegisterTask(s.handleTask)
	return nil
}

func (s *Service) handleQuery(c *core.Core, q core.Query) core.Result {
	ctx := context.Background() // TODO: core should pass context to handlers

	switch m := q.(type) {
	case QueryStatus:
		// Validate all paths before execution
		for _, path := range m.Paths {
			if err := s.validatePath(path); err != nil {
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
		for _, path := range m.Paths {
			if err := s.validatePath(path); err != nil {
				return c.LogError(err, "git.handleTask", "path validation failed")
			}
		}
		results, err := PushMultiple(ctx, m.Paths, m.Names)
		if err != nil {
			// Log for observability; partial results are still returned.
			_ = c.LogError(err, "git.handleTask", "push multiple had failures")
		}
		return core.Result{Value: results, OK: true}
	}
	return core.Result{}
}

func (s *Service) validatePath(path string) error {
	if !filepath.IsAbs(path) {
		return coreerr.E("git.validatePath", "path must be absolute: "+path, nil)
	}

	workDir := s.opts.WorkDir
	if workDir != "" {
		rel, err := filepath.Rel(workDir, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return coreerr.E("git.validatePath", "path "+path+" is outside of allowed WorkDir "+workDir, nil)
		}
	}
	return nil
}

// Status returns last status result.
func (s *Service) Status() []RepoStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.lastStatus)
}

// All returns an iterator over all last known statuses.
func (s *Service) All() iter.Seq[RepoStatus] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Values(slices.Clone(s.lastStatus))
}

// Dirty returns an iterator over repos with uncommitted changes.
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
func (s *Service) DirtyRepos() []RepoStatus {
	return slices.Collect(s.Dirty())
}

// AheadRepos returns repos with unpushed commits.
func (s *Service) AheadRepos() []RepoStatus {
	return slices.Collect(s.Ahead())
}
