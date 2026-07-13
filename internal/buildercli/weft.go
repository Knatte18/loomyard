// weft.go implements weftCommit, the package-local helper every builder verb
// that reaches a batch-boundary commit point calls to stage, commit, and
// push the builder artifacts it just wrote (state.json, a batch report,
// outcome.yaml) through the weft junction -- copied from perchcli's own
// block-exit weft Commit+Push (internal/perchcli/run.go), including its
// lock-exclusion rationale: lock files (run.lock, state.json.lock) are
// machine-local advisory-lock artifacts, not builder state, so committing
// them would leak runtime noise into durable weft history and materialize
// stale lock files on every other machine's weft pull.

package buildercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/weftengine"
)

// builderWeftPathspec returns the scoped _lyx pathspec every builder weft
// commit stages under, with the machine-local runtime artifacts excluded: any
// *.lock file (run.lock, state.json.lock -- advisory OS locks) and the pause
// flag (_lyx/builder/<PauseFlagName>). Both are per-machine runtime state,
// never durable builder state, so committing them would leak runtime noise
// into weft history and materialize on every other machine's weft pull -- the
// pause flag in particular could read as a spurious pause request elsewhere
// (it is present on disk during poll's terminal commit whenever a pause raced
// the last in-flight batch). Extracted from weftCommit so the exclusion set is
// asserted directly by a unit test rather than only implicitly through a live
// commit. The pause-flag pattern uses a trailing "*/builder/<flag>" glob so it
// matches whether or not layout.RelPath prefixes the _lyx path.
func builderWeftPathspec(layout *hubgeometry.Layout) []string {
	return append(
		weftengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName}),
		":(exclude)*.lock",
		":(exclude)*/builder/"+builderengine.PauseFlagName,
	)
}

// weftCommit stages and commits every change under layout's scoped _lyx
// pathspec (excluding the machine-local *.lock files and pause flag -- see
// builderWeftPathspec) through the weft junction, then pushes, using
// "builder: <label>" as the commit message. It reports whether a commit was
// actually made (false when there was nothing staged) and any error from
// either the commit or the push step -- mirroring perchcli's block-exit sync
// exactly.
func weftCommit(layout *hubgeometry.Layout, label string) (bool, error) {
	weftWorktree := layout.WeftWorktree()
	opts := weftengine.EnvSyncOptions()
	pathspec := builderWeftPathspec(layout)

	committed, err := weftengine.Commit(weftWorktree, pathspec, fmt.Sprintf("builder: %s", label), opts)
	if err != nil {
		return false, err
	}
	if err := weftengine.Push(weftWorktree, opts); err != nil {
		return committed, err
	}
	return committed, nil
}
