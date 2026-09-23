MILL_REVIEW_BEGIN
# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-09-23
```

## Findings

### [BLOCKING:design] Smoke test never asserts start's exit code
**Location:** Batch 2 / Card 4 (and Batch 2 "Batch Tests"). **Issue:** `runLoomCLINoFatal` (`internal/loomcli/smoke_test.go`) returns `err == nil` for a non-zero exit, and `TestSmokeDriverStrand_ReentrantAcrossThreeBootstraps` discards the exit code (`firstOut, _, err`). A refusal (`driver strand did not come up`) still leaves a live strand in place (the "Not-ready refusal leaves the strand in place" decision), so the count/live assertions pass on a readiness regression. Lowering `startup_timeout_s` to 10 turns that regression from a 30s-timeout failure into a silent pass. The card's claims that the test "proves the llm arm's readiness await succeeds" and that a regression surfaces "as a refusal envelope" are therefore unbacked. **Fix:** require Card 4 to assert exit code 0 on the first and third `loom start --no-attach` (and the second), failing with the captured output otherwise.

### [NIT:scope] Card 2 ready-path test leaves the startup window unspecified
**Location:** Batch 1 / Card 2, "pending → trust prompt → ready". **Issue:** unlike the undismissable and tick-cap cases, this case names no `Config`. Under a zero `StartupTimeoutS`, `fakeClock` advances one interval after tick 1, so `classifyStartupWindow` (`wait.go`) answers `OutcomeDied` on the trust-prompt tick and the test returns `(false, nil)`. **Fix:** give the case an explicit window, for example `StartupTimeoutS: 1, PollIntervalMS: 600`, as `TestRun_Wait_Died_ViaStartupTimeout_TrustDismissRecorded` uses.

### [NIT:consistency] wait.go header's "only place that sleeps" claim goes stale
**Location:** Batch 1 / Card 1, file header edit. **Issue:** the header states "Wait is the only place in the run loop that sleeps". `AwaitStarted` now sleeps too, and the card only appends a sentence. **Fix:** have the card also reword that sentence to cover `AwaitStarted`.

### [NIT:consistency] Call-site tally prose in the tripwire goes stale
**Location:** Batch 1 / Card 1, `completionsignal_enforcement_test.go`. **Issue:** `auditedFileContractCallSites`'s doc comment opens with "Eight sites: …". The card updates only the "as of" phrasing and adds a justification sentence, so the tally goes wrong once `"AwaitStarted": 2` lands. **Fix:** tell the card to update or drop the tally in the same edit.

## Verdict

REQUEST_CHANGES
The smoke test cannot detect a readiness refusal, so the live success path stays unproven.
MILL_REVIEW_END
