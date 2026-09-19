# Batch: tagged reap tests

```yaml
task: 'reed: per-hub daemon reaps orphaned sessions'
batch: 'tagged reap tests'
number: 4
cards: 5
verify: go test -tags integration -race -run 'TestWatchdogReap|TestWatchdogIntegration' ./internal/reedcli/
depends-on: [3]
```

## Batch Scope

This batch delivers every assertion that can only be made against a live tmux server and a real process tree: the end-to-end orphan reap, the never-entered orphan, the healthy-sibling non-interference guard, the empty-shell degradation, the process half of the reap, and loop liveness during a reap under `-race`.
It is one batch because all six cases share one fixture shape — a hub built through `internal/hubforge`, one or two booted sessions, a deleted worktree directory, and `runWatchdogLoop` driven in-process with compressed timings — and because several of them are assertions about the same single reap.
Card 15 declares that shared shape as three named helpers — `newReapFixture`, `compressedReapTiming` and `orphanWorktree` — and cards 16 through 19 call those identifiers rather than each building a fixture of its own.
It depends on batch 3: every case drives the wired loop.

All new cases carry the `integration` build tag, matching the file they extend and the existing daemon test tier, rather than being split across `integration` and `smoke`. That keeps one `-tags` value on this batch's verify command.

Every top-level test function this batch adds is named with the `TestWatchdogReap` prefix, without exception. This is a batch-wide rule, restated on each card that adds one, because this batch's verify is `-run`-filtered: a case named outside the filter's two alternatives compiles clean and then never executes, so a mis-named test reads as a passing batch while asserting nothing.

Batch-local decision beyond `## Shared Decisions`: every test drives `runWatchdogLoop` directly, in-process, through its `watchdogTiming` parameter. None starts a real `lyx reed watchdog` process — that would re-exec the test binary, which CONSTRAINTS.md's Live-Substrate Spawn Observability rule forbids outright and `suppressWatchdogSpawn` exists to prevent.

## Cards

### Card 15: the tagged reap fixture

- **Context:**
  - `internal/reedcli/watchdog_integration_test.go`
  - `internal/reedcli/smoke_teardown_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/testmain_test.go`
  - `internal/reedcli/watchdog.go`
  - `internal/hubforge/hub.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/watchdogreap_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedcli/watchdogreap_integration_test.go`, opening with the `//go:build integration` constraint line and a header comment stating that it holds the orphan-reap tier's live assertions, that it is a separate file from the existing watchdog integration file so the reap's fixture and helpers stay together, and that every case drives the loop in-process with compressed timings rather than spawning a daemon.

  Add exactly three new helpers, named and shaped as follows, so cards 16 through 19 call one agreed identifier each rather than inventing their own:

  `func newReapFixture(t *testing.T, pairNames ...string) reapFixture` builds a hub through `hubforge.NewHub(t, ".")` (never hand-assembled, per the hubforge Fabric-Fixture Invariant), adds one pair per name in `pairNames`, boots a session for the prime worktree and for each pair, and returns them. Its result type is a small struct declared in this same file:

  ```go
  type reapFixture struct {
  	hub       *hubforge.Hub
  	engines   []*reedengine.Engine
  	worktrees []string
  	tmuxPath  string
  }
  ```

  `engines` and `worktrees` are index-aligned, prime first, so a case can orphan `worktrees[1]` and still assert against `engines[0]`. `tmuxPath` is taken from the first engine.

  `func compressedReapTiming() watchdogTiming` returns a `watchdogTiming` whose `DiscoveryCycle` is on the order of tens of milliseconds, with `IdleCycles` and `OrphanGoneCycles` both at their production value of 3. The cadence is what compresses, never the confirmation count, so every case still proves the three-consecutive-cycle rule rather than bypassing it. No case waits on the production 5s cycle.

  `func orphanWorktree(t *testing.T, worktreeRoot string)` removes the booted worktree's directory from disk while its tmux session stays live on the hub socket — the exact state the reap exists to clean up.

  All three call `t.Helper()` as their first statement, matching the existing helpers in this package's tagged tier.

  Reuse rather than duplicate what the package already has: `watchdogIntegrationEngine(t, worktreeRoot)` for engine construction and `waitForCondition(t, timeout, cond)` for polling both already live in `internal/reedcli/watchdog_integration_test.go`, in the same package and the same build-tag tier, so they are directly callable from this new file. `newReapFixture` builds its engines through `watchdogIntegrationEngine`; do not write a second engine-construction helper.

  The package's `TestMain` already arms the hermetic git test environment, so this file adds none.
- **Commit:** `test(reedcli): add the tagged orphan-reap fixture`

### Card 16: end-to-end orphan reap and sibling non-interference

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/watchdogreap_integration_test.go`
  - `internal/reedengine/overlay.go`
- **Edits:**
  - `internal/reedcli/watchdogreap_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the end-to-end case, named so it matches the batch verify's `TestWatchdogReap` prefix. Boot two worktree sessions on one hub, delete one worktree's directory, drive `runWatchdogLoop` with the compressed timing, and assert that the orphan's session is gone from `reedengine.ListSessions` against the hub socket, and that its pane children have exited.

  Assert the sibling in the same test, not a separate one: the healthy session on the same hub socket survives the orphan's reap untouched and is still listed afterwards. Keeping both assertions in one test is deliberate — a broken exact-match kill target takes out the prefix-sharing sibling, and that must fail loudly in the same run that proves the reap works, rather than in a test someone could skip.

  Assert the timing rule too: the session is still live after fewer than `OrphanGoneCycles` affirmative cycles have elapsed, and gone after enough have. A reap firing on the first observation must fail this case.

  Cancel the loop's context and wait for `runWatchdogLoop` to return before the test ends, so the deferred `WaitGroup` wait is exercised on every run rather than left to chance.
