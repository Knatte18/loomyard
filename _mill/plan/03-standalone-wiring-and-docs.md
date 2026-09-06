# Batch: standalone-wiring-and-docs

```yaml
task: 'webster standalone mode: run refuses to start Master; logs write untracked into target repo'
batch: 'standalone-wiring-and-docs'
number: 3
cards: 5
verify: go test ./internal/webstercli/... ./internal/burlercli/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...
depends-on: [1, 2]
```

## Batch Scope

This batch is where both defects actually close: the two `wireStandalone` functions switch to `shuttleengine.NewDetachedRunner` (F16) and redirect the durable trace sink to `standalonegeom.LogsDir(stateDir)` (F22), and the documentation and guard tests that keep both fixes true land beside them.
It consumes all three interfaces batches 1 and 2 produce and adds no new exported surface of its own.
The two call-site cards are separated by package rather than merged, because each carries its own doc-comment rewrite and its own package's wiring tests.

Batch-local decisions:

- Card 7 writes only the source-level pre-run guard.
  The discussion's other `cmd/lyx` testing item — `stencilSeedTarget` reporting `ok == false` for a non-hub location — is already pinned by the shipped `TestStencilSeedTarget_PlainRepoHasNoHub`, so card 7 references that test rather than duplicating it.
- Card 8's whole substance is lifting the sink suppression inside an existing integration test.
  Without that lift its new assertion would pass vacuously, which is precisely why the shipped test passes today while F22 is live.

## Cards

### Card 5: switch webstercli's standalone wiring to the detached runner and redirect the trace sink

- **Context:**
  - `internal/webstercli/cli.go`
  - `internal/shuttleengine/run.go`
  - `internal/standalonegeom/logsdir.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/logger/sink.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/webstercli/wiring.go`
  - `internal/webstercli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/webstercli/wiring.go`, inside `wireStandalone`, replace the `shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)` call with `shuttleengine.NewDetachedRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)`.
  `wireHub`'s own `shuttleengine.NewRunner` call is not touched.
  In the same function, immediately after the `standalonestate.Derive(target)` call returns successfully and before any other statement in the function, call `logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)`.
  Placement is the requirement, not a preference: the sink is armed lazily on the first Info-or-above record, so the redirect only binds if it runs before anything in this function can log.
  Add the `github.com/Knatte18/loomyard/internal/logger` import;
  `standalonegeom` and `standalonestate` are already imported.
  `wireHub` must not gain a sink call of any kind.
  `wireStandalone`'s doc comment enumerates what the function does, in order;
  fold both new steps into that enumeration at their real positions — the redirect right after the `Derive` step, and the detached-constructor choice where the runner is described — and state in it that the redirect is what keeps a standalone invocation from writing trace files into the operator's repository.
  In `internal/webstercli/wiring_test.go`, add a test that drives `wireStandalone` and then reaches a public entry point on the constructed runner, asserting no told-path error surfaces.
  This is F16's direct regression test and it fails against today's source;
  a test that only checks the runner is non-nil proves nothing, because `NewRunner`/`NewDetachedRunner` hold their verdict on `toldErr` and surface it only when a verb runs.
  Add a second test asserting the durable sink directory was set to `standalonegeom.LogsDir(stateDir)` for the derived state directory, redirecting `XDG_STATE_HOME` and `LOCALAPPDATA` with `t.Setenv` and resolving the expected `stateDir` through the file's existing `hash8For` helper convention, exactly as the shipped standalone cases already do.
  Observe the directory by forcing a write and checking the filesystem, never by reading `internal/logger`'s own state: `sinkDirOverride` is unexported, `internal/logger` exposes no accessor for it, and `internal/webstercli` is a different package, so there is nothing to read from here.
  The mechanism is that a non-empty override bypasses the `testing.Testing()` sink suppression in `ensureDurableSink` — that suppression is reached only on the empty-override branch — so after `wireStandalone` returns, one `logger.Info` call from the test arms the sink at whatever directory the override names, with no `LYX_TRACE` redirect needed.
  Assert a `trace-*.log` file then exists under `standalonegeom.LogsDir(stateDir)`.
  Add a third test asserting `wireHub` leaves the durable sink directory untouched, so a later refactor cannot quietly route hub mode through the standalone path.
  Observe that the same way: call `logger.SetDurableSinkDir(sentinelDir)` for a `t.TempDir()` sentinel before calling `wireHub`, emit one `logger.Info` afterwards, and assert the trace file landed in `sentinelDir`.
  A `wireHub` that had overwritten the override would have put the file somewhere else, and the setter's own reset of `sinkOnce` is what makes each such test arm a fresh sink rather than reusing an earlier test's.
  Every test in this file that causes the override to be set — whether by reaching `wireStandalone` or by calling `logger.SetDurableSinkDir` directly — must register `t.Cleanup(func() { logger.SetDurableSinkDir("") })`, per the overview's `sink-override-is-process-global` decision;
  without it the override leaks into every later test in the same binary and defeats the `testing.Testing()` sink suppression for all of them.
  The new `wireHub`-sentinel test is explicitly in that set: it never reaches `wireStandalone`, but it calls the process-global setter itself, so it carries the same cleanup.
  Among the shipped tests that reach `wireStandalone` and therefore also need it: `TestWire_ModeStandaloneSelectsStandaloneMode`, `TestWire_PlanDirResolution`'s standalone subtests, `TestWire_StandaloneRootsResolveToTarget`, and `TestWire_MatcherNeverNilOpenerNilOnlyInStandalone`'s standalone subtest.
