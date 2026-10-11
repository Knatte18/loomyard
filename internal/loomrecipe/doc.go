// Package loomrecipe owns loom's recipe-backed producer-list construction and is the drop-in
// replacement for loomshed.New. internal/loomcli is its only production caller.
//
// It takes every absolute path from its caller and has no direct production import of
// internal/lyxcwd, per the Told-Geometry Invariant.
//
// The bounce budget of every review segment (a segment holding a Bouncer row) is not declared in the loom recipe: New takes it from shedrecipe.Env.ReviewMaxBounces and Routing from its argument,
// and both set it on every row of those segments.
// PR-Gate keeps the recipe's own max_bounces.
//
// The second value New takes from the env is the Discussion-Write producer: when shedrecipe.Env.DiscussionSeats is true, New runs that row on the DiscussionSeats engine instead of the recipe's DiscussionWrite.
// The row keeps its name, gates and routing, and Routing never reads the choice.
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
