MILL_REVIEW_BEGIN
# Review: Deploy cited spec/design docs to target repos like stencils — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-09-13
```

## Findings

### [BLOCKING:scope] Card 20 points the new SpecsDir guard's negative-path test at the wrong file
**Location:** batch 5 / card 20
**Issue:** Card 20 says the new `requireAbsRoot("Bouncer"/"BurlerRound", "SpecsDir", ...)` guards' "missing-root table" lives in `internal/shedrecipe/entries_simple_test.go`, and instructs adding a `SpecsDir` row there "beside the existing StencilsDir row." That file's own header states it covers only "the nine value-only entries" (Preflight, Publish, Finalize, LoomPreflight, Batchifier, DiscussionValidate, PlanValidate, Stub, Webster) — Bouncer and BurlerRound are not among them, and none of `entries_simple_test.go`'s `simpleEntryCases()` rows validates `StencilsDir` at all. The actual `StencilsDir`-blank negative tests for these two entries are `TestBouncerEntry_ConstructionFailures/BlankEnvStencilsDir` in `internal/shedrecipe/entries_bouncer_test.go` (line 408) and `TestBurlerRoundEntry_RubricStencil/EmptyStencilsDirFailsOnRubricStencilPathOnly` in `internal/shedrecipe/entries_burler_test.go` (line 183) — neither file appears anywhere in card 20's Context/Edits or in the plan's "All Files Touched" list. Since `fixture_test.go`'s shared `newTestEnv` gets a populated `SpecsDir`, no existing test breaks, but the new guards ship with zero negative-path coverage, silently missing the card's own stated goal ("covered by the same mechanism that already covers its sibling"), and the batch's own `go test ./internal/shedrecipe/...` verify cannot detect the gap.
**Fix:** Add `entries_bouncer_test.go` and `entries_burler_test.go` to card 20's Edits/Context, and add a `BlankEnvSpecsDir`-style subtest to each (mirroring `BlankEnvStencilsDir`/`EmptyStencilsDirFailsOnRubricStencilPathOnly`), rather than editing `entries_simple_test.go` for this purpose.

### [NIT:consistency] Card 20 omits the now-unused-import check it gives card 19 for the analogous file
**Location:** batch 5 / card 20 (`internal/shedrecipe/entries_burler.go`)
**Issue:** After card 20 replaces `entries_burler.go`'s `stencilstore.Read` + `stencil.StripLeadingComment` pair (the sole uses of each in that file, per `internal/shedrecipe/entries_burler.go`'s current `rubricStencil != ""` branch) with `shedadapters.ReadRubric(...)`, both the `internal/stencilstore` and `internal/stencil` imports become unused, which fails the build. Card 19 explicitly flags the equivalent hazard for `bouncer.go` ("Removing the now-unused stencil import ... is fine if nothing else in it uses the package; check before deleting") but card 20 gives no equivalent instruction for `entries_burler.go`, even though the risk is identical (and here both packages, not just one, become unused).
**Fix:** Add the same now-unused-import caveat to card 20's `entries_burler.go` instructions.

## Verdict

REQUEST_CHANGES
Card 20 wires the new SpecsDir guards' regression coverage into a test file that structurally cannot cover them.
MILL_REVIEW_END
