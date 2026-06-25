MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-25
```

## Findings

### [NIT] initcli_test.go uses os.Getwd/os.Chdir instead of t.Chdir
**Location:** `internal/initcli/initcli_test.go:33`, `:112`, `:201`
**Issue:** Three test functions call `os.Getwd()` and `os.Chdir(dir)` + `defer os.Chdir(oldCwd)` instead of `t.Chdir(dir)`; all other integration tests in the suite use `t.Chdir`.
**Fix:** Replace the `os.Getwd`/`os.Chdir`/`defer os.Chdir(oldCwd)` blocks in `TestRunInit_FirstRun`, `TestRunInit_Idempotent`, and `TestRunInit_NoPairing` with `t.Chdir(dir)`.

### [NIT] checkout.go rollback does not re-point junctions on failure path
**Location:** `internal/warp/checkout.go:97-103`
**Issue:** When `switchOrForkWeft` fails, `rollbackHostSwitch` switches the host branch back but does not re-run `WireJunctions`. Low-severity because `WireJunctions` was not called before the failure, so junctions are unchanged.
**Fix:** Add a comment clarifying the invariant rather than a code change.

### [NIT] post-checkout.sh warns on `lyx warp reconcile` but `lyx warp checkout` is more actionable
**Location:** `internal/warp/post-checkout.sh:54`
**Issue:** The drift warning suggests `lyx warp reconcile`, but for a diverged raw `git checkout` the more actionable command is `lyx warp checkout`.
**Fix:** Consider changing the warning message to suggest `lyx warp checkout` for immediate resolution.

### [NIT] cleanup.go BranchPrefix stripping undocumented in plan card 24
**Location:** `internal/warp/cleanup.go:111`
**Issue:** Card 24 does not mention stripping `w.cfg.BranchPrefix`, but the code correctly strips it and is tested.
**Fix:** No code change needed; documentation-only.

## Verdict

APPROVE
MILL_REVIEW_END
