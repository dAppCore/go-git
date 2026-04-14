# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Multi-repository git operations library. Parallel status checks, sequential push/pull (for SSH passphrase prompts), error handling with stderr capture.

**Module:** `dappco.re/go/git`
**Go:** 1.26+

## Build & Test

```bash
go test ./... -v           # Run all tests
go test -run TestName      # Run single test
golangci-lint run ./...    # Lint (see .golangci.yml for enabled linters)
```

## Architecture

Two files:
- `git.go` — Core operations: Status, Push, Pull, PushMultiple. Stdlib only, no framework dependency.
- `service.go` — Core framework integration via `dappco.re/go/core`. Exposes query types (QueryStatus, QueryDirtyRepos, QueryAheadRepos) and task types (TaskPush, TaskPull, TaskPushMultiple). Service uses `core.ServiceRuntime` with query/task handler registration in `OnStartup`. Also provides iterator methods (All, Dirty, Ahead) using `iter.Seq`.

## Key Design Decisions

- **Status is parallel** (goroutine per repo), **Push/Pull are sequential** (SSH passphrase prompts need terminal interaction via stdin/stdout).
- All paths must be absolute. Service enforces WorkDir boundary via `validatePath`.
- `QueryStatus` and `StatusOptions` are structurally identical — cast directly between them.
- Errors are wrapped as `*GitError` with captured stderr and command args.

## Test Conventions

- `_Good` / `_Bad` suffix pattern for success / failure cases.
- Tests use real git repos created by `initTestRepo()` in temp directories.
- Service helper tests (in `service_test.go`) construct `Service` structs directly without the framework.
- Framework integration tests (in `service_extra_test.go`) use `core.New()` and test handler dispatch.

## Coding Standards

- UK English in comments
- `Co-Authored-By: Virgil <virgil@lethean.io>` in commits
- Conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `chore:`

## Forge Remote

```bash
git remote add forge ssh://git@forge.lthn.ai:2223/core/go-git.git
```
