MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-20
```

## Findings

### [BLOCKING:consistency] doc.go's "one constructor returning an error" claim is false
**Location:** batch 3, Card 8 (`# Told, never derived` amendment). **Issue:** the Requirements instruct doc.go to state the Bouncer is "the one constructor in the package returning an error" because "the other three either validate nothing or take already-constructed engines that validated themselves" — but `internal/shedadapters/perch.go`'s `NewPerchProducer` (`func NewPerchProducer(...) (*PerchProducer, error)`, already in Card 5's own Context list) already returns an error and already performs real eager validation (`perchengine.ValidRunID(runIDPrefix)`, a nil-`factory` check, and three non-empty-string checks on `runDirBase`/`scratchDirBase`/`stencilsDir`) — it neither "validates nothing" nor "takes an already-constructed engine that validated itself." **Fix:** reword Card 8's requirement to state the Bouncer is the *second* validating, error-returning constructor in the package (after `NewPerchProducer`), or drop the uniqueness claim and instead contrast it correctly with `SingleLLMProducer`/`WebsterProducer`, which do return a bare pointer.

### [BLOCKING:design] `ensureFocus`'s own failure mode is unspecified
**Location:** batch 3, Card 6 (`Declare ensureFocus(round int)`). **Issue:** every other `archiveStaleOutputs` call site in `Call` is explicitly paired with "degrading on failure," but `ensureFocus` — invoked unconditionally from both the seed-call fallback and from `settle`'s `BLOCKING` branch — has no stated return type or failure contract for its own internal `archiveStaleOutputs`/`writeFocus` calls. This matters most for the `settle` call site: `settle` has already committed to returning the harvested/replayed `BLOCKING` verdict with its ledger pointer (per the "never discard a judgment that provably happened" rule), so the plan must say explicitly whether an `ensureFocus` failure there is swallowed (logged only) so the verdict result is still returned, or whether it can override that result — leaving this to the implementer's discretion risks either a swallowed error that silently leaves the round producer's next read path broken, or a regression that discards a genuine verdict over an unrelated write failure. **Fix:** add a sentence pinning `ensureFocus`'s signature (e.g., no error return, or an error that is only ever logged via `logger.Warn` and never propagated) and state explicitly that its failure must never change `Call`'s own outcome/pointer.

### [NIT:consistency] Card 7's real-template-bytes phrasing invites a fragile file read over the established package-import pattern
**Location:** batch 3, Card 7. **Issue:** "seed it with the real template bytes read from `contracts/stencils/bouncer/bouncer-template-seed.md`..." reads as a raw on-disk file read, but the precedent this plan should follow (`internal/treadleengine/judge_test.go`) imports the `contracts/stencils` Go package directly and uses its exported `[]byte` vars (e.g. `stencils.TreadleTemplateJudgeCircling`) rather than reading a relative path off disk. **Fix:** name the exact identifiers (`stencils.BouncerTemplateSeed`, `stencils.BouncerTemplateJudge`) and the import, mirroring `judge_test.go`, rather than describing the source as a file path.

### [NIT:scope] Judge-side harvest-with-Run-error has no explicit test case
**Location:** batch 4, Card 10 ("Harvest"). **Issue:** the seed-side harvest case (Card 7) explicitly covers both a `Run` error and a non-`OutcomeDone` outcome after real output was written, but the judge-side harvest case (Card 10) only mentions "reports a non-completion outcome," leaving the symmetric `Run`-error-after-real-output case for the judge path untested even though the same harvest mechanism (`judged(n)` evaluated regardless of what the run reported) is claimed to cover it. **Fix:** add a `Run`-error variant to Card 10's harvest scenario alongside the non-completion-outcome one, matching Card 7's seed-side symmetry.

## Verdict

REQUEST_CHANGES
Two BLOCKING findings: a false uniqueness claim for doc.go and an unspecified failure contract for ensureFocus.
MILL_REVIEW_END
