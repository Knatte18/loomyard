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
deletes two low-value scenarios. Net: 4 fewer fixture builds across `drift_test.go` and
`status_test.go`, no coverage lost. The two files are independent but grouped here as the
"inspection" subsystem. Batch-local: read-only assertions move to run BEFORE the sibling's
mutation, so they observe the same healthy state.

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

### Card 8: Delete low-value PairInSync_JunctionPointsElsewhere

- **Context:**
  - `internal/warp/status_test.go`
- **Edits:**
  - `internal/warp/drift_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete `TestPairInSync_JunctionPointsElsewhere` — the wrong-target
  junction case. Junction failure is already covered by `TestPairInSync_BrokenJunction`
  (now carrying the in-sync pre-check from card 7) and by `TestStatus_JunctionHealth`
  (read-then-break-then-read). No production path loses its only coverage.
- **Commit:** `test(warp): drop redundant PairInSync_JunctionPointsElsewhere`

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

### Card 10: Delete low-value Status_CodeguidePollutionReportOnly

- **Context:**
  - `internal/warp/status_test.go`
- **Edits:**
  - `internal/warp/status_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Delete `TestStatus_CodeguidePollutionReportOnly`. The pollution-detection
  mechanism is already covered by `TestStatus_LyxPollutionDetected`; this test only checks a
  transitional report-only flag for `_codeguide`. Keep `TestStatus_LyxPollutionDetected`,
  `TestStatus_JunctionHealth`, and `TestStatus_InSyncVsDrifted`.
- **Commit:** `test(warp): drop redundant Status_CodeguidePollutionReportOnly`

## Batch Tests

`verify` runs `TestPairInSync` (the merged drift tests) and `TestStatus` (the consolidated
status tests), chained with `&&`. Confirms the pre-condition fold-ins observe the healthy
state correctly and that the two deletions left both files compiling and green.
