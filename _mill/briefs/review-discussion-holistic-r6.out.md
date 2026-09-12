MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class, mid reasoning effort
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:consistency] Context skip loses the crash-resume permanently
**Section:** `### detection-site` (skip #3 rationale) / Q&A "operator Ctrl-C"
**Issue:** "Skipping loses nothing — the next `drive` detects the same ones" is true for triggers 2–5 but false for trigger 1: the Ctrl-C arm persists `StatePaused` (`internal/shedengine/run.go:200`), so the next `drive`'s entry read sees `paused`, not `running`, and the in-memory crash observation is gone — the exact permanent loss the same section refuses to accept for the error path ("a crash-resume followed by a failing producer would be lost permanently").
**Fix:** State a disposition: either exempt a detected crash-resume from the done-context skip (file it, skip the rest), or record the loss explicitly as a fourth accepted imprecision and drop the "loses nothing" claim.

### [NIT:consistency] "`false` skips everything" vs. the unconditional entry step
**Section:** `### on-by-default-with-a-knob` / **Scope** (In, entry step)
**Issue:** The knob is a told input of the extracted function, but the entry lock probe plus the extra status decode live in `drive`'s closure and run unconditionally, so `selfreport: false` still pays the cost the rationale says it avoids ("performing the reads would be pure cost").
**Fix:** Say whether `drive` guards the entry observation on `c.cfg`'s bool or deliberately keeps it unconditional, and align the Config test's "returns before doing anything" wording with that choice.

### [NIT:consistency] Trigger-5 title has no occurrence discriminator
**Section:** `### one-issue-per-anomaly`
**Issue:** The section's premise is "two genuinely separate occurrences of the same kind are two issues", but trigger 5's title (`<bouncer-row> — <ledger-key>`) is identity-keyed only, so a key that recurs after being resolved — including across an `archiveRunDir` generation — is suppressed by the marker forever.
**Fix:** State that trigger 5 is deliberately identity-deduped rather than occurrence-deduped, so the plan does not read the general premise as applying to it.

## Verdict

REQUEST_CHANGES
Context-skip rationale is false for crash-resume; that loss needs an explicit disposition.
MILL_REVIEW_END
