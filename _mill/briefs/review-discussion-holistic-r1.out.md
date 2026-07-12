I have enough to complete the review. The discussion is thorough and decisive; I verified the key source claims (WhereFiles has no external consumers, pathCovers is boundary-aware, the current parser/validator shapes, the implementer template's "never read 00-overview.md" line) and checked the cited mill references.

MILL_REVIEW_BEGIN
# Review: Add typed file-ops to lyx's plan-format

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-12
```

## Findings

### [NOTE] Mill reference paths not resolvable from this worktree
**Section:** Technical context — "Mill reference points"
**Issue:** The cited `plugins/mill/templates/plan-batch.md` and `plugins/mill/scripts/_plan_validate.py` do not exist under the lyx worktree; they live in a separate repo (`C:\Code\millhouse\...\plugins\mill\...`).
**Fix:** Note that these are millhouse-repo paths (or that the discussion's own canonical Rename-mechanic/grammar text is the authoritative source, since mill is precedent-not-dependency).

### [NOTE] Non-Moves cross-field mutual exclusivity is unenforced
**Section:** Decisions — typed-card-fields / validator-check-set
**Issue:** Fields are called "mutually exclusive," but the only overlap check is `move-redundant` (Moves vs Creates/Deletes); a path listed in both `Edits:` and `Creates:`, or `Creates:` and `Deletes:`, within one card is caught by no enumerated check.
**Fix:** State whether intra-card overlap across the non-Moves fields is intentionally unchecked convention or a missing check to add.

## Verdict

APPROVE
Scope, decisions, checks, and testing are complete and decisive; only two non-blocking NOTEs.
MILL_REVIEW_END
