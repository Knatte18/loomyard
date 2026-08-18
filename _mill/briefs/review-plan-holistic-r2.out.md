MILL_REVIEW_BEGIN
# Review: the standalone CLI path — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Batch 5 mirrors batch-4 output without a declared dependency
**Location:** batch 05 (perchcli-standalone-entry), cards 20, 21, 24 vs. batch 04's `depends-on: [1, 2]` on batch 05.
**Issue:** Card 20's `Context:` lists `internal/burlercli/wiring.go` (created by batch 4's card 13) to mirror; card 21 requires `internal/perchcli/cli.go`'s rewrite to follow "the same body shape card14 specifies for burler" (batch 4's card 14 product); card 24's `Context:` lists `internal/burlercli/wiring_test.go` (batch 4's card 18). Batch 05's own DAG entry only depends on `[1, 2]`, and batch 04's scope note states burler "lands before perch's in reviewer attention even though the two run in parallel" — i.e. the plan explicitly expects no ordering guarantee between 4 and 5, yet three of batch 5's cards need batch 4's files to already exist on disk to be non-cold-start.
**Fix:** Either add `4` to batch 5's `depends-on`, or drop the cross-batch file mirroring from cards 20/21/24's `Context:`/Requirements and have them derive structure from webstercli's wiring files only (already correctly listed and pre-existing).

### [BLOCKING:scope] `shuttleengine.LoadConfig`'s declaring file missing from Context
**Location:** batch 04 card 13 (`internal/burlercli/wiring.go`), batch 05 card 20 (`internal/perchcli/wiring.go`).
**Issue:** Both cards' Requirements call `shuttleengine.LoadConfig(anchorPath, "shuttle")` / `shuttleengine.LoadConfig(stateDir, "shuttle")` twice each, but `Context:` lists only `internal/shuttleengine/run.go` (declares `NewRunner`), not `internal/shuttleengine/config.go` (declares `LoadConfig` and `Config`). `reedengine.LoadConfig` and `burlerengine.LoadConfig`'s files are correctly listed for comparison — this one function's home file is the odd one out in both cards.
**Fix:** Add `internal/shuttleengine/config.go` to both cards' `Context:`. (Partially mitigated in practice since `internal/webstercli/wiring.go`, already in both cards' Context, shows the identical call verbatim — but the criterion is mechanical and the gap is real.)

### [BLOCKING:consistency] Batch 3 doesn't apply its own env-redirect decision to pre-existing tests
**Location:** overview "every test reaching wireStandalone redirects the state root" Shared Decision (applies to batches 3, 4, 5) vs. batch 03 card 12.
**Issue:** The decision requires "any test — new or pre-existing — whose call path reaches `wireStandalone`" to set both `XDG_STATE_HOME` and `LOCALAPPDATA`. Batches 4 and 5 each carry a card (16, 23) that goes back and fixes their package's own pre-existing standalone-reaching tests this way. Batch 3 has no equivalent: `internal/webstercli/cli_integration_test.go`'s two pre-existing tests (`TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate`, `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged`) already reach `wireStandalone` today and only redirect `XDG_STATE_HOME`, and card 12's requirements only add a new test, never touching these two.
**Fix:** Add a requirement to card 12 (or a new card) redirecting `LOCALAPPDATA` too in those two pre-existing tests, matching cards 16/23's treatment, or narrow the Shared Decision's "pre-existing" language to state it does not bind already-shipped tests outside a card's own edit list.

## Verdict

REQUEST_CHANGES
Batch 5's cross-batch file dependency on unshipped batch-4 output is the load-bearing issue; the other two are smaller completeness gaps.
MILL_REVIEW_END
