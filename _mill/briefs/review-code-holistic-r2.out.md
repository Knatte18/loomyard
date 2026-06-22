MILL_REVIEW_BEGIN
# Review: Optimise and slim the rest of the test suite — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-22
```

## Findings

### [NIT] Timing doc misattributes menu_test's serial reason to os.Chdir
**Location:** `docs/benchmarks/test-suite-timing.md:219-220`
**Issue:** The parallel-safety note states both `cli_test.go` and `menu_test.go` "use `os.Chdir`", but `menu_test.go` has no `os.Chdir`; it stays serial because of `t.Setenv("BOARD_SKIP_GIT", "1")` in every test function.
**Fix:** Change the description for `menu_test.go` from `os.Chdir` to `t.Setenv`.

## Verdict

APPROVE
One trivial doc inaccuracy (serial reason for menu_test); no blocking issues.
MILL_REVIEW_END
