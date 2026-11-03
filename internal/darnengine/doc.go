// Package darnengine owns the darn recipe's hub-wide configuration and the tree scope classification every darn verify site reads.
//
// `darn.yaml` is a hub-wide config module with these keys: `writer`, the model-spec of the darn writer's session; `writer_timeout_min`, its wall-clock budget in minutes; `tree_scope`, the key that lets a diff wholly under the hub's non-production trees run a narrower set of targets, described in the tree scope paragraph below; `verify_attempts`, how many times a verify gate, the writer's or a Code-Review fix round's, re-prompts its session before it halts the run for the parent.
// The darn recipe's Code-Review perch takes its models, fans, budgets and auto-accept flag from `loom.yaml`'s Code-Review keys, as loom's does, so darn.yaml holds none of them.
// The retired `verify` key, once the hub's whole verify command, is dropped at load and pruned by `lym config reconcile`: the hub's `landing` gate in `gate.yaml` replaces it.
// The driver, tackler and arbiter keys that once overrode `shed.yaml` here are retired: `shed.yaml` holds them alone, a hub file that still sets one loads with the value dropped, and a local file, profile block or `run_settings` object that sets one is refused with the way forward of setting it under the `shed` module.
// The retired `tree_scope_verify` key is dropped at load too, and MigrateConfig, darn's reconcile migration, prunes it while it is still in the hub file:
// held empty it empties `tree_scope` so an off hub stays off, and held set it keeps `tree_scope`.
// A hub file without the key is left alone, so trees the operator adds later are never touched.
// The module is hub-wide, so a task cannot weaken `tree_scope` through its own tree, and `lym config darn --set` writes it from the prime.
// LoadConfig loads the file strictly and validates the `writer` model-spec at load time, leaving `verify_attempts` to its reader.
//
// Tree scope lets a diff wholly under the hub's non-production trees run each darn verify site's own gate key over the touched trees' targets, in place of the key's scope.
// `tree_scope` maps a worktree-relative tree path to the test target the tree contributes, an empty target meaning the tree has no tests of its own.
// Tree scope is on exactly when `tree_scope` holds a tree, and removing every tree turns it off.
// A tree-scoped site runs its row's key, so the tiers are the key's and never an operator-written command's.
// `tree_scope` is a guard key, so it is not set in the local layer or through a task's `run_settings`, and it is an open map, so `--set tree_scope.<tree>=<target>` writes one tree and reconcile carries the hub's trees whole.
// LoadConfig trims one trailing slash from each tree key, and refuses a key that is empty, holds a backslash or holds an empty, `.` or `..` segment, and two keys equal after the trim; each wraps ErrTreeScopeKey and names the key as written.
// ErrTreeScopeKey names no way forward, since `--set` cannot remove a key the hub file holds; a caller that refuses on a load error returns it through WithEditorWayForward, which appends the editor way forward.
// TreeScopedTargets classifies a diff's changed paths: a path lies in a tree when it equals the tree path or starts with it and a slash, a path touches every tree it lies in, the diff is tree-scoped when every path lies in some tree, and an empty path list is tree-scoped.
// The targets are the touched trees' non-empty targets, deduplicated, in tree-path order, so no path text reaches the call.
// When no touched tree contributes a target, as for a docs-only diff, the answer is scoped with no targets, which every darn site runs as nothing.
// `lym config darn --set tree_scope.tools=./tools/... --set tree_scope.docs=` turns tree scope on for a hub.
// A tree-scoped diff gets no whole-module run before it lands, so a break outside the listed trees that neither the trees' tests nor the guard-marked scans exercise can reach the parent; the bound is what the hub lists as non-production.
//
// DarnTable composes the darn writer's session as a one-seat seat table, from a DarnInputs the caller tells it, the resolved Config and a model registry.
// The lead reads the decision record at the told path as its scope, the board entry's body under `## Goal` with the record's `Added after Discussion` entries over it, and writes neither the record nor the support log.
// It checks that no landed work or other entry overtook the board entry, then edits and tests the repository, commits on the task branch, and writes the change description at the told path in the format the final-summary spec defines.
// A lead that finds the entry outdated writes a stop report as its description and creates the marker at the told stop marker path, and the producer halts the run on the marker.
// A lead that finds a stop condition the task's body names holds writes the report the body asks for as its description and creates both that marker and a second one at the told condition stop marker path;
// the producer then halts the run with the report alone as the reason, since the report states its own way forward.
// It is non-interactive, carries the description as its one output file, and renders `(none)` for prior work the spawn has nothing to tell for.
// A spawn that continues earlier work is told so through that value, and its stencil continues from the commits already on the task branch.
// When the caller tells it a cold fixer clears the row's verify gate, the prompt's Step 4 has the writer and its subagents run no gate, and the writer acts only on what the fixer leaves;
// otherwise the step has them run the gate after each commit.
// The seat engine renders the parent and edit directives, so DarnTable reads no parent name.
// DarnTable refuses an empty required field by name and a writer model-spec the registry cannot resolve.
//
// The package is told every path it reads and imports no resolver.
package darnengine
