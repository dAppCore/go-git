// Package git provides utilities for git operations across multiple repositories.
package git

import (
	"bytes"
	"context" // Note: intrinsic — cancellation propagation for git subprocesses and iterators; no core equivalent
	"fmt"
	"iter"    // Note: intrinsic — public lazy sequence API for repository operations; no core equivalent
	"os"      // Note: intrinsic — interactive git subprocess standard streams; no core equivalent
	"os/exec" // Note: intrinsic — executing the git CLI for repository operations; no core equivalent
	"path/filepath"
	"slices" // Note: intrinsic — collecting and cloning iterator-backed result slices; no core equivalent
	"strconv"
	"strings"
)

func withBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
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
func Status(ctx context.Context, opts StatusOptions) []RepoStatus {
	return slices.Collect(StatusIter(withBackground(ctx), opts))
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
func StatusIter(ctx context.Context, opts StatusOptions) iter.Seq[RepoStatus] {
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
func getStatus(ctx context.Context, path, name string) RepoStatus {
	ctx = withBackground(ctx)
	status := RepoStatus{
		Name: name,
		Path: path,
	}

	if err := requireAbsolutePath("git.getStatus", path); err != nil {
		status.Error = err
		return status
	}

	// Get current branch
	branch, err := gitCommand(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		status.Error = err
		return status
	}
	status.Branch = trim(branch)

	// Get porcelain status
	porcelain, err := gitCommand(ctx, path, "status", "--porcelain")
	if err != nil {
		status.Error = err
		return status
	}

	// Parse status output
	for _, line := range strings.Split(porcelain, "\n") {
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
	ahead, behind, err := getAheadBehind(ctx, path)
	if err != nil {
		// We don't fail the whole status for missing upstream branches.
		// We do surface other ahead/behind failures on the result.
		status.Error = err
	}
	status.Ahead = ahead
	status.Behind = behind

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
	msg := strings.ToLower(trim(err.Error()))
	return strings.Contains(msg, "no upstream")
}

func requireAbsolutePath(op string, path string) error {
	if filepath.IsAbs(path) {
		return nil
	}
	msg := fmt.Sprintf("path must be absolute: %s", path)
	return &GitError{
		Args:   []string{op, path},
		Err:    fmt.Errorf("%s: %s", op, msg),
		Stderr: msg,
	}
}

// getAheadBehind returns the number of commits ahead and behind upstream.
func getAheadBehind(ctx context.Context, path string) (ahead, behind int, err error) {
	ctx = withBackground(ctx)
	if err := requireAbsolutePath("git.getAheadBehind", path); err != nil {
		return 0, 0, err
	}

	aheadArgs := []string{"rev-list", "--count", "@{u}..HEAD"}
	aheadStr, err := gitCommand(ctx, path, aheadArgs...)
	if err == nil {
		ahead, err = parseGitCount("ahead", aheadStr)
		if err != nil {
			return 0, 0, gitParseError(aheadArgs, aheadStr, err)
		}
	} else if isNoUpstreamError(err) {
		err = nil
	}

	if err != nil {
		return 0, 0, err
	}

	behindArgs := []string{"rev-list", "--count", "HEAD..@{u}"}
	behindStr, err := gitCommand(ctx, path, behindArgs...)
	if err == nil {
		behind, err = parseGitCount("behind", behindStr)
		if err != nil {
			return 0, 0, gitParseError(behindArgs, behindStr, err)
		}
	} else if isNoUpstreamError(err) {
		err = nil
	}

	return ahead, behind, err
}

func parseGitCount(label, value string) (int, error) {
	n, err := strconv.ParseInt(trim(value), 10, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s count: %w", label, err)
	}
	return int(n), nil
}

func gitParseError(args []string, output string, err error) *GitError {
	return &GitError{
		Args:   slices.Clone(args),
		Err:    err,
		Stderr: fmt.Sprintf("invalid git count output %q: %v", trim(output), err),
	}
}

// Push pushes commits for a single repository.
//
// Example:
//
//	err := Push(ctx, "/home/user/Code/core/agent")
//
// Uses interactive mode to support SSH passphrase prompts.
func Push(ctx context.Context, path string) error {
	ctx = withBackground(ctx)
	if err := requireAbsolutePath("git.push", path); err != nil {
		return err
	}
	return gitInteractive(ctx, path, "push")
}

// Pull pulls changes for a single repository.
//
// Example:
//
//	err := Pull(ctx, "/home/user/Code/core/agent")
//
// Uses interactive mode to support SSH passphrase prompts.
func Pull(ctx context.Context, path string) error {
	ctx = withBackground(ctx)
	if err := requireAbsolutePath("git.pull", path); err != nil {
		return err
	}
	return gitInteractive(ctx, path, "pull", "--rebase")
}

// IsNonFastForward checks if an error is a non-fast-forward rejection.
func IsNonFastForward(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "fetch first") ||
		strings.Contains(msg, "tip of your current branch is behind")
}

// gitInteractive runs a git command with terminal attached for user interaction.
func gitInteractive(ctx context.Context, dir string, args ...string) error {
	ctx = withBackground(ctx)
	if err := requireAbsolutePath("git.interactive", dir); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	// Connect to terminal for SSH passphrase prompts
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	// Capture stderr for error reporting while also showing it
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderrTee{capture: stderr}

	if err := cmd.Run(); err != nil {
		return &GitError{
			Args:   args,
			Err:    err,
			Stderr: stderr.String(),
		}
	}

	return nil
}

type stderrTee struct {
	capture interface {
		Write([]byte) (int, error)
	}
}

func (w stderrTee) Write(p []byte) (int, error) {
	if _, err := os.Stderr.Write(p); err != nil {
		return 0, err
	}
	return w.capture.Write(p)
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
func PushMultiple(ctx context.Context, paths []string, names map[string]string) ([]PushResult, error) {
	results := slices.Collect(PushMultipleIter(withBackground(ctx), paths, names))
	var lastErr error

	for _, result := range results {
		if result.Error != nil {
			lastErr = result.Error
		}
	}

	return results, lastErr
}

// PushMultipleIter pushes multiple repositories sequentially and yields each
// per-repository result in input order.
func PushMultipleIter(ctx context.Context, paths []string, names map[string]string) iter.Seq[PushResult] {
	ctx = withBackground(ctx)
	return func(yield func(PushResult) bool) {
		for _, path := range paths {
			name := repoName(path, names)

			result := PushResult{
				Name: name,
				Path: path,
			}

			if err := requireAbsolutePath("git.pushMultiple", path); err != nil {
				result.Error = err
			} else if err := Push(ctx, path); err != nil {
				result.Error = err
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
func PullMultiple(ctx context.Context, paths []string, names map[string]string) ([]PullResult, error) {
	results := slices.Collect(PullMultipleIter(withBackground(ctx), paths, names))
	var lastErr error

	for _, result := range results {
		if result.Error != nil {
			lastErr = result.Error
		}
	}

	return results, lastErr
}

// PullMultipleIter pulls changes for multiple repositories sequentially and yields
// each per-repository result in input order.
func PullMultipleIter(ctx context.Context, paths []string, names map[string]string) iter.Seq[PullResult] {
	ctx = withBackground(ctx)
	return func(yield func(PullResult) bool) {
		for _, path := range paths {
			name := repoName(path, names)

			result := PullResult{
				Name: name,
				Path: path,
			}

			if err := requireAbsolutePath("git.pullMultiple", path); err != nil {
				result.Error = err
			} else if err := Pull(ctx, path); err != nil {
				result.Error = err
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
func gitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	ctx = withBackground(ctx)
	if err := requireAbsolutePath("git.command", dir); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", &GitError{
			Args:   args,
			Err:    err,
			Stderr: stderr.String(),
		}
	}

	return stdout.String(), nil
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
	cmd := "git " + strings.Join(e.Args, " ")
	stderr := trim(e.Stderr)

	if stderr != "" {
		return fmt.Sprintf("git command %q failed: %s", cmd, stderr)
	}
	if e.Err != nil {
		return fmt.Sprintf("git command %q failed: %v", cmd, e.Err)
	}
	return fmt.Sprintf("git command %q failed", cmd)
}

// Unwrap returns the underlying error for error chain inspection.
func (e *GitError) Unwrap() error {
	return e.Err
}

func trim(s string) string {
	return strings.TrimSpace(s)
}
