# Batch: cmd-lyx-root-wiring

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: cmd-lyx-root-wiring
number: 7
cards: 5
verify: go test -tags integration ./cmd/lyx/... ./internal/logger/...
depends-on: [2, 4, 5, 6]
```

## Batch Scope

Wires the root `PersistentPreRunE` in `cmd/lyx/main.go` to mint/adopt/export the trace ID and arm the durable sink, suppressed under `testing.Testing()` per discussion.md's `no-arming-under-test` decision, and wires trigger (b) (force-open on non-zero exit) into `run()`/`main()`. Adds the suppression-under-test unit test and the one `//go:build integration` test that spawns a real `lyx` binary. Closes out `internal/logger`'s package doc rewrite once every mechanism (trace, sink, retention, spans, fan-out) has landed. Depends on batches 2, 4, 5, and 6 — every logger mechanism this wiring activates.

## Cards

### Card 27: Root hook — mint/adopt/export and arm, suppressed under test

- **Context:**
  - `internal/reedengine/headerpane.go`
  - `cmd/lyx/main_integration_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `cmd/lyx/main.go`'s `newRoot()` (lines 70-133), extend the existing `PersistentPreRunE` (currently just `logger.SetVerbosity(verbosity); return nil`, lines 95-98):

  ```go
  PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
  	logger.SetVerbosity(verbosity)
  	if !testing.Testing() {
  		logger.MintOrAdoptAndExport()
  		logger.Arm()
  	}
  	return nil
  },
  ```

  `testing.Testing()` (stdlib `testing` package, reports true for any binary built by `go test`, tag-gated or not — confirmed this is true even inside `cmd/lyx/main_integration_test.go`'s `//go:build integration`-gated call to `run()`) gates both the mint/export (batch 2's `MintOrAdoptAndExport`) and the arm (batch 4's `Arm`) together, in one `if`, matching `internal/reedengine/headerpane.go`'s `headerLaunchLine` precedent for gating a production code path on test detection (CONSTRAINTS.md's Live-Substrate Spawn Observability entry names this precedent explicitly). `cobra.EnableTraverseRunHooks = true` (already set at `main.go:113`) guarantees this hook fires before every module's own `PersistentPreRunE` and before every subcommand body, so mint/arm always precede any code that could emit.
- **Commit:** `feat(cmd/lyx): mint/adopt/export trace-ID and arm durable sink in the root hook, suppressed under test`

### Card 28: `run()`/`main()` — trigger (b) wiring

- **Context:** none
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `cmd/lyx/main.go`, change both `main()` (lines 39-48) and `run()` (lines 55-63) so `clihelp.RunRoot`'s result is captured into a named variable before being returned/passed to `os.Exit`, and call `logger.NotifyExit(code)` (batch 4's Card 14) immediately after capturing it and before returning:

  ```go
  func main() {
  	root := newRoot()
  	root.SetOut(os.Stdout)
  	root.SetErr(os.Stderr)
  	code := clihelp.RunRoot(root, os.Stdout)
  	logger.NotifyExit(code)
  	os.Exit(code)
  }

  func run(args []string, out io.Writer) int {
  	root := newRoot()
  	root.SetOut(out)
  	root.SetErr(out)
  	root.SetArgs(args)
  	code := clihelp.RunRoot(root, out)
  	logger.NotifyExit(code)
  	return code
  }
  ```

  This is the implementation site discussion.md's `sink-open-triggers` decision names explicitly ("trigger (b) belongs in `cmd/lyx/main.go`'s `run()`, after `clihelp.RunRoot` returns its exit code and before the process exits"). Because `NotifyExit` itself calls `ensureDurableSink()`, which is gated by `testing.Testing()`/`LYX_TRACE` inside `sinkOnce.Do` (batch 4's Card 10), this call is safe to make unconditionally from both `main()` and `run()` — it is a no-op under plain `go test` with `LYX_TRACE` unset, exactly like `main_integration_test.go`'s existing in-process call to `run()` (which still reports `testing.Testing() == true` per the earlier research, so this new call does not open a file there either, consistent with why the integration test (Card 30) must spawn a real binary instead).
- **Commit:** `feat(cmd/lyx): call NotifyExit after RunRoot to force the durable sink on non-zero exit`

### Card 29: Suppression-under-test unit test

- **Context:**
  - `internal/reedengine/headerpane_test.go`
