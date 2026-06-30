I have now read all 38 files in the manifest. Here is my review.

MILL_REVIEW_BEGIN
# Review: Harden the Path Invariant: close enforcement hole + fix geometry leaks — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-30
```

## Findings

### [NIT] config_test.go repurposes instead of deletes path test cases
**Location:** `internal/boardengine/config_test.go:68-184`
**Issue:** Plan (Card 15) says to delete the absolute, relative, and env path-resolution test cases; the implementer instead repurposed all three to assert `cfg.Path == ""` (proving `yaml:"-"` ignores the key). This is strictly better coverage than deletion, but is a divergence from the written plan.
**Fix:** No change needed — the repurposed tests are preferable; note the deviation in the batch-done record.

### [NIT] Missing explicit --board-path override test in cli_test.go
**Location:** `internal/boardcli/cli_test.go`
**Issue:** Card 15 says to add a test that `--board-path <abs>` overrides the paths-derived data dir. The `TestCLIRerender` case verifies the no-flag path indirectly (Home.md appears at `paths.BoardDir(filepath.Dir(cwd))`), but there is no test exercising the `--board-path` branch.
**Fix:** Add a sub-case that passes `--board-path <absDir>` and asserts Home.md is created at `<absDir>` instead of `Hub/_board`.

## Verdict

APPROVE
Two NITs only; all constraints, contracts, and enforcement correctness verified clean.
MILL_REVIEW_END
