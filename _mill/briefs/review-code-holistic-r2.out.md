MILL_REVIEW_BEGIN
# Review: Move config templates home by removing the lyxtest->configreg edge — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-25
```

## Findings

### [NIT] template_test.go files not listed in plan's All Files Touched
**Location:** `internal/board/template_test.go`, `internal/worktree/template_test.go`, `internal/weft/template_test.go`
**Issue:** Three `template_test.go` files exist on disk and are not listed in the plan's `## All Files Touched` or in any card's `Creates:`/`Edits:` list; they are referenced only in batch 2's `## Batch Tests` section as expected key tests.
**Fix:** If pre-existing (the most likely reading, since the batch tests section treats them as already existing), add them to `00-overview.md`'s `## All Files Touched` as pre-existing context; if created by this PR, add them to card 7/8/9's `Creates:` list in the batch file.

## Verdict

APPROVE
All three batches are faithfully implemented: leaf invariant enforced, configtmpl fully deleted, configreg re-pointed to features, SeedConfig correct, all seeding call sites updated, `_lyx`/config paths routed through helpers throughout.
MILL_REVIEW_END
