MILL_REVIEW_BEGIN
# Review: burler: split the round prompt into an orchestrator + three instruction files

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: claude-opus-4-8 (Claude Opus 4.8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Orchestrator guard test collides with retained A/B framing
**Section:** Testing (orchestrator guard test) + Decision "Marker → file distribution"
**Issue:** The orchestrator keeps the two-jobs framing, which today reads "Fix every finding you recorded — even if the verdict was APPROVED (non-blocking polish still gets fixed)" (template.md:18) — a miniature of instruction 3's fix-everything rule — yet the guard must assert the orchestrator "excludes the fix-everything phrasing"; the boundary between legitimate job-B summary and "instruction 3 body" is unspecified.
**Fix:** Pin the guard to precise disjoint tokens (e.g. instruction-3's "not whether it gets fixed" and YAML keys `verdict:`/`findings:` in colon form) and state explicitly what job-B summary text the orchestrator is allowed to retain, so the guard cannot false-fail the framing or force stripping it.

### [NOTE] "True lazy read" still relies on prompt discipline for no-preview
**Section:** Decision "Delivery mechanism" / Problem
**Issue:** The orchestrator hands the agent all three absolute instruction paths up front; the "never preview a later file" property rests on a prompt instruction, so nothing structurally prevents read-ahead — the same discipline-reliance the Problem criticizes ("nothing structurally stops the agent from reading ahead").
**Fix:** Note in the plan that the win is "downstream content not pre-loaded into context," not a structural bar on early reads, so the goal is not oversold.

## Verdict

GAPS_FOUND
One guard-test ambiguity to resolve; otherwise decisions, scope, and constraint coverage are complete.
MILL_REVIEW_END
