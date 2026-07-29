MILL_REVIEW_BEGIN
# Review: fabric: config-driven junction list — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusxhigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: plan/
date: 2026-07-29
```

## Findings

### [BLOCKING] Batch-2 authoritative verify omits two migrated packages
**Location:** 00-overview.md Batch Index, batch 2 (`verify:` line ~30)
**Issue:** The authoritative `batches:` yaml gives batch 2 `verify: go test -tags integration ./internal/fabricengine/ ./internal/initengine/`, but card 6 migrates `WireJunctions` call sites in `internal/configcli/configcli_integration_test.go:59` and `internal/loomengine/preflight_integration_test.go:41`; the batch file header AND the overview's own "Go verify commands" Shared Decision (line 73) both say the verify spans all four packages — so those two packages' compiler-forced migration goes unverified by the batch mill-go actually runs.
**Fix:** Append `./internal/configcli/ ./internal/loomengine/` to batch 2's `verify:` in the overview's authoritative `batches:` block so it matches 02-fabricengine-wiring.md's header.

### [NIT] Reconcile re-wire and health-check name-sets can diverge
**Location:** Batch 2, card 6 (reconcile.go:155) vs card 9 (reconcile.go:323)
**Issue:** In `Reconcile`, `WireJunctions(hostLayout, slug, filterHubReserved(t.cfg.Dirs()))` sources names from the acting cwd's config, while `checkJunctionHealth(hostLayout)` (card 9) loads names from each pair's own weft base; for a non-acting worktree with a wider `pathspec`, health could flag an extra junction unhealthy that the re-wire never creates, looping `JunctionRepointed` without repairing.
**Fix:** Note the assumption (uniform per-hub pathspec) explicitly, or source Reconcile's re-wire names from `junctionNames(hostLayout weft base)` to match the health check.

## Verdict

REQUEST_CHANGES
Plan is sound and source-accurate; only the batch-2 authoritative verify scope must be corrected.
MILL_REVIEW_END
