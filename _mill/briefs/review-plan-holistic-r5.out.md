MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus-class model (harness reports claude-opus-5); self-identification is best-effort and unverifiable from inside the session
reviewed_file: plan/
date: 2026-08-29
```

## Findings

### [BLOCKING:design] Card 13's empty-layout coverage claim is false post-reap
**Location:** batch 4 / card 13
**Issue:** Card 13 orders the implementer to write, in the rewritten doc comment, that `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` still pins "`up` with zero placeable strands must never emit a zero-cell layout string" — but `applyLayoutLockedOpts` returns at `len(live) < 2` (apply.go:211) *before* the `anyPlacedStrand` guard (apply.go:214), and after the reap both `up` calls in this test reach apply with exactly one pane, so the `anyPlacedStrand` branch is never entered by this test again.
**Fix:** State the real post-fix disposition instead — the smoke-tier proof of the zero-cell hazard is no longer reachable from this fixture (unit coverage survives in `apply_test.go`'s `TestApplyLayoutLockedOpts_GuardSkipsReturnZeroResult`) — rather than instructing a comment that asserts coverage the test no longer has.

### [BLOCKING:consistency] Card 12's survivor list contradicts prose cards 4 and 15 mandate
**Location:** batch 3 / card 12, vs batch 2 / card 4 and batch 4 / card 15
**Issue:** Card 4 requires `planPaneTarget`'s replacement doc comment to state the function "never adopts an existing pane" plus the R4-F5/M16 rationale, and card 15 requires a doc comment saying "under adoption the pane pid provably survives"; both land in files card 12 greps, yet card 12 lists only `e.adoptPaneGenerationLocked` as `spawn.go`'s survivor and no `smoke_lifecycle_test.go` survivor at all, while ruling that a hit is legitimate "only when 'adopt' means something other than pane adoption" and that anything else "is a missed rewrite".
**Fix:** Extend card 12's survivor enumeration (and its disposition rule) to cover the historical-rationale prose cards 4 and 15 explicitly commission, so the closing sweep does not report deliberate text as a gap.

### [BLOCKING:scope] No card rewrites spawn_test.go's kept adopt-vs-split comment
**Location:** batch 2 / card 5 (`internal/reedengine/spawn_test.go:40-45`)
**Issue:** Card 5 keeps `SoleCorpseUnbound_NeverAdopted_SplitOffTheCorpse` "with their current expectations" and speaks only to its *name*, leaving its comment asserting a live decision that no longer exists — "so the next add must split, not adopt, even though no strand holds a binding" — which card 12's `grep -rn "adopt"` reaches with no card owning the rewrite, and card 12 reports rather than fixes.
**Fix:** Name that comment (and `SeveralUntrackedAlivePanes_...`'s "Adoption picked it" narrative) explicitly in card 5's `Requirements:` alongside the four converted cases.

### [BLOCKING:scope] Card 2's Context omits internal/reedengine/strand.go
**Location:** batch 1 / card 2
**Issue:** Card 2's `Requirements:` names `AddStrand`/`UpdateStrand` and asserts "those paths never call `ensureHeaderPaneLocked`", a claim the implementer must encode into the rewritten reap-gate comment, but both functions live in `strand.go`, which appears in neither `Context:` (apply.go, lifecycle.go) nor `Edits:` (reconcile.go, reconcile_test.go).
**Fix:** Add `internal/reedengine/strand.go` to card 2's `Context:`.

### [NIT:consistency] Card 16 leaves the scrubbed-state guard's new vacuity undispositioned
**Location:** batch 4 / card 16, re `TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp`
**Issue:** Card 16 argues the test passes unchanged because its `pane_top == 0` assertion is "satisfied" by a sole full-height header — true, but that assertion becomes vacuous once the recovering `up` leaves one pane, so half the R4-F4 guard stops guarding while the plan presents the vacuity as a reason it is fine.
**Fix:** Say so in card 16 and require the confirmed-out note to record that only the exit-0 half of that guard remains load-bearing after the reap.

## Verdict

REQUEST_CHANGES
One false coverage claim, one self-contradicting sweep, and two scope gaps.
MILL_REVIEW_END
