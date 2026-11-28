// Package reviewbudget declares the review budget key family that loom and darn config are built on:
// the run-wide keys `loom.review.max_bounces`, `loom.review.hard_max_bounces` and `loom.review.amend_max_bounces`;
// the per-perch keys `<perch>.max_bounces`, `<perch>.hard_max_bounces` and `<perch>.auto_accept`.
//
// A soft cap of 0 means zero rounds in every layer; a null per-perch soft cap takes the run-wide one.
// A hard cap of 0 takes the run-wide hard cap, and the run-wide hard cap is at least 1.
// The amendment allowance is run-wide only, at least 0, and never null; 0 settles every amendment with no review round.
// No budget key is a guard: a budget of 0 is the off switch, and only the auto-accept keys are guard keys.
//
// The package imports only the standard library and configengine, and declares no config template.
package reviewbudget
