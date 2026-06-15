MILL_REVIEW_BEGIN
# Review: internal/paths: subpath init + mirrored system dirs — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-15
```

## Findings

### [NIT] LauncherSpawnRel doc comment wrong collapse form
**Location:** `internal/paths/paths.go:181`
**Issue:** Doc comment says "collapses to `..\<slug>`" (one level) but the plan spec (`01-paths-geometry.md:79`) and the actual `filepath.Rel` computation both yield `..\..\<slug>` (two levels: up from `<slug>/` then up from `_launchers/`); the implementation is correct, only the comment is wrong.
**Fix:** Change the comment to "collapses to `..\..\<slug>`".

## Verdict

APPROVE
One comment-only nit; all code, tests, and docs are correct and complete.
MILL_REVIEW_END
