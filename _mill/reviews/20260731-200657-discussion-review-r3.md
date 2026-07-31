MILL_REVIEW_BEGIN
# Review: fabric: clone-does-everything + subpath-in-weft + init dissolution

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] Repo-wide fabric.yaml on-disk path vs ConfigFile
**Section:** two-repo-wide-files-on-weft-main / Technical context (config.go) / Constraints (Weft Git)
**Issue:** The Constraints block calls both files "single top-level files on weft:main," but Technical context says `LoadConfig` reads the repo-wide fabric.yaml "at BoardDir" — and `configengine.Load(base,"fabric",…)` resolves `hubgeometry.ConfigFile(base,"fabric")` = `<base>/_lyx/config/fabric.yaml` (config.go:56), not a top-level file; the exact on-disk path and read mechanism are contradictory/unspecified.
**Fix:** State whether the repo-wide fabric.yaml lives at `BoardDir/_lyx/config/fabric.yaml` (honoring the Hub Geometry Invariant's "every `<module>.yaml` resolves through ConfigFile" rule and reusing `configengine.Load`) or at the weft root (bypassing both), and reconcile that choice with the "top-level file" exclusion-safety wording.

### [GAP] configsync still materializes a per-worktree fabric.yaml
**Section:** clone-runs-reconcileall-once / Technical context (config.go)
**Issue:** `configsync.ReconcileAll` writes a per-worktree fabric.yaml via `hubgeometry.ConfigFile` and folds legacy branch_prefix+pathspec into it (configsync_test.go:115, TestReconcileAll_MigratesLegacyFabricConfig), yet this task moves both fields to a repo-wide file that `LoadConfig` reads instead — leaving an unaddressed dual-source: what the per-worktree fabric.yaml now contains, whether the fabric `ConfigTemplate` drops those keys, and where the migration logic goes.
**Fix:** Specify configsync's new fabric behavior — does ReconcileAll stop materializing per-worktree fabric.yaml (or write the repo-wide one), and does the strict template still validate pathspec/branch_prefix per-worktree? Name configsync/configengine as a touched seam, not just "clone invokes it."

### [NOTE] "run lyx fabric clone" is the wrong remedy for an unwired worktree
**Section:** Technical context (config.go retarget) / init-dissolves-to-fabric-verbs
**Issue:** The eight `config.go` "not initialized here; run \"lyx init\"" strings are retargeted to "run `lyx fabric clone`," but for an existing-but-unwired worktree the correct remedy is `reconcile`/`add`, not re-cloning the hub.
**Fix:** Pick the retarget wording per-context (clone for missing hub, reconcile/add for an unwired worktree) rather than a single "clone" for all eight.

## Verdict

GAPS_FOUND
Fabric config relocation's interaction with configengine/configsync is under-specified in two places.
MILL_REVIEW_END
