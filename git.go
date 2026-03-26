// Package git provides utilities for git operations across multiple repositories.
package git

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"sync"
	"syscall"

	"dappco.re/go/core"
)

var gitBinary struct {
	once sync.Once
	path string
	err  error
}

// RepoStatus represents the git status of a single repository.
// status := git.RepoStatus{Name: "core", Path: "/srv/repos/core"}
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
// dirty := status.IsDirty()
func (s *RepoStatus) IsDirty() bool {
	return s.Modified > 0 || s.Untracked > 0 || s.Staged > 0
}

// HasUnpushed returns true if there are commits to push.
// ahead := status.HasUnpushed()
func (s *RepoStatus) HasUnpushed() bool {
	return s.Ahead > 0
}

// HasUnpulled returns true if there are commits to pull.
// behind := status.HasUnpulled()
func (s *RepoStatus) HasUnpulled() bool {
	return s.Behind > 0
}

// StatusOptions configures the status check.
// opts := git.StatusOptions{Paths: []string{"/srv/repos/core"}, Names: map[string]string{"/srv/repos/core": "core"}}
type StatusOptions struct {
	// Paths is a list of repo paths to check.
	Paths []string
	// Names maps paths to display names.
	Names map[string]string
}

// Status checks git status for multiple repositories in parallel.
// statuses := git.Status(ctx, git.StatusOptions{Paths: []string{"/srv/repos/core"}})
func Status(ctx context.Context, opts StatusOptions) []RepoStatus {
	var wg sync.WaitGroup
	results := make([]RepoStatus, len(opts.Paths))

	for i, repoPath := range opts.Paths {
		wg.Add(1)
		go func(idx int, repoPath string) {
			defer wg.Done()
			name := opts.Names[repoPath]
			if name == "" {
				name = repoPath
			}
			results[idx] = getStatus(ctx, repoPath, name)
		}(i, repoPath)
	}

	wg.Wait()
	return results
}

// getStatus gets the git status for a single repository.
func getStatus(ctx context.Context, repoPath, name string) RepoStatus {
	status := RepoStatus{
		Name: name,
		Path: repoPath,
	}

	if !isAbsolutePath(repoPath) {
		status.Error = core.E("git.getStatus", core.Concat("path must be absolute: ", repoPath), nil)
		return status
	}

	branch, err := gitCommand(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		status.Error = err
		return status
	}
	status.Branch = core.Trim(branch)

	porcelain, err := gitCommand(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		status.Error = err
		return status
	}

	for _, line := range core.Split(porcelain, "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]

		if x == '?' && y == '?' {
			status.Untracked++
			continue
		}

		if slices.Contains([]byte{'A', 'D', 'R', 'M'}, x) {
			status.Staged++
		}
		if slices.Contains([]byte{'M', 'D'}, y) {
			status.Modified++
		}
	}

	ahead, behind, err := getAheadBehind(ctx, repoPath)
	if err == nil {
		status.Ahead = ahead
		status.Behind = behind
	}

	return status
}

// getAheadBehind returns the number of commits ahead and behind upstream.
func getAheadBehind(ctx context.Context, repoPath string) (ahead, behind int, err error) {
	aheadStr, err := gitCommand(ctx, repoPath, "rev-list", "--count", "@{u}..HEAD")
	if err == nil {
		ahead, _ = strconv.Atoi(core.Trim(aheadStr))
	} else if !isNoUpstreamError(err) {
		return 0, 0, err
	}

	behindStr, err := gitCommand(ctx, repoPath, "rev-list", "--count", "HEAD..@{u}")
	if err == nil {
		behind, _ = strconv.Atoi(core.Trim(behindStr))
	} else if !isNoUpstreamError(err) {
		return 0, 0, err
	}

	return ahead, behind, nil
}

func isNoUpstreamError(err error) bool {
	if err == nil {
		return false
	}
	msg := core.Lower(err.Error())
	return core.Contains(msg, "no upstream")
}

// Push pushes commits for a single repository.
// err := git.Push(ctx, "/srv/repos/core")
func Push(ctx context.Context, path string) error {
	return gitInteractive(ctx, path, "push")
}

// Pull pulls changes for a single repository.
// err := git.Pull(ctx, "/srv/repos/core")
func Pull(ctx context.Context, path string) error {
	return gitInteractive(ctx, path, "pull", "--rebase")
}

