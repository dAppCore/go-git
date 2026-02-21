# CLAUDE.md — go-git

## Overview

Multi-repository git operations library. Parallel status checks, sequential push/pull (for SSH passphrase prompts), error handling with stderr capture.

**Module:** `forge.lthn.ai/core/go-git`
**Extracted from:** `forge.lthn.ai/core/go-scm/git/`
**Coverage:** 96.7%

## Build & Test

```bash
go test ./... -v        # Run all tests
go test -run TestName   # Run single test
```

## Architecture

Two files:
- `git.go` — Core operations: Status, Push, Pull, PushMultiple (no framework dependency)
- `service.go` — Core framework integration: queries (QueryStatus, QueryDirtyRepos, QueryAheadRepos) and tasks (TaskPush, TaskPull, TaskPushMultiple)

## Key Types

```go
type RepoStatus struct {
    Name, Path, Branch string
    Modified, Untracked, Staged, Ahead, Behind int
    Error error
}

func Status(ctx context.Context, opts StatusOptions) []RepoStatus  // parallel
func Push(ctx context.Context, path string) error                   // interactive (stdin/stdout)
func Pull(ctx context.Context, path string) error                   // interactive (stdin/stdout)
func PushMultiple(ctx context.Context, paths []string, names map[string]string) []PushResult  // sequential
func IsNonFastForward(err error) bool
```

## Test Naming

`_Good`, `_Bad`, `_Ugly` suffix pattern. Tests use real git repos via `initTestRepo()`.

## Coding Standards

- UK English in comments
- `Co-Authored-By: Virgil <virgil@lethean.io>` in commits
- Errors wrapped as `*GitError` with stderr capture

## Dependency

Only `forge.lthn.ai/core/go/pkg/framework` for ServiceRuntime integration. The core git operations (`git.go`) use only stdlib.

## Forge Remote

```bash
git remote add forge ssh://git@forge.lthn.ai:2223/core/go-git.git
```
