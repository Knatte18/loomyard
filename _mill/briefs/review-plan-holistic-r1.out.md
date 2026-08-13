MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-12
```

## Findings

### [BLOCKING:consistency] Batch 8 frontmatter card count wrong
**Location:** `08-fabricengine-external.md` frontmatter (`cards: 13`) **Issue:** The file contains exactly 11 cards, numbered 42–52 (verified by counting every `### Card` heading); there is no card 53+ in this file, and batch 9 correctly resumes numbering at 53. **Fix:** Correct the frontmatter to `cards: 11`.

### [BLOCKING:consistency] Batch 9 frontmatter card count wrong
**Location:** `09-fabricengine-inpackage-weft.md` frontmatter (`cards: 11`) **Issue:** The file contains exactly 10 cards, numbered 53–62; batch 10 correctly resumes at 63, so the global 1–76 numbering is intact, but this batch's own header overstates its card count by one. **Fix:** Correct the frontmatter to `cards: 10`.

### [NIT:consistency] Batch 4 scope prose overstates its site counts
**Location:** `04-small-consumers.md`, `## Batch Scope` ("nineteen `Copy*` sites and fifteen `SeedConfig` sites") **Issue:** Both the batch's own 7 cards' per-file counts and a direct `grep -c` over the seven target files sum to 17 `Copy*` sites and 14 `SeedConfig` sites, not 19/15 — the aggregate totals used later in the plan (batch 10's "132 migrated", the overview's "56 SeedConfig sites") are computed from the correct 17/14, so only this one summary sentence is off. **Fix:** Correct the sentence to "seventeen `Copy*` sites and fourteen `SeedConfig` sites."

### [NIT:consistency] NewPairedForTest counts conflate raw text hits with real call sites
**Location:** `08-fabricengine-external.md`, `## Batch Scope` ("twenty-two `NewPairedForTest` sites", "eighteen… go away") and Card 51 ("the ten… calls in `warpforward_integration_test.go` and the two in `checkout_index_refresh_test.go`"), plus Card 50's "four… calls" for `weftgit_exclude_test.go` **Issue:** These figures match a bare `grep -c NewPairedForTest` (which also counts the `export_test.go` declaration and `t.Fatalf("fabricengine.NewPairedForTest…")` string-literal error messages), not actual call expressions. Verified real invocation counts: `warpforward_integration_test.go` has 5 (not 10), `checkout_index_refresh_test.go` has 1 (not 2), `weftgit_exclude_test.go` has 2 (not 4), `fabric_test.go` (the one that stays) has 1. Total real call sites ≈ 9, not 22. **Fix:** Recount with a call-expression-aware grep (as the plan correctly did for `Copy*`, e.g. `gitkit\.Copy\w+\(`) and correct the prose; the named files and technique are still right, so this doesn't change what to migrate, only the stated counts.

### [NIT:consistency] Verify-shape decision doesn't state the scout-tag case it already uses
**Location:** `00-overview.md` § Decision "the verify shape is vet-both-tags plus scoped tests"; `01-gitkit-leaf.md` verify line **Issue:** The decision states `-tags smoke` is added "when the batch touches a smoke-tagged file" but says nothing about `-tags scout`; batch 1's verify line nonetheless adds `go vet -tags scout ./...` because it edits `internal/scoutengine/{ensureserver,refs,toolchain}_integration_test.go`, which are actually `//go:build scout`-tagged (confirmed by reading those files). **Fix:** Extend the shared decision's rule to also cover `-tags scout` when a batch touches a scout-tagged file.

### [NIT:consistency] "two seed-commit messages" misdescribes hub.go
**Location:** `02-fabrictest-dissolution.md`, Card 7 **Issue:** Says "the two seed-commit messages `"fabrictest: seed warp template"`… become their `hubforge` equivalents," but `internal/fabricengine/fabrictest/hub.go`'s `buildBareTemplate` has exactly one `commitAll(scratch, "fabrictest: seed warp template")` call; the "two" actually belongs to the two README bodies named later in the same sentence. **Fix:** Reword to "the seed-commit message… and the two README bodies naming `fabrictest`."

## Verdict

REQUEST_CHANGES
Two batch frontmatters have wrong card counts; several scope/requirement sentences overstate site counts — fix before landing.
MILL_REVIEW_END
