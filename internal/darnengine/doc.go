// Package darnengine owns the darn recipe's hub-wide configuration and the verify command every darn verify reads.
//
// `darn.yaml` is a hub-wide config module with four keys: `writer`, the model-spec of the darn writer's session; `writer_timeout_min`, its wall-clock budget in minutes; `verify`, the shell command the darn verify gate, Publish and Finalize run in the task worktree; and `verify_attempts`, how many times the verify gate re-prompts the writer's session before it halts the run for the parent.
// `verify` runs any shell command in the task worktree, which is why the module is hub-wide: a task cannot weaken it through its own tree, and `lyx config darn --set` writes it from the prime.
// LoadConfig loads the file strictly and validates the `writer` model-spec at load time, leaving `verify` and `verify_attempts` to their readers.
// VerifyCommand loads the file at each call and returns the `verify` value; a missing file and an empty value are errors that name the way forward, so no verify skips.
//
// DarnSpec composes the darn writer's session from a DarnInputs the caller tells it, the resolved Config and a model registry.
// The session reads the board entry named by the slug, edits and tests the repository, commits on the task branch, and writes the change description at the told path in the format the final-summary spec defines.
// It is non-interactive, carries the description as its one output file, and renders `(none)` for rejection findings or prior work the spawn has nothing to tell for.
// A spawn that answers a rejection or continues earlier work is told so through those two values, and its stencil continues from the commits already on the task branch.
// DarnSpec refuses an empty required field by name and a writer model-spec the registry cannot resolve.
//
// The package is told every path it reads and imports no resolver.
package darnengine
