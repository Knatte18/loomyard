The enforcement scan only bans `os.Getwd` and `--show-toplevel`. The `rev-parse --abbrev-ref HEAD` query is fine. The constraint is satisfied.

MILL_REVIEW_BEGIN
# Review: Ensure weft branches are orphan branches — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [NIT] Card 3 case 3 should assert error is from the guard, not a later failure
**Location:** Batch 1 / Card 3, case 3
**Issue:** Asserting only the substring `"detached HEAD"` plus zero-residue does not by itself prove the guard fired *before* step 7 creation rather than the spawn failing downstream and rolling back; both produce the same observable.
**Fix:** Note that the guard runs before step 7 so the host worktree dir is never even attempted — the existing zero-residue assertion plus message substring is acceptable; consider adding that no rollback path was needed.

### [NIT] Decision wording on rollback vs. pre-creation guard differs from discussion
**Location:** overview `## Shared Decisions` (detached-head-guard) vs. discussion.md
**Issue:** discussion.md `detached-head-guard` says the abort "performs the existing full paired rollback," while the overview/Card 2 correctly place the guard before any creation so no rollback runs; the two phrasings could confuse the implementer.
**Fix:** The plan's "before any creation, no rollback needed" framing is the correct one; the divergence is harmless since nothing is created.

### [NIT] Card 4 docs anchor is precise; confirm exact insertion line
**Location:** Batch 1 / Card 4
**Issue:** `### Junction model` (docs/overview.md:133) is followed by `### Weft suffix convention` (line 141); the new `### Branch model` belongs between them.
**Fix:** Insert the new subsection at line ~140, before `### Weft suffix convention`.

## Verdict

APPROVE
Plan faithfully implements all decisions; constraints, DAG, sequencing, and discriminating tests are sound.
MILL_REVIEW_END