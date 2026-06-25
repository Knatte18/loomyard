MILL_REVIEW_BEGIN
# Review: Move config templates home by removing the lyxtest->configreg edge — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-25
```

## Findings

### [BLOCKING] Raw `_lyx/config` path literals in lyxtest_test.go

**Location:** `internal/lyxtest/lyxtest_test.go:264,273,317,327`

**Issue:** `TestSeedConfig` constructs filesystem paths via `filepath.Join(tmpDir, "_lyx", "config", "module1.yaml")` and `filepath.Join(tmpDir, "_lyx", "config", "module2.yaml")` (lines 264, 273) instead of `paths.ConfigFile(tmpDir, "module1")` / `paths.ConfigFile(tmpDir, "module2")`. `TestCopyPaired_NeutralFixture` uses `filepath.Join(fixture.WeftPrime, "_lyx", "config", "placeholder")` (line 317) instead of `filepath.Join(paths.ConfigDir(fixture.WeftPrime), "placeholder")`, and `filepath.Join(fixture.WeftPrime, "_lyx", "config", "weft.yaml")` (line 327) instead of `paths.ConfigFile(fixture.WeftPrime, "weft")`. All four are resolved `_lyx/config` paths in test code; CONSTRAINTS.md explicitly states this rule applies to test code and that routing through helpers makes layout migrations track automatically.

**Fix:** Replace the four path constructions with `paths.ConfigFile(...)` / `paths.ConfigDir(...)` helpers; `internal/paths` is already imported in this file. The string-content assertions on lines 290-293 (comparing against `git ls-files` output) are exempt as string-content assertions and need no change.

## Verdict

REQUEST_CHANGES
One blocking violation: raw `_lyx/config` path literals in `lyxtest_test.go` tests violate the config-path constraint.
MILL_REVIEW_END