- **Commit:** `test(reedcli): assert the end-to-end orphan reap and sibling non-interference`

### Card 17: never-entered orphan

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/watchdogreap_integration_test.go`
- **Edits:**
  - `internal/reedcli/watchdogreap_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the never-entered case, naming its top-level test function with the `TestWatchdogReap` prefix so this batch's own `-run 'TestWatchdogReap|TestWatchdogIntegration'` filter selects it — a name outside those two prefixes compiles and then silently never runs. The worktree directory is already gone before `runWatchdogLoop`'s first cycle runs, so the daemon never entered the session and it was never in `known`. Assert it is still reaped.

  This is the case `enterSession` can structurally never reach — its `resolveWatchedSession` call spawns git against a missing directory and fails, so the name is logged at Debug and skipped every cycle forever. It is the operator-visible shape after a crash-and-restart, and it is the reason the reap reads the live session-name list rather than the daemon's own `known` map. An implementation that iterates `known` passes card 16 and fails this one, which is exactly what this case is for.
- **Commit:** `test(reedcli): assert a never-entered orphan is still reaped`

### Card 18: empty shell degrades rather than refuses

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/watchdogreap_integration_test.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/proctree_linux.go`
- **Edits:**
  - `internal/reedcli/watchdogreap_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the empty-shell case, naming its top-level test function with the `TestWatchdogReap` prefix so this batch's own `-run 'TestWatchdogReap|TestWatchdogIntegration'` filter selects it — a name outside those two prefixes compiles and then silently never runs. Call `runWatchdogLoop` directly with an empty `shellPath` against a live hub socket and an orphaned worktree, and assert the session is killed and its pane root pids have exited.

  This is where the discussion's degrade-never-refuse decision is proven at the live tier. Together with the untagged assertion that the flag pre-flight has no shell parameter to reject, both halves are covered: nothing rejects an empty shell, and an empty shell still reaps.

  Assert the pane **root** pids specifically, not their descendants. The reason is portability of the assertion, not this platform's behaviour: on Linux — where this suite runs — `descendantClosurePIDs` reads no `Engine` field at all and walks `/proc` identically whatever the shell is, so an empty shell changes nothing here and the descendants would in fact die. It is the Windows body that degrades to returning the roots unchanged when its probe cannot spawn. Asserting only the roots keeps this case asserting the guarantee the empty-shell configuration makes on *every* platform, rather than one that happens to hold on the one it executes on today. The descendant assertion belongs to card 19's fully-configured reap, which makes no such platform carve-out.

  This case runs the loop in-process and starts no `lyx reed watchdog` process of any kind.
- **Commit:** `test(reedcli): assert an empty --shell still reaps`

### Card 19: the process half and loop liveness under `-race`

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/watchdogreap_integration_test.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/proctree_linux.go`
- **Edits:**
  - `internal/reedcli/watchdogreap_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two cases that only this tier can make, naming both top-level test functions with the `TestWatchdogReap` prefix so this batch's own `-run 'TestWatchdogReap|TestWatchdogIntegration'` filter selects them — a name outside those two prefixes compiles and then silently never runs.

  The process half: after an end-to-end reap, assert the orphan's pane **child** pids are confirmed exited, not merely signalled — a descendant of a pane child is dead after the reap returns. This is the only tier that exercises the descendant closure and the wait-then-force-kill sequence at all, and it is what proves the closure was computed before the kill: a closure computed after `kill-session` collapses to the pane roots alone and leaves the descendant alive, so this case fails on a reordered implementation while the untagged call-order assertion still passes.

  Loop liveness during a reap: with a reap in flight, assert the discovery loop still ticks and still enters a newly-appeared session, that cancelling the daemon's context returns promptly rather than after the reap's full graceful-plus-force budget, and that the reaped name leaves the in-flight set on a later tick without its goroutine having blocked. This is the regression guard for running the reap off-loop; an inline implementation fails it.

  Both cases run under the `-race` flag the batch verify carries. The in-flight set, the gone-counter map and the `known` map are all written only on the loop goroutine, with the reap goroutines' sole cross-goroutine act being a non-blocking channel send, so a correct implementation has nothing for the race detector to find. An implementation that deletes from the in-flight map inside the reap goroutine trips it here.
- **Commit:** `test(reedcli): assert the reap's process half and loop liveness under -race`

## Batch Tests

`verify: go test -tags integration -race -run 'TestWatchdogReap|TestWatchdogIntegration' ./internal/reedcli/` runs both the new reap cases and the existing watchdog integration cases, in the `integration` tier, with the race detector armed.

The `-tags integration` value is required twice over: it is the tag on the file this batch creates, and the tag on `internal/reedcli/watchdog_integration_test.go`, which the run re-executes so batch 3's mechanical call-site update is proven against a live server rather than only compiled.

The `-run` pattern is what keeps this from being the package's whole tagged suite: `internal/reedcli` also carries a large `smoke`-tagged set and other `integration` cases whose fixtures boot full lifecycles, none of which this batch touches.
Both alternatives in the pattern are load-bearing — dropping the second would stop re-running the existing daemon assertions the reap pass could have regressed.

`-race` is scoped to this batch rather than the others because this is the only batch that exercises the loop's goroutine interaction at all;
the pure seams in batches 1 and 2 are single-goroutine by construction.
