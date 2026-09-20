MILL_REVIEW_BEGIN
# Review: Producer gates: mechanical gates before session release

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-09-20
```

## Findings

### [BLOCKING:design] Gate insertion point vs `finalize` unnamed
**Section:** "A Done with no live session still runs the gate" / Technical context
**Issue:** The decision enumerates five/six "call sites the gate must route through", but `classifyStartupWindow` (wait.go:456) and `classifyDeadlineExpiry` (wait.go:486) return an `Outcome`, not a `Result`, and cannot host a gate; every Done in fact converges on four `run.finalize` calls (wait.go:222, :249, :258, and :518 inside `finishedDespiteMechanismFailure`), and the discussion never says whether the hook sits inside `finalize` or at each of those callers.
**Fix:** State the insertion point explicitly against `finalize` — which sites re-enter the poll loop after `Send` (the events-tick Done alone) and which return immediately with `Attempts` as-is — so the plan does not target classifier functions that structurally cannot carry it.

### [NIT:consistency] `ParsePlan` has two read faults, not one
**Section:** "`error` is never 'not passed'" — the `ParsePlan` carve-out
**Issue:** The doc says `ParsePlan` "wraps its one genuine read fault with `%w`"; it wraps two — the overview read (parse.go:106/111) and the per-card read reached through `parseCardFile` (parse.go:152 → :319/:324). The `errors.As(*fs.PathError)` rule holds for both, so the disposition is unchanged, but the count and the "when and only when" phrasing are wrong.
**Fix:** Say "every `%w`-wrapped `os.ReadFile` fault in `ParsePlan` (overview and card file)", and add the card-file read fault to `NewPlanGate`'s test matrix.

### [NIT:design] Gate repair leaves a stale round review/fixer report
**Section:** "The four sites and how each gets its gate" — `Plan-Burler`/`Discussion-Burler`
**Issue:** A burler round writes `ReviewPath`/`FixerReportPath` before its gate runs; a failed gate re-prompts, the agent repairs the artifact, the gate passes, and `Engine.Run` then parses a review/report written against the pre-repair artifact. The one-line re-prompt says nothing about refreshing either.
**Fix:** Record the disposition — either accept the stale report explicitly as harmless, or have the re-prompt line state that the round's report must reflect the repair.

## Verdict

REQUEST_CHANGES
Gate insertion point relative to `finalize` must be named before planning.
MILL_REVIEW_END
