MILL_REVIEW_BEGIN
# Review: lyx mux remove errors when it empties the last session — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-15
```

## Findings

### [BLOCKING] Contradicts passing smoke test that pins corpse behavior
**Location:** Card 3 + Shared Decision "docs corrected... Windows are unverified"
**Issue:** `internal/muxcli/smoke_lifecycle_test.go`'s `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` is a committed, passing smoke test whose comment states the exact "corpse-pane" claim Card 3 declares false, and whose assertions (`remove` of the sole strand returns 0, line 178; a subsequent `add` yields a *live* second strand, line 199) can only hold if the session SURVIVES on the production psmux binary — `add` calls `requireSessionLocked` and never re-boots (add.go). The plan corrects the strand.go/doc.go copies of the claim but never mentions this test, so it leaves a second, still-authoritative encoding of the "false" claim in the tree and, if the plan's premise holds, the test breaks (second add hits the no-session error).
**Fix:** Bring `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` (and its comment) into scope; reconcile the environment-conditional behavior and update it in the same commit.

### [BLOCKING] "psmux last-pane behavior unverified" is factually wrong
**Location:** Shared Decision "Windows + crash-link are unverified notes"
**Issue:** The decision asserts psmux last-pane behavior is unverified, but the smoke test above verifies (on psmux) that killing the sole strand's pane leaves the session usable — the OPPOSITE of Card 3's rewritten doc.go/strand.go claim ("killing a session's true last pane destroys the session"). The rewrite thus states tmux behavior as universal and mislabels a verified psmux fact as unknown. The bug's exit-1 repro (Decision 1) evidently comes from a different binary/version, which the plan never distinguishes.
**Fix:** Qualify the corrected assumption per binary (tmux destroys; psmux corpses, per the smoke test) rather than asserting one universal behavior.

### [BLOCKING] verify scope cannot catch the muxcli contradiction
**Location:** 00-overview batch index / 01 Batch Tests (`go test -tags integration ./internal/muxengine/`)
**Issue:** verify runs only the muxengine package; the conflicting `smoke`-tagged test lives in `internal/muxcli`, so the plan's own verify would never surface the regression/contradiction the fix introduces there.
**Fix:** Extend verify to also exercise the affected muxcli smoke test, or document its exclusion with reason.

### [BLOCKING] Card 5 regression may never exercise the swallow path
**Location:** Card 5
**Issue:** The integration test runs against the configured psmux, where (per the smoke test) removing the sole pane leaves the session alive — so `RemoveStrand` returns via the normal path and the new `applyErr`/`removalEmptiedSession` swallow branch (Card 2) is never triggered. Its assertions (nil error, zero persisted strands) pass without covering the fix. Card 4 tests only the pure helper, so the Card 2 wiring is untested end-to-end on the box it runs on.
**Fix:** Force the emptied-session path in the regression (or state it reproduces only on tmux) so the swallow branch is actually covered.

### [BLOCKING] Card 3 Context omits overlay.go
**Location:** Card 3 (Context: none)
**Issue:** Card 3's Requirements reference `hasSession` and `listPanes` and their exit-1→`(false,nil)` mapping — functions defined in `overlay.go`, which is neither in Context nor Edits; per the Context-completeness rule this is a cold-start gap.
**Fix:** Add `internal/muxengine/overlay.go` to Card 3's Context.

## Verdict

REQUEST_CHANGES
Plan's core premise conflicts with a passing muxcli smoke test it neither scopes nor reconciles.
MILL_REVIEW_END
