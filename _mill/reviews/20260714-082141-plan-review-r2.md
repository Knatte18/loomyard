MILL_REVIEW_BEGIN
# Review: Reduce git spawns in warpengine integration tests — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-14
```

## Findings

### [NIT] Card 4 fixture Add uses SkipPush; reconcile_test uses SkipGit
**Location:** batch reduce-redundant-resolve / Card 4
**Issue:** The card seeds extra pairs via `w.Add(f.Layout, slug_i, AddOptions{SkipPush: true})`, but the proven pattern in `reconcile_test.go` (line 54) uses `AddOptions{SkipGit: true}`; SkipPush still runs the host push (add.go step 11) against the copied host bare, an avoidable network round-trip in a spawn-sensitive guard's setup phase.
**Fix:** Use `AddOptions{SkipGit: true}` to match the established fixture pattern and skip all pushes during setup.

## Verdict

APPROVE
Plan is source-grounded, spawn-guard isolates Resolve's `--show-toplevel` correctly, invariants preserved.
MILL_REVIEW_END
