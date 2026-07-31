MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (claude-sonnet-4-5), invoked here under the name "Sonnet 5"/claude-sonnet-5
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [BLOCKING] Card 44 cites refs.go internals not in its Context/Edits
**Location:** batch 13 (`13-scoutengine-logger-conversion.md`), Card 44
**Issue:** Card 44's Requirements justifies "no separate teardown call site" by citing `refs.go`'s `teardownConnection`'s `connKindSupervised` branch at lines 146-151 (a bare `return`) — but Card 44's Context is `none` and Edits is only `internal/scoutengine/ensureserver.go`; `refs.go` is not listed. I verified the citation is factually accurate (confirmed against source), but per the plan's own Context-completeness rule the implementer is entitled to read only Context/Edits files, so this reasoning is uncheckable cold-start. `refs.go` happens to be listed in Card 41's Context (same batch) but not Card 44's.
**Fix:** Add `internal/scoutengine/refs.go` to Card 44's `Context:`.

### [NIT] Adoption-scope criterion applied inconsistently in batch 10
**Location:** batch 10, Card 37
**Issue:** Card 37 explicitly excludes `run.go:447-448` from a Warn because "that error already names round in its wrapped message" (negative case of the stop-rule), yet includes `run.go:426-429` (`moveStaleArtifacts` failure) even though that error is itself already wrapped with path/operation context by `moveStaleArtifacts`/`moveStaleIfExists` (`state.go:204,213`). The card also omits the near-identical first `moveStaleArtifacts` call at `run.go:183` from its candidate list entirely.
**Fix:** Either drop 426-429 as "already named," or explicitly justify why it differs from 447-448 and add 183 to the candidate list (or note the omission is deliberately left to the implementer's own re-audit, which the card's "floor not ceiling" language already partially covers).

### [NIT] Inconsistent rigor on an identical bare-`cmd.Start()` pattern
**Location:** batch 9 Card 34 (reedengine `lifecycle.go:357-359`) vs. batch 11 Card 38 (fabricengine `spawn.go:62`)
**Issue:** Both are a bare, unwrapped `cmd.Start()` error return for a detached spawn. Card 34 mandates a `logger.Warn` unconditionally; Card 38 only asks the implementer to "confirm whether it is already logged... add a call only to a site that genuinely qualifies," despite `spawn.go:62`'s `return cmd.Start()` carrying zero identifying context (no exe/args wrap) — the same shape Card 34 treats as an automatic qualifier.
**Fix:** State plainly in Card 38 that `spawn.go:62` qualifies (bare `cmd.Start()`, no wrap) and require the `logger.Warn`, for consistency with Card 34's treatment of the identical pattern.

### [NIT] `identity.go`'s `TerminalOutcome` has no branch to attach a Warn to
**Location:** batch 12, Card 39
**Issue:** `internal/perchengine/identity.go:100-103`'s `TerminalOutcome` is a single unconditional `return Outcome(outcome), ok, err` with no existing `if err != nil` check — Card 39's Requirements ("Add `logger.Warn` naming `runDir` and the error") doesn't note that an error-check branch must first be introduced, only that a "call" is added.
**Fix:** Note in Card 39 that this site requires introducing an `if err != nil { ... }` branch before the log call, not just inserting a call into existing branching.

## Verdict

REQUEST_CHANGES — one Context-completeness gap (Card 44/refs.go) plus minor stop-rule application inconsistencies across adoption batches.
MILL_REVIEW_END
