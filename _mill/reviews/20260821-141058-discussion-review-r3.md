MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe

```yaml
duration_s: 249.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Moved Run-tests lose their row-1 fake
**Section:** `preflight-row` × `test-ownership` / Testing
**Issue:** `internal/loomshed/fixture_test.go:77-79` states Preflight and WebsterRun are the *only two injectable rows*, and both `sequence_test.go` (`TestSequence_FullRunBlocksAtPublish`) and `resume_test.go` actually `Run` the whole list with `Preflight: fakeAlwaysDoneProducer{}`; once `preflightEntry` builds row 1 from `Env.Cwd` the recipe path offers no substitution point, so those moved tests would call the real `preflightshed` producer → `preflight.Check` → `lyxcwd.Resolve` → a `git rev-parse` spawn, which both breaks the run at row 1 against a `t.TempDir()` and violates the Test Tier Purity Invariant the Constraints section pins for the new package (`internal/preflightshed`'s own `Check`-driving tests are Tier 2 for exactly this reason).
**Fix:** decide and state how row 1 is neutralised in `internal/loomrecipe`'s tier-1 Run tests (e.g. substituting `Producer` on the built `[]ProducerDef` after `Build`, or a `New` variant returning the list before assembly), or state that these two tests become integration-tagged instead.

### [NIT:decision] `TestWire_PreflightIsTheAdapter` has no stated disposition
**Section:** `preflight-row` / Testing → `internal/loomcli`
**Issue:** `internal/loomcli/wiring_test.go:100-121` asserts `c.deps.Preflight` is `*preflightshed.preflightProducer`; that field disappears entirely under this Decision, so the test is deleted-subject rather than "repointed at the new Env values" like the other `wire` assertions.
**Fix:** name it explicitly — deleted (its property now lives in `shedrecipe`'s `preflightEntry`) or restated as `Env.Cwd == c.cwd`.

### [NIT:consistency] Registry-shape premise overstated
**Section:** `test-ownership` (`TestRegistry_ShipsTwelveEntries` stays)
**Issue:** the rationale claims moving it "would leave `package shedrecipe` with no in-package assertion of its registry's shape", but `internal/shedrecipe/registry_test.go`'s `TestNames` already asserts `Names()`↔`registry` key agreement and sortedness; what is unique to the moved test is the exact twelve-name pin.
**Fix:** reword the rationale to the exact-contents pin; the decision itself stands.

### [NIT:consistency] roadmap.md edit scope may leave falsified Done entries
**Section:** `docs`
**Issue:** the roadmap edit is scoped to "move the item Planned→Done and close the group framing", but `manifest/roadmap.md:160` ("`loomshed.New` keeps its own Go literal producer list … nothing downstream of this piece consumes it yet") and `:168` (describing `internal/shedbuild/equivalence_test.go` as shipped) are falsified by this task too.
**Fix:** state whether those two shipped-entry sentences are updated here or deliberately left as historical record.

## Verdict

REQUEST_CHANGES
Row-1 Preflight is no longer injectable; the moved Run tests have no stated tier-1 path.
MILL_REVIEW_END
