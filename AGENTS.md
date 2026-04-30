# go-git Agent Notes

This repository provides `dappco.re/go/git`, a Core-compatible git service and
helper package. It wraps status, push, pull, and multi-repository operations in
the `dappco.re/go` `Result` shape so callers can branch on `r.OK` and inspect
per-repository errors without importing standard-library compatibility shims.

Keep production code on the Core wrapper surface. Use `core.Context`,
`core.Result`, `core.Path*`, `core.WriteFile`, `core.NewBuffer`,
`core.Sprintf`, `core.NewError`, and the Core assertion helpers instead of
direct imports of formatting, filesystem, process, string, or error packages.
The package still exposes iterator-based APIs for streaming repository results;
tests should collect those iterators directly in the sibling test file for the
source that defines the symbol.

Public symbol coverage follows the repository's AX-7 convention. Every public
function or method in `git.go` is tested in `git_test.go` with
`TestGit_<Symbol>_{Good,Bad,Ugly}`, and every public service method in
`service.go` is tested in `service_test.go` with the matching
`TestService_<Symbol>_{Good,Bad,Ugly}` name. Examples live in
`git_example_test.go` and `service_example_test.go`, use `Println` from
`dappco.re/go`, and keep their output deterministic.

The tests create local temporary git repositories and bare remotes. They do not
contact network remotes, and they configure a local test author before making
commits. When changing push or pull behavior, keep the fixtures local and make
failure cases explicit through relative paths or repositories with no matching
remote state.
