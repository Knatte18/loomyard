// Package darnengine owns the darn recipe's hub-wide configuration.
// It also owns the tree scope classification that every darn verify site reads.
//
// `darn.yaml` is a hub-wide config module with these keys:
// `writer`, the model-spec of the darn writer's session;
// `writer_timeout_min`, its wall-clock budget in minutes;
// `tree_scope`, described in the tree scope paragraph below;
// `verify_attempts`, how many times a verify gate re-prompts its session before it halts the run for the parent.
// A verify gate is the writer's or a Code-Review fix round's.
// `verify_budget_min` is how long, in minutes, a verify gate runs before the run's parent gets one notice.
// The attempts go on, and 0 turns the notice off.
// The darn recipe's Code-Review perch takes its models, fans, budgets and auto-accept flag from `loom.yaml`.
// darn.yaml holds none of them.
// A `verify` key in the hub file is dropped at load and pruned by `lym config reconcile`;
// the hub's `landing` gate in `gate.yaml` holds the verify command.
// The driver, tackler and arbiter keys belong to `shed.yaml` alone.
// A hub file that sets one loads with the value dropped.
// Setting one in a local file, profile block or `run_settings` object is refused, naming the `shed` module.
// A `tree_scope_verify` key is dropped at load too, and MigrateConfig prunes it while it is in the hub file:
// held empty it empties `tree_scope` so an off hub stays off, and held set it keeps `tree_scope`.
// A hub file without the key is left alone, so trees the operator adds later are never touched.
// The module is hub-wide, so a task cannot weaken `tree_scope` through its own tree,
// and `lym config darn --set` writes it from the prime.
// LoadConfig loads the file strictly and validates the `writer` model-spec at load time.
// It leaves `verify_attempts` to its reader.
//
// A merge task is the darn task that brings a beam's parent branch into the beam.
// Its slug carries the `-merge-in` suffix with an optional number.
// IsMergeTaskWriter reports whether a strand is a merge task's writer, from the strand name alone,
// so a guard that reads nothing else can let that session run the merge verbs its work needs.
//
// Tree scope applies to a diff wholly under the hub's non-production trees.
// Each darn verify site then runs its own gate key over the touched trees' targets, in place of the key's scope.
// `tree_scope` maps a worktree-relative tree path to the test target the tree contributes.
// An empty target means the tree has no tests of its own.
// Tree scope is on exactly when `tree_scope` holds a tree, and removing every tree turns it off.
// A tree-scoped site runs its row's key, so the tiers are the key's and never an operator-written command's.
// `tree_scope` is a guard key, so it is not set in the local layer or through a task's `run_settings`,
// and it is an open map,
// so `--set tree_scope.<tree>=<target>` writes one tree and reconcile carries the hub's trees whole.
// LoadConfig trims one trailing slash from each tree key.
// It refuses a key that is empty, holds a backslash or holds an empty, `.` or `..` segment.
// It also refuses two keys that are equal after the trim.
// Each refusal wraps ErrTreeScopeKey and names the key as written.
// ErrTreeScopeKey names no way forward, since `--set` cannot remove a key the hub file holds;
// a caller that refuses on a load error returns it through WithEditorWayForward.
// That wrapper appends the editor way forward.
// TreeScopedTargets classifies a diff's changed paths:
// a path lies in a tree when it equals the tree path or starts with it and a slash;
// a path touches every tree it lies in;
// the diff is tree-scoped when every path lies in some tree, and an empty path list is tree-scoped.
// The targets are the touched trees' non-empty targets, deduplicated, in tree-path order,
// so no path text reaches the call.
// When no touched tree contributes a target, as for a docs-only diff, the answer is scoped with no targets.
// Every darn site runs that as nothing.
// `lym config darn --set tree_scope.tools=./tools/... --set tree_scope.docs=` turns tree scope on for a hub.
// A tree-scoped diff gets no whole-module run before it lands.
// A break outside the listed trees reaches the parent unless the trees' tests or the guard-marked scans exercise it.
// The bound is what the hub lists as non-production.
//
// DarnTable composes the darn writer's session as a one-seat seat table.
// It takes a DarnInputs the caller tells it, the resolved Config and a model registry.
// The lead reads the decision record at the told path.
// Its scope is the board entry's body under `## Goal`, with the record's `Added after Discussion` entries over it.
// It writes neither the record nor the support log.
// It checks that no landed work or other entry overtook the board entry.
// It then edits and tests the repository and commits on the task branch.
// It writes the change description at the told path, in the format the final-summary spec defines.
// A lead that finds the entry outdated writes a stop report as its description.
// It creates the marker at the told stop marker path, and the producer halts the run on the marker.
// A lead that finds a stop condition the task's body names holds writes the report the body asks for as its description.
// It creates both that marker and a second one at the told condition stop marker path;
// the producer then halts the run with the report alone as the reason, since the report states its own way forward.
// The session is non-interactive and carries the description as its one output file.
// It renders `(none)` for prior work the spawn has nothing to tell for.
// A spawn that continues earlier work is told so through that value,
// and its stencil continues from the commits already on the task branch.
// The caller tells DarnTable whether a cold fixer clears the row's verify gate.
// With a cold fixer, the prompt's Step 4 has the writer and its subagents run no gate;
// the writer acts only on what the fixer leaves.
// Without one, the step has them run the gate after each commit.
// The seat engine renders the parent and edit directives, so DarnTable reads no parent name.
// DarnTable refuses an empty required field by name and a writer model-spec the registry cannot resolve.
//
// The package is told every path it reads and imports no resolver.
package darnengine
