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
// This package sits above internal/loomshed rather than inside it because internal/shedrecipe's
// registry already imports loomshed for six of its constructors -- a loomshed -> shedbuild ->
// shedrecipe -> loomshed production import cycle would not compile.
package loomrecipe