- **Commit:** `fix(webstercli): wire standalone through the detached runner and redirect the trace sink`

### Card 6: switch burlercli's standalone wiring to the detached runner and redirect the trace sink

- **Context:**
  - `internal/burlercli/cli.go`
  - `internal/shuttleengine/run.go`
  - `internal/standalonegeom/logsdir.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/logger/sink.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/burlercli/wiring.go`
  - `internal/burlercli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the identical two changes card 5 makes, to `internal/burlercli/wiring.go`'s own `wireStandalone`: replace its `shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)` call with `shuttleengine.NewDetachedRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)`, and call `logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)` immediately after `standalonestate.Derive(target)` returns and before any other statement in the function.
  Add the `github.com/Knatte18/loomyard/internal/logger` import.
  `wireHub`'s `shuttleengine.NewRunner` call is not touched and `wireHub` must not gain a sink call.
  This function's doc comment carries a "Two asymmetries are worth calling out" paragraph that exists to stop a later reader simplifying the stencils handling away;
  keep that paragraph intact and fold the two new steps into the enumeration above it, the same way card 5 does for the webster copy.
  In `internal/burlercli/wiring_test.go`, add the same three assertions card 5 adds, adapted to this package's own fixtures: a `wireStandalone` runner that reaches a public entry point without a told-path error, a sink directory set to `standalonegeom.LogsDir(stateDir)`, and a `wireHub` that leaves the sink directory untouched.
  Observe the sink directory exactly the way card 5 specifies — force a write and check the filesystem, never read `internal/logger`'s own state, which is unexported and has no accessor.
  For the standalone case that means one `logger.Info` call after `wireStandalone` returns (the non-empty override bypasses the `testing.Testing()` suppression, so no `LYX_TRACE` redirect is needed) followed by an assertion that a `trace-*.log` file exists under `standalonegeom.LogsDir(stateDir)`;
  for the hub case it means a `logger.SetDurableSinkDir(sentinelDir)` sentinel set before `wireHub`, one `logger.Info` after, and an assertion that the file landed in the sentinel directory.
  Register `t.Cleanup(func() { logger.SetDurableSinkDir("") })` in every test in this file that causes the override to be set — whether by reaching `wireStandalone` or by calling `logger.SetDurableSinkDir` directly.
  That set includes the shipped `TestWireStandalone_NeverReadsLoc`, `TestWire_ModeStandaloneSelectsStandaloneMode`, `TestWire_StandalonePinnedValues` and `TestWire_StencilsDirFlag`, and it also includes the new `wireHub`-sentinel test, which never reaches `wireStandalone` but calls the process-global setter itself.
- **Commit:** `fix(burlercli): wire standalone through the detached runner and redirect the trace sink`

### Card 7: add the source-level guard on root pre-run logging order

- **Context:**
  - `cmd/lyx/spawnobservability_test.go`
  - `cmd/lyx/main.go`
  - `cmd/lyx/stencilseed.go`
  - `cmd/lyx/stencilseed_integration_test.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/prerunlogging_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `cmd/lyx/prerunlogging_test.go` in package `main`, untagged, containing one test that parses `cmd/lyx/main.go` and asserts the root command's `PersistentPreRunE` function body contains no `logger.Info` or `logger.Warn` call ahead of its `seedStencils(cmd)` call.
  Those two are the whole set to guard against: `internal/logger` exports `Debug`, `Info` and `Warn` and no `Error` at all, and `Debug` is below the Info-or-above threshold that arms the sink, so a `Debug` call in the pre-run is harmless and must not fail this guard.
  Parse with `go/parser` and walk the resulting AST rather than scanning for substrings, following the precedent `cmd/lyx/spawnobservability_test.go` establishes in this same package and for the same reason it gives: a doc-comment mention is not a call, and a substring guard would demand allowlist entries for files that call nothing.
  The file header comment must record why this guard exists and why it is a source-level guard rather than a behavioural one.
  The two standalone `wireStandalone` redirects added by cards 5 and 6 run in a module `PersistentPreRunE`, which cobra runs after root's because `cobra.EnableTraverseRunHooks` is true, so the redirect binds only if nothing in root's pre-run has already armed the sink by emitting an Info-or-above record.
  Record that a behavioural test of the same property cannot be written: `seedStencils` returns before resolving anything under `testing.Testing()`, so a test asserting "root pre-run emits no Info+ record in standalone" would pass through the test guard rather than through the standalone gate, and would keep passing after the dependency it claims to pin had already broken.
  Record also that the other half of the same dependency — `stencilSeedTarget` reporting `ok == false` for a non-hub location, which is what keeps `seedStencils` from reaching its own log calls in standalone — is already pinned by `TestStencilSeedTarget_PlainRepoHasNoHub` in `cmd/lyx/stencilseed_integration_test.go`, and is deliberately not duplicated here.
  Add no production change in this card.
