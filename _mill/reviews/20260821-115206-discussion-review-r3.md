MILL_REVIEW_BEGIN
# Review: Shed recipe: loader/builder

```yaml
duration_s: 195.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:scope] Twelve-engine fixture omits `Env.Landing`
**Demoted-from:** BLOCKING
**Section:** Testing → `Build`, the `shedrecipe.Names()` case
**Issue:** The section states `newTestEnv` "already does the whole job" and pins the needed set at "eleven things", but `internal/shedrecipe/fixture_test.go`'s `newTestEnv` leaves `Env.Landing` zero, and `publishEntry`/`finalizeEntry` call `landingshed.NewPublish`/`NewFinalize`, which reject a nil `Deps.OpenFabric`/`PushBranch`/`OpenParentFabric` and then require `mergeresolve.New`'s non-nil `Fabric`/`Shuttle` plus non-empty `WorktreeRoot`/`ScratchDir`/`StencilsDir` — so two of the twelve engines fail construction with that fixture.
**Fix:** Add `Env.Landing` to the inventory with the seven `landingshed.Deps` fields it needs, naming `coverage_guard_test.go`'s `coverageGuardLandingDeps`/`coverageGuardNilFabricOpener`/`coverageGuardFakeMergeShuttle` (or `loomshed/fixture_test.go`'s `testLandingDeps`) as the pattern, and correct the "eleven things"/"newTestEnv does the whole job" wording.

### [NIT:consistency] Coverage-guard helpers described as an `Env` pattern
**Section:** Technical context → `internal/shedrecipe/coverage_guard_test.go`
**Issue:** That paragraph calls the four helpers "the pattern for constructing a `shedrecipe.Env` and a `loomshed.Deps`", which the later Testing section explicitly contradicts ("fills no `Env` at all") — verified: `coverage_guard_test.go` builds only `loomshed.Deps`.
**Fix:** Reword the earlier paragraph to `loomshed.Deps`-shaped values only and point the `Env` half at `fixture_test.go`.

### [NIT:consistency] `config` mapping check assigned to two owners
**Section:** Decisions → "Validation split" vs "Decode strategy"
**Issue:** The validation split lists "`config` a mapping if present" among the shape checks `Parse` performs itself, while the decode decision and the test list assign that error to yaml's own `cannot unmarshal !!str into map[string]interface {}` decode failure with no row identity.
**Fix:** Drop the item from `Parse`'s own-check list, or state explicitly that it is decoder-reported rather than self-performed.

### [NIT:design] Missing `version` is indistinguishable from `version: 0`
**Section:** Decisions → "Strict unknown-key rejection and a required `version`"; Testing → `Parse`
**Issue:** With `Version int` and `KnownFields(true)`, an absent `version` and a literal `version: 0` both decode to `0`, so the two test scenarios listed as erroring distinctly "naming the offending value" cannot be told apart; an empty document (yaml `Decode` returns `io.EOF`) also has no stated message contract.
**Fix:** State that both cases share one "unsupported version 0" message (or choose `*int`), and name the empty-input/EOF behaviour.

## Verdict

APPROVE
Fixture inventory for the twelve-engine build case omits Landing; three minor consistency gaps.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
