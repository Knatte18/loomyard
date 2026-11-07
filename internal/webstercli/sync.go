// sync.go implements fabricSync, the package-local helper that each batch-boundary commit point calls.
// It stages, commits and pushes the webster artifacts just written: state.json, a batch report and outcome.yaml.
// It does so through the fabric repo, using fabricengine.Fabric.Commit.
// Machine-local runtime artifacts never reach the pathspec this helper stages against.
// They live under .lym: run.lock, mutate.lock, treadle's round-loop pause flag and webster's rendered fork prompts.
// The pathspec is positive only, with no ":(exclude)" magic of its own.
// fabricSync takes its fabric handle from a lazy opener, and its scope from a separate anchor-relative path.
// The scope is passed separately because fabricengine.Fabric exposes no anchor-relative path of its own.
// A nil opener means standalone mode, which has no fabric repo:
// fabricSync then reports (false, nil) without touching git.
package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// fabricSync stages and commits the _lym pathspec under anchorRel through the fabric repo that open returns.
// A nil open means standalone mode: fabricSync then reports (false, nil) without touching git.
func fabricSync(open func() (*fabricengine.Fabric, error), anchorRel, label string) (bool, error) {
	opts := fabricengine.EnvSyncOptions()

	if open == nil {
		return false, nil
	}

	files := fabricengine.ScopedPathspec(anchorRel, []string{lymdirs.LymDirName})

	var committed bool
	if !opts.SkipGit {
		f, err := open()
		if err != nil {
			return false, err
		}
		res, err := f.Commit(files, fmt.Sprintf("webster: %s", label), nil, opts)
		committed = res.Committed()
		if err != nil {
			return committed, err
		}
	}
	return committed, nil
}
