MILL_REVIEW_BEGIN
# Review: Rename Cobra modules to <module>cli, extract kernels as <module>engine

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-29
```

## Findings

### [GAP] weft spawnPush/scopedPathspec are called by cli, not engine
**Section:** ambiguous-file-placement / Technical context (weft)
**Issue:** The discussion places `spawn.go` (`spawnPush`) and `weft.go` (`scopedPathspec`) in `weftengine` with the rationale "invoked by the engine Sync/Push" / "used by sync.go", but both are actually called from the CLI layer — `cli.go:230` (`spawnPush(l.WeftWorktree())`, sync RunE) and `cli.go:104` (`scopedPathspec(l.RelPath, cfg.Dirs())`, PersistentPreRunE); weft has no engine-side `Sync()`. After the move `weftcli` cannot call these unexported symbols, so the build breaks unless they are exported.
**Fix:** State that `scopedPathspec`→`ScopedPathspec` and `spawnPush`→`SpawnPush` must be exported in `weftengine` (or that `spawnPush` stays in `weftcli`), and correct the "invoked by engine Sync/Push" rationale for the weft case.

### [GAP] warp clone split leaves cli calling unexported engine helpers
**Section:** ambiguous-file-placement (warp clone.go)
**Issue:** `runCloneWithReset` stays in `warpcli` but calls `deriveHostName` (→engine), references the `hubSuffix` const (→engine), and uses the `removeAll` test seam — which is also used by `teardownHub` (→engine) and swapped by `clone_integration_test.go:310`. Unlike the explicit ghissues seam resolution, the discussion does not say `deriveHostName`/`hubSuffix` must be exported, nor which package owns the shared `removeAll` seam after the split.
**Fix:** Specify the exported engine surface the cli half needs (`DeriveHostName`, `HubSuffix`) and resolve ownership of the `removeAll` seam (one exported seam vs. separate cli/engine seams, and which the relocated reset test swaps).

### [NOTE] board engine-export list omits BriefTask/MergeStatusUpdate
**Section:** Technical context (board exports)
**Issue:** `boardcli/cli.go` references `BriefTask` and `MergeStatusUpdate`, which are absent from the discussion's "Engine exports already present" list.
**Fix:** Note these (already exported) types move to `boardengine`; harmless but keeps the list accurate.

## Verdict
GAPS_FOUND — two file-placement decisions split a caller from a now-unexported callee without specifying the export.
MILL_REVIEW_END
