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
// Drift-Baseline sits between the Discussion-Review ring and Plan-Write, and Drift-Await between the Plan-Review ring and Webster.
// PR-Review's Stuck is always a Halt, so it carries no on_stuck, and Finalize's reroute names Parent-Align.
// The darn recipe reads `Preflight → Record-Write → Drift-Baseline → Drift-Await → Darn-Write → Drift-Verify → Code-Review → Describe → PR-Review → Finalize → Friction-Reflect`, with no Parent-Align, and Finalize's reroute names Code-Bouncer.
// Every Drift row carries the entry gate `drift.enabled`, so a hub that leaves regression off records each of them skipped.
// Its Code-Review rows are loom's, with the same names, segment, rubric and `loom.yaml` keys; only their answer key differs, the board entry snapshot and the decision record in place of the record and the plan.
//
// New and Routing take the recipe name and serve every embedded loom recipe; New builds it and Routing projects its routing.
// The darn recipe's verify budget is not declared in it: when a recipe's build row runs the DarnWrite engine, New stamps shedrecipe.Env.DarnVerifyAttempts, the `verify_attempts` of `darn.yaml`, onto the verify gates of that row and of the Code-Review segment's fix row at every build and refuses a value below 1.
// Nor is the bounce budget of the darn perch: New takes it from shedrecipe.Env.Segments["Code-Review"], whose budget is the Code-Review perch's resolved soft cap from `loom.yaml`, and Routing from a map keyed by segment, and both set it on the perch's rows and refuse a missing Code-Review entry and a value below 0.
// A budget of 0 turns the perch off as loom's does: New stamps `zero_rounds` on Code-Bouncer, which settles approved at once on both `Drift-Verify`'s Done and `Finalize`'s reroute, and every edge is kept.
// The `Describe` row carries the entry gate `budget.Code-Review`, so a budget of 0 skips it and the writer's own change description stands.
// RetiredRows keeps each recipe's retired row names as that recipe's data: the writer row's former name, `Darn`, maps onto `Darn-Write`, and the landing row's former name, `Merge-In`, onto `Parent-Align`, with the entry row's former names, `Loom-Preflight` and `Tapestry-Preflight`, onto `Preflight`.
// The caller carries a run parked under an old name across with shedengine.RenameProducers.
//
// NewPawl is the pawl recipe's own builder, since pawl is not a loom recipe: it builds the embedded pawl recipe, a beam's life, and PawlRouting projects that recipe's routing.
// The recipe reads `Preflight → Beam-Up → Beam-Await → Parent-Align → Code-Review → Beam-Describe → PR-Review → Finalize → Beam-Down`, with no reroute edge:
// a merge Finalize cannot land halts the run for the orch.
// Its Code-Review perch is darn's, with the same names, segment, rubric and `loom.yaml` keys, the project entry's board snapshot as its only answer key and no carry-over.
// NewPawl takes the perch's budget from shedrecipe.Env.Segments["Code-Review"], and PawlRouting from a map keyed by segment, as New and Routing do, and both refuse a missing Code-Review entry and a value below 0.
// A budget of 0 stamps `zero_rounds` on Code-Bouncer, which settles approved at once, and Beam-Describe carries no entry gate, so it writes from the entry and the commits whatever the budget.
// A run at Parent-Align or any row after it holds its beam; before it the beam is free to start members.
//
// Rows and RowNames serve the tapestry, darn, batten and pawl recipes, and EntryNumbers numbers any recipe's rows for the panes that draw them:
// a review segment is one entry and every other row its own, so a recipe added to Rows is numbered with no change to its readers.
//
// IsLoom, ServedByLoom, HasEngine, RecordEngine, VerifySource and RetiredRows answer a recipe's contract, engines, record engine, verify source and retired rows by recipe name, so a consumer reads the recipe's rows and keys instead of comparing its name.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
// ServedByLoom is the one place that names pawl as the single non-loom recipe `lym loom` still serves.
//
// AmendmentTarget answers the optional `amendment_target` key of the tapestry, darn, batten or pawl recipe: the row a plan amendment routes to, empty when the recipe sets none.
//
// Every review segment of the tapestry and darn recipes sets one `role_prefix` on its Bouncer, BurlerReview and BurlerFix rows, and no two segments share a value.
// A test pins that beside the carry-over check, which pins that each Bouncer row's `carry_over` names the row's own segment.
//
// This package sits above internal/loomshed rather than inside it because internal/shedrecipe's
// registry already imports loomshed for six of its constructors -- a loomshed -> shedbuild ->
// shedrecipe -> loomshed production import cycle would not compile.
package loomrecipe
