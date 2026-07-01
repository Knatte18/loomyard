MILL_REVIEW_BEGIN
# Review: Add lyx init --undo / deinit command — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-01
```

## Findings

### [BLOCKING] `--undo` step 4 breaks the "clean no-op on never-paired host" goal
**Location:** Batch 4 / Card 8 (`runUndo` step 4), Card 10 (missing test)
**Issue:** Card 8 step 4 gates `os.RemoveAll` on `os.Stat(weftLyxDir)`, but then calls `weftengine.Commit(l.WeftWorktree(), ...)` unconditionally with no check that `l.WeftWorktree()` itself exists. When a host was never paired at all (no `warp add` ever run — the exact "no weft pairing" condition `runInit` itself hard-gates on, and the case the overview's "no separate pre-gate" Shared Decision explicitly invokes by analogy), `Commit`'s `ensureLockDir` does `os.MkdirAll(weftPath/.weft, …)`, which silently *creates* the nonexistent `<slug>-weft` directory tree as a side effect, then `git add --` fails with "not a git repository" (exit 128), and `Commit` returns an error — so `runUndo` returns a non-zero/error result instead of the "clean no-op" the Shared Decision promises, and leaves a stray `<slug>-weft/.weft/` directory on disk.
**Fix:** In Card 8 step 4, check `l.WeftWorktree()` (or `WeftRepoRoot`/an existence probe) before calling `Commit`/`Push`; skip both and set `weftContentStatus := "not_present"` when no weft worktree exists at all. Add a Card 10 test (e.g. `TestRunInit_Undo_NoWeftPairing`) using a bare host repo with no weft sibling (mirroring `initcli_test.go`'s existing `TestRunInit_NoPairing` fixture) asserting `--undo` exits 0 with no filesystem mutation.

## Verdict

REQUEST_CHANGES
Card 8/10 miss the truly-unpaired-host no-op case the "no pre-gate" decision was designed to cover.
MILL_REVIEW_END