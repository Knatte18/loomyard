// Package loomrecipe owns loom's recipe-backed producer-list construction and is the drop-in
// replacement for loomshed.New. internal/loomcli is its only production caller.
//
// It takes every absolute path from its caller and has no direct production import of
// internal/lyxcwd, per the Told-Geometry Invariant.
//
// The bounce budget of every review segment (a segment holding a Bouncer row) is not declared in the loom recipe.
// It is the perch's own entry in the per-perch map, 0 included, falling back to `review_max_bounces` where the map holds none:
// New takes the run-wide value from shedrecipe.Env.ReviewMaxBounces and the per-perch ones from shedrecipe.Env.RowMaxBounces, keyed by the segment's Bouncer row name, and Routing takes both from its arguments.
// Both set the resolved value on every row of the segment, the Bouncer, the review row and the fix row, so the engine and the routing projection hold one value per perch.
// A resolved budget of 0 also stamps `zero_rounds` on the segment's Bouncer row, which then settles approved at once; the segment keeps its route.
// A negative per-perch entry is a wiring error naming its Bouncer row, and a run-wide value below 0 is refused.
// PR-Gate keeps the recipe's own max_bounces.
//
// The loom recipe's landing tail reads `Describe → Merge-In → Merge-Review → Publish → PR-Align → PR-Gate → Finalize`.
// PR-Align always ends Done, so it carries no on_stuck and sits outside the PR-Review segment.
// The darn recipe's tail reads `Describe → Merge-In → Publish → PR-Align → PR-Gate → Finalize`.
//
// NewDarn is the second builder: it builds the embedded darn recipe, and DarnRouting projects that recipe's routing.
// Its verify budget is not declared in the recipe: NewDarn stamps shedrecipe.Env.DarnVerifyAttempts, the `verify_attempts` of `darn.yaml`, onto the verify gates of the `Darn-Write` and `Darn-Burler-Fix` rows at every build and refuses a value below 1.
// Nor is the bounce budget of its Darn-Review perch: NewDarn takes it from shedrecipe.Env.DarnReviewMaxBounces, the `review_max_bounces` of `darn.yaml`, and DarnRouting from its argument, and both set it on the perch's rows and refuse a value below 0.
// A budget of 0 turns the perch off: both route `Darn-Write`'s Done straight to Publish, so a run reaches neither the perch's rows nor the `Describe` row after them, and the writer's own change description stands.
// DarnRetiredRows maps the writer row's former name, `Darn`, onto `Darn-Write`, so the caller can carry a darn run parked under the old name across with shedengine.RenameProducers.
//
// This package sits above internal/loomshed rather than inside it because internal/shedrecipe's
// registry already imports loomshed for six of its constructors -- a loomshed -> shedbuild ->
// shedrecipe -> loomshed production import cycle would not compile.
package loomrecipe
