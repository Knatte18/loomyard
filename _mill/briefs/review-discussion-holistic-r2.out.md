MILL_REVIEW_BEGIN
# Review: fabric: unify warp + weft into one git-coordination module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Index Tier-1 test claim vs git-dir spawn
**Section:** Testing (TDD candidates) + Decision "Correspondence index"
**Issue:** Correspondence-index ops (`RecordCorrespondence`, `WeftSHAForWarpSHA`) are claimed as untagged Tier-1 "pure logic" tests, but the index path is resolved via `git rev-parse --git-dir` (a git spawn through `gitexec.RunGit`), which the Test Tier Purity Invariant bans in untagged files — so those tests cannot be untagged as written.
**Fix:** State that the sorted-index/nearest-older/parse logic is exercised against an explicit path (no git), separated from the git-dir resolution, or tag the resolving tests `integration`; name which layer owns `--git-dir`.

### [NOTE] "per-clone" index vs `--git-dir` (per-worktree)
**Section:** Decision "Correspondence index: gitignored local cache"
**Issue:** Rationale calls the index "per-clone rebuildable," but `git rev-parse --git-dir` in a linked worktree returns the per-worktree gitdir, not the shared common dir — yielding a distinct index per weft worktree, not per clone.
**Fix:** Confirm the intended scope (per-worktree vs `--git-common-dir` shared) and reconcile the wording, since it changes what `RebuildIndex` reconstructs.

### [NOTE] RevertWithWeft warp-reset side unstated
**Section:** Decision "RevertWithWeft: nearest-older with explicit gap report"
**Issue:** The decision describes only resetting weft to the nearest-older correspondence and the gap classification; it never states whether `RevertWithWeft(warpSHA)` also resets warp (the design-doc sketch resets warp first, then weft). The discussion is declared authoritative scope, so silence risks a weft-only implementation.
**Fix:** State explicitly that warp is reset to `warpSHA` as part of the method (per design doc), or that the caller does so.

## Verdict

GAPS_FOUND
One test-tier conflict blocks planning; two notes on index scope and revert contract.
MILL_REVIEW_END
