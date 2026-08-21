MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (Sonnet 5 / claude-sonnet-5, per harness system info)
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [BLOCKING:consistency] Publish/Finalize entries skip the error-text-prefix decision
**Location:** batch 3, card 7 (`entries_simple.go`, `publishEntry`/`finalizeEntry`)
**Issue:** The Shared Decision "error text prefix" requires every error this package returns to start with `shedrecipe: `, but card 7 literally instructs `publishEntry`/`finalizeEntry` to "return landingshed.NewPublish(env.Landing)" with no wrapping — unlike card 14/15 (Bouncer/BurlerRound), which explicitly say "surfacing its error wrapped rather than discarded." A raw pass-through surfaces `landingshed: NewPublish: ...`, not `shedrecipe: Publish: ...`.
**Fix:** Add the same "surface the error wrapped with the shedrecipe: Publish:/Finalize: prefix" instruction card 14/15 already carry.

### [BLOCKING:scope] Card 10's Context omits the landingshed files its own Requirements cite
**Location:** batch 3, card 10 (`registry_test.go`/`entries_simple_test.go`)
**Issue:** Requirements state "an Env whose Landing is the zero landingshed.Deps makes the underlying constructor reject" — referencing `landingshed.NewPublish`/`NewFinalize`'s rejection behaviour — but neither `internal/landingshed/publish.go`, `finalize.go`, nor `deps.go` is in card 10's `Context:` list (only `entries_simple.go`, which may carry a paraphrase per card 7's own godoc instruction, not the same as reading the source).
**Fix:** Add `internal/landingshed/publish.go` and `internal/landingshed/finalize.go` to card 10's `Context:`.

### [NIT:consistency] Card 24 mischaracterizes the roadmap's dependency phrasing
**Location:** batch 6, card 24 (`manifest/roadmap.md`)
**Issue:** Card 24 says "the two remaining items that say 'Depends on the engine-registry item above' must be reworded," but only the loader/builder item literally contains that phrase; the `loom: convert` item instead says "Depends on all three items above" — a different phrase needing its own count fix (three → two), not a search-and-reword of the quoted text. Low risk since `manifest/roadmap.md` is in `Edits:` and therefore implicitly read.
**Fix:** Reword card 24 to separately name both items' actual current text rather than implying they share one phrase.

### [NIT:consistency] Card 21's "seven tests" count is already wrong before this task's edit
**Location:** batch 6, card 21 (`CONSTRAINTS.md`, Told-Geometry Invariant)
**Issue:** The existing "Machine-enforced" bullet card 21 points at already enumerates nine packages/tests (tokenvocab, pattern, buildinfo, standalonestate, shedengine, treadleengine, loomshed, landingshed, mergeresolve), not seven, yet the trailing line reads "the seven tests named above." Card 21 says only "update that count to match the new membership," risking a mechanical seven→eight increment that perpetuates the pre-existing miscount instead of landing on the correct ten.
**Fix:** Instruct card 21 to recount the enumerated packages (currently nine) and set the total to ten, not merely increment the existing number.

## Verdict

REQUEST_CHANGES
Two BLOCKING findings: an error-prefix decision violated for Publish/Finalize, and a Context-completeness gap in card 10.
MILL_REVIEW_END
