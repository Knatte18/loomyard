MILL_REVIEW_BEGIN
# Review: loom: Plan-Write/Plan-Validate approval deadlock (F7)

```yaml
duration_s: 211.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverified from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [BLOCKING:design] Keystone fixture flip targets the wrong writer
**Section:** Testing — "The one thing that fixture gets wrong" / Q&A round-2 gap 2
**Issue:** `fixture_test.go:545`'s `seedPlanValidateFixture(t, dir, true)` never reaches `Plan-Validate`: `loomshed.NewPlanWrite`'s rotation archives the seeded plan and `fakeLoomShuttle`'s `"plan"` branch rewrites the overview with `planFixtureOverview(true)` at `fixture_test.go:393` (doc at `:283-287` states this explicitly), so flipping `:545` to `false` changes nothing at row 8 and passes unchanged under today's code — the claim "under today's code that flip fails at `Plan-Validate`" is false.
**Fix:** Name `fixture_test.go:393`'s `planFixtureOverview(true)` — the `Plan-Write` stand-in, which the stencil forbids from self-approving — as the write that must flip, and re-derive the regression-test claim (and the `approved: true`-after-run assertion) from that.

### [NIT:scope] `revalidate_test.go`'s corruption is undone by the new seam
**Demoted-from:** BLOCKING
**Section:** Technical context — "Existing tests that encode the old behaviour"
**Issue:** `fakeLoomBurler.corruptPlanOverview` (`fixture_test.go:131-135`) injects its regression solely as `planFixtureOverview(false)`, and `TestSequence_PlanRevalidateCatchesPostSegmentRegression` depends on `plan-unapproved` firing at `Plan-Revalidate`; with the new `Approve` seam running on `Plan-Bouncer`'s APPROVED settle *after* the burler round, the flag is flipped back to `true` and that test's bounce no longer occurs — yet neither the file nor the fake is listed among the tests that must move.
**Fix:** Add `internal/loomrecipe/revalidate_test.go` + `fakeLoomBurler.corruptPlanOverview` to the must-move list and state the disposition (a genuinely format-invalid corruption, e.g. a Card Index/file mismatch, rather than `approved: false`).

### [BLOCKING:design] Negative-case test has no seam to remove the recipe key
**Section:** Testing — "Also add the negative case"
**Issue:** `loomrecipe.New` parses the embedded `recipes.LoomRecipe` unconditionally (`loomrecipe.go:86`), so "with `approve_seam` removed from the `Plan-Bouncer` row, the same fixture must halt at `Plan-Revalidate`" is not reachable through `buildSequenceFixture` + `New`; and leaving `env.ApprovePlan` nil against the shipped recipe makes `New` fail at `requireSeam` instead of running.
**Fix:** State the mechanism — e.g. a modified recipe document through `shedbuild.Parse([]byte(...))` as `overlay_seam_guard_test.go:270` already does — or replace the negative case with one the shipped fixture path can express.

## Verdict

REQUEST_CHANGES
Keystone regression claim rests on the wrong fixture writer; two dependent tests unaddressed.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
