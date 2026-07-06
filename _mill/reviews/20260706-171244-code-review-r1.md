MILL_REVIEW_BEGIN
# Review: Add Effort to shuttle's run Spec — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-06
```

## Findings

### [BLOCKING] Claude marker string leaks into provider-invariant files
**Location:** `internal/shuttleengine/engine.go:97` and `internal/shuttleengine/wait.go:147`
**Issue:** `engine.go`'s `EventAsk` doc comment reads "a provider tool call (e.g. Claude's AskUserQuestion)" and `wait.go`'s `pollEventsTick` doc comment reads "a live AskUserQuestion tool call classify as a real-time asking" — both are in `internal/shuttleengine`, not `claudeengine`. This is exactly the leak card 6 explicitly warned against ("no `Stop`/`AskUserQuestion` Claude marker strings leak into this provider-invariant file") and the overview's Shared Decision "provider-seam containment" explicitly lists "the literal `AskUserQuestion` tool name" as Claude-specific vocabulary that must live ONLY under `claudeengine`. This is the Shuttle Provider-Seam Invariant's semantic half (a review obligation per `CONSTRAINTS.md`), and it was violated in two of this batch's own new/edited doc comments.
**Fix:** Reword both comments to stay provider-neutral, e.g. "a live, in-progress tool-call signal the engine surfaces (see `claudeengine`'s `ParseEvents` for the concrete provider mapping)" — drop the literal `Claude`/`AskUserQuestion` names from `engine.go` and `wait.go`.

## Verdict

REQUEST_CHANGES
Two doc comments in `engine.go`/`wait.go` leak the Claude-specific `AskUserQuestion` marker into provider-invariant files, violating the seam invariant.
MILL_REVIEW_END
