I have verified the discussion's key claims against source: `shuttleengine.Spec`/`Result`/`Runner.Run` signatures, `Outcome` set, `Spec.validate` pre-existing-output rejection, `stencil.Fill` top-level-marker semantics, the shuttlecli wiring line, the hand-run severity vocabulary (`BLOCKING/MEDIUM/LOW/NIT`), and that the sandbox shuttle suite + `.cmd` launcher exist as templates to copy. All check out. A few gaps remain in the parse/contract specification.

MILL_REVIEW_BEGIN
# Review: Build burler - the review+fix round worker

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-08
```

## Findings

### [GAP] APPROVED-with-BLOCKING-finding not rejected
**Section:** review-file-format-and-parse
**Issue:** The strictness list rejects `BLOCKING` + zero BLOCKING findings but is silent on the symmetric case — `verdict: APPROVED` carrying a `severity: BLOCKING` finding — which directly contradicts the stated goal "a malformed round must never look approved."
**Fix:** Decide and pin the symmetric rule (reject APPROVED when any finding has BLOCKING severity, or state explicitly why it is allowed).

### [NOTE] Fixer-report must always be written or run never reaches `done`
**Section:** prompt-template / result-and-outcome-mapping
**Issue:** `done` requires every `OutputFiles` entry to exist; on an APPROVED-nothing-to-fix round the agent could skip the fixer-report, leaving the run to hit `timeout` instead of `done`.
**Fix:** State that the template mandates writing the fixer-report unconditionally (even when empty/"nothing fixed").

### [NOTE] Prior-round file existence validation unspecified
**Section:** profile-shape / prior-round-hydration
**Issue:** Target/Fasit paths "must exist at Run (fail loud)," but whether `PriorReviews`/`PriorFixerReports` paths are existence-validated (vs silently passed into the prompt) is not stated.
**Fix:** Pin whether missing prior-round paths fail loud like Target/Fasit or are skipped.

## Verdict

GAPS_FOUND
One parse-rule hole (APPROVED-with-BLOCKING-finding); two minor contract clarifications.
MILL_REVIEW_END
