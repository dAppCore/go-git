// Package git provides utilities for git operations across multiple repositories.
package git

import (
	"iter"

	core "dappco.re/go"
)

func withBackground(ctx core.Context) core.Context {
	if ctx != nil {
		return ctx
	}
	return core.Background()
}

func collectSeq[T any](seq iter.Seq[T]) []T {
	var out []T
	for value := range seq {
		out = append(out, value)
	}
	return out
}

func resultError(r core.Result) *GitError {
	if gitErr, ok := r.Value.(*GitError); ok {
		return gitErr
	}
	if err, ok := r.Value.(error); ok {
		return &GitError{Err: err, Stderr: err.Error()}
	}
	if r.Value != nil {
		msg := core.Sprint(r.Value)
		return &GitError{Err: core.E("git.resultError", msg, nil), Stderr: msg}
	}
	return &GitError{Err: core.E("git.resultError", "operation failed", nil), Stderr: "operation failed"}
}

func gitCmd(dir string, args ...string) *core.Cmd {
	cmdArgs := append([]string{"env", "git"}, args...)
	return &core.Cmd{
		Path: "/usr/bin/env",
		Args: cmdArgs,
		Dir:  dir,
	}
}

func lastPushError(results []PushResult) *GitError {
	var last *GitError
	for _, result := range results {
		if result.Error != nil {
			last = resultError(core.Fail(result.Error))
		}
	}
	return last
}

func lastPullError(results []PullResult) *GitError {
	var last *GitError
	for _, result := range results {
		if result.Error != nil {
			last = resultError(core.Fail(result.Error))
		}
	}
	return last
}

// RepoStatus represents the git status of a single repository.
type RepoStatus struct {
	Name      string
	Path      string
	Modified  int
	Untracked int
	Staged    int
	Ahead     int
	Behind    int
	Branch    string
	Error     error
}

// IsDirty returns true if there are uncommitted changes.
func (s *RepoStatus) IsDirty() bool {
	return s.Modified > 0 || s.Untracked > 0 || s.Staged > 0
}

// HasUnpushed returns true if there are commits to push.
func (s *RepoStatus) HasUnpushed() bool {
	return s.Ahead > 0
}

// HasUnpulled returns true if there are commits to pull.
func (s *RepoStatus) HasUnpulled() bool {
	return s.Behind > 0
}

// StatusOptions configures the status check.
type StatusOptions struct {
	// Paths is a list of repo paths to check
	Paths []string
	// Names maps paths to display names
	Names map[string]string
}

// Status checks git status for multiple repositories in parallel.
//
// Example:
//
//	statuses := Status(ctx, StatusOptions{Paths: []string{"/home/user/Code/core/agent"}})
func Status(ctx core.Context, opts StatusOptions) []RepoStatus {
	return collectSeq(StatusIter(withBackground(ctx), opts))
}

func repoName(path string, names map[string]string) string {
	if names == nil {
		return path
	}

	name := names[path]
	if name == "" {
		return path
	}
	return name
}

// StatusIter checks git status for multiple repositories in parallel and yields
// the results in input order.
func StatusIter(ctx core.Context, opts StatusOptions) iter.Seq[RepoStatus] {
	ctx = withBackground(ctx)
	return func(yield func(RepoStatus) bool) {
		type indexedStatus struct {
			idx int
			st  RepoStatus
		}

		if len(opts.Paths) == 0 {
			return
		}

		results := make(chan indexedStatus, len(opts.Paths))
		for i, path := range opts.Paths {
			go func(idx int, repoPath string) {
				name := repoName(repoPath, opts.Names)
				results <- indexedStatus{
					idx: idx,
					st:  getStatus(ctx, repoPath, name),
				}
			}(i, path)
		}

		statuses := make([]RepoStatus, len(opts.Paths))
		ready := make([]bool, len(opts.Paths))
		next := 0

		for received := 0; received < len(opts.Paths); received++ {
			result := <-results
			statuses[result.idx] = result.st
			ready[result.idx] = true

			for next < len(statuses) && ready[next] {
				if !yield(statuses[next]) {
					return
				}
				next++
			}
		}
	}
}

