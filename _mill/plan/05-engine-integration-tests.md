# Batch: engine-integration-tests

```yaml
task: "Give codeintel a persistent, session-long daemon"
batch: "engine-integration-tests"
number: 5
cards: 2
verify: go test -tags integration -run=^$ ./internal/codeintelengine/...
depends-on: [4]
```

## Batch Scope

This batch adds the two `//go:build integration` proofs the task needs — one for item 5's supervised dispatch flip, one for item 4's `--in-file` resolve — against a real gopls. It is separated from the production batches so its edits to integration-tagged test files (`ensureserver_integration_test.go`, `supervised_integration_test.go`, `refs_integration_test.go`) do not force batch 4's fast untagged `verify:` to carry `-tags integration` (the `verify-excludes-edited-tagged-test` invariant requires the editing batch's `verify:` to include `-tags integration`, but running the integration tests in a per-round gate would spawn a real gopls — against the repo convention that integration tests run out-of-band). This batch's own `verify: go test -tags integration -run=^$ ./internal/codeintelengine/...` satisfies the invariant by compiling every integration-tagged test in the package while `-run=^$` runs **none** of them — so the gate catches a compile-breaking edit without ever installing or spawning gopls. Full execution stays out-of-band (`-tags integration` on a machine with gopls). It depends on batch 4, which transitively brings in the supervised flip (card 12), the `InFile` engine resolve (batch 2), and the `--in-file` CLI flag (batch 3) these tests exercise.

## Cards

### Card 13: Integration-prove the supervised dispatch flip end to end

- **Context:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/daemonstate.go`
  - `internal/codeintelengine/registry.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver_integration_test.go`
  - `internal/codeintelengine/supervised_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` test to `ensureserver_integration_test.go` (e.g. `TestEnsureServer_Integration_SupervisedDispatch`) skip-gated on `exec.LookPath("gopls")` with `t.Skip(builtins()["go"].InstallHint)`, matching the file's existing tests. It calls `ensureServer(ctx, "go", builtins()["go"], <targetDir>, <worktreeRoot>, 30*time.Second)` and asserts: the returned `connKind` is `connKindSupervised`; a daemon state file exists at `hubgeometry.Layout{WorktreeRoot: worktreeRoot}.CodeintelDaemonStateFile("go")` with an alive PID; and a second `ensureServer` call reuses the same daemon (same PID, stable `Address`) rather than spawning a new one — mirroring `supervised_integration_test.go`'s reuse assertions. Per the `connKindSupervised` teardown rule the test must never `close()`/`kill()` the returned client, but must reap the spawned daemon in `t.Cleanup` (read the state PID and `os.FindProcess(...).Kill()`), exactly as `supervised_integration_test.go` and `supervised_test.go` already do. Update `ensureserver_integration_test.go`'s header comment to note it now also covers `ensureServer`'s supervised dispatch (native is now the fallback path, still directly exercised). Update `supervised_integration_test.go`'s header comment that claims `ensureSupervised` is driven "never through ensureServer's dispatch, which has no live path to it in V1" — that is now false; reword it to reflect that `ensureServer` now dispatches Go to `ensureSupervised` and this file additionally proves the strategy internals directly.
- **Commit:** `test(codeintelengine): integration-prove ensureServer supervised dispatch and reuse`

### Card 14: Integration-prove the `--in-file` resolve end to end

- **Context:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/supervised_test.go`
  - `internal/codeintelengine/registry.go`
  - `internal/codeintelengine/errors.go`
- **Edits:**
  - `internal/codeintelengine/refs_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `//go:build integration` test to `refs_integration_test.go` (e.g. `TestReferences_InFile_Integration`) skip-gated on `exec.LookPath("gopls")` with `t.Skip(builtins()["go"].InstallHint)`, following the existing `TestReferences_Integration` conventions in the same file (the `repoRoot(t)` target-dir helper and the 60s/30s timeouts). Cover the two cases the discussion specifies: (1) **single-match resolve** — pick a known top-level symbol in a real repo file (e.g. `Resolve` in `internal/hubgeometry/hubgeometry.go`, the same symbol `TestReferences_Integration` already uses) and resolve it via `References(ctx, Options{Registry: builtins(), TargetDir: repoRoot(t), WorktreeRoot: t.TempDir(), Lang: "go", Query: Query{InFile: &InFileQuery{File: <abs path to that file>, Name: "Resolve"}}, Timeout: 30*time.Second})` — `TargetDir: repoRoot(t)` for correct indexing but `WorktreeRoot: t.TempDir()` so the now-supervised daemon anchors in an isolated temp dir, never the real repo's `.lyx/codeintel/go/` — asserting no error and that the result includes the declaration site — proving the `documentSymbol`→position→`textDocument/references` path works end to end against real gopls (the `Query.InFile` analogue of the existing `Query.Pos` test); (2) **same-name-in-two-types ambiguity** — write a tiny self-contained Go module to `t.TempDir()` (a `go.mod` plus one `.go` file declaring the same method name, e.g. `Open`, on two distinct types), root the lookup there, and assert the `Query.InFile` resolve of that name returns an error satisfying `errors.Is(err, ErrAmbiguousSymbolSentinel)` — the case that distinguishes exhaustive per-file `documentSymbol` matching from a single hit. **Daemon isolation + reap (both subcases):** because `builtins()`'s Go entry has `HasNativeDaemon: true`, after batch 4's flip **each** subcase routes through `ensureServer`→`ensureSupervised` and spawns a real lyx-owned daemon that `teardownConnection`'s `connKindSupervised` branch deliberately never kills — so each must anchor `WorktreeRoot` at an isolated `t.TempDir()` (case 2 may reuse its own module temp dir or a separate one) and reap the spawned daemon in `t.Cleanup` by reading the state file at `hubgeometry.Layout{WorktreeRoot: <that temp dir>}.CodeintelDaemonStateFile("go")` and `os.FindProcess(state.PID).Kill()`, exactly as card 13's new test and `supervised_integration_test.go` already do. **Also fix the pre-existing leak in the same file:** `TestReferences_Integration`'s "live gopls references" subtest calls `References` with `Registry: builtins()` and **no** `WorktreeRoot` field, so post-flip it anchors a supervised daemon at a relative `.lyx/codeintel/go/` under the test binary's cwd and never reaps it — set that subtest's `WorktreeRoot: t.TempDir()` (keep its `TargetDir: root`) and add the same `t.Cleanup` daemon-reap. Compile-check with `go test -tags integration -run=^$ ./internal/codeintelengine/...` before committing (the same command as this batch's `verify:`); full execution runs out-of-band with `-tags integration` on a machine with gopls.
- **Commit:** `test(codeintelengine): integration-prove --in-file resolve and same-name ambiguity`

## Batch Tests

`verify: go test -tags integration -run=^$ ./internal/codeintelengine/...` compiles the whole engine test binary **with** the `integration` build tag — so both new integration-tagged tests (cards 13 and 14) and every pre-existing `*_integration_test.go` in the package are type-checked against the post-flip production code — while `-run=^$` matches no test name, so nothing executes: no gopls install, no daemon spawn, no network. This is the compile-only gate that satisfies the `verify-excludes-edited-tagged-test` invariant (the `verify:` carries `-tags integration`) without pulling integration execution into the per-round gate, honoring the package's out-of-band integration convention. Actual execution of these tests (a live supervised dispatch + reuse; a live `--in-file` resolve and same-name ambiguity) is run separately with `-tags integration` on a machine with gopls, alongside the pre-existing `supervised_integration_test.go` / `refs_integration_test.go` / `ensureserver_integration_test.go` coverage.
