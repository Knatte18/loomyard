MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-21
```

## Findings

### [BLOCKING] `rewriteOriginURL` spawns `git remote set-url` per test copy

**Location:** internal/lyxtest/lyxtest.go:334-343
**Issue:** The plan's "template-once + per-test filesystem copy" shared decision explicitly forbids `git remote set-url` because it "re-introduces a spawn." `rewriteOriginURL` calls `exec.Command("git","remote","set-url","origin",newURL)`, invoked once per repo in every CopyHostHub/CopyWeft and twice per CopyPaired. This negates the "zero per-test git spawns" claim and the offline-copy guarantee.
**Fix:** Replace `rewriteOriginURL` with a pure text edit: read `<copiedRepo>/.git/config`, replace the single `url = …` line under `[remote "origin"]` (assert exactly one match), write it back — no subprocess.

### [NIT] `TestRunCLI_EnvMapToOption` uses raw os.Getwd()/os.Chdir() instead of t.Chdir()

**Location:** internal/weft/weft_integration_test.go:130-139
**Fix:** Replace manual cwd save/restore with a single `t.Chdir(hubPath)`.

### [NIT] `TestRunCLI_UnknownSubcommand` exercises ErrNotAGitRepo, not the unknown-subcommand path

**Location:** internal/weft/cli_test.go:15-25
**Fix:** Use `lyxtest.CopyWeft(t)` so the CLI reaches the dispatch table and hits the default case.

### [NIT] `TestMustRun_Failure` runs a successful command in its "failure" subtest

**Location:** internal/lyxtest/lyxtest_test.go:211-224
**Fix:** Use a command guaranteed to fail to confirm MustRun calls tb.Fatalf on non-zero exit.

### [NIT] CopyHostHub and CopyWeft call tb.TempDir() twice

**Location:** internal/lyxtest/lyxtest.go:409,415 and 512,518
**Fix:** Call tb.TempDir() once per fixture and place both repos inside it (match CopyPaired).

## Verdict

REQUEST_CHANGES
Blocking: rewriteOriginURL spawns git remote set-url per test copy, violating the plan's explicit "text-edit only" shared decision.
MILL_REVIEW_END
