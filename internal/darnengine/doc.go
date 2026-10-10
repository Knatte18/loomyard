// Package darnengine owns the darn recipe's hub-wide configuration and the verify command every darn verify reads.
//
// `darn.yaml` is a hub-wide config module with four keys: `writer`, the model-spec of the darn writer's session; `writer_timeout_min`, its wall-clock budget in minutes; `verify`, the shell command the darn verify gate, Publish and Finalize run in the task worktree; and `verify_attempts`, how many times the verify gate re-prompts the writer's session before it halts the run for the parent.
// `verify` runs any shell command in the task worktree, which is why the module is hub-wide: a task cannot weaken it through its own tree, and `lyx config darn --set` writes it from the prime.
// LoadConfig loads the file strictly and validates the `writer` model-spec at load time, leaving `verify` and `verify_attempts` to their readers.
// VerifyCommand loads the file at each call and returns the `verify` value; a missing file and an empty value are errors that name the way forward, so no verify skips.
//
// The package is told every path it reads and imports no resolver.
package darnengine
