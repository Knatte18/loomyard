// forkrecords.go forks a new pair's weft branch without the parent's shed run records.
//
// A weft branch forked from its parent inherits every committed run directory the parent carries, including a run record for the child's own slug, which makes the child's seed refuse a disagreeing one.
// createWeftWorktreeDroppingRuns stops the inheritance at its source: it never writes the run-records root to the new worktree's disk,
// and Add's first weft commit records the root's deletion.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// createWeftWorktreeDroppingRuns creates a new weft worktree on branch, forking from startPoint like createWeftWorktree but leaving the run-records root (shedrun.RunsRootRel under l's anchor) out of the working tree.
// It reports whether startPoint tracks anything under that root, which is what tells the caller its first commit has deletions to stage.
//
// The fork is `worktree add --no-checkout`, so nothing is written yet, and the same two mutations createWeftWorktree records are recorded right after it succeeds.
// The working tree is then written from the index:
// when the root is untracked, `read-tree HEAD` and `checkout-index -a -u` write everything;
// when it is tracked, the root leaves the index (`rm -r --cached`) for the write-out and is restored from HEAD (`read-tree -m HEAD`) afterwards,
// so its entries stay tracked but absent on disk.
// The drop is an index manipulation on a worktree that never had the files, so it deletes nothing and stays outside the Fabric Destruction Chokepoint.
// An index-only removal after a full checkout would not do: shedrun reads seeds from disk.
func createWeftWorktreeDroppingRuns(rec *Mutations, l *lyxcwd.Location, slug, branch, startPoint string) (rootTracked bool, err error) {
	weftPath := RecordsWorktreePath(l, slug)
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return false, fmt.Errorf("resolve weft repo root: %w", err)
	}
	if err := containedWorktreeAdd(weftRepoRoot, l.HubPath, weftPath, func(worktreePath string) []string {
		return []string{"worktree", "add", "--no-checkout", "-b", branch, worktreePath, startPoint}
	}); err != nil {
		return false, fmt.Errorf("create weft worktree %q for branch %q failed: %w", weftPath, branch, err)
	}
	rec.Append(KindWorktreeCreated, weftPath, "")
	rec.AppendRef(KindBranchCreated, branch, refDetail("weft", weftRepoRoot, ""))

	root := ScopedPathspec(l.AnchorRel, []string{shedrun.RunsRootRel()})[0]
	root = filepath.ToSlash(root)

	if _, err := gitexec.Run([]string{"read-tree", "HEAD"}, weftPath); err != nil {
		return false, fmt.Errorf("populate weft index in %q: %w", weftPath, err)
	}
	rootTracked, err = PathTracked(weftPath, root)
	if err != nil {
		return false, err
	}
	if rootTracked {
		if _, err := gitexec.Run([]string{"rm", "-r", "--cached", "-q", "--", root}, weftPath); err != nil {
			return false, fmt.Errorf("drop run records from weft index in %q: %w", weftPath, err)
		}
	}
	if _, err := gitexec.Run([]string{"checkout-index", "-a", "-u"}, weftPath); err != nil {
		return false, fmt.Errorf("write weft worktree %q: %w", weftPath, err)
	}
	if rootTracked {
		if _, err := gitexec.Run([]string{"read-tree", "-m", "HEAD"}, weftPath); err != nil {
			return false, fmt.Errorf("restore run records in weft index in %q: %w", weftPath, err)
		}
	}
	if _, err := ensureWeftLockDirAt(weftPath); err != nil {
		return false, fmt.Errorf("create weft lock dir in %q: %w", weftPath, err)
	}
	return rootTracked, nil
}
