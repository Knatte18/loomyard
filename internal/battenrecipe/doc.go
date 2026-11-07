// Package battenrecipe owns the batten recipe's construction.
// It parses contracts/recipes.BattenRecipe.
// It assembles the result into a *shedengine.Shed against a caller-supplied shedrecipe.Env.
//
// It also holds the test of where a batten run stands against its Land row:
// PastLand reports whether the run is past the row.
// LandOutcomeOf tells a run that landed from one that ended without landing.
// A reader of a run past Land goes through them.
//
// It takes every absolute path from its caller.
// It has no direct production import of internal/lymcwd, per PATTERN-cwd-resolution.
//
// This package describes one repository throughout.
// It is not in the Fabric Vocabulary Invariant's owner set.
// So none of its identifiers, string literals, or comments may name either fabric-internal side;
// write "the task worktree" and "the pair" instead.
package battenrecipe
