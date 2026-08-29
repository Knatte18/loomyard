# Batch: reconcile-gate-and-reap-log

```yaml
task: 'reed: pane reap isn''t applied consistently across up/add''s mutating paths'
batch: 'reconcile-gate-and-reap-log'
number: 1
cards: 3
verify: go test ./internal/reedengine/ -run 'TestPlanReconcile|TestReconcileLocked'
depends-on: []
```

## Batch Scope

This batch delivers the whole of `reconcile.go`'s share of the fix: the untracked-reap authorization gains an alive-header disjunct, `planReconcile`'s return shape splits the two kill reasons apart, and `reconcileLocked` gains the `logger.Info` line that makes a destructive reap observable.
It is one batch because all three changes land in one function pair (`planReconcile` / `reconcileLocked`) in one file, and the return-shape split exists only to serve the log line — separating them would mean writing an intermediate shape nothing consumes.

The external interface batch 2 consumes is behavioural, not structural: `reconcileLocked`'s signature is unchanged, but after this batch a reconcile fires the untracked reap whenever the header is alive, which is what makes batch 2's reap-before-allocate chokepoint do anything.

This batch deliberately leaves `internal/reedcli`'s smoke tests failing.
`TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` and `TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile` both encode the pre-fix premise and are rewritten in batch 4;
neither is in this batch's `verify:` scope nor in `pipeline.done_gate` (which carries no `smoke` tag), so this is a known, planned interval rather than a regression.

Batch-local decision beyond `## Shared Decisions`: the two killed-id lists are logged under distinct keys rather than merged into one, so the log line satisfies the-reap-logs-what-it-destroys' distinguishability requirement without a caller having to cross-reference the struct.

## Cards

### Card 1: Split planReconcile's return into a named reconcilePlan struct

- **Context:**
  - `internal/reedengine/apply.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/reconcile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Introduce an unexported struct type `reconcilePlan` in `internal/reedengine/reconcile.go` with exactly four fields: `clearedGUIDs []string`, `deadPanesToKill []string`, `untrackedPanesToKill []string`, `keptDeadPane string`.
  Change `planReconcile`'s signature from `func planReconcile(strands []Strand, live []LivePane, headerPaneID string) (clearedGUIDs []string, panesToKill []string, keptDeadPane string)` to `func planReconcile(strands []Strand, live []LivePane, headerPaneID string) reconcilePlan`.
  Inside `planReconcile`, the existing dead-pane kill loop appends to `deadPanesToKill` and the existing `if anyBoundPresent` untracked-reap block appends to `untrackedPanesToKill`;
  the shared local `killSet` map keeps its current role of preventing a pane from being scheduled twice and must keep being written by both loops.
  This card is a pure refactor: no pane that is killed today may stop being killed, and no pane that is spared today may start being killed.
  In `reconcileLocked`, replace the tuple destructuring with a single `plan := planReconcile(...)`, then run the `kill-pane` loop over `plan.deadPanesToKill` first and `plan.untrackedPanesToKill` second — that order reproduces today's single merged slice exactly, and the existing `killed` accumulator and its error-wrapping (`fmt.Errorf("kill pane %s: %w", id, err)`) stay as they are.
  Update the binding-clear loop to read `plan.clearedGUIDs`.
  Update `planReconcile`'s own doc comment to describe the struct and to state why the two kill reasons are carried apart (the reap log line distinguishes them).
  Correct `reconcile.go`'s file-header comment in the same card: its opening sentence says `planReconcile` "decides which strand pane bindings to clear and which dead panes to kill", which this card falsifies by making untracked kills a first-class field of the returned struct rather than an unnamed half of one merged slice.
  This card owns that correction because it is the card that makes the sentence wrong;
  leaving it for the batch 3 doc sweep would park a false file header at the top of the file for three batches, and neither of card 12's grep terms reaches it, so nothing downstream would catch it either.
  In `internal/reedengine/reconcile_test.go`, rewrite `TestPlanReconcile`'s table so each case carries `wantDeadPanesToKill` and `wantUntrackedPanesToKill` in place of the single `wantPanesToKill` field, and update the assertion body to compare all three slices plus `keptDeadPane` against the struct's fields via the existing `equalStringSlices` helper.
  Every existing case keeps its current expected outcome — assign each existing `wantPanesToKill` entry to whichever of the two new fields matches the reason that case exercises (`NonSoleDeadPaneScheduledForKillAndBindingCleared`, `AllDeadKeepsFirstPaneAndKillsTheRest` and `DeadHeaderExemptWhileDeadStrandPaneStillKilled` are dead-pane kills; `UntrackedAlivePaneKilledWhileBoundContentPresent` and `HeaderPaneNeverReapedAsUntrackedWhileStrandBound` are untracked kills).
  Do not change any case's behavioural expectation in this card.
- **Commit:** `refactor(reedengine): return planReconcile's kill schedule as a named struct`

