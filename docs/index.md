---
title: go-git
description: Multi-repository Git operations library for Go with parallel status checking and Core framework integration.
---

# go-git

**Module:** `forge.lthn.ai/core/go-git`

**Go version:** 1.26+

**Licence:** [EUPL-1.2](../LICENSE.md)

## What it does

go-git is a Go library for orchestrating Git operations across multiple repositories. It was extracted from `forge.lthn.ai/core/go-scm/git/` into a standalone module.

The library provides two layers:

1. **Standalone functions** -- pure Git operations that depend only on the standard library.
2. **Core service integration** -- a `Service` type that plugs into the Core DI framework, exposing Git operations via the query/task message bus.

Typical use cases include multi-repo status dashboards, batch push/pull workflows, and CI tooling that needs to inspect many repositories at once.

## Quick start

### Standalone usage (no framework)

```go
package main

import (
    "context"
    "fmt"

    git "forge.lthn.ai/core/go-git"
)

func main() {
    statuses := git.Status(context.Background(), git.StatusOptions{
        Paths: []string{"/home/dev/repo-a", "/home/dev/repo-b"},
        Names: map[string]string{
            "/home/dev/repo-a": "repo-a",
            "/home/dev/repo-b": "repo-b",
        },
    })

    for _, s := range statuses {
        if s.Error != nil {
            fmt.Printf("%s: error: %v\n", s.Name, s.Error)
            continue
        }
        fmt.Printf("%s [%s]: modified=%d untracked=%d staged=%d ahead=%d behind=%d\n",
            s.Name, s.Branch, s.Modified, s.Untracked, s.Staged, s.Ahead, s.Behind)
    }
}
```

### With the Core framework

```go
package main

import (
    "fmt"

    "forge.lthn.ai/core/go/pkg/core"
    git "forge.lthn.ai/core/go-git"
)

func main() {
    c, err := core.New(
        core.WithService(git.NewService(git.ServiceOptions{
            WorkDir: "/home/dev/projects",
        })),
    )
    if err != nil {
        panic(err)
    }

    // Query status via the message bus.
    result, err := c.Query(git.QueryStatus{
        Paths: []string{"/home/dev/projects/repo-a"},
        Names: map[string]string{"/home/dev/projects/repo-a": "repo-a"},
    })
    if err != nil {
        panic(err)
    }

    statuses := result.([]git.RepoStatus)
    for _, s := range statuses {
        fmt.Printf("%s: dirty=%v ahead=%v\n", s.Name, s.IsDirty(), s.HasUnpushed())
    }
}
```

## Package layout

| File | Purpose |
|------|---------|
| `git.go` | Standalone Git operations -- `Status`, `Push`, `Pull`, `PushMultiple`, error types. Zero framework dependencies. |
| `service.go` | Core framework integration -- `Service`, query types (`QueryStatus`, `QueryDirtyRepos`, `QueryAheadRepos`), task types (`TaskPush`, `TaskPull`, `TaskPushMultiple`). |
| `git_test.go` | Tests for standalone operations using real temporary Git repositories. |
| `service_test.go` | Tests for `Service` filtering helpers (`DirtyRepos`, `AheadRepos`, iterators). |
| `service_extra_test.go` | Integration tests for `Service` query/task handlers against the Core framework. |

## Dependencies

| Dependency | Purpose |
|------------|---------|
| `forge.lthn.ai/core/go/pkg/core` | DI container, `ServiceRuntime`, query/task bus (used only by `service.go`). |
| `github.com/stretchr/testify` | Assertions in tests (test-only). |

The standalone layer (`git.go`) uses only the Go standard library. It shells out to the system `git` binary -- there is no embedded Git implementation.

## Build targets

Defined in `.core/build.yaml`:

| OS | Architecture |
|----|-------------|
| Linux | amd64 |
| Linux | arm64 |
| Darwin | arm64 |
| Windows | amd64 |
