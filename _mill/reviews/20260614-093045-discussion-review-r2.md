I have verified all the load-bearing claims. The discussion is thorough and round 2 has reconciled the r1 GAPs (HealthCheck for board-absent hard error; config stays cwd-based with only container/relpath from FindRoot; path-form normalization; hubname from Main entry).

Let me assess remaining gaps. The discussion is detailed across decisions, scope, constraints, testing, and Q&A. A few items warrant scrutiny:

1. **`worktree remove` portal/launcher teardown when target dir is gone**: scope line 49-50 says teardown "runs even if the worktree dir is already gone", but `remove.go:40-42` hard-errors when target doesn't exist before reaching link cleanup. The discussion does not specify reordering remove so portal/launcher teardown happens despite the early not-found return. This is a real ambiguity in the remove flow.

2. **Rollback `git worktree remove --force` failure handling**: the rollback itself can fail (each step). Discussion says "Leave zero residue" but doesn't address what happens if a rollback step itself errors.

3. **`ide menu` board config resolution cwd**: `board.LoadConfig(cwd, "board")` with `path: ../_board` relative to cwd — works only if menu runs from worktree root. Launcher cd's to relpath dir; consistent. Not a gap.

Let me confirm point 1 is genuinely unaddressed by re-reading scope and the remove decision — it's the strongest candidate. The scope text says teardown is best-effort and runs even if worktree dir gone, but the implementation path (early return on missing target) conflicts and the discussion never reconciles the ordering. This is a NOTE-to-GAP level item for a plan writer.

Now I'll compose the review.

MILL_REVIEW_BEGIN
# Review: mhgo-portals-launchers

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-14
```

## Findings

### [NOTE] remove early-return vs. portal teardown ordering
**Section:** Scope → `worktree remove`
**Issue:** Scope says portal/launcher teardown "runs even if the worktree dir is already gone," but `internal/worktree/remove.go:40-42` hard-errors on a missing target before any cleanup, and the discussion never states how the new teardown is sequenced around that early return.
**Fix:** State that portal/launcher teardown happens before (or independent of) the target-exists check so it still runs when the worktree dir is gone.

### [NOTE] rollback step self-failure unspecified
**Section:** Decisions → transactional-add-rollback
**Issue:** "Leave zero residue" / "full rollback" does not say what happens if a rollback step itself fails (e.g. `git worktree remove --force` errors), or whether the original error is preserved over the rollback error.
**Fix:** Specify rollback is best-effort, continues through all steps on partial failure, and surfaces the original add error (rollback errors logged, not masking it).

### [NOTE] HealthCheck "readable/parseable" is partly contradictory
**Section:** Decisions → board-is-sole-tasks-reader; Testing → board HealthCheck
**Issue:** HealthCheck is described as confirming `tasks.json` is "readable/parseable" yet also "fast (stat-level, no full parse beyond readability)" — parseable and stat-only-no-parse are in tension; the implementer must guess how deep the check goes.
**Fix:** Pin the contract to one level (e.g. stat dir + open/read `tasks.json`, no JSON unmarshal), and align the test wording accordingly.

## Verdict

APPROVE — all r1 GAPs reconciled and source claims verified; only non-blocking NOTEs remain.
MILL_REVIEW_END