### Card 2: Authorize the untracked reap on an alive header as well as a bound present pane

- **Context:**
  - `internal/reedengine/apply.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/reconcile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `planReconcile`, compute a new local `headerAlive` next to the existing `anyBoundPresent`: it is true only when `headerPaneID != ""` and some entry of `live` has `ID == headerPaneID` and `Dead == false`.
  Change the untracked-reap block's condition from `if anyBoundPresent {` to `if anyBoundPresent || headerAlive {`.
  Leave `boundPaneIDs`, `anyBoundPresent`, and `exemptPaneIDs` computed exactly as they are today — `headerAlive` is a third, separate local, never folded into any of them, so the header stays exempt from being killed by presence (a header corpse is still never killed) while only an *alive* header authorizes killing anything else.
  Rewrite the untracked-reap block's comment to state the new rule and why aliveness rather than presence: the reap fires from `AddStrand`/`UpdateStrand` once batch 2's chokepoint lands, and those paths never call `ensureHeaderPaneLocked`, so a dead-but-present header must not be allowed to authorize reaping the session's only alive pane.
  Do not change the dead-pane kill loop, the `keptDeadPane` rule, or the header's exemption from the dead-pane kill.
  In `internal/reedengine/reconcile_test.go`, two existing cases need opposite treatment;
  do not conflate them.
  `UntrackedPanesUntouchedWhenNothingBound` sets no `headerPaneID`, so `headerAlive` is false and its "nothing reaped" expectation is correct both before and after this change — it is in fact the absent-header case this card would otherwise add.
  Keep its expectation exactly as it is, rewrite only its comment (which today justifies the outcome by "reed has nothing to lay out", not by the absent anchor), and rename it if its name reads as a general no-strand rule rather than the absent-header one.
  `HeaderAloneNeverMakesAnyBoundPresentTrue` is the one whose expectation is now wrong: its header is alive, so the reap fires and its untracked pane is killed.
  Rewrite both its expectation and its comment, keeping the point its comment exists to make — `anyBoundPresent` is still derived from `boundPaneIDs` alone and is still false here;
  the kill comes from the new `headerAlive` disjunct beside it, never from folding the header into `boundPaneIDs`.
  Add table cases covering: an alive header with zero strands and one untracked alive pane, where that pane is killed as an *untracked* kill and the header is not;
  an alive header with zero strands and several untracked panes (an old header pane plus an orphaned strand pane, M22's shape), where all of them are killed and the current header is spared;
  a header present but `Dead: true` with no strand bound and one alive untracked pane, where nothing is reaped;
  a non-empty `headerPaneID` naming no entry in `live` at all, with no strand bound and one alive untracked pane, where nothing is reaped — this is `headerAlive`'s third way of being false, distinct from the empty-id and present-but-dead cases above, and it is reachable on the `add` path once an operator kills the header pane outright, since no verb but `up`/`resume` rebuilds it;
  and a dead header alongside a strand bound to a present pane, where the reap fires anyway via `anyBoundPresent` and the header corpse is still spared.
  The alive-header-alongside-a-bound-strand shape needs no new case: `HeaderPaneNeverReapedAsUntrackedWhileStrandBound` already covers it exactly (a strand on `%1`, an alive `%header`, and a foreign `%7` that is reaped), and card 1 preserves it unchanged.
  Confirm it still passes rather than adding a duplicate of it.
  Keep every existing dead-pane case's expectation unchanged — this card must not alter `keptDeadPane` behaviour or the dead-header exemption.
- **Commit:** `fix(reedengine): let an alive header authorize the untracked pane reap`

### Card 3: Log the reap, and add the logger-capture test helper it needs

- **Context:**
  - `internal/reedengine/spawn.go`
  - `internal/logger/logger.go`
  - `internal/logger/sink.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/lifecycle_test.go`
- **Edits:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/reconcile_test.go`
- **Creates:**
  - `internal/reedengine/logcapture_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/reconcile.go`, add `"github.com/Knatte18/loomyard/internal/logger"` to the import block and emit exactly one `logger.Info` call at the end of `reconcileLocked`, only when at least one pane was killed.
  The call carries the `"socket"`/`"session"` key shape `loadOrInitStateLocked` already uses in `spawn.go` (`e.Socket()` and `e.SessionName()` as values) plus two further keys separating the dead-pane kills from the untracked kills, so the untracked half is distinguishable in the log without cross-referencing anything.
  Those two keys must carry the ids actually destroyed — the accumulated locals described below — never `plan.deadPanesToKill` / `plan.untrackedPanesToKill`.
  The two sources agree on the success path and diverge on the partial-kill path, where the scheduled lists would name panes the failing `kill-pane` never reached;
  a log claiming to have destroyed a pane that is still alive is worse than no log, and this is a destruction record, not a record of intent.
  One line per `reconcileLocked` call, never one per pane.
  A reconcile that killed nothing must emit nothing.
  The line must fire on the partial-kill error path too, not only on the success path.
  `reconcileLocked` returns `killed, fmt.Errorf("kill pane %s: %w", ...)` from inside the loop, so a failure on the third of four panes returns with two panes already destroyed — and every caller discards the returned `killed` slice on error (`reconcileApplyPersistLocked`'s `return nil, fmt.Errorf("reconcile: %w", err)`, `Resume`'s identical site, and the new call site card 6 adds).
  A success-only log would therefore leave exactly the silent destruction this log exists to prevent, in the one case where the operator most needs the trace.
  Implement it so both paths are covered: accumulate the killed ids into two locals as the loops progress — one for the dead-pane kills, one for the untracked kills — and emit the single `Info` line from a `defer` that fires whenever either local is non-empty.
  A `defer` is preferred over duplicating the call before each `return` so the two paths cannot drift apart later.
  The line still carries the same key shape either way, and the error itself still propagates unchanged — logging is additive here and must not swallow or alter the returned error.
  A reconcile that killed nothing still logs nothing, on either path.
  Add a comment above it recording why this is `Info` and not `Debug` (CONSTRAINTS.md's Live-Substrate Spawn Observability lifecycle-vs-probe split: this is a lifecycle teardown) and why the reap needs a trace at all (card 2 makes it fire on the zero-strand precondition, so it goes from near-dormant to routine, and it destroys panes an operator may have created).
  Create `internal/reedengine/logcapture_test.go` — an untagged test file in package `reedengine` — holding a single helper `captureLogOutput(t *testing.T) *bytes.Buffer`.
  It must call both `logger.SetOutput(&buf)` and `logger.SetVerbosity(1)`, and register one `t.Cleanup` restoring `logger.SetVerbosity(0)` and `logger.SetOutput(os.Stderr)`.
  Both calls are required: `internal/logger`'s stderr half defaults to the Warn threshold and its durable half is disabled outright under `testing.Testing()`, so `SetOutput` alone captures nothing and an Info-asserting test would fail inexplicably.
  `os.Stderr` is the restore target because `internal/logger` exports no getter for its current writer and `os.Stderr` is that package's own declared default.
  Give the file a header comment saying exactly that.
  In `internal/reedengine/reconcile_test.go`, add one focused test — name it `TestReconcileLocked_LogsTheUntrackedPanesItReaps` — driving `reconcileLocked` through the `newTestEngine` fixture with an `e.tmux.execHook` that answers `kill-pane` successfully.
  The `TestReconcileLocked_` prefix is required, not stylistic: this batch's `verify:` filters on `-run 'TestPlanReconcile|TestReconcileLocked'`, and `go test -run` exits 0 when a pattern matches nothing, so a name outside that prefix would leave the new log line with no gate at all.
  `execHook` is the field declared in `internal/reedengine/overlay.go` that replaces the real subprocess exec for both the run and capture paths;
  `internal/reedengine/lifecycle_test.go` shows the switch-on-`args[0]` shape to follow.
  Assert three things: a reconcile which kills untracked panes emits an `Info` line naming those pane ids;
  a reconcile which kills nothing emits no output at all;
  and a reconcile whose `kill-pane` fails partway — scripted by an `execHook` that succeeds for the first pane and returns an error for the second — still emits a line naming the pane it did destroy, while returning the error to its caller.
  That third case must assert both halves: the destroyed pane's id **is** present in the logged output, and the failed pane's id is **absent** from it.
  Asserting only the first half cannot tell the accumulated actually-killed locals apart from the scheduled `plan.*ToKill` lists, since both contain the first pane — the absence half is what pins the distinction the requirement above turns on.
  Without this whole case the `defer` could be removed and the suite would stay green.
  Assert on the pane ids' presence, not on the message's exact wording — the point is that the destructive path is observable.
- **Commit:** `feat(reedengine): log the panes a reconcile reaps`

## Batch Tests

`verify: go test ./internal/reedengine/ -run 'TestPlanReconcile|TestReconcileLocked'` covers `internal/reedengine/reconcile_test.go`'s `TestPlanReconcile` (the pure gate and kill-schedule table, including every new case card 2 adds), `TestReconcileLocked_NoDeadPanes_ClearsGoneBindingsWithoutTouchingTmux` (the existing composing-half test, which must keep passing across card 1's signature change), and card 3's new `TestReconcileLocked_*` log-line test.

The scope is deliberately narrower than the package: every symbol this batch changes is private to `reconcile.go` and has exactly one caller (`reconcileLocked`), so no other test file in the package can be affected except by a compile error, which `verify` surfaces anyway because `go test` builds the whole package before running the `-run` filter.
The overview's module-wide `verify:` (`go build ./... && go vet ...`) then catches anything outside the package.
