# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Multi-repository git operations library. Parallel status checks, sequential push/pull (for SSH passphrase prompts), error handling with stderr capture.

**Module:** `dappco.re/go/git`
**Go:** 1.26+

The Go module has been moved under `go/` and the repo root now hosts cross-language/ancillary artefacts.

## Repo Layout

```text
core/go-git/
├── go/                  ← Go module root (dappco.re/go/git)
├── tests/               ← non-Go-mixed helper fixtures (keep at root)
├── docs/                ← shared docs (symlinked from go/docs)
├── .woodpecker.yml
├── sonar-project.properties
├── README.md
├── CLAUDE.md
└── AGENTS.md
```

## Go Resolution Modes

Two practical ways this module is consumed:

| Mode | When | What runs |
|------|------|-----------|
| **Module mode (default)** | Local development and CI jobs run from `go/` | `go test`, `go vet`, `go mod tidy`, etc. use `go/go.mod` directly. |
| **`GOWORK=off` explicit** | Reproducibility checks | Forces pure `go.mod` resolution and bypasses any outer workspace fallback. This mode is used by the requested verification commands and local parity checks. |

## Build & Test

```bash
cd go
go test ./... -v           # Run all tests
go test -run TestName      # Run single test
golangci-lint run ./...    # Lint (see .golangci.yml for enabled linters)
```

## Architecture

Two files:
- `go/git.go` — Core operations: Status, Push, Pull, PushMultiple. Stdlib only, no framework dependency.
- `go/service.go` — Core framework integration via `dappco.re/go/core`. Exposes query types (QueryStatus, QueryDirtyRepos, QueryAheadRepos, QueryBehindRepos) and task types (TaskPush, TaskPull, TaskPushMultiple, TaskPullMultiple). Service uses `core.ServiceRuntime` with query and action handler registration in `OnStartup`. Also provides iterator methods (All, Dirty, Ahead, Behind) using `iter.Seq`.

## Key Design Decisions

- **Status is parallel** (goroutine per repo), **Push/Pull are sequential** (SSH passphrase prompts need terminal interaction via stdin/stdout).
- All paths must be absolute. Service enforces WorkDir boundary via `validatePath`.
- `QueryStatus` and `StatusOptions` are structurally identical — cast directly between them.
- Errors are wrapped as `*GitError` with captured stderr and command args.

## Test Conventions

- `_Good` / `_Bad` suffix pattern for success / failure cases.
- Tests use real git repos created by `initTestRepo()` in temp directories.
- Service helper tests (in `go/service_test.go`) construct `Service` structs directly without the framework.
- Service tests in `go/service_test.go` can construct `Service` structs directly or via `core.New()` to exercise handler dispatch in the relevant scenarios.
- Module tests and CLI fixtures in `tests/` remain at repo root because `tests/` is mixed-language.

## Coding Standards

- UK English in comments
- `Co-Authored-By: Virgil <virgil@lethean.io>` in commits
- Conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `chore:`

## GitHub Remote

```bash
git remote add origin git@github.com:dAppCore/go-git.git
```
