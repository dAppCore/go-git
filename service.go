package git

import (
	"context"
	"fmt"
	"iter"
	"path/filepath"
	"slices"
	"strings"

	core "dappco.re/go"
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

// TaskPullMultiple requests git pull for multiple paths.
type TaskPullMultiple struct {
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
	lastStatus []RepoStatus
}

const (
	actionGitPush         = "git.push"
	actionGitPull         = "git.pull"
	actionGitPushMultiple = "git.push-multiple"
	actionGitPullMultiple = "git.pull-multiple"
	statusLockName        = "git.status"
)

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

	s.Core().Action(actionGitPush, func(ctx context.Context, opts core.Options) core.Result {
		path := opts.String("path")
		return s.runPush(ctx, path)
	})

	s.Core().Action(actionGitPull, func(ctx context.Context, opts core.Options) core.Result {
		path := opts.String("path")
		return s.runPull(ctx, path)
	})

	s.Core().Action(actionGitPushMultiple, func(ctx context.Context, opts core.Options) core.Result {
		paths, names, err := multipleActionPayload(opts, actionGitPushMultiple)
		if err != nil {
			return s.logError(err, actionGitPushMultiple, "invalid action payload")
		}
		return s.runPushMultiple(ctx, paths, names)
	})

	s.Core().Action(actionGitPullMultiple, func(ctx context.Context, opts core.Options) core.Result {
		paths, names, err := multipleActionPayload(opts, actionGitPullMultiple)
		if err != nil {
			return s.logError(err, actionGitPullMultiple, "invalid action payload")
		}
		return s.runPullMultiple(ctx, paths, names)
	})

	return core.Ok(nil)
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
	case TaskPullMultiple:
		return s.handleTask(c, m)
	default:
		return core.Fail(nil)
	}
}

func (s *Service) handleQuery(c *core.Core, q core.Query) core.Result {
	ctx := c.Context()

	switch m := q.(type) {
	case QueryStatus:
		paths, err := s.validatePaths(m.Paths)
		if err != nil {
			return c.LogError(err, "git.handleQuery", "path validation failed")
		}

		statuses := Status(ctx, StatusOptions{
			Paths: paths,
			Names: resolvedNames(m.Paths, paths, m.Names),
		})

		statusLock := c.Lock(statusLockName)
		statusLock.Mutex.Lock()
		s.lastStatus = statuses
		statusLock.Mutex.Unlock()

		return core.Ok(statuses)

	case QueryDirtyRepos:
		return core.Ok(s.DirtyRepos())

	case QueryAheadRepos:
		return core.Ok(s.AheadRepos())
	case QueryBehindRepos:
		return core.Ok(s.BehindRepos())
	}
	return core.Fail(nil)
}

func (s *Service) handleTask(c *core.Core, t any) core.Result {
	ctx := c.Context()

	switch m := t.(type) {
	case TaskPush:
		return s.runPush(ctx, m.Path)

	case TaskPull:
		return s.runPull(ctx, m.Path)

	case TaskPushMultiple:
		return s.runPushMultiple(ctx, m.Paths, m.Names)

	case TaskPullMultiple:
		return s.runPullMultiple(ctx, m.Paths, m.Names)
	}

	return c.LogError(gitServiceError("git.handleTask", "unsupported task type", nil), "git.handleTask", "unsupported task type")
}

func (s *Service) runPush(ctx context.Context, path string) core.Result {
	path, err := s.validatePath(path)
	if err != nil {
		return s.logError(err, "git.push", "path validation failed")
	}
	if err := Push(ctx, path); err != nil {
		return s.logError(err, "git.push", "push failed")
	}
	return core.Ok(nil)
}

func (s *Service) runPull(ctx context.Context, path string) core.Result {
	path, err := s.validatePath(path)
	if err != nil {
		return s.logError(err, "git.pull", "path validation failed")
	}
	if err := Pull(ctx, path); err != nil {
		return s.logError(err, "git.pull", "pull failed")
	}
	return core.Ok(nil)
}

func (s *Service) runPushMultiple(ctx context.Context, paths []string, names map[string]string) core.Result {
	resolvedPaths, err := s.validatePaths(paths)
	if err != nil {
		return s.logError(err, "git.push-multiple", "path validation failed")
	}
	results, err := PushMultiple(ctx, resolvedPaths, resolvedNames(paths, resolvedPaths, names))
	if err != nil {
		err = s.logAggregateError(err, "git.push-multiple", "push multiple had failures")
	}
	return resultWithOK(results, err == nil)
}

func (s *Service) runPullMultiple(ctx context.Context, paths []string, names map[string]string) core.Result {
	resolvedPaths, err := s.validatePaths(paths)
	if err != nil {
		return s.logError(err, "git.pull-multiple", "path validation failed")
	}
	results, err := PullMultiple(ctx, resolvedPaths, resolvedNames(paths, resolvedPaths, names))
	if err != nil {
		err = s.logAggregateError(err, "git.pull-multiple", "pull multiple had failures")
	}
	return resultWithOK(results, err == nil)
}

