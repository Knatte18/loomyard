// Package darnengine owns the darn recipe's hub-wide configuration and the tree scope classification every darn verify site reads.
//
// `darn.yaml` is a hub-wide config module with these keys: `writer`, the model-spec of the darn writer's session; `writer_timeout_min`, its wall-clock budget in minutes; `tree_scope` and `tree_scope_verify`, the keys that let a diff wholly under the hub's non-production trees run a narrower command, described in the tree scope paragraph below; `verify_attempts`, how many times a verify gate, the writer's or a Darn-Review fix round's, re-prompts its session before it halts the run for the parent; and `review_max_bounces`, the bounce budget of the Darn-Review perch that reviews the writer's committed diff before Publish, defaulting to 1, where 0 runs neither the review nor the Describe row after it; and `review_hard_max_bounces`, the round number at or past which that perch never continues a round, defaulting to 2; and `review_amend_max_bounces`, the review rounds an amendment generation gets after a settled Darn-Review subject changed, defaulting to 1, where 0 settles every amendment approved with no review round; and `darn_reviewer_model` and `darn_fixer_model`, the perch's per-round reviewer and fixer models in burlerengine's model-spec list grammar, where an empty key takes `loom.yaml`'s shared `reviewer_model` or `fixer_model`;
// and `darn_reviewer_fan` and `darn_fixer_fan`, the perch's per-round reviewer and fixer fans from `lenses.yaml`, in the `lenses` package's fan grammar, empty by default so every round runs solo with the single fixer.
// LoadConfig refuses a set model key that is an empty list, a value of another shape or an entry that is no model-spec.
// The two fan keys are closed to the local layer, and LoadConfig refuses an empty list, a value of another shape and an entry naming a fan `lenses.yaml` does not resolve.
// MigrateConfig renames the retired `review_fan` and `review_fix_fan` to the two fan keys for `lyx config reconcile`.
// The three budget keys come from `internal/reviewbudget` and are ordinary keys, settable per run through the task's `run_settings`, and `ValidateLocalValues` refuses one below its bound.
// The retired `verify` key, once the hub's whole verify command, is dropped at load and pruned by `lyx config reconcile`: the hub's `landing` gate in `gate.yaml` replaces it.
// The module is hub-wide, so a task cannot weaken `tree_scope_verify` through its own tree, and `lyx config darn --set` writes it from the prime.
// LoadConfig loads the file strictly and validates the `writer` model-spec at load time, leaving `verify_attempts` and `review_max_bounces` to their readers.
// It refuses a `review_max_bounces` or `review_amend_max_bounces` below 0, a `review_hard_max_bounces` below 1, and one below `review_max_bounces`, each naming the command that fixes it for the layer its value comes from: `lyx config darn --layer local --set` in the task worktree plus `lyx board upsert` for a local value, `lyx config darn --set` from the prime for the hub file.
//
// Tree scope lets a diff wholly under the hub's non-production trees run only its trees' command, in place of the `landing` gate's call at Merge-In, Finalize and Publish, and skip the `publish` gate.
// `tree_scope` maps a worktree-relative tree path to the test target the tree contributes, an empty target meaning the tree has no tests of its own.
// `tree_scope_verify` is the command a tree-scoped diff runs, holding `{targets}`, and it is exactly one `lyx gate test` call holding `--wait` and no shell control operator (`;`, `&`, `|`, a newline, a backtick, `$(`, `<` or `>`), so the whole tree-scoped command is slotted and waits out a busy hub.
// Bound: the refusal sees static text only, as the shell guard does, and a script the call runs is outside it.
// Tree scope is on only when both keys are non-empty.
// `tree_scope_verify` is an executable key and `tree_scope` a guard key, so neither is set in the local layer or through a task's `run_settings`, and `tree_scope` is an open map, so `--set tree_scope.<tree>=<target>` writes one tree and reconcile carries the hub's trees whole.
// LoadConfig trims one trailing slash from each tree key, and refuses a key that is empty, holds a backslash or holds an empty, `.` or `..` segment, and two keys equal after the trim; each wraps ErrTreeScopeKey and names the key as written.
// It refuses a `tree_scope_verify` set while `tree_scope` holds no tree, wrapping ErrTreeScopeWithoutTrees, one without `{targets}`, and one that is not a single waiting gate call; each names its `--set` command.
// ErrTreeScopeKey names no way forward, since `--set` cannot remove a key the hub file holds; a caller that refuses on a load error returns it through WithEditorWayForward, which appends the editor way forward.
// TreeScopedCommand classifies a diff's changed paths: a path lies in a tree when it equals the tree path or starts with it and a slash, a path touches every tree it lies in, the diff is tree-scoped when every path lies in some tree, and an empty path list is tree-scoped.
// The command is `tree_scope_verify` with `{targets}` replaced by the touched trees' non-empty targets, deduplicated, in tree-path order, so no path text reaches it.
// When no touched tree contributes a target, as for a docs-only diff, the answer is scoped with an empty command, which every darn site runs as nothing.
// `lyx config darn --set tree_scope.tools=./tools/... --set tree_scope.docs= --set "tree_scope_verify=lyx gate test --wait --tier 1 {targets}"` turns tree scope on for a hub, and `lyx config darn --set tree_scope_verify=` turns it off.
// A tree-scoped diff gets no whole-module run before it lands, so a break outside the listed trees that neither the trees' tests nor the guard-marked scans exercise can reach the parent; the bound is what the hub lists as non-production.
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
