// Package loomrecipe owns loom's recipe-backed producer-list construction and is the drop-in
// replacement for loomshed.New. internal/loomcli is its only production caller.
//
// It takes every absolute path from its caller and has no direct production import of
// internal/lyxcwd, per the Told-Geometry Invariant.
//
// The bounce budget of every review segment (a segment holding a Bouncer row) is not declared in the
// recipe: New takes it from shedrecipe.Env.ReviewMaxBounces and Routing from its argument, and both
// set it on every row of those segments.
// PR-Gate keeps the recipe's own max_bounces.
//
// The second value New takes from the env is the Discussion-Write producer: when shedrecipe.Env.DiscussionSeats is true, New runs that row on the DiscussionSeats engine instead of the recipe's DiscussionWrite.
// The row keeps its name, gates and routing, and Routing never reads the choice.
//
// NewDarn is the second builder: it builds the embedded darn recipe, and DarnRouting projects that recipe's routing.
// The recipe has no review segment, so no review budget applies.
// Its verify budget is not declared in the recipe either: NewDarn stamps shedrecipe.Env.DarnVerifyAttempts, the `verify_attempts` of `darn.yaml`, onto the Darn row's verify gate at every build and refuses a value below 1.
// DarnRouting reads no budget, so a caller that never builds a Shed is never refused by it.
//
// This package sits above internal/loomshed rather than inside it because internal/shedrecipe's
// registry already imports loomshed for six of its constructors -- a loomshed -> shedbuild ->
// shedrecipe -> loomshed production import cycle would not compile.
package loomrecipe
