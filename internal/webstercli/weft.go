// weft.go implements weftCommit, the package-local helper every webster verb
// that reaches a batch-boundary commit point calls to stage, commit, and
// push the webster artifacts it just wrote (state.json, a batch report,
// outcome.yaml) through the weft junction via fabricengine.Fabric.Commit.
// Machine-local runtime artifacts (run.lock, mutate.lock, both round-loop
// modules' pause flags, webster's rendered fork prompts) are never staged in
// the first place: they are excluded solely by the weft repo's
// .git/info/exclude, seeded by fabricengine's seedWeftArtifactExcludes --
// this helper passes only a positive pathspec, with no ":(exclude)" magic of
// its own.
package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// weftCommit stages and commits every change under layout's scoped _lyx
// pathspec through the weft junction, then fires an async push, using
// "webster: <label>" as the commit subject. Per Fabric.Commit's contract,
// the weft commit also carries a "Warp-SHA: <host HEAD>" trailer in its own
// blank-line-separated paragraph and records a warp<->weft correspondence
// entry in the weft gitdir's index. It reports whether a weft commit was
// actually made (false when there was nothing staged) and any error from
// the commit step.
func weftCommit(layout *hubgeometry.Layout, label string) (bool, error) {
	weftWorktree := layout.WeftWorktree()
	opts := fabricengine.EnvSyncOptions()
	files := fabricengine.ScopedPathspec(layout.RelPath, []string{hubgeometry.LyxDirName})

	// SkipGit is checked here, before fabricengine.New's stat-based path
	// validation: the CI/test bypass must never require a real weft
	// worktree to exist on disk, but New (unlike Commit itself) validates
	// both paths unconditionally.
	var committed bool
	if !opts.SkipGit {
		f, err := fabricengine.New(layout.WorktreeRoot, weftWorktree)
		if err != nil {
			return false, err
		}
		res, err := f.Commit(files, fmt.Sprintf("webster: %s", label), nil, opts)
		committed = res.WeftCommitted
		if err != nil {
			return committed, err
		}
	}
	return committed, nil
}
