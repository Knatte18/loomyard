I have enough to complete the review. Key finding: Card 2 adds a *required* `-loomyard` check in the suite branch of `run()`, but `main_test.go`'s `TestRun_SuiteRoutesSuiteToLaunch` (line 340) calls `run([]string{"-parent", tmpDir, "suite"})` with no `-loomyard` — that test will now return 1 and fail, and the batch verify (`go test ./tools/sandbox/...`) runs it. No card touches `main_test.go`.

MILL_REVIEW_BEGIN
# Review: Sandbox suite: emit findings JSON on the shared analysis contract — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-30
```

## Findings

### [BLOCKING] main_test.go breaks; not in any card's Edits
**Location:** Batch 1 / Card 2 (and Card 7)
**Issue:** Card 2 makes `-loomyard` required in the `suite` branch of `run()`, but `tools/sandbox/main_test.go:340` `TestRun_SuiteRoutesSuiteToLaunch` calls `run([]string{"-parent", tmpDir, "suite"})` with no `-loomyard`, so it now returns 1 and fails the batch verify (`go test ./tools/sandbox/...` runs main_test.go); `main_test.go` is in no card's Edits and absent from "All Files Touched".
**Fix:** Add `tools/sandbox/main_test.go` to Card 2 (or 7) Edits and to "All Files Touched"; update that test to pass `-loomyard <t.TempDir()>` and add the host `.git/info` exclude/report expectations as needed.

### [NIT] No coverage for the missing-`-loomyard` error path
**Location:** Batch 1 / Card 2
**Issue:** The new "`-loomyard is required`" return-1 branch in `run()` has no test, so a regression dropping the guard goes unnoticed.
**Fix:** Add a `run([]string{"-parent", tmpDir, "suite"})` case asserting non-zero exit and that `launchAgent` is not called, in the same main_test.go edit.

### [NIT] SANDBOX-SUITE.md intro prose still says "bug to file"
**Location:** Batch 1 / Card 4
**Issue:** Card 4 enumerates sections to rewrite but omits the "## What this is" intro (`SANDBOX-SUITE.md:8`, "treating every break … as a LoomYard bug to file"), which still implies the old file-an-issue mechanism.
**Fix:** Have Card 4 also reword the intro to reflect recording findings in `sandbox-report.json` rather than filing.

## Verdict

REQUEST_CHANGES
One card breaks an existing test the batch verify runs; fix the untracked main_test.go.
MILL_REVIEW_END
