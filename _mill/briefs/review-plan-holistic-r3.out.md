MILL_REVIEW_BEGIN
# Review: reed: header pane's boot sometimes leaves shell/log noise in its scrollback — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-4.5 (Claude Sonnet, distinct from the dictated "sonnetxhigh" label)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

### [BLOCKING:scope] Card 10 names a test function from a file outside its Context
**Location:** batch 2 / card 10 **Issue:** Requirements says "Re-run card 7's `TestSmokeHeaderDeclinesStencilSeedPass` to confirm it is now green," naming a specific test function defined in `internal/reedcli/smoke_headerseed_test.go`, but that file is not listed in card 10's `Context:` (`internal/clihelp/annotations.go`, `cmd/lyx/stencilseed.go`) or `Edits:` (`internal/reedcli/header.go`). **Fix:** Add `internal/reedcli/smoke_headerseed_test.go` to card 10's `Context:` list.

## Verdict

REQUEST_CHANGES
One minor Context-completeness gap in batch 2 card 10; otherwise the plan is sound, decision-aligned, and well-sequenced.
MILL_REVIEW_END
