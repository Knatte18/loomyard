// Package loomrecipe owns loom's recipe-backed producer-list construction and is the drop-in
// replacement for loomshed.New. internal/loomcli is its only production caller.
//
// It takes every absolute path from its caller and has no direct production import of
// internal/lyxcwd, per the Told-Geometry Invariant.
//
// The bounce budget of every review segment (a segment holding a Bouncer row) is not declared in the loom recipe.
// It is the segment's own budget, 0 included:
// New takes it from shedrecipe.Env.Segments, keyed by segment, and Routing takes a map of the same keys as its argument.
// Both set the value on every row of the segment, the Bouncer, the review row and the fix row, so the engine and the routing projection hold one value per segment.
// A budget of 0 also stamps `zero_rounds` on the segment's Bouncer row, which then settles approved at once; the segment keeps its route.
// New refuses a review segment with no Env.Segments entry, naming the segment and the loom wiring's segment-to-perch table, and a negative budget, naming the segment, its config key and file.
// Routing refuses a review segment its map lacks and a negative entry, each naming the segment.
// PR-Gate keeps the recipe's own max_bounces.
//
// The loom recipe's landing tail reads `Describe → Merge-In → Merge-Review → Publish → PR-Align → PR-Gate → Finalize`.
// PR-Align always ends Done, so it carries no on_stuck and sits outside the PR-Review segment.
// The darn recipe's tail reads `Describe → Merge-In → Publish → PR-Align → PR-Gate → Finalize`.
//
// NewDarn is the second builder: it builds the embedded darn recipe, and DarnRouting projects that recipe's routing.
// Its verify budget is not declared in the recipe: NewDarn stamps shedrecipe.Env.DarnVerifyAttempts, the `verify_attempts` of `darn.yaml`, onto the verify gates of the `Darn-Write` and `Darn-Burler-Fix` rows at every build and refuses a value below 1.
// Nor is the bounce budget of its Darn-Review perch: NewDarn takes it from shedrecipe.Env.Segments["Darn-Review"], whose budget is the `review_max_bounces` of `darn.yaml`, and DarnRouting from a map keyed by segment, and both set it on the perch's rows and refuse a missing Darn-Review entry and a value below 0.
// A budget of 0 turns the perch off: both route `Darn-Write`'s Done straight to Publish, so a run reaches neither the perch's rows nor the `Describe` row after them, and the writer's own change description stands.
// DarnRetiredRows maps the writer row's former name, `Darn`, onto `Darn-Write`, so the caller can carry a darn run parked under the old name across with shedengine.RenameProducers.
//
// Rows and RowNames serve the loom, darn and batten recipes, and EntryNumbers numbers any recipe's rows for the panes that draw them:
// a review segment is one entry and every other row its own, so a recipe added to Rows is numbered with no change to its readers.
//
// Every review segment of the loom and darn recipes sets one `role_prefix` on its Bouncer, BurlerReview and BurlerFix rows, and no two segments share a value.
// A test pins that beside the carry-over check, which pins that each Bouncer row's `carry_over` names the row's own segment.
//
// This package sits above internal/loomshed rather than inside it because internal/shedrecipe's
// registry already imports loomshed for six of its constructors -- a loomshed -> shedbuild ->
// shedrecipe -> loomshed production import cycle would not compile.
package loomrecipe
