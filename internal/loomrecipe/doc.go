// Package loomrecipe owns loom's recipe-backed producer-list construction and is the drop-in
// replacement for loomshed.New. internal/loomcli is its only production caller.
//
// It takes every absolute path from its caller and has no direct production import of internal/lymcwd, per the Told-Geometry Invariant.
//
// The bounce budget of every review segment (a segment holding a Bouncer row) is not declared in the tapestry recipe.
// It is the segment's own budget, 0 included:
// New takes it from shedrecipe.Env.Segments, keyed by segment, and Routing takes a map of the same keys as its argument.
// Both set the value on every row of the segment, the Bouncer, the review row and the fix row, so the engine and the routing projection hold one value per segment.
// A budget of 0 also stamps `zero_rounds` on the segment's Bouncer row, which then settles approved at once; the segment keeps its route.
// New refuses a review segment with no Env.Segments entry, naming the segment and the loom wiring's segment-to-perch table, and a negative budget, naming the segment, its config key and file.
// Routing refuses a review segment its map lacks and a negative entry, each naming the segment.
//
// The tapestry recipe's tail after Webster reads `Parent-Align → Drift-Verify → Code-Review → Describe → PR-Review → Finalize → Friction-Reflect`.
// Drift-Baseline sits between the Discussion-Review ring and Parts-Write, the two parts rows sit between it and Plan-Write, and Drift-Await sits between the Plan-Review ring and Webster.
// PR-Review's Stuck is always a Halt, so it carries no on_stuck, and Finalize declares no reroute.
// The darn recipe reads `Preflight → Record-Write → Drift-Baseline → Drift-Await → Darn-Write → Drift-Verify → Code-Review → Describe → PR-Review → Finalize → Friction-Reflect`, with no Parent-Align.
// Every Drift row carries the entry gate `drift.enabled`, so a hub that leaves regression off records each of them skipped.
// Its Code-Review rows are loom's, with the same names, segment, rubric and `loom.yaml` keys; only their answer key differs, the decision record alone in place of the record and the plan.
//
// The sampler recipe reads `Preflight → Record-Write → Sampler-Session → Code-Review → Describe → PR-Review → Finalize → Friction-Reflect`, with no Drift rows and no Parent-Align.
// Its Code-Review rows are the darn recipe's, each gated by `sampler.code_review`, so with that key false they are recorded skipped and Code-Bouncer routes on to Describe, which carries no entry gate.
//
// New and Routing take the recipe name and serve every embedded loom recipe, tapestry, darn, pawl and sampler; New builds it and Routing projects its routing.
// The darn recipe's verify budget is not declared in it: when a recipe's build row runs the DarnWrite engine, New stamps shedrecipe.Env.DarnVerifyAttempts, the `verify_attempts` of `darn.yaml`, onto the verify gates of that row and of the Code-Review segment's fix row at every build, each only where it declares a verify gate, and refuses a value below 1.
// Nor is the bounce budget of the darn perch: New takes it from shedrecipe.Env.Segments["Code-Review"], whose budget is the Code-Review perch's resolved soft cap from `loom.yaml`, and Routing from a map keyed by segment, and both set it on the perch's rows and refuse a missing Code-Review entry and a value below 0.
// A budget of 0 turns the perch off as loom's does: New stamps `zero_rounds` on Code-Bouncer, which settles approved at once on `Drift-Verify`'s Done, and every edge is kept.
// The `Describe` row carries the entry gate `budget.Code-Review`, so a budget of 0 skips it and the writer's own change description stands.
// RetiredRows keeps each recipe's retired row names as that recipe's data: the writer row's former name, `Darn`, maps onto `Darn-Write`, and the landing row's former name, `Merge-In`, onto `Parent-Align`, with the entry row's former names, `Loom-Preflight` and `Tapestry-Preflight`, onto `Preflight`.
// The caller carries a run parked under an old name across with shedengine.RenameProducers.
//
// The pawl recipe, a beam's life, is a loom recipe New builds like the others.
// It reads `Preflight → Record-Write → Beam-Up → Beam-Await → Parent-Align → Beam-Describe → PR-Review → Finalize → Beam-Down`, with no review segment and no reroute edge:
// Record-Write writes the project record, Beam-Await is its build row, held while the members' runs work in the beam, and a merge Finalize cannot land halts the run for the orch.
// A run at Parent-Align or any row after it holds its beam; before it the beam is free to start members.
//
// Rows and RowNames serve the tapestry, darn, batten, pawl and sampler recipes, and EntryNumbers numbers any recipe's rows for the panes that draw them:
// a review segment is one entry and every other row its own, so a recipe added to Rows is numbered with no change to its readers.
//
// IsLoom, HasEngine, RecordEngine, VerifySource and RetiredRows answer a recipe's contract, engines, record engine, verify source and retired rows by recipe name, so a consumer reads the recipe's rows and keys instead of comparing its name.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
//
// CodePerchGate answers the entry gate of a recipe's Code-Review Bouncer row, built over a told Env, and nil for a recipe whose perch carries none (tapestry, darn, pawl):
// a reader of whether the perch is off follows that gate as well as the budget.
//
// AmendmentTarget answers the optional `amendment_target` key of the tapestry, darn, batten, pawl or sampler recipe: the row a plan amendment routes to, empty when the recipe sets none.
//
// Every review segment of the tapestry and darn recipes sets one `role_prefix` on its Bouncer, BurlerReview and BurlerFix rows, and no two segments share a value.
// A test pins that beside the carry-over check, which pins that each Bouncer row's `carry_over` names the row's own segment.
//
// This package sits above internal/loomshed rather than inside it because internal/shedrecipe's
// registry already imports loomshed for six of its constructors -- a loomshed -> shedbuild ->
// shedrecipe -> loomshed production import cycle would not compile.
package loomrecipe
