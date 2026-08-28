MILL_REVIEW_BEGIN
# Review: reed: watchdog daemon — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-28
```

## Prior non-blocking item — resolved

Card 20's keepalive-survival test now checks the actual envelope shape: `internal/reedcli/header_test.go:127`
asserts `strings.Contains(out, `"ok":true`) || strings.Contains(out, `"error"`)`, matching `output.Ok`/`output.Err`'s
real key set (`internal/output/output.go:17-46`), rather than the prior round's "status"/"error" substring check
that could not detect the leak. No new escalation needed; this is fixed, not merely unchanged.

## Findings

None. Every batch's cards are realised exactly as specified, cross-batch contracts hold, and every Shared
Decision is applied consistently:

- `internal/shell.Touch` (card 1-2) is implemented and tested on both dialects; `resizeHookCommand` (card 4)
  composes it correctly and its exact-string test (`watchdog_test.go`) matches the plan's literal expectation.
- The `Watchdog` config key (card 3) lands in `Config`, both embedded templates identically, and
  `ConfigTemplate`'s godoc count is updated to five keys.
- `liveBoxLocked`'s widened signature (card 6), `applyLayoutLockedOpts`/`applyOpts`/`applyResult` (card 7),
  `withTryOpLock` (card 8), and `reapplyLayout`/`hookInstalledLocked`/`ReapplyResult` (card 9) all match their
  specified signatures, ordering (probe after `listPanes`, before apply), and non-fatal/non-persisting contracts.
- `pinGeometryOptionsLocked`'s hook lifecycle (card 10) installs the plain replacing form (no `-a`), unsets on
  off/invalid, and removes the signal file correctly; the boot-path loud failure (card 11) is wired in
  `ensureServerAndSessionLocked` with `newTestEngine`'s fixture repaired exactly as directed.
- The pure state machine (`watchState`, card 15) and the driver (`Engine.Watch`/`watchLoop`, card 16) implement
  the debounce/coalesce/retry-cap/mode-promotion/never-demote rules precisely, including the poll-mode "never
  touch state" and signal-mode-only `Failed`/`Deferred`/`Succeeded` calls.
- The header tail (card 19) sequences print → `logger.SetOutput(io.Discard)` → `headerWatch` → `headerPark` →
  unconditional `return nil`, exactly as specified, and `logger.SetOutput`'s actual implementation
  (`internal/logger/logger.go:408-413`) confirms the stderr-only-rebind claim in the surrounding comments.
- `doc.go` (card 21), `manifest/roadmap.md` (card 22), and `SANDBOX-REED-SUITE.md`'s M26 scenario plus the M25/M26
  session-log lines (card 23) are all present and accurate.
- `watchdog_integration_test.go` (card 24) carries the correct build tag, reuses the existing pty harness and
  `contract_integration_test.go`'s `waitUntil` with no duplicate harness, and covers all eight required scenarios
  (grow, shrink, burst-coalesce, degraded-path, survives-induced-failure, focus-never-stolen, no-self-trigger,
  hook-probe-matches-live-tmux).
- Test coverage is exhaustive and hermetic throughout batches 1-4 (`watchdog_test.go`, `apply_test.go`,
  `windowsize_test.go`, `lock_test.go`, `reapply_test.go`, `lifecycle_test.go`, `watchloop_test.go`,
  `header_test.go`), with no untagged spawns or ≥1s sleeps observed.
- No out-of-plan files, no duplicated helpers across batches (`reapply_test.go` and `watchloop_test.go` share
  fixture shape by direct reuse, not reimplementation), and no CONSTRAINTS.md violations found (Told-Geometry,
  Durable-vs-Ephemeral State, Shell Mechanics Seam, Config Strictness, Test Tier Purity, Live-Substrate Spawn
  Observability, CLI/Cobra all hold).

## Verdict

APPROVE
Implementation matches the plan precisely across all four batches; the prior round's sole flagged issue is fixed.
MILL_REVIEW_END
