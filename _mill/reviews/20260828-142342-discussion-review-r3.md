MILL_REVIEW_BEGIN
# Review: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
duration_s: 158.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-28
```

## Findings

### [NIT:scope] Affected-test enumeration stops at reedengine
**Demoted-from:** BLOCKING
**Section:** Scope → In / Testing **Issue:** The method used to find artefacts whose premise the change falsifies covered `internal/reedengine` comments and one smoke test, but two `internal/reedcli` smoke tests are in neither In nor Out: `smoke_panecwd_test.go` builds its whole two-case table on adoption (`"The FIRST strand adopts the session's initial pane…"`, case `"adopted initial pane (control)"`, lines 28-30/72) — after dropping adoption both cases become splits and the control/split contrast the test exists for is gone, silently — and `smoke_lifecycle_test.go:390` (`TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile`) asserts in comment that the first strand "lands on the session's other (pre-header) pane", which the new gate reaps on `up`. **Fix:** State the enumeration method used to sweep adoption/initial-pane premises across all of `internal/reedcli` and `internal/reedengine`, and give those two tests an explicit in/out disposition.

### [BLOCKING:design] Kills now precede persistence on a failing launch
**Section:** Decisions → reap-before-allocate-is-a-chokepoint-in-launchStrandLocked **Issue:** "Safe in every caller" addresses only the launching strand; it does not address that `AddStrand`/`UpdateStrand` reach `SaveState` only *after* `launchStrandLocked` returns nil (`strand.go:300-312`, `343-349`), so a split/send-keys failure now returns with tmux panes already killed and other strands' bindings cleared in memory but never written — a destructive-then-unpersisted window that does not exist today, where reconcile always runs in the `reconcileApplyPersistLocked` tail immediately ahead of `SaveState`. **Fix:** State the disposition — accept it as self-healing on the next verb, or require the plan to persist the reap's binding clears before the split.

### [NIT:consistency] "single line to change" vs. return-shape change
**Section:** Technical context → the reap itself **Issue:** `planReconcile`'s untracked reap is called "the single line to change", while the-reap-logs-what-it-destroys requires that same function's return shape to carry untracked kills apart from dead-pane kills. **Fix:** Drop or qualify the "single line" phrasing so a plan writer does not treat the gate flip as the whole edit to `reconcile.go`.

## Verdict

REQUEST_CHANGES
Affected-test enumeration incomplete; the new pre-launch kill's failure window has no stated disposition.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