func multipleActionPayload(opts core.Options, op string) ([]string, map[string]string, error) {
	r := opts.Get("paths")
	paths, ok := r.Value.([]string)
	if !r.OK || !ok {
		return nil, nil, gitServiceError(op, "paths must be []string", nil)
	}

	r = opts.Get("names")
	if !r.OK || r.Value == nil {
		return paths, nil, nil
	}
	names, ok := r.Value.(map[string]string)
	if !ok {
		return nil, nil, gitServiceError(op, "names must be map[string]string", nil)
	}
	return paths, names, nil
}

func (s *Service) validatePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", gitValidationError("path must be absolute: "+path, path, s.opts.WorkDir, nil)
	}

	path = filepath.Clean(path)
	workDir := s.opts.WorkDir
	if workDir == "" {
		return path, nil
	}

	workDir = filepath.Clean(workDir)
	if !filepath.IsAbs(workDir) {
		return "", gitValidationError("WorkDir must be absolute: "+s.opts.WorkDir, path, s.opts.WorkDir, nil)
	}

	resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		msg := "failed to resolve WorkDir: " + workDir
		return "", gitValidationError(msg, path, workDir, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		msg := "failed to resolve path: " + path
		return "", gitValidationError(msg, path, workDir, err)
	}

	resolvedWorkDir = filepath.Clean(resolvedWorkDir)
	resolvedPath = filepath.Clean(resolvedPath)
	if !pathWithinWorkDir(resolvedPath, resolvedWorkDir) {
		msg := "path " + resolvedPath + " is outside of allowed WorkDir " + resolvedWorkDir
		return "", gitValidationError(msg, path, workDir, nil)
	}
	return resolvedPath, nil
}

func pathWithinWorkDir(path, workDir string) bool {
	rel, err := filepath.Rel(workDir, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (s *Service) validatePaths(paths []string) ([]string, error) {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		validPath, err := s.validatePath(path)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, validPath)
	}
	return resolved, nil
}

func resolvedNames(paths, resolvedPaths []string, names map[string]string) map[string]string {
	if names == nil {
		return nil
	}

	resolved := make(map[string]string, len(names)+len(resolvedPaths))
	for path, name := range names {
		resolved[path] = name
	}
	for i, path := range paths {
		if i >= len(resolvedPaths) {
			break
		}
		name := names[path]
		if name != "" {
			resolved[resolvedPaths[i]] = name
		}
	}
	return resolved
}

func (s *Service) logError(err error, op, msg string) core.Result {
	if s.ServiceRuntime == nil || s.Core() == nil {
		return core.Fail(err)
	}
	return s.Core().LogError(err, op, msg)
}

func (s *Service) logAggregateError(err error, op, msg string) error {
	r := s.logError(err, op, msg)
	if loggedErr, ok := r.Value.(error); ok {
		return loggedErr
	}
	return err
}

func resultWithOK(value any, ok bool) core.Result {
	r := core.Ok(value)
	r.OK = ok
	return r
}

func gitValidationError(msg, path, workDir string, err error) *GitError {
	args := []string{"git.validatePath", "path=" + path}
	if workDir != "" {
		args = append(args, "workDir="+workDir)
	}
	return gitServiceErrorWithArgs(args, msg, err)
}

func gitServiceError(op, msg string, err error) *GitError {
	return gitServiceErrorWithArgs([]string{op}, msg, err)
}

func gitServiceErrorWithArgs(args []string, msg string, err error) *GitError {
	if err == nil {
		err = fmt.Errorf("%s", msg)
	} else {
		err = fmt.Errorf("%s: %w", msg, err)
	}
	return &GitError{
		Args:   args,
		Err:    err,
		Stderr: msg,
	}
}

// Status returns last status result.
func (s *Service) Status() []RepoStatus {
	statusLock := s.statusLock(nil)
	if statusLock != nil {
		statusLock.Mutex.RLock()
		defer statusLock.Mutex.RUnlock()
	}
	return slices.Clone(s.lastStatus)
}

// All returns an iterator over all last known statuses.
func (s *Service) All() iter.Seq[RepoStatus] {
	statusLock := s.statusLock(nil)
	if statusLock != nil {
		statusLock.Mutex.RLock()
		defer statusLock.Mutex.RUnlock()
	}
	return slices.Values(slices.Clone(s.lastStatus))
}

// filteredIter returns an iterator over status entries that satisfy pred.
func (s *Service) filteredIter(pred func(RepoStatus) bool) iter.Seq[RepoStatus] {
	statusLock := s.statusLock(nil)
	if statusLock != nil {
		statusLock.Mutex.RLock()
		defer statusLock.Mutex.RUnlock()
	}
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

func (s *Service) statusLock(c *core.Core) *core.Lock {
	if c != nil {
		return c.Lock(statusLockName)
	}
	if s.ServiceRuntime == nil || s.Core() == nil {
		return nil
	}
	return s.Core().Lock(statusLockName)
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
