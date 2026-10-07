// remoteonly.go implements Fabric.RemoteOnlyCommits, the read of how far the remote task branch has run ahead of the warp checkout's HEAD.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// RemoteOnlyCommits fetches the warp origin and returns the tip of the remote branch named like the warp checkout's current branch,
// and the commits on it that HEAD lacks, newest first.
// The tip and the commit list are both empty when the repository has no origin remote or the remote has no such branch;
// a remote branch at or behind HEAD returns its tip and no commits.
// A failed fetch or branch read returns the error, and the caller decides whether it refuses or degrades.
// It mutates no ref other than the remote-tracking refs the fetch refreshes.
func (f *Fabric) RemoteOnlyCommits() (tip string, commits []string, err error) {
	if _, urlErr := f.warp.RemoteURL(originRemoteName); urlErr != nil {
		return "", nil, nil
	}

	branch, err := f.warp.CurrentBranch()
	if err != nil {
		return "", nil, fmt.Errorf("fabricengine: remote-only commits: %w", err)
	}
	head, err := f.warp.CurrentSHA()
	if err != nil {
		return "", nil, fmt.Errorf("fabricengine: remote-only commits: %w", err)
	}

	// The tip is read from the remote itself, not the tracking ref: a branch deleted on the remote leaves a stale tracking ref behind.
	tips, err := remoteBranchTips(f.warpPath)
	if err != nil {
		return "", nil, err
	}
	tip, ok := tips[branch]
	if !ok {
		return "", nil, nil
	}

	// The fetch comes after the tip is read, so a tip that advances in between still has its ancestor's objects local.
	if _, err := gitexec.Run([]string{"fetch", "--no-tags", originRemoteName}, f.warpPath); err != nil {
		return "", nil, fmt.Errorf("fabricengine: remote-only commits: fetch %q: %w", originRemoteName, err)
	}

	commits, err = f.warp.CommitsNotIn(tip, head)
	if err != nil {
		return "", nil, fmt.Errorf("fabricengine: remote-only commits: %w", err)
	}
	return tip, commits, nil
}
