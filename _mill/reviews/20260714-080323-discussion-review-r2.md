MILL_REVIEW_BEGIN
# Review: Reduce git spawns in warpengine integration tests

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-14
```

## Findings

### [NOTE] SiblingLayout does not clean its input; Resolve does
**Section:** Decision: sibling-layout-method / Technical context
**Issue:** Spec sets `Cwd`/`WorktreeRoot` from the raw `worktreeRoot` arg, while `Resolve` sets `Cwd = filepath.Clean(cwd)` and `WorktreeRoot = clean(rev-parse output)`; byte-for-byte equivalence relies on callers pre-cleaning (they do: status.go:87, reconcile.go:98) and on List's `worktree <path>` matching `rev-parse --show-toplevel` after Clean.
**Fix:** State the "input must be a cleaned worktree root" precondition in godoc (or `filepath.Clean` internally), and have the equivalence test assert `WorktreeRoot` explicitly, not just Hub/Prime/RelPath.

### [NOTE] Guard logic duplicated at two call sites
**Section:** Decision: sibling-layout-method
**Issue:** The `if filepath.Dir(root) != l.Hub → Resolve else SiblingLayout` guard is described inline at both status.go and reconcile.go, with no decision on factoring it (the rejected `LayoutForKnownRoot` free function is a different concern).
**Fix:** Note whether the guard is inlined at each site or lifted into one small warpengine helper, so a plan writer picks one deliberately.

### [NOTE] Spawn-count guard assertion form left open
**Section:** Decision: spawn-count-regression-guard / Testing
**Issue:** Post-fix, `--show-toplevel` from Status over an all-sibling fixture is exactly 0 (`List` spawns `worktree list --porcelain`, not `--show-toplevel`), so the rationale's "bounded by the one-time List setup" is imprecise, and a single-N `== 0` assertion is brittle.
**Fix:** Prefer running at two N values and asserting the count does not grow (the non-scaling property the guard exists to lock), rather than pinning a single constant.

## Verdict

APPROVE
Round-1 Hub-divergence gap is closed by the guard plus a non-sibling equivalence case; only minor notes remain.
MILL_REVIEW_END
