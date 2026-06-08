MILL_REVIEW_BEGIN
# Review: config-layer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\mhgo\wts\config-layer\_mill\discussion.md
date: 2026-06-08
```

## Findings

### [GAP] Read on a missing board dir is not actually fine
**Section:** Gotchas (bullet 4) / board-dir-autocreate
**Issue:** The claim "missing board dir on a read path is already fine — `store.Load` treats a missing `tasks.json` as empty" is contradicted by source: `store.go:62` acquires a read lock via `flock.New(<boardPath>/tasks.json.swaplock).RLock()` *before* the `os.ReadFile`, and flock opens that lock file with `O_CREATE` (no parent-dir creation). After `mhgo init` (which does not create the board dir), a first `list`/`get` opens a swaplock inside a nonexistent dir and fails before the `os.IsNotExist` fallback is reached.
**Fix:** Decide read-path behavior on a missing board dir (e.g. read methods return empty when the dir is absent, or also `MkdirAll` on reads) rather than asserting it already works.

### [NOTE] Config-load cost on read paths not addressed
**Section:** spawn-sync-path / testability
**Issue:** Read methods (`GetTask`/`ListTasksBrief`/`ListTasksFull`, `wiki.go:164-190`) bypass `writeOp`; the discussion specifies `os.Getwd()` + `LoadConfig` once at the top of `RunCLI`, so reads also pay config-load + the `_mhgo/` existence error, but only writes are discussed for the new path. This is consistent but unstated for reads.
**Fix:** Confirm reads also require `<cwd>/_mhgo/` and error identically when absent (matches config-location), so a plan writer does not exempt read subcommands.

## Verdict

GAPS_FOUND
One stated read-path behavior is contradicted by the locking code and needs an explicit decision.
MILL_REVIEW_END
