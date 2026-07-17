MILL_REVIEW_BEGIN
# Review: Master Builder: new, parallel fork-based implementation module

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [NOTE] status/pause verbs have no describing decision
**Section:** Scope (verb surface) / Decisions
**Issue:** `status` and `pause` are listed in the verb surface but no decision states their behaviour or output shape; only builder-parity is implied.
**Fix:** Add one line noting they mirror builder's `status`/`pause` (same envelope, same pause-flag mechanics) so a plan writer does not re-invent them.

### [NOTE] Zero-batch / empty-plan path unstated
**Section:** run-verb-shape
**Issue:** Behaviour when `Validate`/`ParsePlan` yields zero batches (nothing to fork) is not addressed — whether `run` reaches `outcome: done` immediately and whether `summary.md` is still required.
**Fix:** State the empty-plan terminal outcome and whether the summary/outcome validation still applies at zero batches.

### [NOTE] Attribution accounting across a no_report re-fork
**Section:** fork-audit-policy / state-schema
**Issue:** It is not spelled out whether a `record-batch` call returning `no_report` advances `AttributedForkTranscripts`, which determines whether the retry's audit sees "1 new" (normal) or "more than one" (the legitimate-retry warning branch).
**Fix:** Pin whether attribution updates on the no_report call or only on terminal classification, so the transcript-count branches are unambiguous.

### [NOTE] Recovery cold strand not covered by any audit
**Section:** recover-batch-reentrant-verb / fork-audit-policy
**Issue:** The fork-audit policy covers Master (parent) and forks; the recovery strand is a separate cold session and its write/weft discipline rests on prompt discipline only, unremarked.
**Fix:** Note explicitly that the recovery strand relies on builder-parity template discipline (no weft, per-card host commits) and is intentionally outside the fork audit.

## Verdict

APPROVE
Mature after two rounds; remaining items are non-blocking clarifications, no gaps require resolution before planning.
MILL_REVIEW_END
