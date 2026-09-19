// Package lifecyclerecipe owns the lifecycle recipe's construction: parsing
// contracts/recipes.LifecycleRecipe and assembling it into a *shedengine.Shed against a
// caller-supplied shedrecipe.Env. internal/lifecyclecli is its only production caller.
//
// It takes every absolute path from its caller and has no direct production import of
// internal/lyxcwd, per the Told-Geometry Invariant (CONSTRAINTS.md).
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary
// Invariant's owner set, so none of its identifiers, string literals, or comments may name either
// fabric-internal side -- write "the task worktree" and "the pair" instead of naming either side
// by name.
package lifecyclerecipe
