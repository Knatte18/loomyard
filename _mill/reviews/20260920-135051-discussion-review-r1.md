# Review: Producer gates: mechanical gates before session release

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewer_self_id: claude-fable-5
reviewed_file: _mill/discussion.md
date: 2026-09-20
```

## Findings

### [NIT:consistency] GateOutcome.FindingsPath dangles after finalize
**Section:** Contract shape / Findings always ride a file
**Issue:** `finalize` deletes the run directory on Done cleanup, so on exhaustion the `FindingsPath` the producer receives in `Result` already points at a deleted file; the contract comment ("the findings file of the LAST failing attempt") doesn't say so.
**Suggested fix:** Document `FindingsPath` as informational-only (log text, never dereferenced after `Wait` returns), or clear it on cleanup.

### [NIT:design] Budget's route into the Burler sites is unstated
**Section:** The four sites and how each gets its gate
**Issue:** `Gate` is `func() (GateResult, error)` with no budget, and `burlerengine.RunOpts` is said to gain only `Gate` — how `gate_attempts` reaches the widened `burlerengine.Shuttle`/`shedadapters.Shuttle` seams is left implicit.
**Suggested fix:** One sentence naming the carrier (e.g. the gated seam takes gate + budget together, or `RunOpts` gains both fields).

## Verdict

APPROVE
Decisions verified against the code (deny-hook, ForkSubagents wiring, Send contract, finalize, 17→14 rows); narrowings are grounded and recorded.
