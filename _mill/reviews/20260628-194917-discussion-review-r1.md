MILL_REVIEW_BEGIN
# Review: Sandbox test-suite launcher and task harvester

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\sandbox-suite\_mill\discussion.md
date: 2026-06-28
```

## Findings

### [NOTE] `gh` precondition omitted from black-box inputs
**Section:** Technical context / Constraints (black-box)
**Issue:** The in-scope scheme refresh replaces the capture step with `lyx ghissues create`, but `internal/ghissues/cli.go` shells out to the `gh` CLI (targeting `Knatte18/loomyard`); the black-box input list says the agent gets "only `lyx` (PATH) + the copied scheme."
**Fix:** State that `gh` (installed + authenticated) is a Hub precondition in the scheme/launcher, else findings cannot be filed.

### [NOTE] "Propagate exit code" collapses under `go run`
**Section:** Scope / Decisions (Interactive TUI)
**Issue:** `sandbox.cmd` runs the tool via `go run`, which flattens any non-zero child exit to status 1, so the exact claude exit code cannot survive the wrapper despite "wait for it and propagate its exit code."
**Fix:** Clarify that propagation is best-effort (zero vs non-zero) under `go run`, or document the limitation.

### [NOTE] Hub-missing check target unspecified
**Section:** Scope / Testing (Hub-missing error)
**Issue:** "no Hub host repo present" does not say whether `suite` stats the Hub dir (`lyx-test-HUB`) or the host subdir (`lyx-test-HUB/lyx-test`); `cloneHub` puts the host clone (with a real `.git` dir) at the subdir, which is the write/cwd target.
**Fix:** Specify the check is on the host subdir (`<parent>/lyx-test-HUB/lyx-test`), matching the `hostDirName` const.

## Verdict

APPROVE
Thorough and decided; only minor clarifications, none blocking plan writing.
MILL_REVIEW_END
