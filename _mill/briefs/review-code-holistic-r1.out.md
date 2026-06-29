I now have all the information needed to write the review.

MILL_REVIEW_BEGIN
# Review: Sandbox test-suite launcher and task harvester — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-29
```

## Findings

### [NIT] Plan conflict resolved correctly but worth noting

**Location:** `tools/sandbox/test-scheme.md` (verdict key section)
**Issue:** Card 1 said "keep ✅/⚠️/❌ verdict bucketing" but also "use ASCII-friendly markdown" — these conflict; the implementation chose ASCII (`OK`/`WARN`/`FAIL`), which is consistent with the ASCII-friendly rule and with `docs/sandbox-hub.md`'s prose. No functional defect; the plan had an internal contradiction.
**Fix:** No code change needed; the ASCII choice is correct. Record the resolution in a plan comment if round 2 is filed.

### [NIT] Unnecessary IIF for type assertion in `launchAgent`

**Location:** `C:\Code\loomyard\wts\sandbox-suite\tools\sandbox\suite.go:58-65`
**Issue:** The outer-scope capture of `exitErr` is done via an immediately invoked function, but a plain `if exitErr, ok := err.(*exec.ExitError); ok { return exitErr.ExitCode() }` achieves the same result without the closure ceremony.
**Fix:** Replace the IIF block with the idiomatic inline two-value type assertion.

### [NIT] Exit-code caveat in docs is imprecise

**Location:** `C:\Code\loomyard\wts\sandbox-suite\docs\sandbox-hub.md:119-123`
**Issue:** The sentence "go run cannot forward non-zero exit codes to the calling shell" is inaccurate; modern `go run` does forward `os.Exit` codes. The real caveat is that the sandbox tool collapses claude's numeric exit code to 0/1 via `run() → runSuite()`.
**Fix:** Reword to "the sandbox tool reports success (0) or failure (1) only; claude's precise exit code is not preserved under `go run`."

## Verdict

APPROVE
All plan cards are fully realised; no constraint violations; test coverage is complete.
MILL_REVIEW_END
