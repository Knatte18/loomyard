MILL_REVIEW_BEGIN
# Review: Rename internal/paths to internal/hubgeometry

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-30
```

## Findings

### [GAP] codeguide guard hardcodes filename "paths.go"
**Section:** Scope / Testing (codeguide_guard_test.go)
**Issue:** `internal/paths/codeguide_guard_test.go:48` skips the geometry file by filename literal `if d.Name() == "paths.go"`; after `git mv paths.go → hubgeometry.go` this skip stops matching, so the `_codeguide` tree-scan will scan hubgeometry.go (which contains `WeftCodeguideDir`/`_codeguide`) and fail — yet the scope's string-sweep only targets `internal/paths`/`package paths`, not filename literals, and Testing mischaracterizes the hardcode as "package path/name."
**Fix:** State explicitly that the `"paths.go"` literal must become `"hubgeometry.go"` (the skip is filename-based, not package-name-based) or the codeguide guard breaks.

### [NOTE] Doc enumeration omits envsource.md
**Section:** Technical context — "Doc/instruction references to find"
**Issue:** `docs/shared-libs/envsource.md:5` references `internal/paths` ("`internal/envsource` imports `internal/paths`") but is absent from the enumerated doc list.
**Fix:** Add `docs/shared-libs/envsource.md` to the list (the mandated grep re-discovery would catch it, but the explicit enumeration should be complete).

## Verdict

GAPS_FOUND
One load-bearing filename literal (`"paths.go"` in codeguide guard) is imprecisely flagged and will break a test if missed.
MILL_REVIEW_END
