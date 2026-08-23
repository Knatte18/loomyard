MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5
reviewed_file: plan/
date: 2026-08-23
```

## Findings

### [BLOCKING:design] Card 16 assigns struct fields Card 17 hasn't created yet
**Location:** batch 4, cards 16-17. **Issue:** Card 16's edit to `wiring.go` adds `c.registry = registry`, `c.runner = runner`, `c.landingCfg = landingCfg` at the bottom of `wire()`, but `loomCLI` (`cli.go`) gains those three fields only in Card 17, sequenced and committed after Card 16 — verified against the current struct, which ends at `runDeps websterengine.RunDeps` with no `registry`/`runner`/`landingCfg`. Card 16's own commit does not compile. **Fix:** Reorder so Card 17's struct-field addition lands before Card 16's `wire()` assignments (swap the two cards), or merge them into one card/commit.

### [BLOCKING:consistency] Card 20's "current last entry" claim in roadmap.md is false
**Location:** batch 5, card 20. **Issue:** Card 20 anchors the new Done item "after `1. **preflight: split into two Shed rows...**`, the current last entry in that section" (roadmap.md:193), but verified against the live file that entry sits well before the section's actual end — twenty-plus more Done entries follow it down to `shedadapters: Burler-round producer` (~326-329), which is the real last entry immediately before `## Maintenance` (331). Following the card literally inserts the new entry mid-section, not at the end. **Fix:** Re-anchor the insertion point on the file's true last Done entry (`shedadapters: Burler-round producer`), or, if the newest-first convention at the section's top is intended instead, anchor there and say so explicitly.

### [BLOCKING:scope] Context omits files batch 1's Requirements name (cards 2, 7)
**Location:** batch 1, cards 2 and 7. **Issue:** Card 2's `OpenParent` requirement calls `Open(parentLoc)` and returns `*Fabric`, both declared in `open.go`/`fabric.go` (verified) — Card 2's Context lists only `lyxcwd.go`. Card 7's integration tests use `hubforge.AddPair`'s `Path`/`Branch` result fields, `gitkit.MustRun`, and `fabricengine.WarpForTest` (verified against `hub.go`/`export_test.go`) — none of these appear in Card 7's sole listed Context file, `open_integration_test.go`, which uses neither `AddPair` nor `WarpForTest`. **Fix:** Add `internal/fabricengine/open.go` and `fabric.go` to Card 2's Context; add `internal/hubforge/hub.go` and `internal/fabricengine/export_test.go` to Card 7's Context.

## Verdict

REQUEST_CHANGES
A card-ordering compile break and a factually wrong roadmap anchor point must be fixed before this plan proceeds.
MILL_REVIEW_END
