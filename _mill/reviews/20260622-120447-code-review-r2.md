MILL_REVIEW_BEGIN
# Review: Extract internal/fslink cross-OS link primitive — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [NIT] Stale inline comment in remove.go step-5 line
**Location:** `C:\Code\loomyard\wts\extract-fslink\internal\worktree\remove.go:87`
**Issue:** The inline comment still reads `"catches nested junctions that removeLinks misses"` — card 8 required updating step-5/step-6 inline comments to say `fslink.RemoveLinksIn`; only the function-level doc block (lines 37-38) was updated.
**Fix:** Change `removeLinks misses` to `fslink.RemoveLinksIn misses` in the line-87 comment.

### [NIT] RemovesSymlinks test targets files, not directories
**Location:** `C:\Code\loomyard\wts\extract-fslink\internal\fslink\fslink_test.go:421`
**Issue:** `Create(link1, filepath.Join(dir, "target1.txt"))` passes a regular file as target; on Windows, junctions require directory targets, so `FSCTL_SET_REPARSE_POINT` will fail and the entire `RemovesSymlinks` subtest silently skips, leaving `RemoveLinksIn` untested on Windows.
**Fix:** Change the targets in `RemovesSymlinks` to directories (`os.Mkdir`) so the test exercises real link removal on all platforms.

## Verdict

APPROVE
Two nits only; no correctness, contract, or constraint violations found.
MILL_REVIEW_END
