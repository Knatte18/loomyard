MILL_REVIEW_BEGIN
# Review: Extract yamlengine and migrate config via lyx update — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-24
```

## Findings

### [NIT] ide/menu.go _lyx literal left unrefactored
**Location:** Batch 5, Card 15
**Issue:** Card 15 only adds error handling to `ide/menu.go`; the hardcoded `"_lyx"` at `internal/ide/menu.go:68` (`filepath.Join(entry.Path, l.RelPath, "_lyx")`) is not switched to `paths.LyxDirName`, though discussion.md (line 73) lists `ide/menu.go` among the consumers to centralize.
**Fix:** Have Card 15 also replace that literal with `paths.LyxDirName` (not a build-enforced invariant, so non-blocking).

## Verdict

APPROVE
Plan faithfully implements all decisions; DAG, numbering, and coverage are sound; one cosmetic literal-centralization gap.
MILL_REVIEW_END