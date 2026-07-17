// weft.go implements weftCommit, the package-local helper every webster verb
// that reaches a batch-boundary commit point calls to stage, commit, and
// push the webster artifacts it just wrote (state.json, a batch report,
// outcome.yaml) through the weft junction -- copied verbatim from
// buildercli's own weftCommit (internal/buildercli/weft.go), including its
// lock-exclusion rationale: lock files (run.lock, mutate.lock -- advisory
// OS locks) are machine-local runtime artifacts, not webster state, so
// committing them would leak runtime noise into durable weft history and
// materialize stale lock files on every other machine's weft pull. Adapted
// for webster with one addition buildercli has no analog for: webster's
// rendered fork prompts (_lyx/webster/prompts/*) are machine-local
// re-renderable artifacts (BeginBatch rewrites each batch's own the next
// time it begins) -- committing them would be weft noise and a
// cross-machine confusion, the same class of exclusion as the pause flag,
// so they are excluded here too.
package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/weftengine"
)

// websterWeftPathspec returns the scoped _lyx pathspec every webster weft
// commit stages under, with the machine-local runtime artifacts excluded:
// any *.lock file (run.lock, mutate.lock -- advisory OS locks), the pause
// flag (_lyx/webster/<builderengine.PauseFlagName> -- webster reuses
// builder's own pause-flag mechanics by import, per the
// reuse-by-import-never-copy decision), and every rendered fork prompt
// (_lyx/webster/prompts/*). All three are per-machine or purely-derived
// runtime state, never durable webster state, so committing them would leak
// runtime noise into weft history and materialize on every other machine's
// weft pull -- the pause flag in particular could read as a spurious pause
// request elsewhere (it is present on disk during record-batch's terminal
// commit whenever a pause raced the last in-flight batch). Extracted from
// weftCommit so the exclusion set is asserted directly by a unit test rather
// than only implicitly through a live commit. The pause-flag and prompts
// patterns use a trailing "*/webster/..." glob so they match whether or not
// layout.RelPath prefixes the _lyx path.
func websterWeftPathspec(layout *hubgeometry.Layout) []string {
	return append(
		weftengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName}),
		":(exclude)*.lock",
		":(exclude)*/webster/"+builderengine.PauseFlagName,
		":(exclude)*/webster/prompts/*",
	)
}

// weftCommit stages and commits every change under layout's scoped _lyx
// pathspec (excluding the machine-local *.lock files, the pause flag, and
// the rendered fork prompts -- see websterWeftPathspec) through the weft
// junction, then pushes, using "webster: <label>" as the commit message. It
// reports whether a commit was actually made (false when there was nothing
// staged) and any error from either the commit or the push step --
// mirroring buildercli's weftCommit exactly.
func weftCommit(layout *hubgeometry.Layout, label string) (bool, error) {
	weftWorktree := layout.WeftWorktree()
	opts := weftengine.EnvSyncOptions()
	pathspec := websterWeftPathspec(layout)

	committed, err := weftengine.Commit(weftWorktree, pathspec, fmt.Sprintf("webster: %s", label), opts)
	if err != nil {
		return false, err
	}
	if err := weftengine.Push(weftWorktree, opts); err != nil {
		return committed, err
	}
	return committed, nil
}