- **Edits:**
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a test to `cmd/lyx/main_test.go` mirroring `internal/reedengine/headerpane_test.go`'s `TestHeaderLaunchLine` shape (lines 47-64): call `run(...)` (or `newRoot()` directly and invoke its `PersistentPreRunE`) and assert that no `LYX_TRACE_ID` is exported to the environment (`os.Getenv("LYX_TRACE_ID")` unchanged from before the call, using `t.Setenv` to pin a known starting state) and that the durable sink never opens (no file appears under a `SetDurableSinkDir`-pointed `t.TempDir()`, batch 4's Card 12) as a result of running a command through `run()` in-process. Like `TestHeaderLaunchLine`'s own final assertion, also pin that `testing.Testing()` reports `true` inside this test process itself, so the suppression wiring's precondition cannot silently decay into a constant `false` without the test catching it.
- **Commit:** `test(cmd/lyx): pin that the root hook mints/exports/arms nothing under testing.Testing()`

### Card 30: Integration test — real binary, non-zero exit, header record

- **Context:**
  - `internal/reedcli/smoke_test.go`
- **Edits:**
  - `cmd/lyx/main_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend `cmd/lyx/main_integration_test.go` (already `//go:build integration`-tagged) with a new test that builds the `lyx` binary the way `internal/reedcli/smoke_test.go`'s `buildLyxBinary` does (lines 769-782 — critically, resolve the repo root via `runtime.Caller(0)` baked at compile time, not a relative parent-directory walk from `cwd`, since a pre-compiled test binary run from elsewhere has no automatic cwd) and spawns it as a real child process against a fixture worktree, with an argument that fails cheaply and deterministically — an unknown subcommand, which `clihelp.GroupRunE` already rejects with a non-zero exit (per discussion.md's `sink-open-triggers` "This also settles the integration test's trigger" paragraph). Assert: a trace file exists under `<fixture WorktreeRoot>/.lyx/logs/`, its name matches the `trace-<UTC>-<16-hex>-<pid>.log` grammar, and its first line is the header record naming the spawned command and a trace-ID. This is the **only** test that proves geometry resolution, root-hook wiring, and the real file path together — an in-process call to `run()` cannot, since `testing.Testing()` is true there and the root hook (Card 27) self-suppresses. The package needs a `TestMain` calling `lyxtest.HermeticGitEnv()` per the Hermetic Git Test Environment Invariant (this test spawns real git via the fixture worktree setup) — confirm whether the cmd/lyx test package already has one before adding a duplicate.
- **Commit:** `test(cmd/lyx): add integration test spawning a real binary to prove root-hook wiring and the header record`

### Card 31: internal/logger package doc rewrite

- **Context:**
  - `internal/logger/trace.go`
  - `internal/logger/span.go`
  - `internal/logger/sink.go`
  - `internal/logger/retention.go`
- **Edits:**
  - `internal/logger/logger.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Rewrite `internal/logger/logger.go`'s package doc (currently `logger.go:1-38`, describing the package as "a minimal log/slog wrapper" documenting only `LYX_LOG_LEVEL`/`LYX_LOG_FILE`) to cover, in the same "# Activation outside the lyx CLI" style of headed sections:
  - The trace/span model: `TraceID()`, `StartSpan`/`Child`/`End`, the `trace=`/`span=` fields stamped on every line.
  - The durable sink and its retention: worktree-anchored `.lyx/logs`, one file per process, lazy open on first Info+ or non-zero exit, the 14-day/newest-50 sweep, the 8 MiB cap and truncation marker.
  - `LYX_TRACE_ID` (adopt/export) and `LYX_TRACE=1` (test-entry-activation), alongside the existing `LYX_LOG_LEVEL`/`LYX_LOG_FILE` documentation (do not delete that section — this is a rewrite that adds sections, not a wholesale replacement of the accurate existing material).
  - The level policy verbatim from discussion.md's `level-policy` decision (Warn/Info/Debug definitions, and the "nothing at Warn inside a loop body that can iterate more than ~10 times without a state change" hard rule) — this decision explicitly states the policy is "to be stated in the internal/logger package doc."
  - State explicitly, per `dual-handler-fan-out`'s "`LYX_LOG_FILE` duplication is intended" bullet, that an Info+ line reaching both `LYX_LOG_FILE` (if set) and the durable trace file is correct, not a bug — so a future reader does not "fix" the apparent duplication.
- **Commit:** `docs(logger): rewrite the package doc to cover trace, spans, the durable sink, and the level policy`

## Batch Tests

`verify: go test -tags integration ./cmd/lyx/... ./internal/logger/...` — the `-tags integration` flag is required at this batch boundary specifically because this batch edits `cmd/lyx/main_integration_test.go` (Card 30); running with the tag also compiles and runs Cards 27-29 and 31's untagged tests (the tag only adds files, it never excludes untagged ones). Every other batch's `verify:` in this plan deliberately omits the tag, since only this batch touches the integration-tagged file.
</content>
