MILL_REVIEW_BEGIN
# Review: Local lyx sandbox for manual experimentation — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-26
```

## Findings

### [NIT] Banned-token scan also reads comments in main.go
**Location:** Batch 1 / Card 1
**Issue:** `enforcement_test.go` does a plain `strings.Contains` over full file content (comments included) and only skips `_test.go` files, so a code comment in non-test `tools/sandbox/main.go` like "avoid `os.Getwd`" or one quoting "...`--show-toplevel`" would fail the build — exactly the tokens Card 1's prose tells the implementer to think about.
**Fix:** Add a one-line caution that `main.go` must not contain the literal strings `os.Getwd` / `--show-toplevel` even in comments or docstrings (the seam comment may reference `os.RemoveAll`, which is fine).

### [NIT] `lyx`-not-on-PATH exec failure handling left implicit
**Location:** Batch 1 / Card 1
**Issue:** The `error-surface-verbatim` decision names "lyx not on PATH (exec fails)" as a by-design failure, but Card 1 only says "propagate a non-zero exit"; an exec-startup error is not an `ExitError` and has no subprocess stderr, so without explicit handling it risks a bare non-zero exit with no legible cause.
**Fix:** Require that a non-`ExitError` from the clone run be written to stderr with a clear "lyx not found on PATH" cause before exiting non-zero.

### [NIT] No test asserts clone-error propagation
**Location:** Batch 1 / Card 1 (main_test.go cases a–d)
**Issue:** The listed tests cover hub-path computation and which seam is invoked, but none asserts the central `error-surface-verbatim` behavior — that a `cloneRun` returning an error yields a non-zero exit/propagated error.
**Fix:** Add a subtest where the stubbed `cloneRun` returns an error and assert the decision function surfaces it.

## Verdict

APPROVE
Plan is constraint-compliant and complete; only minor hardening NITs remain.
MILL_REVIEW_END