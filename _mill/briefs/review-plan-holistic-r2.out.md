MILL_REVIEW_BEGIN
# Review: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-08
```

## Findings

### [BLOCKING] Card 5 Context missing load.go for named scoutengine.LoadRegistry
**Location:** batch 01-location-threading, card 5 **Issue:** Requirements names `scoutengine.LoadRegistry` and asserts a specific claim about its error semantics ("Its returned error carries scoutengine.LoadRegistry failures only"), but `LoadRegistry` is defined in `internal/scoutengine/load.go`, which is not listed in card 5's `Context:` (`lyxcwd.go`, `refs.go`, `registry.go`) or `Edits:` (`scoutcli/cli.go`). **Fix:** Add `internal/scoutengine/load.go` to card 5's `Context:` list.

## Verdict

REQUEST_CHANGES
Card 5 names `scoutengine.LoadRegistry` without listing its defining file (`load.go`) in Context, violating Context completeness.
MILL_REVIEW_END
