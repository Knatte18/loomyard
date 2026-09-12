MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-09-12
```

## Findings

### [BLOCKING:scope] Card 7's Context omits two files its own Requirements name
**Location:** 03-drive-wiring.md, card 7 (`internal/loomcli/selfreport.go`) **Issue:** Requirements calls `loomengine.RenderAnomalyBody` (declared in `internal/loomengine/anomalybody.go`, created by card 6) as a bare mention with no signature given, and separately mentions `selfreportengine.DefaultLabels()` (declared in `internal/selfreportengine/selfreport.go`, edited by card 1) with no shape given either — neither file is listed in card 7's Context or Edits. `selfreportengine.CreateIssue` is also named, but its full signature is inlined in the same sentence, so that one mention is self-contained; `RenderAnomalyBody` and `DefaultLabels()` are not. **Fix:** Add `internal/loomengine/anomalybody.go` and `internal/selfreportengine/selfreport.go` to card 7's Context list.

### [NIT:consistency] "four documentation edits" but three files are listed
**Location:** 00-overview.md, Shared Decision `docs-land-with-the-observable-change` **Issue:** the decision's prose says "the four documentation edits" then parenthesizes exactly three files (`manifest/designs/self-report-tier1.md`, `manifest/roadmap.md`, `docs/overview.md`), matching card 9's actual three doc edits — the numeral is simply wrong. **Fix:** Change "four" to "three".

### [NIT:consistency] Batch 3's Batch Tests section overstates what its cards touch
**Location:** 03-drive-wiring.md, Batch Tests section **Issue:** states verify runs "internal/loomcli and internal/loomengine — the two packages this batch's code touches," but no card in batch 3 edits or creates any `loomengine` file (only `loomcli` files and docs); `loomengine` is consumed via import, not touched by this batch's own cards. **Fix:** Reword to "consumes" for loomengine, or drop the "touches" framing for it.

## Verdict

REQUEST_CHANGES
Card 7's Context list is missing two files its own Requirements name (`anomalybody.go`, `selfreportengine/selfreport.go`); two minor wording inconsistencies elsewhere.
MILL_REVIEW_END
