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

### [BLOCKING] Card 44 targets a scoutengine teardown site that does not exist
**Location:** batch 13 (scoutengine-logger-conversion), Card 44
**Issue:** Card 44 asks the implementer to "locate this package's normal-exit teardown path for the supervised daemon... its clean-shutdown counterpart to the wedged-kill escalation" and add a matching `logger.Info`. I read `ensureserver.go` in full: the only `proc.KillPID` call is the wedged-daemon escalation already converted by Card 43 (line 465); the function's own doc comment states the daemon "ends on its own: its own idle timeout... or a future restart's stale-socket cleanup finding it already dead" — i.e. scoutengine's own code never observes a clean daemon exit. `refs.go`'s `teardownConnection`'s `connKindSupervised` branch is a bare `return` (deliberately does nothing, by design, since the daemon must outlive the call) — not a teardown event either. No such call site exists anywhere in the package.
**Fix:** Drop the "normal-exit teardown" half of Card 44 (there is nothing to instrument — the daemon's exit is invisible to scoutengine by design), or explicitly redefine "both halves of the lifecycle" as spawn (Card 44) + wedged-kill (already Card 43's Warn), and say so, rather than instructing the implementer to find a call site the codebase does not have.

### [BLOCKING] Card 37's run.go:447-448 citation contradicts its own "retries once" framing
**Location:** batch 10 (treadleengine-adoption), Card 37, first bullet
**Issue:** The card describes "a died/timeout/failed `RunAttempt` call (which the caller retries once)... before the retry proceeds" and cites `run.go:447-448`. Actual lines 447-448 are `if err != nil { return roundOutcome{}, e.errf("round %d attempt run: %w", round, err) }` — the `RunAttempt` call's own Go-error path, which returns immediately with **no retry at all**, on any attempt number. The branch that actually retries once (died/timeout outcome, silent fall-through to attempt 2 when `attempt==1`) is at lines 486-490, uncited. Additionally, the cited 447-448 error is already wrapped with `"round %d attempt run: %w"`, which per `adoption-scope`'s negative case (context already names round) may not even qualify for a new call.
**Fix:** Point the "before the retry proceeds" Warn at the died/timeout branch (~486-490, the attempt==1 fall-through case), not 447-448; re-evaluate whether 447-448 independently qualifies given it already carries `round` context.

### [NIT] cmd/lyx/main.go line citations are stale by ~13 lines
**Location:** batch 7 (cmd-lyx-root-wiring), Cards 27 and 28
**Issue:** Card 27 cites `PersistentPreRunE`'s current body at "lines 82-85" (actual: 95-98); Card 28 cites `main()` at "lines 39-44" (actual: 39-48) and `run()` at "lines 46-52" (actual: 55-63). `newRoot()`'s own span (70-133) is correct. Every code snippet quoted alongside these citations is exact, so this is self-correcting via search, but stands out given the plan's otherwise verified-accurate line citations everywhere else (reedengine, treadleengine, scoutengine, perchengine all checked exact).
**Fix:** Refresh the three line-range citations against the current file.

## Verdict

REQUEST_CHANGES
Two BLOCKING citation/target defects (a phantom scoutengine teardown site, a mismatched retry-path citation) need correction before implementation.
MILL_REVIEW_END