- **Commit:** `test(lyx): guard root pre-run against logging ahead of the standalone sink redirect`

### Card 8: extend the standalone integration test to observe the redirected sink

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/standalonegeom/logsdir.go`
  - `internal/logger/sink.go`
- **Edits:**
  - `internal/webstercli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the shipped `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged` in `internal/webstercli/cli_integration_test.go` rather than adding a parallel test beside it: it already redirects `XDG_STATE_HOME` and `LOCALAPPDATA`, already asserts the target directory gains no entries, and already drives the `status` verb for exactly the reason this extension needs.
  The extension's whole substance is lifting the durable sink's suppression, with `t.Setenv("LYX_TRACE", "1")` before the `RunCLIIn` call.
  Without that lift the sink is suppressed entirely under `testing.Testing()` and the new assertion would pass for the same wrong reason the shipped emptiness assertion passes today while F22 is live.
  The extension must add its own `stateDir, _, err := standalonestate.Derive(target)` call, mirroring the sibling `TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate` in the same file.
  `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged` does not compute `stateDir` today — it redirects `XDG_STATE_HOME` and `LOCALAPPDATA` but never calls `Derive` — so the value has to be introduced before it can be asserted against.
  After the invocation, assert positively that a `trace-*.log` file exists under `standalonegeom.LogsDir(stateDir)` for that derived `stateDir`, and that the file's first line carries the target repository as its `worktree_root=` field.
  That positive assertion is the load-bearing one — it is what fails against today's source, where nothing is written there at all — while the shipped "target gained no entries" assertion is what pins the defect's actual symptom.
  Keep the cleanliness assertion scoped to `status`, an invocation that reaches wiring.
  Do not add an assertion that an invocation failing before `standalonestate.Derive` leaves the target clean.
  Attribute that residual to the shipped `lyx` binary rather than to this test's own call path, and say so in the doc comment in exactly those terms: `logger.NotifyExit` is called only from `cmd/lyx/main.go` — mentioned, not read — and this test drives `RunCLIIn` directly, so `NotifyExit` never executes here at all.
  What the doc comment records is therefore why the assertion is scoped rather than a mechanism the test itself exercises: under the real binary a pre-redirect non-zero exit force-arms the sink and the cwd fallback writes one trace file into the target, over a bounded window — cobra flag-parsing failures, a root pre-run failure, and a `Derive` failure — so a broader claim would be false of the shipped path even though this test could not observe it either way.
  Register `t.Cleanup(func() { logger.SetDurableSinkDir("") })`, since `LYX_TRACE=1` plus a live override arms a real sink for the rest of the binary otherwise.
  Add the `logger`, `standalonegeom` and `filepath` imports the extension needs;
  `standalonestate`, `os`, `strings` and `filepath` handling already exist in the file.
