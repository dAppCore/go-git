// Package git provides utilities for git operations across multiple repositories.
package git

import (
	"bytes"
	"context"
	"fmt"
	goio "io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	coreerr "forge.lthn.ai/core/go-log"
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

	// Validate path to prevent directory traversal
	if !filepath.IsAbs(path) {
		status.Error = coreerr.E("git.getStatus", "path must be absolute: "+path, nil)
		return status
	}

	// Get current branch
	branch, err := gitCommand(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		status.Error = err
		return status
	}
	status.Branch = strings.TrimSpace(branch)

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
		if slices.Contains([]byte{'A', 'D', 'R', 'M'}, x) {
			status.Staged++
		}

		// Modified in working tree
		if slices.Contains([]byte{'M', 'D'}, y) {
			status.Modified++
		}
	}

	// Get ahead/behind counts
	ahead, behind, err := getAheadBehind(ctx, path)
	if err != nil {
		// We don't fail the whole status if ahead/behind fails (might be no upstream)
		// but we could log it or store it if needed. For now, we just keep 0.
	}
	status.Ahead = ahead
	status.Behind = behind

	return status
}

// getAheadBehind returns the number of commits ahead and behind upstream.
func getAheadBehind(ctx context.Context, path string) (ahead, behind int, err error) {
	// Try to get ahead count
	aheadStr, err := gitCommand(ctx, path, "rev-list", "--count", "@{u}..HEAD")
	if err == nil {
		ahead, _ = strconv.Atoi(strings.TrimSpace(aheadStr))
	} else {
		// If it failed because of no upstream, don't return error
		if strings.Contains(err.Error(), "no upstream") || strings.Contains(err.Error(), "No upstream") {
			err = nil
		}
	}

	if err != nil {
		return 0, 0, err
	}

	// Try to get behind count
	behindStr, err := gitCommand(ctx, path, "rev-list", "--count", "HEAD..@{u}")
	if err == nil {
		behind, _ = strconv.Atoi(strings.TrimSpace(behindStr))
	} else {
		if strings.Contains(err.Error(), "no upstream") || strings.Contains(err.Error(), "No upstream") {
			err = nil
		}
	}

	return ahead, behind, err
}

// Push pushes commits for a single repository.
// Uses interactive mode to support SSH passphrase prompts.
func Push(ctx context.Context, path string) error {
	return gitInteractive(ctx, path, "push")
}

// Pull pulls changes for a single repository.
// Uses interactive mode to support SSH passphrase prompts.
func Pull(ctx context.Context, path string) error {
	return gitInteractive(ctx, path, "pull", "--rebase")
}

// IsNonFastForward checks if an error is a non-fast-forward rejection.
func IsNonFastForward(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "fetch first") ||
		strings.Contains(msg, "tip of your current branch is behind")
}

// gitInteractive runs a git command with terminal attached for user interaction.
func gitInteractive(ctx context.Context, dir string, args ...string) error {
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
	cmd := "git " + strings.Join(e.Args, " ")
	stderr := strings.TrimSpace(e.Stderr)

	if stderr != "" {
		return coreerr.E("", fmt.Sprintf("git command %q failed: %s", cmd, stderr), nil).Error()
	}
	return coreerr.E("", fmt.Sprintf("git command %q failed", cmd), e.Err).Error()
}

// Unwrap returns the underlying error for error chain inspection.
func (e *GitError) Unwrap() error {
	return e.Err
}
