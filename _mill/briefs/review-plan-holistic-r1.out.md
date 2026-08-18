MILL_REVIEW_BEGIN
# Review: the standalone CLI path — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5)
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [BLOCKING:scope] Card 13's Context omits burlerengine.New's declaring file
**Location:** Batch 4, Card 13 (`internal/burlercli/wiring.go`).
**Issue:** Card 13's Requirements call `burlerengine.New(runner, hubgeom.BurlerGeometry(loc), burlerCfg, stencilsDir)` and `burlerengine.New(runner, standalonegeom.BurlerGeometry(target, stateDir), burlerCfg, stencilsDir)`, but `burlerengine.New`'s signature is declared in `internal/burlerengine/engine.go`, which is not in Card 13's `Context:` (only `geometry.go` and `config.go` are listed). No other Card-13 context file shows this call shape — `internal/webstercli/wiring.go` (the file it mirrors) never calls `burlerengine.New`, since webster has no burler dependency — so the implementer has no source for the constructor's parameter order or types. Card 20 (perchcli's equivalent, same call) correctly includes `internal/burlerengine/engine.go` in its own Context, confirming the omission here is a gap rather than a deliberate choice.
**Fix:** Add `internal/burlerengine/engine.go` to Card 13's `Context:` list.

## Verdict
REQUEST_CHANGES
One Context-completeness gap in batch 4 card 13; the DAG, decisions, and every other batch check out.
MILL_REVIEW_END