- **Commit:** `test(webstercli): pin the redirected standalone trace sink end to end`

### Card 9: record the detached-anchor rule in CONSTRAINTS.md

- **Context:**
  - `internal/shuttleengine/run.go`
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Append one bullet to `CONSTRAINTS.md`'s existing **Told-Geometry Invariant** section's bullet list, under that heading and creating no new top-level invariant.
  The clause states that a `shuttleengine` runner whose anchor is deliberately outside its worktree root is constructed only through `shuttleengine.NewDetachedRunner`, only from a standalone CLI's own wiring, and that `NewRunner`'s containment assertion is never relaxed to accommodate it.
  The Told-Geometry Invariant is the correct host because it already carries the "told the absolute paths it operates on, derives none" rule the detached constructor is a special case of, already lists `shuttleengine` among its bound packages, and already names `hubgeom`/`standalonegeom` as the only `Geometry`-struct constructors.
  Do not append to the Shuttle Provider-Seam Invariant instead: that entry is exclusively about provider specifics living under `claudeengine` and carries no anchor-or-worktree geometry content at all, so a geometry clause there would be a category error.
  Follow the file's own bullet style and the project's semantic-line-break rule — one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, never hard-wrapped at a fixed column.
- **Commit:** `docs(constraints): record the detached-anchor construction rule`

## Batch Tests

`verify: go test ./internal/webstercli/... ./internal/burlercli/... ./cmd/lyx/... && go test -tags integration ./internal/webstercli/...` covers every package this batch edits, in both of the build-tag worlds the batch touches.

The first invocation runs the untagged suites: `internal/webstercli` and `internal/burlercli` for cards 5 and 6's wiring assertions and for the shipped mode-truth-table tests that must keep passing, and `cmd/lyx` for card 7's new source guard alongside the package's shipped guard tests — `constructoranchoring_test.go` pins `logger.LogsDir(l)` for hub shapes at two call sites, and this batch must not move either.

The second, separately-chained invocation carries `-tags integration` and is what runs card 8's extended test, which lives behind that tag.
It is appended as its own `go test` call rather than by adding the tag to the first invocation, so the untagged run's package set stays exactly what it is.

Card 9 has no runnable surface of its own;
it is covered by the batch's build passing and by the overview's module-wide `go build ./...`.
