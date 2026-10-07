// weftbranch.go implements resolveWeftBranch, the one place a verb decides where a pair's weft branch comes from.
// The order is fixed: the local branch, else the branch on origin adopted as a local tracking branch, else none,
// and the caller forks.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// resolveWeftBranch finds weftBranch for a verb about to use it.
// A local refs/heads/<weftBranch> returns exists=true, fromOrigin=false.
// Otherwise, when consultOrigin is true and the weft repo has an origin remote, a branch on origin is fetched into its remote-tracking ref and a local branch tracking it is created;
// that returns exists=true, fromOrigin=true and records KindBranchCreated on rec.
// Otherwise it returns exists=false and the caller forks.
// A weft repo with no origin remote skips the origin step.
// A failed probe, fetch or branch creation is a returned error naming origin and the branch, never a fall-through to exists=false.
// It pushes, replaces and deletes nothing on origin.
func resolveWeftBranch(rec *Mutations, l *lyxcwd.Location, weftBranch string, consultOrigin bool) (exists bool, fromOrigin bool, err error) {
	if weftBranchExists(l, weftBranch) {
		return true, false, nil
	}
	if !consultOrigin {
		return false, false, nil
	}

	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return false, false, fmt.Errorf("resolve weft branch %q: resolve weft repo: %w", weftBranch, err)
	}
	if _, urlErr := gitrepo.New(weftRepoRoot).RemoteURL(originRemoteName); urlErr != nil {
		return false, false, nil
	}

	tip, err := remoteHeadTip(weftRepoRoot, weftBranch)
	if err != nil {
		return false, false, err
	}
	if tip == "" {
		return false, false, nil
	}

	trackingRef := originRemoteName + "/" + weftBranch
	if _, err := gitexec.Run(
		[]string{"fetch", "--no-tags", originRemoteName, "refs/heads/" + weftBranch + ":refs/remotes/" + trackingRef},
		weftRepoRoot,
	); err != nil {
		return false, false, fmt.Errorf("fetch weft branch %q from %q: %w", weftBranch, originRemoteName, err)
	}
	if _, err := gitexec.Run([]string{"branch", "--track", weftBranch, trackingRef}, weftRepoRoot); err != nil {
		return false, false, fmt.Errorf("create weft branch %q tracking %q: %w", weftBranch, trackingRef, err)
	}
	rec.AppendRef(KindBranchCreated, weftBranch, refDetail("records", weftRepoRoot, originRemoteName))
	return true, true, nil
}