// IsNonFastForward checks if an error is a non-fast-forward rejection.
// rejected := git.IsNonFastForward(err)
func IsNonFastForward(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return core.Contains(msg, "non-fast-forward") ||
		core.Contains(msg, "fetch first") ||
		core.Contains(msg, "tip of your current branch is behind")
}

// gitInteractive runs a git command with terminal attached for user interaction.
func gitInteractive(ctx context.Context, dir string, args ...string) error {
	result, err := runGitProcess(ctx, dir, gitProcessModeInteractive, args...)
	if err != nil {
		return &GitError{
			Args:   args,
			Err:    err,
			Stderr: result.stderr,
		}
	}

	return nil
}

// PushResult represents the result of a push operation.
// result := git.PushResult{Name: "core", Path: "/srv/repos/core"}
type PushResult struct {
	Name    string
	Path    string
	Success bool
	Error   error
}

// PushMultiple pushes multiple repositories sequentially.
// results, err := git.PushMultiple(ctx, []string{"/srv/repos/core"}, map[string]string{"/srv/repos/core": "core"})
func PushMultiple(ctx context.Context, paths []string, names map[string]string) ([]PushResult, error) {
	results := make([]PushResult, len(paths))
	var lastErr error

	for i, repoPath := range paths {
		name := names[repoPath]
		if name == "" {
			name = repoPath
		}

		result := PushResult{
			Name: name,
			Path: repoPath,
		}

		err := Push(ctx, repoPath)
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
	result, err := runGitProcess(ctx, dir, gitProcessModeCapture, args...)
	if err != nil {
		return "", &GitError{
			Args:   args,
			Err:    err,
			Stderr: result.stderr,
		}
	}

	return result.stdout, nil
}

// Compile-time interface checks.
var _ error = (*GitError)(nil)

// GitError wraps a git command error with stderr output and command context.
// var gitErr *git.GitError
type GitError struct {
	Args   []string
	Err    error
	Stderr string
}

// Error returns a descriptive error message.
// msg := err.Error()
func (e *GitError) Error() string {
	cmd := core.Concat("git ", core.Join(" ", e.Args...))
	stderr := core.Trim(e.Stderr)

	if stderr != "" {
		return core.Concat("git command \"", cmd, "\" failed: ", stderr)
	}
	if e.Err != nil {
		return core.Concat("git command \"", cmd, "\" failed: ", e.Err.Error())
	}
	return core.Concat("git command \"", cmd, "\" failed")
}

// Unwrap returns the underlying error for error chain inspection.
// cause := err.Unwrap()
func (e *GitError) Unwrap() error {
	return e.Err
}

type gitProcessMode int

const (
	gitProcessModeCapture gitProcessMode = iota
	gitProcessModeInteractive
)

type gitProcessResult struct {
	stdout string
	stderr string
}

func runGitProcess(ctx context.Context, dir string, mode gitProcessMode, args ...string) (gitProcessResult, error) {
	var result gitProcessResult

	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, core.Wrap(err, "git.runProcess", "git command cancelled")
	}

	gitPath, err := findGitBinary()
	if err != nil {
		return result, err
	}

	stdinFD := uintptr(syscall.Stdin)
	stdoutFD := uintptr(syscall.Stdout)
	stderrFD := uintptr(syscall.Stderr)
	stdoutReadFD, stdoutWriteFD := -1, -1
	stderrReadFD, stderrWriteFD := -1, -1

	closeReadFDs := func() {
		closeFD(stdoutReadFD)
		closeFD(stderrReadFD)
		stdoutReadFD = -1
		stderrReadFD = -1
	}
	closeWriteFDs := func() {
		closeFD(stdoutWriteFD)
		closeFD(stderrWriteFD)
		stdoutWriteFD = -1
		stderrWriteFD = -1
	}

	switch mode {
	case gitProcessModeCapture:
		stdoutReadFD, stdoutWriteFD, err = openPipe()
		if err != nil {
			return result, core.Wrap(err, "git.runProcess", "open git stdout pipe")
		}

		stderrReadFD, stderrWriteFD, err = openPipe()
		if err != nil {
			closeWriteFDs()
			closeReadFDs()
			return result, core.Wrap(err, "git.runProcess", "open git stderr pipe")
		}

		stdoutFD = uintptr(stdoutWriteFD)
		stderrFD = uintptr(stderrWriteFD)

	case gitProcessModeInteractive:
		stderrReadFD, stderrWriteFD, err = openPipe()
		if err != nil {
			return result, core.Wrap(err, "git.runProcess", "open git stderr pipe")
		}
		stderrFD = uintptr(stderrWriteFD)
	}

	argv := append([]string{gitPath}, args...)
	pid, err := syscall.ForkExec(gitPath, argv, &syscall.ProcAttr{
		Dir:   dir,
		Env:   syscall.Environ(),
		Files: []uintptr{stdinFD, stdoutFD, stderrFD},
	})
	if err != nil {
		closeWriteFDs()
		closeReadFDs()
		return result, core.Wrap(err, "git.runProcess", "start git process")
	}

	closeWriteFDs()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var stdoutCopyErrs <-chan error
	var stderrCopyErrs <-chan error

	if stdoutReadFD >= 0 {
		copyErrs := make(chan error, 1)
		stdoutCopyErrs = copyErrs
		go func(fd int) {
			copyErrs <- streamFD(fd, -1, &stdoutBuf)
		}(stdoutReadFD)
	}
	if stderrReadFD >= 0 {
		copyErrs := make(chan error, 1)
		stderrCopyErrs = copyErrs
		mirrorFD := -1
		if mode == gitProcessModeInteractive {
			mirrorFD = syscall.Stderr
		}
		go func(fd int, mirrorFD int) {
			copyErrs <- streamFD(fd, mirrorFD, &stderrBuf)
		}(stderrReadFD, mirrorFD)
	}

	waitDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(pid, syscall.SIGKILL)
		case <-waitDone:
		}
	}()

	var status syscall.WaitStatus
	for {
		_, err = syscall.Wait4(pid, &status, 0, nil)
		if err == syscall.EINTR {
			continue
		}
		break
	}
	close(waitDone)

	var stdoutCopyErr error
	if stdoutCopyErrs != nil {
		stdoutCopyErr = <-stdoutCopyErrs
	}
	var stderrCopyErr error
	if stderrCopyErrs != nil {
		stderrCopyErr = <-stderrCopyErrs
	}

	result.stdout = stdoutBuf.String()
	result.stderr = stderrBuf.String()

	if stdoutCopyErr != nil {
		return result, core.Wrap(stdoutCopyErr, "git.runProcess", "read git stdout")
	}
	if stderrCopyErr != nil {
		return result, core.Wrap(stderrCopyErr, "git.runProcess", "read git stderr")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, core.Wrap(ctxErr, "git.runProcess", "git command cancelled")
	}
	if err != nil {
		return result, core.Wrap(err, "git.runProcess", "wait for git process")
	}
	if status.Signaled() {
		return result, core.E(
			"git.runProcess",
			core.Concat("git command terminated by signal ", strconv.Itoa(int(status.Signal()))),
			nil,
		)
	}
	if !status.Exited() {
		return result, core.E("git.runProcess", "git command did not exit cleanly", nil)
	}
	if status.ExitStatus() != 0 {
		return result, core.E(
			"git.runProcess",
			core.Concat("git command exited with status ", strconv.Itoa(status.ExitStatus())),
			nil,
		)
	}

	return result, nil
}

