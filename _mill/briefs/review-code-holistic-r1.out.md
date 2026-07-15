MILL_REVIEW_BEGIN
# Review: Reconsider whether lyx mux needs anchor:top at all — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-15
```

## Findings

### [BLOCKING] Stale "fixed top band" comment survives in policy.go
**Location:** `internal/muxengine/render/policy.go:118-121`
**Issue:** `chainDepth`'s doc comment still reads "a parent that is not part of this stack (its actual parent anchors elsewhere, e.g. in the fixed top band)" — `AnchorTop`/the fixed top band no longer exist, so this is both a dead reference and now factually wrong (a missing parent today means hidden/own-window/not-live/no-pane, never "the fixed top band"). Batch 2 card 10 explicitly required sweeping policy.go's doc comments to remove "the fixed top-band set" language, and the plan's batch-2 scope statement asserts "no top-band code, config, or comment remains" — this was also the subject of a prior round-1 review NOTE fix per the plan's own Shared Decisions, so it is a repeat miss, not a new-class issue.
**Fix:** Reword the comment to describe why a parent can be absent from the stack today (hidden, deferred own-window, not-live, or empty-PaneID — matching `partitionByAnchor`'s actual exclusion filter), with no top-band language.

### [NIT] Stale "top-band region" reference in layout.go's file doc
**Location:** `internal/muxengine/render/layout.go:1-8`
**Issue:** The file-header comment still says "so the top-band region and the below-parent stack region can each be rendered independently and then concatenated into one placements list" — there is no top-band region anymore; `Rules` now produces a single stack. `layout.go` was never listed in either batch's `Edits:` (it is explicitly "untouched" mechanics-layer per batch 2's scope note), so this is a plan gap rather than an implementer miss, but it leaves dead prose that contradicts the batch's own stated exit condition ("no top-band code, config, or comment remains").
**Fix:** Reword to describe the single stack region only (drop the top-band clause).

### [NIT] mux-review-prompt.md still instructs walking the retired M6 scenario
**Location:** `docs/reviews/mux-review-prompt.md:209-212`
**Issue:** The "What to TEST" section still tells a future reviewer to "Walk every one" of the suite scenarios including "M6 (≥2-top layout tiling)," but `SANDBOX-MUX-SUITE.md`'s M6 is now a one-line tombstone ("Retired: top-band tiling removed with anchor:top", card 15). This is a forward-looking, actionable instruction (not the historical R6 narrative at line 146, which is fine as history) that will send a reviewer to walk a scenario with no content. Card 14 required removing TOP-BAND items from this file; this specific top-band-labeled scenario reference was missed.
**Fix:** Drop the "M6 (≥2-top layout tiling)" clause from the "suite's own scenarios already map onto..." bullet, or update it to name a current scenario.

## Verdict

REQUEST_CHANGES
A stale, factually-wrong top-band doc comment survives in an edited batch-2 file, contradicting the plan's own exit criterion.
MILL_REVIEW_END
