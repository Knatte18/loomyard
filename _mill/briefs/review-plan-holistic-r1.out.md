I have read all plan files and the referenced source files. Here is my review.

MILL_REVIEW_BEGIN
# Review: Add Effort to shuttle's run Spec — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-06
```

## Findings

### [NIT] Batch 1 scope omits card 2→3 intermediate non-compile
**Location:** Batch effort / Cards 2–3
**Issue:** Card 2 adds an `effort` param to `buildLaunchCmd` but does not edit `claudeengine.go` (Context-only), so `Prepare`'s call site `buildLaunchCmd(bin, promptPath, settingsPath, sessionID, spec.Model, spec.Interactive)` has wrong arity and the `claudeengine` package fails to compile until card 3 threads `spec.Effort`; unlike batch 2, batch 1's scope note never flags this.
**Fix:** Add a one-line note to batch 1's scope that the `claudeengine` call site is fixed in card 3 and the batch verifies only at the end (mirroring batch 2's wording).

### [NIT] Card 9 "branch on Kind" is a no-op for classification
**Location:** Batch ask-signal / Card 9
**Issue:** The card says `pollEventsTick` should "branch on the last event's `Kind`," but both an `EventStop` with no output files and an `EventAsk` resolve to `OutcomeAsking` with `last.Message` — the only real branch is `allOutputFilesExist` → `OutcomeDone` else `OutcomeAsking`, so a `Kind` switch is dead.
**Fix:** Reword the requirement to drop the `Kind` branch (both non-done kinds → asking); `Kind` stays a parse-time discriminator, not a classification input.

## Verdict

APPROVE
Plan is complete, correctly sequenced, and constraint-compliant; only two doc-accuracy nits.
MILL_REVIEW_END