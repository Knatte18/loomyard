// weftbranch.go implements resolveWeftBranch, the one place a verb decides where a pair's weft branch comes from.
// The order is fixed: the local branch, else the branch on origin adopted as a local tracking branch, else none.
// When none exists, the caller forks.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// resolveWeftBranch finds weftBranch for a verb about to use it.
// A local refs/heads/<weftBranch> returns exists=true, fromOrigin=false.
// Otherwise, if consultOrigin is true and the weft repo has an origin remote, the branch is fetched from origin.
// It then creates a local tracking branch and returns exists=true, fromOrigin=true, recording KindBranchCreated on rec.
// Otherwise it returns exists=false and the caller forks.
// A weft repo with no origin remote skips the origin step.
// A failed probe, fetch or branch creation returns an error that names origin and the branch.
// It never falls through to exists=false.
// It pushes, replaces and deletes nothing on origin.
// readOrigin runs each of the probe and the fetch, so the caller decides whether a failed one runs again.
func resolveWeftBranch(rec *Mutations, l *lymcwd.Location, weftBranch string, consultOrigin bool, readOrigin func(side string, read func() error) error) (exists bool, fromOrigin bool, err error) {
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

	var tip string
	if err := readOrigin("weft", func() error {
		var err error
		tip, err = remoteHeadTip(weftRepoRoot, weftBranch)
		return err
	}); err != nil {
		return false, false, err
	}
	if tip == "" {
		return false, false, nil
	}

	ns, err := originNamespaceAt(weftRepoRoot)
	if err != nil {
		return false, false, err
	}
	trackingRef := originRemoteName + "/" + weftBranch
	if err := readOrigin("weft", func() error {
		_, err := gitrepo.New(weftRepoRoot).FetchWithArgs("--no-tags", originRemoteName, ns.Branch(weftBranch)+":refs/remotes/"+trackingRef)
		return err
	}); err != nil {
		return false, false, fmt.Errorf("fetch weft branch %q from %q: %w", weftBranch, originRemoteName, err)
	}
	if _, err := gitexec.Run([]string{"branch", "--track", weftBranch, trackingRef}, weftRepoRoot); err != nil {
		return false, false, fmt.Errorf("create weft branch %q tracking %q: %w", weftBranch, trackingRef, err)
	}
	rec.AppendRef(fabriclayout.KindBranchCreated, weftBranch, refDetail("records", weftRepoRoot, originRemoteName))
	return true, true, nil
}

// readOriginOnce runs read a single time, for a verb that does not run a failed origin read again.
func readOriginOnce(_ string, read func() error) error {
	return read()
}
