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

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/weftengine"
)

// weftCommit stages and commits every change under layout's scoped _lyx
// pathspec (excluding *.lock files) through the weft junction, then pushes,
// using "builder: <label>" as the commit message. It reports whether a
// commit was actually made (false when there was nothing staged) and any
// error from either the commit or the push step -- mirroring perchcli's
// block-exit sync exactly, including its exclude-*.lock pathspec entry.
func weftCommit(layout *hubgeometry.Layout, label string) (bool, error) {
	weftWorktree := layout.WeftWorktree()
	opts := weftengine.EnvSyncOptions()
	pathspec := append(
		weftengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName}),
		":(exclude)*.lock",
	)

	committed, err := weftengine.Commit(weftWorktree, pathspec, fmt.Sprintf("builder: %s", label), opts)
	if err != nil {
		return false, err
	}
	if err := weftengine.Push(weftWorktree, opts); err != nil {
		return committed, err
	}
	return committed, nil
}
