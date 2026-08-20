// sync.go implements fabricSync, the package-local helper every webster verb that reaches a
// batch-boundary commit point calls to stage, commit, and push the webster artifacts it just wrote
// (state.json, a batch report, outcome.yaml) through the fabric repo via
// fabricengine.Fabric.Commit.
// Machine-local runtime artifacts (run.lock, mutate.lock, both round-loop modules' pause flags,
// webster's rendered fork prompts) are never staged in the first place: they live under .lyx and so
// never reach the pathspec this helper stages against -- this helper passes only a positive pathspec,
// with no ":(exclude)" magic of its own.
// fabricSync takes its fabric handle through a lazy opener, never a *lyxcwd.Location, and its
// pathspec scope through a told anchor-relative path: fabricengine.Fabric exposes no anchor-relative
// path of its own, so the scope must be told separately from the handle. A nil opener means standalone
// mode, which has no fabric repo by construction -- fabricSync reports (false, nil) without touching
// git in that case.
package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// fabricSync stages and commits changes under anchorRel's _lyx pathspec through the fabric repo
// obtained by calling open, or reports (false, nil) without touching git when open is nil (standalone
// mode).
func fabricSync(open func() (*fabricengine.Fabric, error), anchorRel, label string) (bool, error) {
	opts := fabricengine.EnvSyncOptions()

	if open == nil {
		return false, nil
	}

	files := fabricengine.ScopedPathspec(anchorRel, []string{lyxdirs.LyxDirName})

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
