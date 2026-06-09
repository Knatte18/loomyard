I now have a complete picture. Let me compile the findings.

Key issues I found:

1. **`os.Unsetenv` used after `t.Setenv` in `config_test.go`** — The plan's Shared Decisions explicitly forbid `os.Setenv`/`os.Unsetenv` in tests (only `t.Setenv` is allowed). Lines 162, 222, 252, 288 in `internal/config/config_test.go` call `os.Unsetenv` after `t.Setenv`, which defeats the auto-restore mechanism of `t.Setenv` (when `t.Setenv` runs cleanup, it restores the value to "", not to "unset"). This is a constraint violation.

2. **`board/config.go` double-stats `_mhgo/`** — `LoadConfig` stats the dir itself (line 66–68), then calls `config.Load` which also stats it (line 39). This is a minor redundancy introduced by the implementation not following the plan exactly (the plan says to call `config.Load` and return its error). The custom error message "not initialized here; run \"mhgo init\"" is intentionally different (more user-friendly) but the double-stat is wasteful.

3. **`lock_test.go` does not test `AcquireReadLock` directly** — The plan batch 1 says the test "covers `AcquireWriteLock`, `AcquireReadLock` (implicitly via the write-lock test)". The test only exercises write-lock paths; `AcquireReadLock` is never called. This is a minor deviation from stated coverage.

4. **`internal/git/git.go` RunGit implementation diverges from plan** — The plan specifies "On `*exec.ExitError`, return ... exitErr.ExitCode()". The implementation instead uses `cmd.ProcessState.ExitCode()` and checks `cmd.ProcessState != nil`. The behavior is equivalent on all normal paths but the code deviates from the plan's spec.

The `os.Unsetenv` issue is the most significant — it violates the Shared Decisions constraint and creates test isolation risk.

MILL_REVIEW_BEGIN
# Review: Extract shared infrastructure (config, git, lock) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-09
```

## Findings

### [BLOCKING] os.Unsetenv used after t.Setenv in config_test.go
**Location:** `internal/config/config_test.go:162, 222, 252, 288`
**Issue:** Four tests call `t.Setenv(name, "")` then immediately `os.Unsetenv(name)`. The Shared Decisions constraint ("No global state or `os.Setenv` in tests") is violated; worse, `t.Setenv`'s cleanup restores the variable to `""` (not to unset), so after cleanup the variable is set to empty string rather than absent — silently changing test-environment state for parallel or subsequent tests.
**Fix:** Remove the `t.Setenv` call entirely; call only `os.Unsetenv` guarded by `t.Cleanup(func() { os.Setenv(name, savedVal) })`, or simpler: skip the unset-dance entirely since the env var names are unique enough that CI will never have them set, matching the plan's own note ("do not use `os.Unsetenv`" in card 16).

### [NIT] board/config.go double-stats _mhgo/ unnecessarily
**Location:** `internal/board/config.go:65-68`
**Issue:** `LoadConfig` stats `_mhgo/` itself before delegating to `config.Load`, which stats it again; the plan (card 16) says to call `config.Load` and return its error, implying the board wrapper need not re-check.
**Fix:** Remove the `os.Stat` block in `LoadConfig` and let `config.Load` return the "not initialized" error directly; wrap it if a more user-friendly message is required.

### [NIT] lock_test.go never exercises AcquireReadLock
**Location:** `internal/lock/lock_test.go`
**Issue:** Batch 1 states the test covers `AcquireReadLock` "implicitly via the write-lock test", but no test in the file calls `AcquireReadLock` at all, directly or indirectly.
**Fix:** Add a sub-test under `TestAcquireWriteLock` (or a standalone test) that calls `lock.AcquireReadLock` and releases it, matching the plan's stated coverage claim.

## Verdict

REQUEST_CHANGES
One constraint violation (`os.Unsetenv` after `t.Setenv`) must be fixed before approval.
MILL_REVIEW_END