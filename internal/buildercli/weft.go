// weft.go implements weftCommit, the package-local helper every builder verb that reaches a batch-boundary commit point calls to stage, commit, and push the builder artifacts it just wrote (state.json, a batch report, outcome.yaml) through the weft junction via fabricengine.Fabric.Commit.
// Machine-local runtime artifacts (run.lock, state.json.lock, both round-loop modules' pause flags, webster's rendered fork prompts) are never staged in the first place: they are excluded solely by the weft repo's .git/info/exclude, seeded by fabricengine's seedWeftArtifactExcludes -- this helper passes only a positive pathspec, with no ":(exclude)" magic of its own.

package buildercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// weftCommit stages and commits every change under layout's scoped _lyx
// pathspec through the weft junction. It reports whether a commit was made
// (false when nothing staged) and any error.
func weftCommit(layout *lyxcwd.Location, label string) (bool, error) {
	weftWorktree := fabricengine.WeftWorktree(layout)
	opts := fabricengine.EnvSyncOptions()
	files := fabricengine.ScopedPathspec(layout.AnchorRel, []string{configengine.LyxDirName})

	// Check SkipGit before fabricengine.New's stat validation to avoid
	// requiring a real weft worktree on disk in CI/test bypass mode.
	var committed bool
	if !opts.SkipGit {
		f, err := fabricengine.New(layout.WorktreePath(), weftWorktree)
		if err != nil {
			return false, err
		}
		res, err := f.Commit(files, fmt.Sprintf("builder: %s", label), nil, opts)
		committed = res.WeftCommitted
		if err != nil {
			return committed, err
		}
	}
	return committed, nil
}
