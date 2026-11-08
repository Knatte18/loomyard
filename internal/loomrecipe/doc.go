// Package loomrecipe owns loom's recipe-backed producer-list construction.
// It takes every absolute path from its caller.
// It has no direct production import of internal/lymcwd, per the Told-Geometry Invariant.
//
// The bounce budget of a review segment (a segment holding a Bouncer row) is not declared in the tapestry recipe.
// It is the segment's own budget, 0 included.
// New takes it from shedrecipe.Env.Segments, keyed by segment, and Routing takes a map of the same keys.
// Both set it on every row of the segment: the Bouncer, the review row and the fix row.
// The engine and the routing projection thus hold one value per segment.
// A budget of 0 also stamps `zero_rounds` on the segment's Bouncer row, which then settles approved at once;
// the segment keeps its route.
// New refuses a review segment with no Env.Segments entry.
// The refusal names the segment and the loom wiring's segment-to-perch table.
// New also refuses a negative budget, naming the segment, its config key and file.
// Routing refuses a review segment its map lacks and a negative entry, each naming the segment.
//
// The tapestry recipe opens `Preflight → Motif`.
// Motif is a block row that runs the motif recipe's Discussion-Write and Discussion-Review ring inline.
// Its tail after Webster reads:
// `Parent-Align → Drift-Verify → Code-Review → Drift-Recheck → Describe → PR-Review → Finalize → Friction-Reflect`.
// Drift-Baseline sits between the Motif block and Parts-Write; the two parts rows sit between it and Plan-Write.
// Drift-Await sits between the Plan-Review ring and Drift-Expect, and Drift-Expect sits before Webster.
// Drift-Expect has an agent write the expectation of the task's changes against the counted capture;
// its Stuck halts the run.
// PR-Review's Stuck is always a Halt, so it carries no on_stuck, and Finalize declares no reroute.
// The darn recipe has no Parent-Align and reads:
// `Preflight → Record-Write → Drift-Baseline → Drift-Await → Drift-Expect → Darn-Write → Drift-Verify → Code-Review → Drift-Recheck → Describe → PR-Review → Finalize → Friction-Reflect`.
// Every Drift row carries the entry gate `drift.enabled`;
// a hub that leaves regression off records each of them skipped.
// In both recipes Drift-Recheck runs the DriftVerify engine like Drift-Verify and carries no on_stuck.
// Code the perch's fixes committed is checked and judged again, and a Stuck halts the run.
// The darn recipe's Code-Review rows are loom's, with the same names, segment, rubric and `loom.yaml` keys;
// only their answer key differs, the decision record alone in place of the record and the plan.
//
// The sampler recipe has no Parent-Align, and reads:
// `Preflight → Record-Write → Drift-Baseline → Drift-Await → Drift-Expect → Sampler-Session → Drift-Verify → Code-Review → Drift-Recheck → Describe → PR-Review → Finalize → Friction-Reflect`.
// Its Drift rows are the darn recipe's, each gated by `drift.enabled` and `drift.scope == full`,
// so they are off by default and a task switches them on with `drift.scope` full in its run settings.
// Its Code-Review rows are the darn recipe's, each gated by `sampler.code_review`.
// With that key false they are recorded skipped and Code-Bouncer routes on to Describe, which carries no entry gate.
//
// The motif recipe makes a sketch, the decision record, from a task.
// It reads `Discussion-Write → Discussion-Review`, the one definition of those rows.
// Run alone it takes the loom contract's record-only form and its own run-profile block,
// and its run ends at the Discussion-Review approval with no landing.
// The tapestry recipe runs it as its Motif block, whose expansion reads none of motif's top-level keys.
// The block's gate is the `undiscussed` check,
// so a run whose board entry is flagged discussed seeds the stored record and goes on at Drift-Baseline.
//
// The raddle recipe splits a Sketch into the board entries the operator approves.
// It reads `Record-Write → Raddle-Write → Raddle-File`, with no review segment.
// It takes the loom contract's record-only form.
// Record-Write writes the Sketch's stored decision record.
// Raddle-Write is one agent session gated by the `raddle` check.
// Raddle-File, a Go row, files the approved proposal on the board and ends the run's entry, so the run lands no code.
// Raddle-File's Stuck returns to Raddle-Write,
// and a third failing gate attempt of Raddle-Write halts the run for its parent.
//
// One table, embeddedRecipes, maps each recipe name to its embedded bytes.
// It is the resolver every expansion is told.
// Every accessor reads the expanded recipe:
// the rows of a block's sub-recipe follow its block row, each carrying its block path,
// so all accessors see the same rows.
// A recipe holding no block row is read as it is.
//
// New and Routing take the recipe name and serve every embedded loom recipe:
// New builds the recipe and Routing projects its routing.
// The darn recipe's verify budget is not declared in it.
// When a recipe's build row runs the DarnWrite engine, New stamps shedrecipe.Env.DarnVerifyAttempts at every build.
// That value is the `verify_attempts` of `darn.yaml`.
// New stamps it onto the verify gates of that row and of the Code-Review segment's fix row;
// a row that declares no verify gate gets none.
// New refuses a value below 1.
// The bounce budget of the darn perch is not declared in the recipe either.
// New takes it from shedrecipe.Env.Segments["Code-Review"], the Code-Review perch's resolved soft cap from `loom.yaml`.
// Routing takes it from a map keyed by segment.
// Both set it on the perch's rows and refuse a missing Code-Review entry and a value below 0.
// A budget of 0 turns the perch off as loom's does:
// New stamps `zero_rounds` on Code-Bouncer, which settles approved at once on `Drift-Verify`'s Done,
// and every edge is kept.
// The `Describe` row carries the entry gate `budget.Code-Review`,
// so a budget of 0 skips it and the writer's own change description stands.
// RetiredRows keeps each recipe's retired row names as that recipe's data:
// `Darn` maps onto `Darn-Write`, and `Merge-In` onto `Parent-Align`;
// `Loom-Preflight` and `Tapestry-Preflight` map onto `Preflight`.
// The caller carries a run parked under a retired name across with shedengine.RenameProducers.
// It does so at arm and with no side effect.
// A row a recipe dropped is declared apart, in the recipe's own `retired` map.
// RetiredRowNames returns that map's keys and never reads RetiredRows.
// The shed moves a run parked at one of them to the row the map names and pauses it.
// No row of the recipe carries the name on.
//
// The pawl recipe, a beam's life, is a loom recipe New builds like the others.
// It has no review segment and no reroute edge.
// It reads `Preflight → Record-Write → Beam-Up → Beam-Await → Finalize → Beam-Down`:
// Record-Write writes the project record.
// Beam-Await is its build row.
// It is held while the members' runs work in the beam.
// It is held also while the beam's orch writes the description and handles the pull request.
// A merge Finalize cannot land halts the run for the orch.
// Its `retired` map sends a run parked at Parent-Align, Beam-Describe or PR-Review back to Beam-Await.
// A run at Finalize or any row after it holds its beam; before it the beam is free to start members.
//
// Rows and RowNames serve every embedded recipe.
// EntryNumbers numbers any recipe's rows for the panes that draw them:
// a review segment is one entry and every other row its own,
// so a recipe added to Rows is numbered with no change to its readers.
//
// IsLoom, HasEngine, RecordEngine, VerifySource and RetiredRows each answer one question about a recipe by its name.
// A consumer thus reads the recipe's rows and keys instead of comparing the name.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
//
// DiscussionApprovalRow answers the row that approves a recipe's discussion:
// the Bouncer row the DiscussionWrite row hands to, and none for a recipe without one.
// PastDiscussionApproval answers whether a run standing at a row, mapped through RetiredRows, is past that approval:
// its row follows every row of the approval row's segment.
// Both read the recipe's expanded rows and never its name:
// darn, sampler and pawl answer no approval row, and tapestry's approval row is the one inside its Motif block.
//
// GateFixerSetting answers which setting a writer row's gate element names for its cold fixer's model-spec,
// so a caller reads whether a row's fixer is on from the recipe's own `fixer` key rather than from a row name.
//
// CodePerchGate answers the entry gate of a recipe's Code-Review Bouncer row, built over a told Env.
// It answers nil for a recipe whose perch carries none, such as tapestry, darn and pawl.
// A reader of whether the perch is off follows that gate as well as the budget.
//
// AmendmentTarget answers the optional `amendment_target` key of a recipe.
// That key is the row a plan amendment routes to, empty when the recipe sets none.
//
// Every review segment of the tapestry and darn recipes sets one `role_prefix`.
// It sits on the segment's Bouncer, BurlerReview and BurlerFix rows, and no two segments share a value.
// A test pins that value.
// Another test, the carry-over check, pins that each Bouncer row's `carry_over` names the row's own segment.
//
// This package sits above internal/loomshed because internal/shedrecipe's registry imports loomshed.
// Building the recipe inside loomshed would make a loomshed -> shedbuild -> shedrecipe -> loomshed import cycle.
package loomrecipe
