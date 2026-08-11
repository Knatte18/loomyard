MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Anthropic), self-assessed
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:scope] UnwireJunctions containment surface cited to the wrong file, and `junction.go` missing from Context
**Location:** 00-overview.md (Decision: cell-enumeration-and-omissions), batch 4 card 13, batch 6 card 16
**Issue:** All three places state "its only gate call is `removeLink` at `unwire.go:143-152`" for `UnwireJunctions`'s hostile-input containment case. Verified against source: `UnwireJunctions` is defined in `internal/fabricengine/junction.go:368`, which calls `unseedLyxJunction` → `unseedJunctionRecords` (`junction.go:420-487`), whose `removeLink(req)` gate call is at `junction.go:483` (pathRequest built `junction.go:474-482`). `unwire.go:143-152` is a different function entirely (`unwireBoardLink`, the `_board`-junction-specific teardown), which `UnwireJunctions` never calls — confirmed by `TestUnwireJunctions_RefusesLinkOutsideItsWorktree` in `destructivegaps_integration_test.go`, which drives `fabricengine.UnwireJunctions(l, slug, []string{"../gap1-escape"})` directly. Because of the wrong citation, `internal/fabricengine/junction.go` is omitted from card 13's and card 16's `Context:` lists even though both cards' `Requirements:` name `UnwireJunctions` and rely on understanding its gate call — an implementer restricted to `Context:` cannot correctly write the hostile `UnwireJunctions` refusal case (card 13) or correctly justify the omission-table row for it (card 16) from the files given.
**Fix:** Correct the citation to `junction.go:474-483` (or the `unseedJunctionRecords`/`UnwireJunctions` function names) in all three locations, and add `internal/fabricengine/junction.go` to card 13's and card 16's `Context:` lists.

## Verdict

REQUEST_CHANGES
One well-evidenced Context/citation gap around `UnwireJunctions`; otherwise the plan's line-level source claims verified exceptionally accurately.
MILL_REVIEW_END