// getStatus gets the git status for a single repository.
func getStatus(ctx core.Context, path, name string) RepoStatus {
	ctx = withBackground(ctx)
	status := RepoStatus{
		Name: name,
		Path: path,
	}

	if r := requireAbsolutePath("git.getStatus", path); !r.OK {
		status.Error = resultError(r)
		return status
	}

	// Get current branch
	branch := gitCommand(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
	if !branch.OK {
		status.Error = resultError(branch)
		return status
	}
	status.Branch = trim(branch.Value.(string))

	// Get porcelain status
	porcelain := gitCommand(ctx, path, "status", "--porcelain")
	if !porcelain.OK {
		status.Error = resultError(porcelain)
		return status
	}

	// Parse status output
	for _, line := range core.Split(porcelain.Value.(string), "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]

		// Untracked
		if x == '?' && y == '?' {
			status.Untracked++
			continue
		}

		// Staged (index has changes)
		if isStagedStatus(x) {
			status.Staged++
		}

		// Modified in working tree
		if isModifiedStatus(y) {
			status.Modified++
		}
	}

	// Get ahead/behind counts
	counts := getAheadBehind(ctx, path)
	if !counts.OK {
		// We don't fail the whole status for missing upstream branches.
		// We do surface other ahead/behind failures on the result.
		status.Error = resultError(counts)
		return status
	}
	ab := counts.Value.(aheadBehind)
	status.Ahead = ab.ahead
	status.Behind = ab.behind

	return status
}

func isStagedStatus(ch byte) bool {
	switch ch {
	case 'A', 'C', 'D', 'R', 'M', 'T', 'U':
		return true
	default:
		return false
	}
}

func isModifiedStatus(ch byte) bool {
	switch ch {
	case 'M', 'D', 'T', 'U':
		return true
	default:
		return false
	}
}

// isNoUpstreamError reports whether an error is due to a missing tracking branch.
func isNoUpstreamError(err error) bool {
	if err == nil {
		return false
	}
	msg := core.Lower(trim(err.Error()))
	return core.Contains(msg, "no upstream")
}

func requireAbsolutePath(op string, path string) core.Result {
	if core.PathIsAbs(path) {
		return core.Ok(path)
	}
	msg := core.Sprintf("path must be absolute: %s", path)
	return core.Fail(&GitError{
		Args:   []string{op, path},
		Err:    core.E(op, msg, nil),
		Stderr: msg,
	})
}

type aheadBehind struct {
	ahead  int
	behind int
}

// getAheadBehind returns the number of commits ahead and behind upstream.
func getAheadBehind(ctx core.Context, path string) core.Result {
	ctx = withBackground(ctx)
	if r := requireAbsolutePath("git.getAheadBehind", path); !r.OK {
		return r
	}

	ahead := 0
	behind := 0
	aheadArgs := []string{"rev-list", "--count", "@{u}..HEAD"}
	aheadStr := gitCommand(ctx, path, aheadArgs...)
	if aheadStr.OK {
		parsed := parseGitCount("ahead", aheadStr.Value.(string))
		if !parsed.OK {
			return core.Fail(gitParseError(aheadArgs, aheadStr.Value.(string), resultError(parsed)))
		}
		ahead = parsed.Value.(int)
	} else if !isNoUpstreamError(resultError(aheadStr)) {
		return aheadStr
	}

	behindArgs := []string{"rev-list", "--count", "HEAD..@{u}"}
	behindStr := gitCommand(ctx, path, behindArgs...)
	if behindStr.OK {
		parsed := parseGitCount("behind", behindStr.Value.(string))
		if !parsed.OK {
			return core.Fail(gitParseError(behindArgs, behindStr.Value.(string), resultError(parsed)))
		}
		behind = parsed.Value.(int)
	} else if !isNoUpstreamError(resultError(behindStr)) {
		return behindStr
	}

	return core.Ok(aheadBehind{ahead: ahead, behind: behind})
}

func parseGitCount(label, value string) core.Result {
	n := core.ParseInt(trim(value), 10, 0)
	if !n.OK {
		return core.Fail(core.E("git.parseGitCount", core.Sprintf("failed to parse %s count", label), resultError(n)))
	}
	return core.Ok(int(n.Value.(int64)))
}

func gitParseError(args []string, output string, err error) *GitError {
	return &GitError{
		Args:   core.SliceClone(args),
		Err:    err,
		Stderr: core.Sprintf("invalid git count output %q: %v", trim(output), err),
	}
}

// Push pushes commits for a single repository.
//
// Example:
//
//	r := Push(ctx, "/home/user/Code/core/agent")
//
// Uses interactive mode to support SSH passphrase prompts.
func Push(ctx core.Context, path string) core.Result {
	ctx = withBackground(ctx)
	if r := requireAbsolutePath("git.push", path); !r.OK {
		return r
	}
	return gitInteractive(ctx, path, "push")
}

// Pull pulls changes for a single repository.
//
// Example:
//
//	r := Pull(ctx, "/home/user/Code/core/agent")
//
// Uses interactive mode to support SSH passphrase prompts.
func Pull(ctx core.Context, path string) core.Result {
	ctx = withBackground(ctx)
	if r := requireAbsolutePath("git.pull", path); !r.OK {
		return r
	}
	return gitInteractive(ctx, path, "pull", "--rebase")
}

// IsNonFastForward checks if an error is a non-fast-forward rejection.
func IsNonFastForward(err error) bool {
	if err == nil {
		return false
	}
	msg := core.Lower(err.Error())
	return core.Contains(msg, "non-fast-forward") ||
		core.Contains(msg, "fetch first") ||
		core.Contains(msg, "tip of your current branch is behind")
}

// gitInteractive runs a git command with terminal attached for user interaction.
func gitInteractive(ctx core.Context, dir string, args ...string) core.Result {
	ctx = withBackground(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return core.Fail(ctxErr)
	}
	if r := requireAbsolutePath("git.interactive", dir); !r.OK {
		return r
	}

	stderr := core.NewBuffer()
	cmd := gitCmd(dir, args...)
	cmd.Stdin = core.Stdin()
	cmd.Stdout = core.Stdout()
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return core.Fail(&GitError{
			Args:   core.SliceClone(args),
			Err:    err,
			Stderr: stderr.String(),
		})
	}

	return core.Ok(nil)
}

