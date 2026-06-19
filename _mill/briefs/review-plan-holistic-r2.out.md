MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-19
```

## Findings

### [NIT] Card 2 declares white-box but package is black-box
**Location:** batch 1 / card 2
**Issue:** Card 2 says "white-box (`package paths`)" but the existing `paths_test.go`/`helpers_test.go` use `package paths_test` (black-box), and all eight new methods are exported, so black-box suffices and matches the file convention.
**Fix:** Use `package paths_test` to match `paths_test.go`; nothing in the card needs unexported access.

### [NIT] Card 6 LoadConfig double-suffixes _lyx path
**Location:** batch 2 / card 6
**Issue:** `config.Load(weftBaseDir, "weft", ...)` reads `<weftBaseDir>/_lyx/config/weft.yaml` (per `internal/config/config.go`), and `weftBaseDir = Join(WeftWorktree(), RelPath)`; the card's prose "reads the real `_lyx/config/weft.yaml`" is correct but the test bullet "errors when `<base>/_lyx` is absent" should make explicit that `config.Load` itself raises this via `FindBaseDir`, so the test asserts the wrapped error, not a weft-specific message.
**Fix:** State the expected error string (the generic `not initialized: _lyx/ directory not found`) the test should match.

### [NIT] Card 18 test uses os.Readlink contra card 9/14
**Location:** batch 3 / card 18
**Issue:** Card 18's test bullet says assert the junction "resolves to the weft `_lyx` (`os.Lstat`/`os.Readlink`, platform-guarded)", but cards 9 and 14 (and `links.go`) deliberately avoid `os.Readlink` on NTFS junctions in favor of `filepath.EvalSymlinks`.
**Fix:** Have card 18's resolution assertion use `filepath.EvalSymlinks` to match the production helpers; reserve `os.Lstat` for the mode-bit check.

### [NIT] Card 13 status bullet contradicts existing overview text
**Location:** batch 2 / card 13
**Issue:** `docs/overview.md` "### Status" already lists task-006 bullets and a "Weft has no Go code yet" sentence; card 13 says "soften" that sentence, which is accurate, but the card should name the exact sentence at line 124 to avoid an ambiguous edit.
**Fix:** Quote the "Weft has no Go code yet — portals are still the live mechanism" line as the specific target for replacement.

## Verdict

APPROVE
Plan is constraint-correct, well-sequenced, and source-grounded; only minor test/doc nits.
MILL_REVIEW_END
