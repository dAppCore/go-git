---
title: Architecture
description: Internal design of go-git -- types, data flow, concurrency model, and error handling.
---

# Architecture

go-git is split into two layers: a standalone operations layer and a Core framework service layer. The standalone layer has no dependencies beyond the standard library; the service layer adds message-bus integration via the Core DI framework.

## Key types

### RepoStatus

The central data type. Represents the Git state of a single repository at a point in time.

```go
type RepoStatus struct {
    Name      string  // Display name (falls back to Path if not provided)
    Path      string  // Absolute filesystem path to the repository
    Modified  int     // Files modified in the working tree
    Untracked int     // Untracked files
    Staged    int     // Files staged in the index
    Ahead     int     // Commits ahead of upstream
    Behind    int     // Commits behind upstream
    Branch    string  // Current branch name
    Error     error   // Non-nil if status check failed
}
```

Three convenience methods classify the state:

- `IsDirty()` -- returns true when `Modified > 0 || Untracked > 0 || Staged > 0`.
- `HasUnpushed()` -- returns true when `Ahead > 0`.
- `HasUnpulled()` -- returns true when `Behind > 0`.

Note that `Ahead` and `Behind` counts require a tracking upstream branch. Without one, both default to zero rather than returning an error.

### GitError

All Git command failures are wrapped in `*GitError`, which captures the command arguments and stderr output:

```go
type GitError struct {
    Args   []string  // Git subcommand and arguments
    Err    error     // Underlying exec error
    Stderr string    // Captured stderr from the Git process
}
```

`GitError` implements the `error` interface. Its `Error()` method prefers the stderr text when available, falling back to the underlying error. It also implements `Unwrap()` so callers can use `errors.Is` and `errors.As` on the chain.

### PushResult

Returned by `PushMultiple`, one per repository:

```go
type PushResult struct {
    Name    string
    Path    string
    Success bool
    Error   error
}
```

### PullResult

Returned by `PullMultiple`, one per repository:

```go
type PullResult struct {
    Name    string
    Path    string
    Success bool
    Error   error
}
```

## Data flow

### Parallel status checking

`Status()` fans out one goroutine per repository path. Each goroutine calls `getStatus()`, which executes three sequential Git commands:

```
Status(ctx, opts)
  |
  +---> goroutine per path
          |
          +---> git rev-parse --abbrev-ref HEAD   (branch name)
          +---> git status --porcelain             (working tree state)
          +---> git rev-list --count @{u}..HEAD    (ahead count)
          +---> git rev-list --count HEAD..@{u}    (behind count)
```

Results are written to a pre-allocated slice indexed by position, so no mutex is needed for the result collection. A `sync.WaitGroup` gates the return.

### Porcelain status parsing

The `--porcelain` output is parsed character by character. Each line has a two-character status prefix:

| Position X (index) | Position Y (working tree) | Interpretation |
|---------------------|---------------------------|----------------|
| `?` | `?` | Untracked file |
| `A`, `D`, `R`, `M`, `U` | any | Staged change |
| any | `M`, `D`, `U` | Working tree modification |

A single file can increment both `Staged` and `Modified` if it has been staged and then further modified. Unmerged paths (`U`) increment both counters, which keeps conflicted repositories visibly dirty.

### Interactive push and pull

`Push()` and `Pull()` use `gitInteractive()`, which connects the child process to the terminal's stdin, stdout, and stderr. This is necessary to support SSH passphrase prompts.

`PushMultiple()` deliberately runs pushes **sequentially** rather than in parallel, because interactive SSH prompts cannot overlap on a single terminal.

Pull uses `--rebase` by default.

### Non-fast-forward detection

`IsNonFastForward(err)` inspects the error message for common Git rejection phrases:

- `"non-fast-forward"`
- `"fetch first"`
- `"tip of your current branch is behind"`

This allows callers to distinguish between network errors and conflicts that require a pull first.

## Service layer

### Registration

`NewService()` returns a factory function compatible with `core.WithService()`:

```go
c, err := core.New(
    core.WithService(git.NewService(git.ServiceOptions{
        WorkDir: "/home/dev/repos",
    })),
)
```

The factory constructs a `Service` embedding `core.ServiceRuntime[ServiceOptions]`.

### Lifecycle

`Service` implements the `Startable` interface. On startup, it registers the query/task bridge handlers and named Core actions for direct action-bus callers:

