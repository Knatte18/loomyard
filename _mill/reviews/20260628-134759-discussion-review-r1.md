I have verified the discussion's technical claims against source. The line references (warp.go:88-110, clihelp/exec.go:108-118, gitexec.go:15-38, output.go, main.go import block 22-30 / AddCommand 94-103 / Long line 83) are all accurate. The decisions are well-supported. A few testability/implementation details are worth recording but none block planning.

MILL_REVIEW_BEGIN
# Review: ghissues module — file LoomYard bugs as GitHub issues

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-28
```

## Findings

### [NOTE] runGH return contract left unspecified
**Section:** Decisions / gh-runner-seam + Testing
**Issue:** Testing requires distinguishing "gh not found" from "gh non-zero exit" and surfacing gh's stderr, but the seam decision names only `var runGH`/`var stdin` without a return shape, so the handler's ability to tell the two cases apart is unstated.
**Fix:** Specify `runGH` mirrors gitexec's `(stdout, stderr, exitCode, err)` tuple with LookPath failure surfaced as `err`/exit -1, so the handler can branch and build distinct messages.

### [NOTE] Label "replace default" depends on pflag semantics
**Section:** Decisions / labels-default-bug + Testing (Custom labels)
**Issue:** "Supplied `--label` flags replace the default `["bug"]`" only works because pflag resets a slice flag on first `Set`; a naive impl that pre-seeds and appends would yield `["bug","enhancement"]`, failing the test.
**Fix:** State the flag is a `StringArray` with default `["bug"]` (not `StringSlice`, which CSV-splits values) and that replacement relies on pflag's first-Set-replaces behaviour.

### [NOTE] Existing help tests won't enforce registration
**Section:** Testing (Registration smoke)
**Issue:** `cmd/lyx/helptree_test.go` and `jsonhelp_test.go` assert required-modules ⊆ output (superset), so they pass whether or not `ghissues` is registered — the smoke coverage is vacuous unless updated.
**Fix:** Have the plan add `"ghissues"` to the `requiredModules` lists in both test files so registration is actually asserted.

## Verdict
APPROVE
Thorough and source-accurate; all decisions justified, only minor implementation/testability details to record.
MILL_REVIEW_END