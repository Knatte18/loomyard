Clean working tree, all cards committed and pushed, verify passes.

{"status":"success","commit_sha":"6f7df10","session_id":"ff10f848-67ed-449c-a82e-b3301a036a80"}

Summary of work done for batch `01-raw-git-error-cleanup`:

- `C:\Code\loomyard\wts\lyx-sandbox-fixes\internal\hubgeometry\hubgeometry.go` — `Resolve()` now returns bare `ErrNotAGitRepo` (no appended git stderr) on non-zero exit.
- `C:\Code\loomyard\wts\lyx-sandbox-fixes\internal\hubgeometry\hubgeometry_test.go` — pinned bare-sentinel assertions in `TestResolve_NotAGitRepo`.
- `C:\Code\loomyard\wts\lyx-sandbox-fixes\internal\idecli\cli.go` / `cli_test.go` — dropped redundant prefix; added `TestRunCLI_NotAGitRepo`.
- `C:\Code\loomyard\wts\lyx-sandbox-fixes\internal\initcli\initcli.go` / `initcli_test.go` — dropped redundant prefix; added `TestRunInit_NotAGitRepo`.
- `C:\Code\loomyard\wts\lyx-sandbox-fixes\internal\configcli\configcli.go` / `reconcile_test.go` — dropped redundant prefix in `runReconcile`; added `TestReconcile_NotAGitRepo`.
- `C:\Code\loomyard\wts\lyx-sandbox-fixes\internal\muxpoccli\cli.go` / `cli_test.go` — fixed doubled "not a git repository" message; added `TestRunCLINotAGitRepo`.

Verify command (`go build ./... && go test -tags integration ./internal/hubgeometry/... ./internal/idecli/... ./internal/initcli/... && go test ./internal/configcli/... ./internal/muxpoccli/...`) passed. All 5 cards committed individually and pushed to `lyx-sandbox-fixes`. Working tree is clean.

Note: `gofmt -l` flags these files, but confirmed pre-existing/repo-wide (checked out with `core.autocrlf=true`, CRLF line endings) — verified via `git stash` that unmodified files show the same gofmt diff, and via a CRLF→LF normalized copy that the actual code content is gofmt-clean. No formatter drift was introduced by this batch.

{"status":"success","commit_sha":"6f7df10","session_id":"ff10f848-67ed-449c-a82e-b3301a036a80"}
