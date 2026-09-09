MILL_REVIEW_BEGIN
# Review: Bump quarry to v0.2.0 and adopt Status.Known()/Rejected()

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-09
```

## Findings

### [BLOCKING:consistency] TDD claim contradicts the test's failing mechanism
**Section:** §Testing, first paragraph ("TDD is a natural fit … made to fail against the current literal switch before the `Known()` swap lands").
**Issue:** §statuses-completeness-test establishes that the test's only failing mechanism is table-vs-`quarry.Statuses` drift; with today's four statuses the coverage assertion passes and the per-status rows (verified against `donecheck.go:180-220`) match current behavior exactly, so the test is green against the literal switch and no red state exists.
**Fix:** Drop the TDD framing (or state explicitly that the test is a non-red regression/drift guard written alongside the swap, not a failing-first test).

### [BLOCKING:consistency] False claim that `Rejected()` removes the last `.Status` selector
**Section:** §Technical context "The two AST tripwires" (line 114) and §comment-prose-updates Note (line 77).
**Issue:** Both assert `unreadableStatusDetail`'s body will no longer name `.Status` after the swap; `resolve.go:142` still returns `fmt.Sprintf("… unrecognized resolve status %q", r.Status)`, and §Technical context itself says both format strings are unchanged — so the allowlist entry stays *used*, not merely unused-but-harmless.
**Fix:** Correct both statements to say the `Status` selector remains in the second branch, and drop the "unused allowlist entry" rationale as a false premise.

## Verdict

REQUEST_CHANGES
Two false premises: an unachievable TDD red state and a misstated post-swap tripwire outcome.
MILL_REVIEW_END
