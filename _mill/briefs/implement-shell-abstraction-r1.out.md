All 5 cards are committed (Cards 12+13 combined into one commit as permitted), verify passes, and the working tree is clean.

{"status":"success","commit_sha":"9dda7fb","session_id":"b8f6a78d-5d87-4bfc-bb07-c619736a3d2a"}

Summary of work for batch `04-shell-abstraction`:

- `internal/shell/shell.go` (new) — `Shell` interface, `ForGOOS()`, `Pwsh()`, `Posix()` constructors.
- `internal/shell/pwsh.go` (new) — pwsh impl (`Quote`/`Invoke`/`ReadFile`).
- `internal/shell/posix.go` (new) — posix impl.
- `internal/shell/shell_test.go` (new) — table tests for both impls, including migrated pwsh-quote cases.
- `internal/shuttleengine/claudeengine/command.go` — removed `pwshSingleQuote`; `buildLaunchCmd`/`buildResumeCmd` now take a leading `shell.Shell` and compose via `sh.Invoke`/`sh.Quote`/`sh.ReadFile`.
- `internal/shuttleengine/claudeengine/claudeengine.go` — `Prepare` obtains `shell.ForGOOS()` and threads it through; `PosixPath` hook-path call left untouched.
- `internal/shuttleengine/claudeengine/command_test.go` — removed `TestPwshSingleQuote`; threaded `shell.Pwsh()`/`shell.Posix()` into build tests, added a posix-shell row.
- `CONSTRAINTS.md` — added "Shell Mechanics Seam" invariant.
- `docs/overview.md` — added `internal/shell` to the shared-infra sentence and directory tree.

Verify passed: `GOOS=linux go build ./internal/shell/... ./internal/shuttleengine/...` and `go test ./internal/shell/... ./internal/shuttleengine/...` — all green.