```go
func (s *Service) OnStartup(ctx context.Context) core.Result {
    s.Core().RegisterQuery(s.handleQuery)
    s.Core().RegisterAction(s.handleTaskMessage)

    s.Core().Action("git.push", func(ctx context.Context, opts core.Options) core.Result {
        return s.runPush(ctx, opts.String("path"))
    })
    s.Core().Action("git.pull", func(ctx context.Context, opts core.Options) core.Result {
        return s.runPull(ctx, opts.String("path"))
    })
    s.Core().Action("git.push-multiple", func(ctx context.Context, opts core.Options) core.Result {
        paths, names, err := multipleActionPayload(opts, "git.push-multiple")
        if err != nil {
            return s.logError(err, "git.push-multiple", "invalid action payload")
        }
        return s.runPushMultiple(ctx, paths, names)
    })
    s.Core().Action("git.pull-multiple", func(ctx context.Context, opts core.Options) core.Result {
        paths, names, err := multipleActionPayload(opts, "git.pull-multiple")
        if err != nil {
            return s.logError(err, "git.pull-multiple", "invalid action payload")
        }
        return s.runPullMultiple(ctx, paths, names)
    })

    return core.Result{OK: true}
}
```

### Query messages

| Type | Returns | Description |
|------|---------|-------------|
| `QueryStatus` | `[]RepoStatus` | Checks Git status for a set of paths (runs in parallel). Updates the cached `lastStatus`. |
| `QueryDirtyRepos` | `[]RepoStatus` | Filters `lastStatus` for repos with uncommitted changes. |
| `QueryAheadRepos` | `[]RepoStatus` | Filters `lastStatus` for repos with unpushed commits. |
| `QueryBehindRepos` | `[]RepoStatus` | Filters `lastStatus` for repos with unpulled commits. |

`QueryStatus` has the same fields as `StatusOptions` and can be type-converted directly:

```go
statuses := Status(ctx, StatusOptions(queryStatus))
```

### Task messages

| Type | Returns | Description |
|------|---------|-------------|
| `TaskPush` | `nil` | Pushes a single repository (interactive). |
| `TaskPull` | `nil` | Pulls a single repository with `--rebase` (interactive). |
| `TaskPushMultiple` | `[]PushResult` | Pushes multiple repositories sequentially. |
| `TaskPullMultiple` | `[]PullResult` | Pulls multiple repositories sequentially with `--rebase`. |

### Path validation

All query and task handlers validate paths before execution:

1. Paths must be absolute (rejects relative paths).
2. If `ServiceOptions.WorkDir` is set, both `WorkDir` and the target path are resolved through symlinks before the boundary check.
3. Validation failures return `*GitError` with command context in `Args` and a diagnostic message in `Stderr`.

```go
func (s *Service) validatePath(path string) (string, error) {
    if !filepath.IsAbs(path) {
        return "", gitValidationError("path must be absolute: "+path, path, s.opts.WorkDir, nil)
    }
    if s.opts.WorkDir != "" {
        resolvedWorkDir, err := filepath.EvalSymlinks(s.opts.WorkDir)
        if err != nil {
            return "", gitValidationError("failed to resolve WorkDir: "+s.opts.WorkDir, path, s.opts.WorkDir, err)
        }
        resolvedPath, err := filepath.EvalSymlinks(path)
        if err != nil {
            return "", gitValidationError("failed to resolve path: "+path, path, s.opts.WorkDir, err)
        }
        if !pathWithinWorkDir(resolvedPath, resolvedWorkDir) {
            return "", gitValidationError("path is outside of allowed WorkDir", path, s.opts.WorkDir, nil)
        }
        return resolvedPath, nil
    }
    return filepath.Clean(path), nil
}
```

### Cached status and iterators

The `Service` caches the most recent `QueryStatus` result in `lastStatus` (protected by `sync.RWMutex`). Several methods expose filtered views:

| Method | Returns | Description |
|--------|---------|-------------|
| `Status()` | `[]RepoStatus` | Clone of the last status slice. |
| `All()` | `iter.Seq[RepoStatus]` | Iterator over all cached statuses. |
| `Dirty()` | `iter.Seq[RepoStatus]` | Iterator over repos where `IsDirty()` is true and `Error` is nil. |
| `Ahead()` | `iter.Seq[RepoStatus]` | Iterator over repos where `HasUnpushed()` is true and `Error` is nil. |
| `Behind()` | `iter.Seq[RepoStatus]` | Iterator over repos where `HasUnpulled()` is true and `Error` is nil. |
| `DirtyRepos()` | `[]RepoStatus` | Collects `Dirty()` into a slice. |
| `AheadRepos()` | `[]RepoStatus` | Collects `Ahead()` into a slice. |
| `BehindRepos()` | `[]RepoStatus` | Collects `Behind()` into a slice. |

Errored repositories are excluded from `Dirty()`, `Ahead()`, and `Behind()` iterators.

## Concurrency model

- **Status checks**: Fully parallel. One goroutine per repository, results collected via indexed slice + WaitGroup.
- **Push/Pull**: Sequential. Interactive terminal I/O requires single-threaded execution.
- **Service state**: `lastStatus` is protected by `sync.RWMutex`. Reads (`Status()`, `All()`, `Dirty()`, `Ahead()`) take a read lock; writes (`handleQuery` for `QueryStatus`) take a write lock. All methods return cloned slices to prevent data races.
