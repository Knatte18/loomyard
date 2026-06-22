MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-21
```

## Findings

### [NIT] buildWeftPrime takes hubPath but sync.Once ignores it on reuse
**Location:** internal/lyxtest/lyxtest.go:130-210
**Fix:** Make buildWeftPrime zero-parameter (match buildHostHub/buildWeftOnly) or document the parameter is only honoured on the first call.

### [NIT] result.Pushed is always true even when SkipPush: true
**Location:** internal/worktree/add.go:191, internal/worktree/add_test.go:147
**Fix:** Set Pushed: !opts.SkipPush && !opts.SkipGit so the field is meaningful and the happy-path assertion is load-bearing.

### [NIT] copyDirRecursive uses filepath.Walk which follows symlinks
**Location:** internal/lyxtest/lyxtest.go:392-439
**Fix:** Use filepath.WalkDir with symlink detection, or document that templates must never contain symlinks.

### [NIT] Error return on template builders is always nil
**Location:** internal/lyxtest/lyxtest.go:46,130,222
**Fix:** Remove the error return (failures panic) and update callers to drop the dead error check.

## Verdict

APPROVE
Blocking finding from R1 is genuinely resolved; all four R1 NITs corrected; no new blocking issues.
MILL_REVIEW_END
