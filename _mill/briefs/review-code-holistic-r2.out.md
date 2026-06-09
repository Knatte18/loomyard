MILL_REVIEW_BEGIN
# Review: Extract shared infrastructure (config, git, lock) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-09
```

## Findings

### [BLOCKING] Non-NotExist stat error silently dropped in config.Load

**Location:** `C:\Code\mhgo\wts\extract-shared-infra\internal\config\config.go:37-41`

**Issue:** `os.Stat(mhgoDir)` may return a non-NotExist error (e.g. permission denied); the implementation checks only `os.IsNotExist(err)` and falls through on any other error, treating a permission-denied directory as present and proceeding to `loadDotEnv` — giving callers a misleading downstream error instead of the root cause.

**Fix:** After the `os.IsNotExist` guard, add `else if err != nil { return nil, fmt.Errorf("stat _mhgo: %w", err) }` to surface the real error.

### [NIT] Stale comment in spawn_other.go references removed hideProcWindow

**Location:** `C:\Code\mhgo\wts\extract-shared-infra\internal\board\spawn_other.go:5`

**Issue:** The file-level comment still says "so hideProcWindow is a no-op" after `hideProcWindow` was removed in card 14.

**Fix:** Remove or rewrite the sentence to reflect that there is no hideProcWindow in this file anymore.

### [NIT] git.go error handling deviates from plan spec

**Location:** `C:\Code\mhgo\wts\extract-shared-infra\internal\git\git.go:24-35`

**Issue:** Card 3 specifies a type assertion on `*exec.ExitError` to obtain the exit code; the implementation instead reads `cmd.ProcessState.ExitCode()` unconditionally, which is functionally equivalent for normal cases but diverges from the documented contract and loses the semantic distinction between "command ran and exited non-zero" vs "command could not be started".

**Fix:** Acceptable as-is given functional equivalence, but aligning with the spec (`if exitErr, ok := err.(*exec.ExitError); ok { ... }`) would make the control flow self-documenting.

## Verdict

REQUEST_CHANGES
One blocking correctness gap in `config.Load`: non-NotExist stat errors are silently dropped.
MILL_REVIEW_END