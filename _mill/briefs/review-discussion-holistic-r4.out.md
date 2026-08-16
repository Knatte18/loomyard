MILL_REVIEW_BEGIN
# Review: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: /home/knatte/Code/loomyard/wts/shed-adapters/_mill/discussion.md
date: 2026-08-16
```

## Findings

### [BLOCKING:design] WebsterProducer's OutputPointer never decided
**Section:** "Webster adapter — `Fresh` fixed false…" + Testing → `WebsterProducer`
**Issue:** The other two adapters pin their pointer explicitly (`OutputFiles[0]`; empty for perch), but the Webster mapping states only `done` → `Done` and never says what `OutputPointer` accompanies it — and both candidates are live, since `RunResult` carries no path (`runlevel.go:150-166`) while `websterengine.SummaryPath(websterDir)` (`summary.go:27`) and `OutcomePath` (`runlevel.go:77`) exist and `RunDeps.WebsterDir` is told.
**Fix:** Add a Decision fixing Webster's `Done` pointer (empty, or `SummaryPath(deps.WebsterDir)`) with rationale, and a matching Testing assertion as for the other two.

### [BLOCKING:consistency] Scope and Q&A still claim a bridge for all three engines
**Section:** Scope "In" bullet 6; Q&A log entry "How is context cancellation handled…"
**Issue:** Both say the ctx bridge goes "into each engine's existing pause seam", which the later Decisions reverse — the bridge is perch-only, with Webster and shuttle explicitly bridgeless; a plan writer enumerating work from Scope would build three bridges.
**Fix:** Reword both to "entry/exit checks for all three, a mid-run bridge for perch only", matching the three cancellation Decisions.

### [NIT:design] `TerminalOutcome`'s error return has no stated disposition
**Section:** "Perch run identity — a run-id that advances only past a terminal block"
**Issue:** `perchengine.TerminalOutcome` returns `(Outcome, bool, error)` (`identity.go:99`) and errors on an unreadable/corrupt `state.json` (a missing file is `("",false,nil)`), but the Decision consumes only the terminal/non-terminal answer and never says whether a probe error fails the `Call` or is treated as non-terminal; the discovery method for "the highest existing `<prefix>-<N>`" is likewise unstated.
**Fix:** State that a `TerminalOutcome` error propagates as the adapter's error, and name the scan rule used to find the current N.

### [NIT:consistency] "Cheap idempotent re-check" no longer holds for perch
**Section:** "Perch adapter — outcome mapping and empty `OutputPointer`" vs. "Perch run identity"
**Issue:** The empty-pointer rationale leans on shed.md:29's gate producer being re-run on resume as a cheap idempotent re-check, but the run-id Decision makes a post-`APPROVED` re-call mint `<prefix>-<N+1>` — a full fresh burler ladder, not a cheap re-check.
**Fix:** Note the accepted cost in the perch mapping Decision so the two Decisions read consistently.

## Verdict

REQUEST_CHANGES
Webster's output pointer is undecided; Scope and Q&A contradict the perch-only bridge decision.
MILL_REVIEW_END
