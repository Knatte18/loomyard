MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5
reviewed_file: plan/
date: 2026-08-18
```

## Findings

### [BLOCKING:scope] Card 28's AppendIntegrationFailure call site not in Context
**Location:** batch `07-webster-told-deps.md`, card 28 ("Record an unlocalized integration failure when there is no bisector")
**Issue:** Requirements direct a new call `AppendIntegrationFailure(deps.Geom.WebsterDir, "unknown", "unknown")` inside the nil-`OpenBisector` branch, but `AppendIntegrationFailure` is declared in `internal/websterengine/summary.go`, which is absent from card 28's `Context:`/`Edits:` (only `integration.go`, `geometry.go`, `recordbatch.go` are listed). `integration.go`'s own `BisectAndEscalate` calls it with the identical argument shape, which softens the gap but doesn't satisfy the rule as written.
**Fix:** Add `internal/websterengine/summary.go` to card 28's `Context:` list.

### [BLOCKING:scope] Card 33's FabricBisector type not in Context
**Location:** batch `07-webster-told-deps.md`, card 33 ("Wire webstercli onto the told Deps in hub mode")
**Issue:** Requirements say `RunDeps` gets "an `OpenBisector` closure that calls `c.openFabric` and returns its result as a `websterengine.FabricBisector`," but `FabricBisector` is declared in `internal/websterengine/integration.go`, which is not in card 33's `Context:`/`Edits:` (`runlevel.go` is listed and does show the field's declared type from card 27, which mitigates but doesn't fully satisfy the rule).
**Fix:** Add `internal/websterengine/integration.go` to card 33's `Context:` list.

### [NIT:consistency] Card 27's "Both FindRun calls" sentence is ambiguous
**Location:** batch `07-webster-told-deps.md`, card 27 ("RunDeps carries a Geometry, a RefMatcher and an opener")
**Issue:** "Both `shuttleengine.FindRun` calls pass `deps.Geom.AnchorRoot`" reads as a claim about `runlevel.go` (card 27's sole `Edits:` file), but that file contains exactly one `FindRun` call (confirmed by source read); the second call lives in `recoverbatch.go`, already converted by card 26. An implementer could search `runlevel.go` for a second call site that doesn't exist.
**Fix:** Reword to something like "The `shuttleengine.FindRun` call in this file passes `deps.Geom.AnchorRoot`, mirroring card 26's identical change to the other call site in `recoverbatch.go`."

## Verdict

REQUEST_CHANGES
Two card `Context:` gaps (summary.go, integration.go) plus one ambiguous cross-file sentence; otherwise the plan is precise and fully source-grounded.
MILL_REVIEW_END
