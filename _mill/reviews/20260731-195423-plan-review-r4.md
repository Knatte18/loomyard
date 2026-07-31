MILL_REVIEW_BEGIN
# Review: Diagnostic tracing (trace) on the logger module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-07-31
```

## Findings

### [BLOCKING] Context-completeness gaps: functions cited from unlisted files
**Location:** batch 4 Card 10; batch 8 Card 33; batch 10 Card 37
**Issue:** Card 10's Requirements cite `configureFromEnv`'s `LYX_LOG_FILE` open flags at `internal/logger/logger.go:86-94`, but `logger.go` is not in Card 10's `Context:`/`Edits:` (only hubgeometry.go, retention.go). Card 33's Requirements call `CleanClaudeEnv` (defined in `internal/reedengine/env.go`) with `Context: none`. Card 37's Requirements cite `moveStaleIfExists` at `internal/treadleengine/state.go:204,213` with no Context and `state.go` absent from Edits. Per the Context-completeness rule this is BLOCKING regardless of whether the cited detail is also paraphrased inline.
**Fix:** Add the missing file to each card's `Context:` list (`logger.go` for Card 10, `env.go` for Card 33, `state.go` for Card 37).

### [BLOCKING] Card 37 mischaracterizes which error is swallowed at run.go:224-227
**Location:** batch 10, Card 37, first bullet (`run.go:224-227`)
**Issue:** The card states "`saveErr`... is swallowed... the `saveErr` itself never surfaces anywhere, logged or otherwise." Actual code: `if saveErr := saveState(runDir, st); saveErr != nil { return Result{}, saveErr }` (line 225) — `saveErr` IS returned directly. It is the original `err` (the gate-command failure being handled) that is silently discarded in that branch, since execution never reaches line 227's `e.errf("round %d gate command: %w", round, err)` when saveErr fires. The card has the two errors backwards.
**Fix:** Reword the card to log the case where `saveState` itself fails while an original gate-command error is in flight, naming both `err` and `saveErr` (not just `saveErr`) so the discarded value is actually the one the log call names.

### [BLOCKING] Retention liveness test suggests a Test-Tier-Purity-violating alternative
**Location:** batch 3, Card 8, second bullet
**Issue:** For a guaranteed-dead PID, the card offers "a very large PID unlikely to be alive, **or** a PID obtained by spawning and immediately waiting a short-lived helper process." The latter would put `exec.Command`/`exec.CommandContext` as a raw substring in `internal/logger/retention_test.go`, an untagged file — a hard failure under the Test Tier Purity Invariant's banned-substring guard (`TestTierPurity_UntaggedTestsSpawnNothing`).
**Fix:** Drop the spawn alternative; mandate only the large-PID-unlikely-to-be-alive approach (or an explicit `//go:build integration`-tagged sibling file if certainty is truly required).

### [NIT] spawn.go line citation off by 3
**Location:** batch 11, Card 38, `internal/fabricengine/spawn.go` bullet
**Issue:** Cites `spawn.go:62` for `return cmd.Start() // intentionally not Wait()ed`; that line is actually `cmd := exec.Command(exe, args...)`. The real `return cmd.Start()` line is 65.
**Fix:** Correct the citation to `spawn.go:65`.

### [NIT] main() line range excludes closing brace
**Location:** batch 7, Card 28
**Issue:** Cites `main()` as "lines 39-47"; the function body actually runs 39-48 (line 48 is the closing brace).
**Fix:** Update to "lines 39-48" (cosmetic; does not affect the instructed edit).

## Verdict

REQUEST_CHANGES
Two context-completeness gaps, one backwards error-flow claim in Card 37, and a Tier-Purity risk in Card 8 need fixing.
MILL_REVIEW_END
