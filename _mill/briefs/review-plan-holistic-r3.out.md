MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: plan/
date: 2026-08-29
```

## Findings

### [BLOCKING:scope] Card 3's log test is unnamed; verify filter can miss it
**Location:** batch 1 / card 3
**Issue:** `Requirements:` says only "add one focused test" while batch 1's `verify:` is `-run 'TestPlanReconcile|TestReconcileLocked'`; a name outside that prefix leaves the new `logger.Info` line with no gate at all (`go test -run` with zero matches exits 0). Cards 7, 15 and 16 all name their new tests explicitly; only the `TestReconcileLocked_*` form in the batch's `Batch Tests` prose implies the constraint.
**Fix:** Name the test in card 3's `Requirements:` with a `TestReconcileLocked_` prefix, as cards 7/15/16 do.

### [BLOCKING:scope] Card 12's Context omits a file its own sweep hits
**Location:** batch 3 / card 12
**Issue:** The second sweep (`grep -rni "untracked reap\|bound present pane\|reap.*does not fire" internal/reedengine/*.go ...`) hits `internal/reedengine/reconcile_test.go:136` ("the dead-pane kill loop, not only the untracked reap, spares it" — a live, legitimate survivor card 2 is told to preserve), but `reconcile_test.go` is absent from card 12's 20-entry `Context:`. The card must disposition every hit and may only read listed files.
**Fix:** Add `internal/reedengine/reconcile_test.go` to card 12's `Context:`.

### [NIT:consistency] Card 2's fifth new case duplicates an existing one
**Location:** batch 1 / card 2
**Issue:** "an alive header alongside a bound strand, where both stay exempt and a third foreign pane is still reaped" is exactly `HeaderPaneNeverReapedAsUntrackedWhileStrandBound` (`reconcile_test.go:114-122`: strand on `%1`, alive `%header`, foreign `%7` killed), which card 1 explicitly preserves unchanged.
**Fix:** Drop that bullet from the add-list, or state it as "confirm the existing case still holds" rather than "add".

### [NIT:consistency] Card 7 names the wrong model test for argv capture
**Location:** batch 2 / card 7
**Issue:** The card cites `TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure` as showing "capture the split argv"; that test (`lifecycle_test.go:385`) captures nothing — `TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath` (`lifecycle_test.go:334`) is the one holding the `splitArgs` capture.
**Fix:** Cite `TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath` for the argv-capture half.

### [NIT:scope] Card 17 misses the third adoption-named site on the same line
**Location:** batch 4 / card 17
**Issue:** The card names the `adopted` local and the `"adopted initial pane (control)"` label, but not the `--name "adopted"` argument on `smoke_panecwd_test.go:60` — a live strand name, not a comment, that card 12's `grep -rn "adopt" internal/reedcli/*.go` would then report as a missed rewrite.
**Fix:** Name the `--name` value alongside the local in card 17's `Requirements:`.

### [NIT:scope] Card 12's survivor list omits spawn.go's generation-probe call
**Location:** batch 3 / card 12
**Issue:** After cards 4 and 6, the one remaining "adopt" hit in `spawn.go` is `e.adoptPaneGenerationLocked(st)` (`spawn.go:205`) inside `loadOrInitStateLocked`; the enumerated survivor file list ("`generation.go`, `server.go`, `generation_test.go`, `attach_test.go`, `reapply_test.go`, `contract_integration_test.go`, and `spawn_test.go`") does not include `spawn.go`, so the hit reads as unjustified against the list even though the card's governing rule spares it.
**Fix:** Add `spawn.go`'s `adoptPaneGenerationLocked` call site to the survivor enumeration.

## Verdict

REQUEST_CHANGES
Two scope gaps: an unnamed gated test and a sweep hit outside its card's Context.
MILL_REVIEW_END
