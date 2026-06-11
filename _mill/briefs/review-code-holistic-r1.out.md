MILL_REVIEW_BEGIN
# Review: Extract shared primitives (paths, output) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-11
```

## Findings

### [NIT] config.md prose intro states wrong error message

**Location:** `C:\Code\mhgo\wts\mhgo-extract-primitives\docs\shared-libs\config.md:25`
**Issue:** The introductory paragraph says `` `config` errors with `not initialized here; run "mhgo init"` `` but `config.FindBaseDir` actually returns `not initialized: _mhgo/ directory not found in <dir>`; the board-level message is produced by `internal/board/config.go` rewrapping. Card 6 required distinguishing the two; the `## Exported helpers` section (lines 99–107) does so correctly, but the earlier prose still carries the wrong literal.
**Fix:** Update line 25 to say `config` errors with `not initialized: _mhgo/ directory not found in <dir>` and note that board rewraps it.

## Verdict

APPROVE
All plan cards fully implemented; one stale prose literal in the docs, no blocking issues.
MILL_REVIEW_END
