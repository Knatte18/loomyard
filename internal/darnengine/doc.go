// Package darnengine owns the darn recipe's hub-wide configuration.
// It also owns the tree scope classification that every darn verify site reads.
//
// `darn.yaml` is a hub-wide config module, so a task cannot weaken `darn.tree_scope` through its own tree.
// LoadConfig loads it strictly and validates the `darn.writer` model-spec at load time.
// It refuses a `darn.report_dir` that leaves the worktree or has a records directory in its path.
// The Code-Review perch's settings come from `loom.yaml`.
// The driver, tackler and arbiter keys come from `shed.yaml` alone;
// setting one in darn.yaml is refused, naming the `shed` module.
//
// IsMergeTaskWriter reports whether a strand is the writer of a merge task, from the strand name alone.
// IsBenchPairSlug reports whether a slug names a bench pair of a task: the task's slug, `-b` and a positive number.
// A darn writer makes such a pair for its own bench, forked from its task worktree.
//
// Tree scope applies to a diff wholly under the hub's non-production trees.
// Each darn verify site then runs its own gate key over the touched trees' targets, in place of the key's scope.
// `darn.tree_scope` maps a worktree-relative tree path to its test target; an empty target means the tree has no tests.
// It is a guard key, so it is not set in the local layer or through `run_settings`.
// LoadConfig refuses a malformed or duplicate tree key, wrapping ErrTreeScopeKey and naming the key as written.
// TreeScopedTargets classifies a diff's changed paths; an empty path list is tree-scoped.
// A tree-scoped diff gets no whole-module run before it lands.
// A break outside the listed trees reaches the parent unless the trees' tests or the guard-marked scans exercise it.
//
// DarnTable composes the darn writer session as a one-seat seat table from a DarnInputs, the Config and a model registry.
// The session is non-interactive and carries the change description as its one output file.
// DarnTable refuses an empty required field by name and a writer model-spec the registry cannot resolve.
//
// The package is told every path it reads and imports no resolver.
package darnengine
