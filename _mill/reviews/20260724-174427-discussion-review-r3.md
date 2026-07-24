MILL_REVIEW_BEGIN
# Review: gitrepo: generic, repo-agnostic git primitives

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-24
```

## Findings

### [NOTE] Snapshot FF-only adopt assumes non-divergent history
**Section:** Decision — Snapshot remote sync (ff-only, adopt-on-conflict)
**Issue:** "adopt-on-conflict" is only correct when a snapshot key advances along a single non-divergent line; a rejected push from a legitimately divergent or history-rewritten SHA would adopt a value whose history omits the local clone's processed commits (marking them done), or wedge into permanent re-processing if the old ref SHA is orphaned.
**Fix:** State the implicit precondition — snapshot keys track a single monotonically-forward (never rebased/force-pushed/divergent) line — so the plan can note divergence is out of the safe model rather than silently relying on it.

### [NOTE] Board's *.lock glob covers both locks, not just push-lock
**Section:** Decision — Lock ownership (push-lock gitignore becomes redundant)
**Issue:** Board's `ensureLockfilesIgnored` ignores via a single `*.lock` glob that covers both `tasks.json.lock` (write-mutex, stays) and the push lock; "dropping the push-lock portion" of one glob is imprecise, and the write-mutex file still needs the same pattern.
**Fix:** Reword to note the `*.lock` entry stays for the domain mutex; this is the later board-rewrite task's concern (explicitly out of scope here), so it does not block planning.

## Verdict

APPROVE
All decisions resolved with rationale; source claims verified; two non-blocking clarifications only.
MILL_REVIEW_END
