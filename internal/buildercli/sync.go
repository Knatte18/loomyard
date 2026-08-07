// sync.go implements fabricSync, the package-local helper every builder verb that reaches a
// batch-boundary commit point calls to stage, commit, and push the builder artifacts it just wrote
// (state.json, a batch report, outcome.yaml) through the fabric repo via
// fabricengine.Fabric.Commit.
// Machine-local runtime artifacts (run.lock, state.json.lock, both round-loop modules' pause flags,
// webster's rendered fork prompts) are never staged in the first place: they are excluded solely by
// the fabric repo's .git/info/exclude, seeded by fabricengine's seedWeftArtifactExcludes -- this
// helper passes only a positive pathspec, with no ":(exclude)" magic of its own.

package buildercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// fabricSync stages and commits every change under layout's scoped _lyx
// pathspec through the fabric repo. It reports whether a commit was made
// (false when nothing staged) and any error.
func fabricSync(layout *lyxcwd.Location, label string) (bool, error) {
	opts := fabricengine.EnvSyncOptions()
	files := fabricengine.ScopedPathspec(layout.AnchorRel, []string{configengine.LyxDirName})

	// Check SkipGit before fabricengine.Open's stat validation to avoid
	// requiring a real fabric repo on disk in CI/test bypass mode.
	var committed bool
	if !opts.SkipGit {
		f, err := fabricengine.Open(layout)
		if err != nil {
			return false, err
		}
		res, err := f.Commit(files, fmt.Sprintf("builder: %s", label), nil, opts)
		committed = res.Committed()
		if err != nil {
			return committed, err
		}
	}
	return committed, nil
}
