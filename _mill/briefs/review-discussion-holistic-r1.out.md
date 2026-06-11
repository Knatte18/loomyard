Confirmed: `internal/output` does not yet exist, and the envelope writers are only in board (plus tests). The discussion's scope is accurate.

I have verified all material claims. The discussion is well-grounded, decisions have rationale and rejected alternatives, testing strategy is named per-helper, failure modes (non-repo, non-existent dir, stat errors, marshal errors) are addressed. Producing the review.

MILL_REVIEW_BEGIN
# Review: mhgo-extract-primitives

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-11
```

## Findings

### [NOTE] FindRoot empty-path contract on non-zero exit unstated
**Section:** Decisions § git.FindRoot
**Issue:** The decision says return an error including stderr on non-zero exit, but does not explicitly state the returned path string must be `""` (the Testing section asserts "empty path" for the non-repo case).
**Fix:** State in the decision that the returned path is `""` whenever a non-nil error is returned, matching the test contract.

### [NOTE] Ok marshal-failure leaves no "ok" line; behaviour unspecified
**Section:** Decisions § internal/output
**Issue:** `data, _ := json.Marshal(...)` with an unmarshalable field (e.g. a func/chan in `fields`) would write an empty line yet still return 0; this mirrors current `writeJSON` but is not called out.
**Fix:** Add a one-line note that ignoring marshal errors is an accepted, behaviour-preserving carry-over (board only passes JSON-safe maps), so no contract change is implied.

## Verdict

APPROVE
Scope, decisions, failure modes, and per-helper tests are well-grounded against source; only minor clarifications remain.
MILL_REVIEW_END
