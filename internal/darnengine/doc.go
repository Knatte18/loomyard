// Package darnengine owns the darn recipe's hub-wide configuration and the verify command every darn verify reads.
//
// `darn.yaml` is a hub-wide config module with these keys: `writer`, the model-spec of the darn writer's session; `writer_timeout_min`, its wall-clock budget in minutes; `verify`, the shell command the darn verify gate, Publish and Finalize run in the task worktree; `verify_attempts`, how many times a verify gate, the writer's or a Darn-Review fix round's, re-prompts its session before it halts the run for the parent; and `review_max_bounces`, the bounce budget of the Darn-Review perch that reviews the writer's committed diff before Publish, defaulting to 1, where 0 runs neither the review nor the Describe row after it; and `review_hard_max_bounces`, the round number at or past which that perch never continues a round, defaulting to 4; and `review_fan` and `review_fix_fan`, the perch's per-round reviewer and fixer fans from `burler.yaml`, in burlerengine's fan grammar, empty by default so every round runs solo with the single fixer.
// The two fan keys are closed to the local layer, and LoadConfig refuses an empty list, a value of another shape and an entry naming a fan `burler.yaml` does not resolve.
// The two budget keys come from `internal/reviewbudget` and are ordinary keys, settable per run through the task's `run_settings`, and `ValidateLocalValues` refuses one below its bound.
// `verify` runs any shell command in the task worktree, which is why the module is hub-wide: a task cannot weaken it through its own tree, and `lyx config darn --set` writes it from the prime.
// LoadConfig loads the file strictly and validates the `writer` model-spec at load time, leaving `verify`, `verify_attempts` and `review_max_bounces` to their readers.
// It refuses a `review_max_bounces` below 0, a `review_hard_max_bounces` below 1, and one below `review_max_bounces`, each naming the command that fixes it for the layer its value comes from: `lyx config darn --layer local --set` in the task worktree plus `lyx board upsert` for a local value, `lyx config darn --set` from the prime for the hub file.
// VerifyCommand loads the file at each call and returns the `verify` value; a missing file and an empty value are errors that name the way forward, so no verify skips.
//
// DarnTable composes the darn writer's session as a one-seat seat table, from a DarnInputs the caller tells it, the resolved Config and a model registry.
// The lead reads the board entry named by the slug, writes and validates the decision record and support log at the two told paths before it changes code, and follows the record's `Added after Discussion` entries over the board entry's body.
// It then edits and tests the repository, commits on the task branch, and writes the change description at the told path in the format the final-summary spec defines.
// It is non-interactive, carries the description as its one output file, and renders `(none)` for rejection findings or prior work the spawn has nothing to tell for.
// A spawn that answers a rejection or continues earlier work is told so through those two values, and its stencil continues from the commits already on the task branch.
// The seat engine renders the parent and edit directives, so DarnTable reads no parent name.
// DarnTable refuses an empty required field by name and a writer model-spec the registry cannot resolve.
//
// The package is told every path it reads and imports no resolver.
package darnengine
