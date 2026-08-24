MILL_REVIEW_BEGIN
# Review: loom: Plan-Write producer — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5, per system context)
reviewed_file: plan/
date: 2026-08-24
```

## Findings

### [BLOCKING:scope] `fakeDiscussionShuttle`→`fakeLoomShuttle` rename is incomplete
**Location:** Batch 2, Cards 8/9/10 (`internal/loomrecipe/fixture_test.go`, `shape_test.go`, `resume_test.go`)
**Issue:** Card 8 says to rename `fakeDiscussionShuttle` and update "its two references" (the `var _` assertion and `buildSequenceFixture`'s construction), both in `fixture_test.go`. But `internal/loomrecipe/resume_test.go` (lines 228, 307: `env.Shuttle.(*fakeDiscussionShuttle)`) is not in any card's Context/Edits anywhere in the plan, and `shape_test.go`'s own `testEnv` (line 88: `Shuttle: &fakeDiscussionShuttle{writeOutputs: false}`) is edited by Card 9 for an unrelated reason with no instruction to rename this site. After Card 8 renames the type, `go test ./internal/loomrecipe/...` — batch 2's own verify — fails to compile: `fakeDiscussionShuttle` is undefined.
**Fix:** Add `internal/loomrecipe/resume_test.go` to a card's Edits/Context and explicitly require updating its two type assertions to `fakeLoomShuttle`; explicitly call out `shape_test.go`'s `testEnv` construction site in Card 9's requirements too.

### [BLOCKING:scope] New `TestWire_PlanSpecEvaluatesToExpectedShape` needs the plan stencil seeded
**Location:** Batch 3, Card 11 (`internal/loomcli/wiring_test.go`)
**Issue:** The card adds a test that calls `c.env.PlanSpec()`, which (via `loomengine.PlanSpec`→`composePlanPrompt`) reads `loom-template-plan` through `stencilstore.Read`, which hard-errors on a missing file (confirmed in `internal/stencilstore/reconcile.go`). `hubLocation` (unmodified per the card) only seeds `loom.yaml`, `landing.yaml`, and the discussion stencil via `seedDiscussionStencil` — nothing seeds `<hub>/_board/_lyx/stencils/loom/loom-template-plan.md`. The new test fails at `c.env.PlanSpec()` with a non-nil error.
**Fix:** Card 11 must add a `seedPlanStencil(t, hub)` helper mirroring `seedDiscussionStencil` (writing `stencils.LoomTemplatePlan`) and call it from `hubLocation`.

### [NIT:consistency] "row 7" mislabels Plan-Write's actual list position
**Location:** Batch 2, Cards 8 and 10 (`internal/loomrecipe/fixture_test.go`/`sequence_test.go` doc comments)
**Issue:** Both cards instruct doc-comment prose calling Plan-Write "row 7," borrowing `manifest/designs/loom.md`'s 14-row table (which includes the never-built `Plan-Sweep` as row 6). In the actual 13-row built list Plan-Sweep has no row at all, so Plan-Write is `wantProducerTable[5]`/`wantSequenceOrder[5]` — row 6. `sequence_test.go`'s own existing, unedited comments already use the real-list numbering ("Publish (row 12)", "row 13" for Finalize), contradicting the new "row 7" text within the same file.
**Fix:** Say "row 6" in both cards' doc-comment instructions.

## Verdict

REQUEST_CHANGES
Two verifiable defects break `go test` under the plan's own verify commands (batches 2 and 3); one doc-comment numbering nit.
MILL_REVIEW_END
