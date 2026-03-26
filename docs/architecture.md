---
title: Architecture
description: Internal design of go-git -- types, data flow, concurrency model, and error handling.
---

# Architecture

go-git is split into two layers: a standalone operations layer and a Core framework service layer. The standalone layer uses Core primitives for strings, structured errors, and git binary discovery; the service layer adds message-bus integration via the Core DI framework.

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
    Err    error     // Underlying process error
    Stderr string    // Captured stderr from the Git process
}
```

`GitError` implements the `error` interface. Its `Error()` method prefers the stderr text when available, falling back to the underlying error. It also implements `Unwrap()` so callers can use `core.Is` and `core.As` on the chain.

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
| `A`, `D`, `R`, `M` | any | Staged change |
| any | `M`, `D` | Working tree modification |

A single file can increment both `Staged` and `Modified` if it has been staged and then further modified.

### Interactive push and pull

`Push()` and `Pull()` use `gitInteractive()`, which connects the child process to the terminal's stdin and stdout while teeing stderr back to the terminal and into the captured `GitError`. This is necessary to support SSH passphrase prompts without losing structured failure details.

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

`NewService()` returns a factory function that binds a `*Service` to a `*core.Core`:

```go
c := core.New()
factory := git.NewService(git.ServiceOptions{
    WorkDir: "/home/dev/repos",
})

raw, err := factory(c)
if err != nil {
    panic(err)
}

svc := raw.(*git.Service)
if err := svc.OnStartup(context.Background()); err != nil {
    panic(err)
}
```

The factory constructs a `Service` embedding `core.ServiceRuntime[ServiceOptions]`.

### Lifecycle

`Service` implements the `Startable` interface. On startup, it registers a query handler and a task handler with the Core message bus:

```go
func (s *Service) OnStartup(ctx context.Context) error {
    s.Core().RegisterQuery(s.handleQuery)
    s.Core().RegisterTask(s.handleTask)
    return nil
}
```

### Query messages

| Type | Returns | Description |
|------|---------|-------------|
| `QueryStatus` | `[]RepoStatus` | Checks Git status for a set of paths (runs in parallel). Updates the cached `lastStatus`. |
| `QueryDirtyRepos` | `[]RepoStatus` | Filters `lastStatus` for repos with uncommitted changes. |
| `QueryAheadRepos` | `[]RepoStatus` | Filters `lastStatus` for repos with unpushed commits. |

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

### Path validation

All query and task handlers validate paths before execution:

1. Paths must be absolute (rejects relative paths).
2. If `ServiceOptions.WorkDir` is set, all paths must be descendants of that directory. This prevents directory traversal.

```go
func (s *Service) validatePath(path string) error {
    if !isAbsolutePath(path) {
        return core.E("git.validatePath", "path must be absolute: "+path, nil)
    }
    if s.opts.WorkDir != "" && !pathWithin(s.opts.WorkDir, path) {
        return core.E("git.validatePath", "path "+path+" is outside of allowed WorkDir "+s.opts.WorkDir, nil)
    }
    return nil
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
| `DirtyRepos()` | `[]RepoStatus` | Collects `Dirty()` into a slice. |
| `AheadRepos()` | `[]RepoStatus` | Collects `Ahead()` into a slice. |

Errored repositories are excluded from `Dirty()` and `Ahead()` iterators.

## Concurrency model

- **Status checks**: Fully parallel. One goroutine per repository, results collected via indexed slice + WaitGroup.
- **Push/Pull**: Sequential. Interactive terminal I/O requires single-threaded execution.
- **Service state**: `lastStatus` is protected by `sync.RWMutex`. Reads (`Status()`, `All()`, `Dirty()`, `Ahead()`) take a read lock; writes (`handleQuery` for `QueryStatus`) take a write lock. All methods return cloned slices to prevent data races.
