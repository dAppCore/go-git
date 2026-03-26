---
title: Development
description: How to build, test, lint, and contribute to go-git.
---

# Development

## Prerequisites

- **Go 1.26** or later
- **Git** (system binary -- the library shells out to `git`)
- **golangci-lint** (for linting)

go-git is part of the Go workspace at `~/Code/go.work`. If you are working within that workspace, module resolution is handled automatically. Otherwise, ensure `GOPRIVATE=forge.lthn.ai/*` is set so Go can fetch private modules.

## Running tests

```bash
# All tests
go test ./... -v

# Single test
go test -run TestGit_GetStatus_CleanRepo_Good -v

# With race detector
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

If you have the `core` CLI installed:

```bash
core go test
core go cov --open
```

Current test coverage: **96.7%**.

Tests create real temporary Git repositories using `initTestRepo()`. This helper initialises a fresh repo with `git init`, configures a user, creates a `README.md`, and makes an initial commit. Some tests additionally set up bare remotes and clones to test push, pull, and ahead/behind counting.

### Test naming convention

Tests follow the `_Good`, `_Bad`, `_Ugly` suffix pattern:

| Suffix | Purpose | Example |
|--------|---------|---------|
| `_Good` | Happy path | `TestGit_GetStatus_CleanRepo_Good` |
| `_Bad` | Expected error conditions | `TestGit_GitCommand_NotARepo_Bad` |
| `_Ugly` | Panic and edge cases | (none currently) |

## Formatting and linting

```bash
# Format
gofmt -w .

# Vet
go vet ./...

# Lint (uses .golangci.yml config)
golangci-lint run ./...
```

Or with the `core` CLI:

```bash
core go fmt
core go vet
core go lint
core go qa          # fmt + vet + lint + test
core go qa full     # + race, vuln, security
```

### Linter configuration

The `.golangci.yml` enables these linters: `govet`, `errcheck`, `staticcheck`, `unused`, `gosimple`, `ineffassign`, `typecheck`, `gocritic`, `gofmt`. Explicitly disabled: `exhaustive`, `wrapcheck`.

## Project structure

```
go-git/
  git.go                 # Standalone Git operations (Core primitives + syscall runner)
  git_test.go            # Tests for standalone operations
  service.go             # Core framework service integration
  service_test.go        # Service helper/iterator tests
  service_extra_test.go  # Service query/task handler integration tests
  go.mod                 # Module definition
  .golangci.yml          # Linter configuration
  .core/
    build.yaml           # Build targets and flags
    release.yaml         # Release and changelog configuration
  docs/
    index.md             # This documentation
    architecture.md      # Internals and data flow
    development.md       # Build, test, contribute (this file)
```

## Coding standards

- **UK English** in all comments and documentation: colour, organisation, centre, initialise.
- **Strict error handling**: all Git command failures are wrapped in `*GitError` with captured stderr.
- **Absolute paths only**: both the standalone functions and the service layer reject relative paths.
- **No CGO**: the build configuration sets `cgo: false`.
- **Conventional commits**: `feat:`, `fix:`, `docs:`, `refactor:`, `chore:`, `perf:`.
- **Co-author line**: include `Co-Authored-By: Virgil <virgil@lethean.io>` when pairing with the agent.

## Adding a new Git operation

1. Add the standalone function to `git.go`. It should accept a `context.Context` and absolute path, return structured results, and wrap errors in `*GitError`.

2. If the operation should be available via the Core message bus, define a query or task type in `service.go` and add a case to `handleQuery` or `handleTask`. All paths must pass through `validatePath()`.

3. Write tests using `initTestRepo()` for real Git operations. Follow the `TestFile_Function_..._{Good,Bad,Ugly}` naming convention.

4. Run the full QA suite before submitting:

```bash
core go qa full
```

## Forge remote

The canonical repository is on Forgejo. Push via SSH:

```bash
git remote add forge ssh://git@forge.lthn.ai:2223/core/go-git.git
git push forge main
```

HTTPS authentication is not supported for pushes.

## Licence

All contributions are licensed under the [European Union Public Licence (EUPL-1.2)](../LICENSE.md).
