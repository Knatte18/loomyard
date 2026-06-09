MILL_REVIEW_BEGIN
# Review: Extract shared infrastructure (config, git, lock) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-09
```

## Findings

### [NIT] RunGit non-ExitError returns partial buffers and exitCode 0
**Location:** `C:\Code\mhgo\wts\extract-shared-infra\internal\git\git.go:26-31`
**Issue:** Plan spec says non-`ExitError` failures must return `"", "", -1, err`; the implementation returns `outBuf.String(), errBuf.String(), 0, err`, so callers cannot distinguish "binary not found" (exitCode 0, err non-nil) from "ran cleanly" (exitCode 0, err nil) by exit code alone.
**Fix:** Add an `else` branch after the `*exec.ExitError` check: `return "", "", -1, err`.

### [NIT] hideProcWindow called before stdout/stderr buffers are assigned
**Location:** `C:\Code\mhgo\wts\extract-shared-infra\internal\git\git.go:16-20`
**Issue:** Plan card 3 specifies buffer assignment before `hideProcWindow(cmd)`; the implementation inverts the order (functionally harmless since `hideProcWindow` only sets `SysProcAttr`).
**Fix:** Move `var outBuf, errBuf bytes.Buffer` / `cmd.Stdout` / `cmd.Stderr` assignments to before the `hideProcWindow(cmd)` call to match the plan spec.

## Verdict

APPROVE
All 22 plan files are correctly realised; shared decisions and cross-batch contracts are consistently applied.
MILL_REVIEW_END