# Plan: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
task: 'reed: pane reap isn''t applied consistently across up/add''s mutating paths'
slug: 'reed-pane-reap-consistency'
approved: false
started: '20260828-144143'
parent: 'main'
root: ""
verify: go build ./... && go vet ./internal/reedengine/ ./internal/reedcli/
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: reconcile-gate-and-reap-log
    file: 01-reconcile-gate-and-reap-log.md
    depends-on: []
    verify: go test ./internal/reedengine/ -run 'TestPlanReconcile|TestReconcileLocked'
  - number: 2
    name: drop-adoption-and-reap-chokepoint
    file: 02-drop-adoption-and-reap-chokepoint.md
    depends-on: [1]
    verify: go test ./internal/reedengine/ -run 'TestPlanPaneTarget|TestLaunchStrandLocked|TestPlanReconcile|TestReconcileLocked|TestValidateSplitCreatedNewPane'
  - number: 3
    name: doc-surface-sweep
    file: 03-doc-surface-sweep.md
    depends-on: [2, 4]
    verify: go test ./internal/reedengine/
  - number: 4
    name: smoke-regressions
    file: 04-smoke-regressions.md
    depends-on: [2]
    verify: go test -tags smoke -timeout 20m ./internal/reedcli/ -run 'TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable|TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile|TestSmokeForeignPaneIsReapedNotAdoptedByAdd|TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader|TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp|TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd|TestSmokeRemoveLastStrandThenAddRunsTheNewCommand'
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: planReconcile returns a named struct, not a widening tuple

- **Decision:** `planReconcile`'s return shape becomes a single unexported struct value `reconcilePlan` with the fields `clearedGUIDs []string`, `deadPanesToKill []string`, `untrackedPanesToKill []string`, and `keptDeadPane string`, replacing today's `(clearedGUIDs, panesToKill, keptDeadPane)` triple.
  `reconcileLocked` kills `deadPanesToKill` first and `untrackedPanesToKill` second, preserving today's append order exactly.
- **Rationale:** discussion.md's the-reap-logs-what-it-destroys requires the untracked kills to be distinguishable from the dead-pane kills and leaves the mechanism to mill-plan.
  A fourth positional return would make the call site's meaning depend on argument order at a point where two of the four are same-typed `[]string` — a named struct makes a mis-wired field a compile-visible mistake rather than a silent swap.
  It also gives the reap-log line and the unit tests one thing to name.
- **Applies to:** all batches

### Decision: planPaneTarget collapses to a single split-target return

- **Decision:** with adoption deleted, `planPaneTarget`'s signature becomes `planPaneTarget(live []LivePane, headerPaneID string) (splitTargetID string, err error)`.
  The `adoptID` return is removed rather than retained as a permanently-empty value, and the `strands` parameter is dropped along with it.
- **Rationale:** discussion.md's drop-pane-adoption-entirely explicitly leaves this choice to mill-plan.
  A retained always-empty return is a seam that reads as "adoption may come back", which is the half-updated state discussion.md's Scope section names as the hazard;
  there is exactly one call site (`launchStrandLocked`), so call-site stability buys nothing.
  `strands` goes for the same reason and by the same test: its only use in the function is the `anyBound` loop that gates adoption, so once adoption is gone the parameter is read nowhere.
  Go compiles an unused parameter without complaint, which is exactly what makes a vestigial one worth removing deliberately — a split-target planner that still asks for the strand table reads as though the strand bindings influence the choice, and they no longer do.
  The split-target rules are a pure function of the live pane set and the header id.
- **Applies to:** all batches

### Decision: the header anchor is aliveness, never presence

- **Decision:** the new reap-gate disjunct is `headerAlive`, meaning `headerPaneID != ""` AND that id appears in `live` AND that entry's `Dead` is false.
  `anyBoundPresent` keeps being computed from real strand bindings alone;
  the new disjunct sits beside it and is never folded into `boundPaneIDs` or `exemptPaneIDs`.
- **Rationale:** discussion.md's reap-gate-accepts-an-alive-header-as-survival-anchor, including its round-2 BLOCKING finding: the chokepoint makes this gate fire from `add`/`update`, which never call `ensureHeaderPaneLocked`, so a corpse header must never authorize destroying the session's only alive pane.
  Folding the header into `boundPaneIDs` would additionally break `exemptPaneIDs`' documented separation of concerns, which `reconcile_test.go`'s `HeaderAloneNeverMakesAnyBoundPresentTrue` case exists to pin.
- **Applies to:** all batches

### Decision: no SaveState inside launchStrandLocked

- **Decision:** the reap-before-allocate chokepoint does not persist state.
  The destructive-then-unpersisted window on a failing launch is an accepted consequence, documented in `spawn.go`'s comment, not closed.
- **Rationale:** discussion.md's reap-before-allocate-is-a-chokepoint-in-launchStrandLocked states this as an explicit instruction to mill-plan and gives the reason: persisting inside the helper would write the half-added strand record on the `add` path, turning a clean failure into a phantom strand `Resume` would later try to launch.
- **Applies to:** batch 2

### Decision: this is a bugfix — no roadmap move, no CONSTRAINTS entry

- **Decision:** no change to `manifest/roadmap.md`, `CONSTRAINTS.md`, or `docs/overview.md`.
  The tightened rule is documented in `internal/reedengine/doc.go` and the two sandbox scenario specs.
- **Rationale:** discussion.md's no-new-CONSTRAINTS-entry, and CLAUDE.md's rule that the roadmap moves only on completing or adding a planned item.
  "Every pane in a reed session is either the header or a bound strand's pane" is a module-internal invariant enforced inside one package, not a cross-cutting structural form other modules must take.
- **Applies to:** all batches

### Decision: untagged tests stay pure; all real-tmux work stays behind the smoke tag

- **Decision:** every unit test added by batches 1 and 2 uses the existing `newTestEngine` fixture plus the `e.tmux.execHook` fake and performs no `exec.Command`, no `hubforge.NewHub`, and no `time.Sleep` at or above one second.
  The two new real-tmux regressions live in `internal/reedcli/smoke_lifecycle_test.go` behind `//go:build smoke`.
- **Rationale:** CONSTRAINTS.md's Test Tier Purity Invariant.
  Both packages already carry a `testmain_test.go`, so no new test package is introduced and the Hermetic Git Test Environment Invariant needs no new work.
- **Applies to:** all batches

### Decision: the reap log line is Info, one line per reconcile, with reed's existing key shape

- **Decision:** `reconcileLocked` emits exactly one `logger.Info` call per invocation that killed at least one pane, carrying `"socket"` and `"session"` keys — matching `loadOrInitStateLocked`'s existing `logger.Warn` shape in `spawn.go` — plus the two killed-id lists as separate keys so the untracked half is distinguishable in the log itself.
  A reconcile that killed nothing emits nothing.
- **Rationale:** discussion.md's the-reap-logs-what-it-destroys.
  `Info` rather than `Debug` per CONSTRAINTS.md's Live-Substrate Spawn Observability lifecycle-vs-probe split;
  one line rather than one per pane because a per-pane line is noise inside `Resume`'s per-strand launch loop.
- **Applies to:** batch 1

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `internal/reedcli/smoke_lifecycle_test.go`
- `internal/reedcli/smoke_panecwd_test.go`
- `internal/reedcli/smoke_teardown_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/logcapture_test.go`
- `internal/reedengine/reconcile.go`
- `internal/reedengine/reconcile_test.go`
- `internal/reedengine/spawn.go`
- `internal/reedengine/spawn_test.go`
- `internal/reedengine/state.go`
- `internal/reedengine/strand.go`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
