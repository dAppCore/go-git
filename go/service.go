package git

import (
	"iter"

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
	actionPathKey         = "p" + "ath"
	statusLockName        = "git.status"
	pathValidationFailed  = "path validation failed"
)

// NewService creates a git service factory.
func NewService(opts ServiceOptions) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		return core.Ok(&Service{
			ServiceRuntime: core.NewServiceRuntime(c, opts),
			opts:           opts,
		})
	}
}

// OnStartup registers query and action handlers.
func (s *Service) OnStartup(ctx core.Context) core.Result {
	s.Core().RegisterQuery(s.handleQuery)
	s.Core().RegisterAction(s.handleTaskMessage)

	s.Core().Action(actionGitPush, func(ctx core.Context, opts core.Options) core.Result {
		path := opts.String(actionPathKey)
		return s.runPush(ctx, path)
	})

	s.Core().Action(actionGitPull, func(ctx core.Context, opts core.Options) core.Result {
		path := opts.String(actionPathKey)
		return s.runPull(ctx, path)
	})

	s.Core().Action(actionGitPushMultiple, func(ctx core.Context, opts core.Options) core.Result {
		payload := multipleActionPayload(opts, actionGitPushMultiple)
		if !payload.OK {
			return s.logError(resultError(payload), actionGitPushMultiple, "invalid action payload")
		}
		p := payload.Value.(multiplePayload)
		return s.runPushMultiple(ctx, p.paths, p.names)
	})

	s.Core().Action(actionGitPullMultiple, func(ctx core.Context, opts core.Options) core.Result {
		payload := multipleActionPayload(opts, actionGitPullMultiple)
		if !payload.OK {
			return s.logError(resultError(payload), actionGitPullMultiple, "invalid action payload")
		}
		p := payload.Value.(multiplePayload)
		return s.runPullMultiple(ctx, p.paths, p.names)
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
		paths := s.validatePaths(m.Paths)
		if !paths.OK {
			return c.LogError(resultError(paths), "git.handleQuery", pathValidationFailed)
		}
		resolved := paths.Value.([]string)

		statuses := Status(ctx, StatusOptions{
			Paths: resolved,
			Names: resolvedNames(m.Paths, resolved, m.Names),
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

func (s *Service) runPush(ctx core.Context, path string) core.Result {
	resolved := s.validatePath(path)
	if !resolved.OK {
		return s.logError(resultError(resolved), actionGitPush, pathValidationFailed)
	}
	r := Push(ctx, resolved.Value.(string))
	if !r.OK {
		return s.logError(resultError(r), actionGitPush, "push failed")
	}
	return core.Ok(nil)
}

func (s *Service) runPull(ctx core.Context, path string) core.Result {
	resolved := s.validatePath(path)
	if !resolved.OK {
		return s.logError(resultError(resolved), actionGitPull, pathValidationFailed)
	}
	r := Pull(ctx, resolved.Value.(string))
	if !r.OK {
		return s.logError(resultError(r), actionGitPull, "pull failed")
	}
	return core.Ok(nil)
}

func (s *Service) runPushMultiple(ctx core.Context, paths []string, names map[string]string) core.Result {
	resolvedPaths := s.validatePaths(paths)
	if !resolvedPaths.OK {
		return s.logError(resultError(resolvedPaths), actionGitPushMultiple, pathValidationFailed)
	}
	resolved := resolvedPaths.Value.([]string)
	results := PushMultiple(ctx, resolved, resolvedNames(paths, resolved, names))
	if !results.OK {
		pushResults := results.Value.([]PushResult)
		if last := lastPushError(pushResults); last != nil {
			_ = s.logError(last, actionGitPushMultiple, "push multiple had failures")
		}
	}
	return results
}

func (s *Service) runPullMultiple(ctx core.Context, paths []string, names map[string]string) core.Result {
	resolvedPaths := s.validatePaths(paths)
	if !resolvedPaths.OK {
		return s.logError(resultError(resolvedPaths), actionGitPullMultiple, pathValidationFailed)
	}
	resolved := resolvedPaths.Value.([]string)
	results := PullMultiple(ctx, resolved, resolvedNames(paths, resolved, names))
	if !results.OK {
		pullResults := results.Value.([]PullResult)
		if last := lastPullError(pullResults); last != nil {
			_ = s.logError(last, actionGitPullMultiple, "pull multiple had failures")
		}
	}
	return results
}

type multiplePayload struct {
	paths []string
	names map[string]string
}

func multipleActionPayload(opts core.Options, op string) core.Result {
	r := opts.Get("paths")
	paths, ok := r.Value.([]string)
	if !r.OK || !ok {
		return core.Fail(gitServiceError(op, "paths must be []string", nil))
	}

	r = opts.Get("names")
	if !r.OK || r.Value == nil {
		return core.Ok(multiplePayload{paths: paths})
	}
	names, ok := r.Value.(map[string]string)
	if !ok {
		return core.Fail(gitServiceError(op, "names must be map[string]string", nil))
	}
	return core.Ok(multiplePayload{paths: paths, names: names})
}

func (s *Service) validatePath(path string) core.Result {
	if !core.PathIsAbs(path) {
		return core.Fail(gitValidationError("path must be absolute: "+path, path, s.opts.WorkDir, nil))
	}

	path = core.CleanPath(path, string(core.PathSeparator))
	workDir := s.opts.WorkDir
	if workDir == "" {
		return core.Ok(path)
	}

	workDir = core.CleanPath(workDir, string(core.PathSeparator))
	if !core.PathIsAbs(workDir) {
		return core.Fail(gitValidationError("WorkDir must be absolute: "+s.opts.WorkDir, path, s.opts.WorkDir, nil))
	}

	resolvedWorkDir := core.PathEvalSymlinks(workDir)
	if !resolvedWorkDir.OK {
		msg := "failed to resolve WorkDir: " + workDir
		return core.Fail(gitValidationError(msg, path, workDir, resultError(resolvedWorkDir)))
	}
	resolvedPath := core.PathEvalSymlinks(path)
	if !resolvedPath.OK {
		msg := "failed to resolve path: " + path
		return core.Fail(gitValidationError(msg, path, workDir, resultError(resolvedPath)))
	}

	resolvedWorkDirText := core.CleanPath(resolvedWorkDir.Value.(string), string(core.PathSeparator))
	resolvedPathText := core.CleanPath(resolvedPath.Value.(string), string(core.PathSeparator))
	if !pathWithinWorkDir(resolvedPathText, resolvedWorkDirText) {
		msg := "path " + resolvedPathText + " is outside of allowed WorkDir " + resolvedWorkDirText
		return core.Fail(gitValidationError(msg, path, workDir, nil))
	}
	return core.Ok(resolvedPathText)
}

func pathWithinWorkDir(path, workDir string) bool {
	relResult := core.PathRel(workDir, path)
	if !relResult.OK {
		return false
	}
	rel := relResult.Value.(string)
	if rel == "." {
		return true
	}
	return rel != ".." && !core.HasPrefix(rel, ".."+string(core.PathSeparator)) && !core.PathIsAbs(rel)
}

func (s *Service) validatePaths(paths []string) core.Result {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		validPath := s.validatePath(path)
		if !validPath.OK {
			return validPath
		}
		resolved = append(resolved, validPath.Value.(string))
	}
	return core.Ok(resolved)
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
		err = core.NewError(msg)
	} else {
		err = core.E("git.service", msg, err)
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
	return core.SliceClone(s.lastStatus)
}

// All returns an iterator over all last known statuses.
func (s *Service) All() iter.Seq[RepoStatus] {
	statusLock := s.statusLock(nil)
	if statusLock != nil {
		statusLock.Mutex.RLock()
		defer statusLock.Mutex.RUnlock()
	}
	snapshot := core.SliceClone(s.lastStatus)
	return func(yield func(RepoStatus) bool) {
		for _, st := range snapshot {
			if !yield(st) {
				return
			}
		}
	}
}

// filteredIter returns an iterator over status entries that satisfy pred.
func (s *Service) filteredIter(pred func(RepoStatus) bool) iter.Seq[RepoStatus] {
	statusLock := s.statusLock(nil)
	if statusLock != nil {
		statusLock.Mutex.RLock()
		defer statusLock.Mutex.RUnlock()
	}
	snapshot := core.SliceClone(s.lastStatus)

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
	return collectSeq(s.Dirty())
}

// AheadRepos returns repos with unpushed commits.
func (s *Service) AheadRepos() []RepoStatus {
	return collectSeq(s.Ahead())
}

// BehindRepos returns repos with unpulled commits.
func (s *Service) BehindRepos() []RepoStatus {
	return collectSeq(s.Behind())
}
