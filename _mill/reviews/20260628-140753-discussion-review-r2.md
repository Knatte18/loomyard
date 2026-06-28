I have verified the load-bearing claims. All eight `Command()` packages found are registered in `main.go` (no orphans), confirming Test A is feasible. Two minor citation inaccuracies surfaced. Writing the review.

MILL_REVIEW_BEGIN
# Review: ghissues module — file LoomYard bugs as GitHub issues

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\ghissues-module\_mill\discussion.md
date: 2026-06-28
```

## Findings

### [NOTE] LookPath citation names wrong binary
**Section:** Technical context — Binary discovery
**Issue:** It cites `exec.LookPath("gh")` as the "established pattern" at `internal/muxpoc/review.go:47` and `up.go:77`, but those exact lines use `exec.LookPath("claude")`, not `"gh"`.
**Fix:** Reword to "LookPath-with-graceful-error pattern (see `muxpoc/review.go:47` for the `claude` precedent)"; the pattern itself is correctly located.

### [NOTE] jsonhelp_test guard reach overstated
**Section:** Scope — Rich `--help` (lines 51-54)
**Issue:** It states a missing `Short` "also fails the JSON-help guards in `cmd/lyx/jsonhelp_test.go`," but those tests hardcode `board`/`warp remove` and do not walk all modules; only `drift_test.go`'s recursive walk catches a missing `ghissues` Short automatically.
**Fix:** Attribute the generic Short guard to `drift_test.go`; the jsonhelp coverage for `ghissues` is one the plan must add (already correctly listed in the Testing section).

## Verdict
APPROVE
Decisions, scope, failure modes, and tests are all resolved; only two minor citation inaccuracies remain.
MILL_REVIEW_END