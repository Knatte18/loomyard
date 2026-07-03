MILL_REVIEW_BEGIN
# Review: Dedicated sandbox suite for mux

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-03
```

## Findings

### [NOTE] File-name references to reword undercounted
**Section:** Scope / Technical context (coverage guard)
**Issue:** The discussion says "three hardcoded error-message strings" name the single file, but `sandbox_coverage_test.go` also has a `t.Fatalf` at line 121 and the package/function doc comments (lines 1-5, 33, 102) that describe single-file behaviour and reference `tools/sandbox/SANDBOX-SUITE.md`.
**Fix:** Note that the parse function's `t.Fatalf` and the test's doc comments must also be updated to the multi-suite model, not just the three assertion strings.

### [NOTE] Vacuous-single-file guard left optional
**Section:** Testing
**Issue:** "Consider asserting the glob matches ≥2 files" is non-committal, yet it is the main protection against a mistyped glob silently passing on one file.
**Fix:** Commit to the ≥2-files assertion (or an equivalent explicit guard) rather than leaving it a "consider".

### [NOTE] attach "no interactive TTY" rationale is imprecise
**Section:** Decision: attach is an operator-assisted visual checkpoint
**Issue:** `launchAgent` inherits the operator's stdin/stdout, so the agent does run in a TTY; the real blocker is that `lyx mux attach` would seize the same terminal the agent runs in and cannot visually confirm panes.
**Fix:** Restate the rationale in terms of terminal-takeover-collision + visual confirmation so the plan's scenario wording (operator uses a second terminal) is grounded correctly.

## Verdict
APPROVE
Scope, decisions, and testing are well-grounded; only minor accuracy NOTEs remain.
MILL_REVIEW_END
