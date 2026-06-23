MILL_REVIEW_BEGIN
# Review: weft producers: _lyx/config, lyx config, codeguide — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-23
```

## Findings

### [BLOCKING] Card 14 e2e integration test entirely absent

**Location:** `C:\Code\loomyard\wts\weft-producers\internal\configcli\configcli_test.go` (entire file)
**Issue:** The file header comment claims an `//go:build integration` e2e test exists, but no such test function is present anywhere in the file. Card 14 requires a `CopyPaired`-based test that calls `worktree.New().Add()`, `t.Chdir`, clears `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` via `t.Setenv`, runs `dispatch` with an injected `weft.RunCLI` commit sync, and asserts the config file is tracked in the weft repo while the host stays pristine. All of these are absent.
**Fix:** Add the integration-tagged e2e test function as specified in Card 14 of `04-lyx-config-command.md`.

### [NIT] Abort test passes ErrAborted as editor error

**Location:** `C:\Code\loomyard\wts\weft-producers\internal\configcli\configcli_test.go:128`
**Issue:** `TestEditOneAbort` supplies `config.ErrAborted` as the fake editor's return value; real editors return an `exec.ExitError`, not a sentinel from the config package.
**Fix:** Use `errors.New("simulated editor exit 1")` instead to reflect a realistic editor failure signal.

## Verdict

REQUEST_CHANGES
One blocking finding: the Card 14 integration e2e test is entirely missing from `configcli_test.go`.
MILL_REVIEW_END