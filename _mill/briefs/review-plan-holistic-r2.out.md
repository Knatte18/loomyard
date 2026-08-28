MILL_REVIEW_BEGIN
# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:scope] Card 3's Context omits lock.go, whose field it edits into lifecycle.go
**Location:** Batch 1, Card 3 (`internal/reedengine/lifecycle.go`)
**Issue:** Requirements instructs writing `headerLaunchLine(shell.ForGOOS(), exe, e.suppressHeaderLaunch)`, referencing the `suppressHeaderLaunch` field Card 1 adds to `Engine` in `internal/reedengine/lock.go` — but Card 3's `Context:` list is only `spawn.go` and `headerpane.go`, and its `Edits:` is only `lifecycle.go`. `lock.go` (the field's defining file) is absent from both, so the implementer is cold-start on the field's existence/type per the Context-completeness rule. Cards 2 and 4 both correctly include `lock.go` in Context for the identical reference, which is why this omission in Card 3 reads as a slip rather than a deliberate choice.
**Fix:** Add `internal/reedengine/lock.go` to Card 3's `Context:` list.

## Verdict

REQUEST_CHANGES
Card 3's Context list omits lock.go despite referencing the Engine field it defines.
MILL_REVIEW_END