func findGitBinary() (string, error) {
	gitBinary.once.Do(func() {
		result := core.Find("git", "Git")
		if !result.OK {
			if err, ok := result.Value.(error); ok {
				gitBinary.err = core.Wrap(err, "git.findBinary", "git binary not found")
			} else {
				gitBinary.err = core.E("git.findBinary", "git binary not found", nil)
			}
			return
		}

		app, ok := result.Value.(*core.App)
		if !ok || app.Path == "" {
			gitBinary.err = core.E("git.findBinary", "git binary not found", nil)
			return
		}

		gitBinary.path = app.Path
	})

	return gitBinary.path, gitBinary.err
}

func openPipe() (int, int, error) {
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return -1, -1, err
	}
	return fds[0], fds[1], nil
}

func closeFD(fd int) {
	if fd >= 0 {
		_ = syscall.Close(fd)
	}
}

func streamFD(fd int, mirrorFD int, buf *bytes.Buffer) error {
	defer closeFD(fd)

	scratch := make([]byte, 4096)
	for {
		n, err := syscall.Read(fd, scratch)
		if err == syscall.EINTR {
			continue
		}

		if n > 0 {
			chunk := scratch[:n]
			_, _ = buf.Write(chunk)
			if mirrorFD >= 0 {
				if err := writeAllFD(mirrorFD, chunk); err != nil {
					return err
				}
			}
		}

		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

func writeAllFD(fd int, data []byte) error {
	for len(data) > 0 {
		n, err := syscall.Write(fd, data)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		data = data[n:]
	}

	return nil
}
