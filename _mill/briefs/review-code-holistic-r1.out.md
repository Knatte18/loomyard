MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-29
```

## Findings

None.

Verified end-to-end against both batches:

- Card 1: `errWorktreeRootGone` sentinel and `validateToldWorktreeRootLive` in `internal/reedengine/server.go:28-37,252-284` match the shape-then-liveness ordering, message content (both causes named, no rename assertion), and sentinel-wrapping rules exactly as specified. Table test and the unreadable-parent non-sentinel test in `server_test.go` cover all five/six required rows.
- Card 2: the `Geometry{` sweep is exactly the five files the plan predicted (`lock_test.go`, `server_test.go`, `header_test.go`, `contract_integration_test.go`, `mouse_boot_integration_test.go`), confirmed via grep — no sixth file. `newTestEngine` and the inline `TestWithOpLock_PathIsUnderDotLyx` fixture both materialize only `WorktreeRoot`, matching the card's constraint.
- Card 3: both `withOpLock`/`withTryOpLock` in `lock.go:89-98,155-164` call the new validator immediately after `validateToldAnchorPath` and before `os.MkdirAll`, with doc comments updated as required. All six new `server_test.go` cases (vanished, regular-file, standalone-non-existent, standalone-first-run success, hub-first-run success, `errors.Is` match) are present and correctly shaped. `doc.go` and `SANDBOX-REED-SUITE.md` (M24/M25 stray-directory assertions) updated in the same batch.
- Card 4: `watchdogDormantCycle`, the `Dormant` timing field, `watchModeDormant`, `tickerPeriodFor`, `dormantFrom` threading, and `handleWatchOutcome`'s sentinel/recovery branches in `watchloop.go` match the spec precisely — one warning on entry (never mode-already-dormant), one info on recovery, restore to the remembered prior mode, no `state.Failed/Succeeded/Deferred` calls on the sentinel path. `watchloop_test.go` adds the six-constant pin, the `tickerPeriodFor` cadence pin, poll- and signal-mode dormancy entry, recovery-to-prior-mode, and the non-sentinel-failure regression guard, all using millisecond-scale timings with mutex-guarded log capture. `doc.go`'s single geometry-lifetime bullet is correctly shared and extended by both batches rather than duplicated.
- Cross-batch contract: `reapplyLayout` (`reapply.go`) calls `withTryOpLock`, so `errWorktreeRootGone` propagates through its `err` return exactly as batch 2 assumes; confirmed the non-acquired/no-error deferral path is distinct from the validation-failure path.
- `standalonegeom.ReedGeometry` sets `WorktreeRoot` to the standalone target and `standalonestate.Derive` creates nothing on disk, consistent with the shared decision that gating is on `WorktreeRoot`, never `AnchorPath`.
- No out-of-plan files, no duplicated helpers, no Test Tier Purity violations (no untagged-file spawns, no ≥1s sleeps) found.

## Verdict

APPROVE
Implementation matches the plan's cards and shared decisions precisely across both batches with no gaps found.
MILL_REVIEW_END
