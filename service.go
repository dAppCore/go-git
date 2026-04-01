package git

import (
	"context"
	"iter"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"dappco.re/go/core"
	coreerr "dappco.re/go/core/log"
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

// QueryBehindRepos requests repos with unpulled commits.
type QueryBehindRepos struct{}

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

// OnStartup registers query and action handlers.
func (s *Service) OnStartup(ctx context.Context) core.Result {
	s.Core().RegisterQuery(s.handleQuery)
	s.Core().RegisterAction(s.handleTaskMessage)

	s.Core().Action("git.push", func(ctx context.Context, opts core.Options) core.Result {
		path := opts.String("path")
		if err := s.validatePath(path); err != nil {
			return s.Core().LogError(err, "git.push", "path validation failed")
		}
		if err := Push(ctx, path); err != nil {
			return s.Core().LogError(err, "git.push", "push failed")
		}
		return core.Result{OK: true}
	})

	s.Core().Action("git.pull", func(ctx context.Context, opts core.Options) core.Result {
		path := opts.String("path")
		if err := s.validatePath(path); err != nil {
			return s.Core().LogError(err, "git.pull", "path validation failed")
		}
		if err := Pull(ctx, path); err != nil {
			return s.Core().LogError(err, "git.pull", "pull failed")
		}
		return core.Result{OK: true}
	})

	s.Core().Action("git.push-multiple", func(ctx context.Context, opts core.Options) core.Result {
		r := opts.Get("paths")
		paths, _ := r.Value.([]string)
		r = opts.Get("names")
		names, _ := r.Value.(map[string]string)
		for _, path := range paths {
			if err := s.validatePath(path); err != nil {
				return s.Core().LogError(err, "git.push-multiple", "path validation failed")
			}
		}
		results, err := PushMultiple(ctx, paths, names)
		if err != nil {
			_ = s.Core().LogError(err, "git.push-multiple", "push multiple had failures")
		}
		return core.Result{Value: results, OK: true}
	})

	return core.Result{OK: true}
}

// handleTaskMessage bridges task structs onto the Core action bus.
func (s *Service) handleTaskMessage(c *core.Core, msg core.Message) core.Result {
	switch m := msg.(type) {
	case TaskPush:
		return s.handleTask(c, m)
	case TaskPull:
		return s.handleTask(c, m)
	case TaskPushMultiple:
		return s.handleTask(c, m)
	default:
		return core.Result{OK: true}
	}
}

func (s *Service) handleQuery(c *core.Core, q core.Query) core.Result {
	ctx := c.Context()

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
	case QueryBehindRepos:
		return core.Result{Value: s.BehindRepos(), OK: true}
	case TaskPush, TaskPull, TaskPushMultiple:
		return s.handleTask(c, m)
	}
	return core.Result{}
}

func (s *Service) handleTask(c *core.Core, t any) core.Result {
	ctx := c.Context()

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
	if !core.PathIsAbs(path) {
		return coreerr.E("git.validatePath", "path must be absolute: "+path, nil)
	}

	workDir := s.opts.WorkDir
	if workDir == "" {
		return nil
	}

	workDir = filepath.Clean(workDir)
	if !core.PathIsAbs(workDir) {
		return coreerr.E("git.validatePath", "WorkDir must be absolute: "+s.opts.WorkDir, nil)
	}
	rel, err := filepath.Rel(workDir, filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return coreerr.E("git.validatePath", "path "+path+" is outside of allowed WorkDir "+workDir, nil)
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

// filteredIter returns an iterator over status entries that satisfy pred.
func (s *Service) filteredIter(pred func(RepoStatus) bool) iter.Seq[RepoStatus] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := slices.Clone(s.lastStatus)

	return func(yield func(RepoStatus) bool) {
		for _, st := range snapshot {
			if st.Error == nil && pred(st) {
				if !yield(st) {
					return
				}
			}
		}
	}
}

// Dirty returns an iterator over repos with uncommitted changes.
func (s *Service) Dirty() iter.Seq[RepoStatus] {
	return s.filteredIter(func(st RepoStatus) bool { return st.IsDirty() })
}

// Ahead returns an iterator over repos with unpushed commits.
func (s *Service) Ahead() iter.Seq[RepoStatus] {
	return s.filteredIter(func(st RepoStatus) bool { return st.HasUnpushed() })
}

// Behind returns an iterator over repos with unpulled commits.
func (s *Service) Behind() iter.Seq[RepoStatus] {
	return s.filteredIter(func(st RepoStatus) bool { return st.HasUnpulled() })
}

// DirtyRepos returns repos with uncommitted changes.
func (s *Service) DirtyRepos() []RepoStatus {
	return slices.Collect(s.Dirty())
}

// AheadRepos returns repos with unpushed commits.
func (s *Service) AheadRepos() []RepoStatus {
	return slices.Collect(s.Ahead())
}

// BehindRepos returns repos with unpulled commits.
func (s *Service) BehindRepos() []RepoStatus {
	return slices.Collect(s.Behind())
}