// PushResult represents the result of a push operation.
type PushResult struct {
	Name    string
	Path    string
	Success bool
	Error   error
}

// PullResult represents the result of a pull operation.
type PullResult struct {
	Name    string
	Path    string
	Success bool
	Error   error
}

// PushMultiple pushes multiple repositories sequentially.
// Sequential because SSH passphrase prompts need user interaction.
func PushMultiple(ctx core.Context, paths []string, names map[string]string) core.Result {
	results := collectSeq(PushMultipleIter(withBackground(ctx), paths, names))
	return resultWithOK(results, lastPushError(results) == nil)
}

// PushMultipleIter pushes multiple repositories sequentially and yields each
// per-repository result in input order.
func PushMultipleIter(ctx core.Context, paths []string, names map[string]string) iter.Seq[PushResult] {
	ctx = withBackground(ctx)
	return func(yield func(PushResult) bool) {
		for _, path := range paths {
			name := repoName(path, names)

			result := PushResult{
				Name: name,
				Path: path,
			}

			if r := requireAbsolutePath("git.pushMultiple", path); !r.OK {
				result.Error = resultError(r)
			} else if r := Push(ctx, path); !r.OK {
				result.Error = resultError(r)
			} else {
				result.Success = true
			}

			if !yield(result) {
				return
			}
		}
	}
}

// PullMultiple pulls changes for multiple repositories sequentially.
// Sequential because interactive terminal I/O needs a single active prompt.
func PullMultiple(ctx core.Context, paths []string, names map[string]string) core.Result {
	results := collectSeq(PullMultipleIter(withBackground(ctx), paths, names))
	return resultWithOK(results, lastPullError(results) == nil)
}

// PullMultipleIter pulls changes for multiple repositories sequentially and yields
// each per-repository result in input order.
func PullMultipleIter(ctx core.Context, paths []string, names map[string]string) iter.Seq[PullResult] {
	ctx = withBackground(ctx)
	return func(yield func(PullResult) bool) {
		for _, path := range paths {
			name := repoName(path, names)

			result := PullResult{
				Name: name,
				Path: path,
			}

			if r := requireAbsolutePath("git.pullMultiple", path); !r.OK {
				result.Error = resultError(r)
			} else if r := Pull(ctx, path); !r.OK {
				result.Error = resultError(r)
			} else {
				result.Success = true
			}

			if !yield(result) {
				return
			}
		}
	}
}

// gitCommand runs a git command and returns stdout.
func gitCommand(ctx core.Context, dir string, args ...string) core.Result {
	ctx = withBackground(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return core.Fail(ctxErr)
	}
	if r := requireAbsolutePath("git.command", dir); !r.OK {
		return r
	}

	cmd := gitCmd(dir, args...)
	stdout := core.NewBuffer()
	stderr := core.NewBuffer()
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return core.Fail(&GitError{
			Args:   core.SliceClone(args),
			Err:    err,
			Stderr: stderr.String(),
		})
	}

	return core.Ok(stdout.String())
}

// Compile-time interface checks.
var _ error = (*GitError)(nil)

// GitError wraps a git command error with stderr output and command context.
type GitError struct {
	Args   []string
	Err    error
	Stderr string
}

// Error returns a descriptive error message.
func (e *GitError) Error() string {
	cmd := core.Concat("git ", core.Join(" ", e.Args...))
	stderr := trim(e.Stderr)

	if stderr != "" {
		return core.Sprintf("git command %q failed: %s", cmd, stderr)
	}
	if e.Err != nil {
		return core.Sprintf("git command %q failed: %v", cmd, e.Err)
	}
	return core.Sprintf("git command %q failed", cmd)
}

func trim(s string) string {
	return core.Trim(s)
}
