# Batch: drift-status

```yaml
task: "Speed up internal/warp integration tests"
batch: "drift-status"
number: 3
cards: 4
verify: go test -tags integration -run TestPairInSync ./internal/warp/ && go test -tags integration -run TestStatus ./internal/warp/
depends-on: [1]
```

## Batch Scope

Consolidates the drift (`PairInSync`) and status read-paths: folds the two read-only
"healthy pair" tests into mutating siblings as pre-condition checks (groups C and D), and
folds two further scenarios (wrong-target junction, `_codeguide` report-only) onto sibling
fixtures rather than deleting them — each carries unique production-branch coverage. Net: 4
fewer fixture builds across `drift_test.go` and `status_test.go`, no coverage lost. The two
files are independent but grouped here as the "inspection" subsystem. Batch-local: folded
assertions run as added sequential steps on the sibling's fixture (no `t.Parallel` between
the shared steps).

## Cards

### Card 7: Group C — fold PairInSync_InSync into BrokenJunction pre-check

- **Context:** none
- **Edits:**
  - `internal/warp/drift_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestPairInSync_InSync` only asserts a freshly set-up pair reports
  `ok=true`, `reason=""` — exactly the precondition `TestPairInSync_BrokenJunction` relies
  on before it removes the junction. Port that in-sync assertion to the start of
  `TestPairInSync_BrokenJunction` (immediately after setup + `Add`, before the
  `fslink.Remove` of the junction). Then delete `TestPairInSync_InSync`. Keep
  `TestPairInSync_BranchDivergence`.
- **Commit:** `test(warp): fold PairInSync_InSync into BrokenJunction pre-check`

### Card 8: Fold PairInSync_JunctionPointsElsewhere into BrokenJunction

- **Context:** none
- **Edits:**
  - `internal/warp/drift_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestPairInSync_JunctionPointsElsewhere` is the **only** test of
  PairInSync's wrong-target branch (junction link exists but points elsewhere — distinct
  from `BrokenJunction`'s missing-junction branch), so it must be folded, not deleted. Add
  a third sequential step to `TestPairInSync_BrokenJunction` (after card 7's in-sync
  pre-check and the missing-junction check): repoint the host `_lyx` junction to a decoy
  target (port the `fslink.Remove` + `fslink.CreateDirLink` to a decoy logic from
  `TestPairInSync_JunctionPointsElsewhere`) and assert `PairInSync` returns `ok=false` with
  a reason referencing the junction/elsewhere. Then delete the standalone
  `TestPairInSync_JunctionPointsElsewhere`. This keeps the wrong-target coverage while still
  removing its separate fixture build (it rides BrokenJunction's fixture).
- **Commit:** `test(warp): fold PairInSync_JunctionPointsElsewhere into BrokenJunction`

### Card 9: Group D — fold Status_PairedViewFields into InSyncVsDrifted pre-check

- **Context:** none
- **Edits:**
  - `internal/warp/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestStatus_PairedViewFields` is read-only — it asserts a healthy pair's
  field population (HostWorktree, WeftWorktree, HostBranch, WeftBranch populated; in-sync).
  `TestStatus_InSyncVsDrifted` starts from the same healthy state before it drifts the weft
  branch. Port the field-population assertions into `TestStatus_InSyncVsDrifted` BEFORE its
  `git checkout -b drifted` mutation. Then delete `TestStatus_PairedViewFields`.
- **Commit:** `test(warp): fold Status_PairedViewFields into InSyncVsDrifted pre-check`

### Card 10: Fold Status_CodeguidePollutionReportOnly into LyxPollutionDetected

- **Context:** none
- **Edits:**
  - `internal/warp/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** `TestStatus_CodeguidePollutionReportOnly` uniquely covers the
  `ReportOnly=true` / empty-`Remedy` branch for `_codeguide`, which `TestStatus_LyxPollutionDetected`
  (asserting the opposite: `ReportOnly=false`, non-empty `Remedy`) does not — both behaviors
  are documented in the status_test.go file header, so it must be folded, not deleted. Add
  the `_codeguide` report-only assertions as a second sequential phase of
  `TestStatus_LyxPollutionDetected` on the same `setupStatusFixture`: after the `_lyx`
  pollution assertions, create the `_codeguide` pollution (port the `os.MkdirAll` +
  `os.WriteFile` + `git add -f` + commit logic) and assert the `_codeguide` pollution entry
  has `ReportOnly=true` and empty `Remedy`. Then delete the standalone
  `TestStatus_CodeguidePollutionReportOnly`. The two pollution checks run sequentially on
  one fixture (no `t.Parallel` between them). Keep `TestStatus_JunctionHealth` and
  `TestStatus_InSyncVsDrifted`.
- **Commit:** `test(warp): fold Status_CodeguidePollutionReportOnly into LyxPollutionDetected`

## Batch Tests

`verify` runs `TestPairInSync` (the merged drift tests) and `TestStatus` (the consolidated
status tests), chained with `&&`. Confirms the pre-condition fold-ins observe the healthy
state correctly and that the two deletions left both files compiling and green.
