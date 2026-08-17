MILL_REVIEW_BEGIN
# Review: Make producer engines runnable without a lyx worktree

```yaml
duration_s: 216.0
verdict: APPROVE
reviewer_model: sonnet
reviewer_self_id: claude-sonnet-5
reviewed_file: manifest/designs/producers-standalone.md, manifest/roadmap.md, _mill/discussion.md
date: 2026-08-17
```

## Findings

### [NIT:consistency] Constraints list omits Durable-vs-Ephemeral Invariant
**Section:** `discussion.md` § Constraints (and design doc's § Related)
**Issue:** discussion.md's Constraints enumeration claims each listed invariant is "named at the specific task that engages it in the design doc," but T6's own brief explicitly reworks the Durable-vs-Ephemeral State Invariant's text ("Say so in the invariant's own text as part of this task's `CONSTRAINTS.md` edit") — yet that invariant appears in neither discussion.md's list nor the design doc's "Related" link list.
**Fix:** Add Durable-vs-Ephemeral State Invariant to both enumeration lists.

## Verdict

APPROVE
Every spot-checked citation (line numbers, import sets, signatures, behaviors) verified exactly against source; only a minor list-completeness NIT found.
MILL_REVIEW_END
