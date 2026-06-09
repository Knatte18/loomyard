MILL_REVIEW_BEGIN
# Review: board-modul (rename fra wiki) + _mhgo-konfigurasjon — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-09
```

## Findings

### [NIT] regexp.MustCompile called per-invocation in expandEnv
**Location:** `internal/board/config.go:162`
**Issue:** `regexp.MustCompile` is called inside `expandEnv` on every `LoadConfig` call; the plan says "implement with a single `regexp.MustCompile`" implying a package-level compiled pattern.
**Fix:** Hoist the compiled regex to a package-level `var envTokenRe = regexp.MustCompile(...)`.

### [NIT] Latent double-start-marker in updateGitignoreBlock
**Location:** `internal/board/init.go:154-195`
**Issue:** When the existing block's content differs from the desired content, the old start-marker line is collected into `beforeBlock` (line 156) and then a new start marker is also appended (line 193), producing two consecutive `# === mhgo-managed ===` lines in the rewritten file.
**Fix:** Do not add the start-marker line to `beforeBlock`; only collect content that is genuinely before the block.

### [NIT] TestEnvExpansionUnsetError mixes t.Setenv with os.Unsetenv
**Location:** `internal/board/config_test.go:196-197`
**Issue:** `t.Setenv("NONEXISTENT_VAR", "")` sets the var (registers cleanup to restore), then `os.Unsetenv("NONEXISTENT_VAR")` removes it directly — the combination works but is unexpected; `t.Setenv` cleanup will try to restore a non-existent var to `""` after the test.
**Fix:** Replace both lines with just `os.Unsetenv("NONEXISTENT_VAR")` (no `t.Setenv`), since the test only needs to ensure the variable is absent; or use `t.Setenv` alone and rely on a variable that was never set in the first place.

## Verdict

APPROVE
All plan cards are realised, shared decisions are applied consistently, cross-batch contracts are correct, and no blocking issues were found.
MILL_REVIEW_END
