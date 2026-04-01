// Package git provides utilities for git operations across multiple repositories.
package git

import (
	"bytes"
	"context"
	goio "io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

	core "dappco.re/go/core"
	coreerr "dappco.re/go/core/log"
)

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
	var wg sync.WaitGroup
	results := make([]RepoStatus, len(opts.Paths))

	for i, path := range opts.Paths {
		wg.Add(1)
		go func(idx int, repoPath string) {
			defer wg.Done()
			name := opts.Names[repoPath]
			if name == "" {
				name = repoPath
			}
			results[idx] = getStatus(ctx, repoPath, name)
		}(i, path)
	}

	wg.Wait()
	return results
}

// getStatus gets the git status for a single repository.
func getStatus(ctx context.Context, path, name string) RepoStatus {
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
	status.Branch = core.Trim(branch)

	// Get porcelain status
	porcelain, err := gitCommand(ctx, path, "status", "--porcelain")
	if err != nil {
		status.Error = err
		return status
	}

	// Parse status output
	for line := range strings.SplitSeq(porcelain, "\n") {
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
		if slices.Contains([]byte{'A', 'C', 'D', 'R', 'M', 'T'}, x) {
			status.Staged++
		}

		// Modified in working tree
		if slices.Contains([]byte{'M', 'D', 'T'}, y) {
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

// isNoUpstreamError reports whether an error is due to a missing tracking branch.
func isNoUpstreamError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(core.Trim(err.Error()))
	return strings.Contains(msg, "no upstream")
}

func requireAbsolutePath(op string, path string) error {
	if core.PathIsAbs(path) {
		return nil
	}
	return coreerr.E(op, "path must be absolute: "+path, nil)
}

// getAheadBehind returns the number of commits ahead and behind upstream.
func getAheadBehind(ctx context.Context, path string) (ahead, behind int, err error) {
	if err := requireAbsolutePath("git.getAheadBehind", path); err != nil {
		return 0, 0, err
	}

	aheadStr, err := gitCommand(ctx, path, "rev-list", "--count", "@{u}..HEAD")
	if err == nil {
		ahead, err = strconv.Atoi(core.Trim(aheadStr))
		if err != nil {
			return 0, 0, coreerr.E("git.getAheadBehind", "failed to parse ahead count", err)
		}
	} else if isNoUpstreamError(err) {
		err = nil
	}

	if err != nil {
		return 0, 0, err
	}

	behindStr, err := gitCommand(ctx, path, "rev-list", "--count", "HEAD..@{u}")
	if err == nil {
		behind, err = strconv.Atoi(core.Trim(behindStr))
		if err != nil {
			return 0, 0, coreerr.E("git.getAheadBehind", "failed to parse behind count", err)
		}
	} else if isNoUpstreamError(err) {
		err = nil
	}

	return ahead, behind, err
}

// Push pushes commits for a single repository.
//
// Example:
//
//	err := Push(ctx, "/home/user/Code/core/agent")
//
// Uses interactive mode to support SSH passphrase prompts.
func Push(ctx context.Context, path string) error {
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
	msg := err.Error()
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "fetch first") ||
		strings.Contains(msg, "tip of your current branch is behind")
}

// gitInteractive runs a git command with terminal attached for user interaction.
func gitInteractive(ctx context.Context, dir string, args ...string) error {
	if err := requireAbsolutePath("git.interactive", dir); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	// Connect to terminal for SSH passphrase prompts
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	// Capture stderr for error reporting while also showing it
	var stderr bytes.Buffer
	cmd.Stderr = goio.MultiWriter(os.Stderr, &stderr)

	if err := cmd.Run(); err != nil {
		return &GitError{
			Args:   args,
			Err:    err,
			Stderr: stderr.String(),
		}
	}

	return nil
}

// PushResult represents the result of a push operation.
type PushResult struct {
	Name    string
	Path    string
	Success bool
	Error   error
}

// PushMultiple pushes multiple repositories sequentially.
// Sequential because SSH passphrase prompts need user interaction.
func PushMultiple(ctx context.Context, paths []string, names map[string]string) ([]PushResult, error) {
	results := make([]PushResult, len(paths))
	var lastErr error

	for i, path := range paths {
		name := names[path]
		if name == "" {
			name = path
		}

		result := PushResult{
			Name: name,
			Path: path,
		}

		err := Push(ctx, path)
		if err != nil {
			result.Error = err
			lastErr = err
		} else {
			result.Success = true
		}

		results[i] = result
	}

	return results, lastErr
}

// gitCommand runs a git command and returns stdout.
func gitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	if err := requireAbsolutePath("git.command", dir); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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
	cmd := "git " + core.Join(" ", e.Args...)
	stderr := core.Trim(e.Stderr)

	if stderr != "" {
		return core.Sprintf("git command %q failed: %s", cmd, stderr)
	}
	if e.Err != nil {
		return core.Sprintf("git command %q failed: %v", cmd, e.Err)
	}
	return core.Sprintf("git command %q failed", cmd)
}

// Unwrap returns the underlying error for error chain inspection.
func (e *GitError) Unwrap() error {
	return e.Err
}
