MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [BLOCKING] weft/status_test.go orphans the fslink import
**Location:** Batch 6, Card 20
**Issue:** Card 20 removes `TestStatus_JunctionOk_Windows`, the sole user of the `internal/fslink` import (line 12 of `weft/status_test.go`); Go treats the resulting unused import as a compile error, so batch-6 verify `go test -tags integration ./internal/weft/` fails to build.
**Fix:** Add to Card 20's Requirements: also drop the now-unused `fslink` import from `weft/status_test.go` after deleting the two junction tests.

### [NIT] overview.md keeps dead links to deleted modules/warp.md
**Location:** Batch 9, Cards 31/32
**Issue:** Card 31 deletes `docs/modules/warp.md`, but `docs/overview.md` links to it at line 227 (`See [modules/warp.md]`) and line 308 (`[modules/warp.md] … (design)`); Card 32 does not list removing/retargeting these links, leaving dangling references in the same batch that deletes the target.
**Fix:** Add to Card 32: remove or retarget the two `modules/warp.md` links in overview.md.

### [NIT] Archival docs left with stale package/command refs
**Location:** Batch 9, Card 33
**Issue:** `docs/roadmap.md` and `docs/benchmarks/test-suite-timing.md` still carry many `internal/git` / `internal/worktree` / `internal/gitclone` / `lyx git-clone` / `lyx worktree` references but sit outside the doc-sweep scope (modules/shared-libs only).
**Fix:** Either note in Card 33 that these are intentionally frozen (historical roadmap / point-in-time benchmark), or add them to the sweep.

## Verdict

REQUEST_CHANGES
One batch-breaking unused-import gap plus two doc-accuracy nits.
MILL_REVIEW_END
