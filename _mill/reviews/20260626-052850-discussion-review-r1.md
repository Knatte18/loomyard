MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-26
```

## Findings

### [NOTE] Template needs one combined container, not reuse
**Section:** Decisions → pre-paired-template / git-pointer-rewrite
**Issue:** Existing `buildHostHub` and `buildWeftPrime` live in *separate* `MkdirTemp` roots, but `git worktree add` bakes absolute paths requiring hub and weft-prime to be siblings under one Hub (`WeftRepoRoot = Hub/<prime>-weft`, `WeftWorktreePath = Hub/<slug>-weft`); "plus the existing hub/bare/weft-prime/weft-bare" reads as reuse.
**Fix:** State the new builder assembles a fresh single container (hub + weft-prime co-located) before the two `worktree add` calls, so relative geometry is consistent for the copy-rewrite.

### [NOTE] Fixed slug means migrated assertions change identifiers
**Section:** Decisions → fixed-slug + Testing
**Issue:** Migrated tests adopt the template's fixed slug "task" (branch == slug under default empty `BranchPrefix`), so any assertion referencing the old test-chosen slug string changes, lightly contradicting "same assertions, behaviourally identical."
**Fix:** Clarify the fixture exposes Slug/Branch and host+weft worktree paths as fields, and that slug-string substitution is the only permitted assertion change (tests needing a specific slug keep real Add, already decided).

### [NOTE] Post-checkout hook side-effect of Add unaddressed
**Section:** Technical context → Production warp.Add
**Issue:** Real `Add` installs the post-checkout hook (`InstallPostCheckoutHook`, add.go:153); the template captures git-worktree creation only and the discussion is silent on the hook (unlike portals/launchers, which it explicitly recreates on-demand).
**Fix:** Note the hook is intentionally omitted and recreated on-demand where needed; impact is low since hook tests (`hook_test.go`) call `InstallPostCheckoutHook` themselves.

### [NOTE] commondir absolute branch left undecided
**Section:** Decisions → git-pointer-rewrite (point 3)
**Issue:** "Verify `commondir` … assert/handle if absolute" leaves the absolute case unresolved.
**Fix:** Pick fail-fast-assert vs rewrite so the copy stays deterministic across git versions.

## Verdict
APPROVE
Scope, decisions, constraints, and testing are well-grounded; only clarifying NOTEs, no blocking gaps.
MILL_REVIEW_END
