MILL_REVIEW_BEGIN
# Review: shedengine: per-producer bounce budget + explicit OnDone routing

```yaml
duration_s: 161.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Episode-ending `Done` entry can be written on a failure path
**Section:** "The count is episode-scoped" / "Reading the episode"
**Issue:** The scan stops at the first `Producer == X && Outcome == Done` entry, but `run.go`'s `case callErr != nil` arm (`run.go:177-185`) calls the same `appendHistory` closure, which records the producer's *returned* `outcome` verbatim — so a producer returning `(Done, _, err)` writes a `done` history entry on a hard-failure path, which would reset X's episode without X ever having succeeded, falsifying the load-bearing premise "a reset is only ever *earned* by a producer succeeding" that the no-run-wide-cap decision rests on. `contracts/specs/loom-status-spec.md:81` already asserts every `history[].outcome` is `done` or `stuck`, so the failure-arm entry's outcome value is under-specified today and becomes budget-bearing under this task.
**Fix:** State a disposition — either the scan requires the terminating entry to have come from the `Done` routing arm (e.g. the failure arms stop recording a `done` outcome), or accept and document that a failure-path `done` entry ends an episode.

### [NIT:consistency] "loomshed migration is behavior-preserving" is true of routing only
**Section:** Decisions → "loomshed migration is mechanical and behavior-preserving"
**Issue:** The decision says the migration "preserves today's observable behavior exactly", but loom's runtime bounce behavior does change in the same commit — `deps.MaxBounces` becomes a per-producer, episode-scoped, cross-invocation budget with no run-wide cap, so a loom run that blocks today may not block, and a resumed run may block sooner.
**Fix:** Scope the claim to `OnDone` routing and cross-reference the budget decisions for the behavior that does change.

### [NIT:scope] Two named loomshed test files carry no `OnDone` chain to wire
**Section:** Scope → "Tests, mechanical re-wiring only"
**Issue:** `internal/loomshed/fixture_test.go` and `internal/loomshed/sequence_test.go` declare no `[]shedengine.ProducerDef` literal and no `.Producers =` assignment (they build through `loomshed.New`), so the stated reason "needs an explicit `OnDone` chain" does not apply to them; `fixture_test.go:118`'s `MaxBounces: 3` re-read is the real, and different, obligation.
**Fix:** Move those two files out of the OnDone re-wiring bullet and list them under the `MaxBounces`-semantics re-read instead.

## Verdict

REQUEST_CHANGES
One episode-reset edge unresolved; the rest is decided and source-accurate.
MILL_REVIEW_END
