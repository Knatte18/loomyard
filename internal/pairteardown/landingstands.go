// landingstands.go holds the one read of whether a pair may be removed while a landing it carries or holds is still owed,
// and the refusal and ledger cleanup the teardown routes build on it.

package pairteardown

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
)

// ErrLandingStands is the sentinel error of a teardown refused while a landing of the pair is still owed.
var ErrLandingStands = errors.New("pairteardown: the run's landing entry stands")

// LandingStands reports whether the pair of slug at pair must stay for a landing still owed in the hub at hubPath.
// That holds while a landing of the run stands on any parent, and while the pair carries the landings of other runs.
// With git and pushes on it also holds for a landing commit of the run on the parent's branch that origin lacks.
// Such a commit counts only when no landing records it and its board entry is not done.
// A lost ledger therefore does not free the pair.
// A dropped landing holds nothing.
// A pair whose code worktree is gone has no parent branch to read.
func LandingStands(hubPath, slug string, pair *lymcwd.Location) (bool, error) {
	entries, err := pushledger.List(hubPath)
	if err != nil {
		return false, err
	}
	pairKey := pushledger.RecordsKey(fabricengine.RecordsWorktree(pair))
	dropped := false
	for key, entry := range entries {
		for _, landing := range entry.Landings {
			if landing.Run == slug || key == pairKey {
				return true, nil
			}
		}
		dropped = dropped || entry.Dropped[slug] != ""
	}
	options := fabricengine.EnvSyncOptions()
	if dropped || options.SkipGit || options.SkipPush {
		return false, nil
	}
	return unrecordedLandingStands(slug, pair)
}

// unrecordedLandingStands reports whether the parent of the pair carries an unsettled landing commit of the run slug.
func unrecordedLandingStands(slug string, pair *lymcwd.Location) (bool, error) {
	if _, err := os.Stat(pair.WorktreePath()); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	origin, found, err := fabricengine.ReadOriginFor(pair, slug)
	if err != nil {
		// A half-made pair has no readable parent, and its removal must still run.
		logger.Warn("pairteardown: origin record not read, so no unrecorded landing holds the pair", "slug", slug, "error", err)
		return false, nil
	}
	if !found {
		return false, nil
	}
	name := origin.ParentWorktree
	if name == "" {
		if name, err = fabricengine.PrimeName(pair); err != nil {
			return false, fmt.Errorf("pairteardown: find the parent of %q: %w", slug, err)
		}
	}
	parent, err := lymcwd.NamedWorktree(pair.HubPath, name)
	if err != nil {
		return false, fmt.Errorf("pairteardown: locate the parent of %q: %w", slug, err)
	}
	unsettled, err := fabricengine.UnsettledLandings(parent)
	if err != nil {
		return false, fmt.Errorf("pairteardown: read the landing commits on the parent of %q: %w", slug, err)
	}
	_, stands := unsettled[slug]
	return stands, nil
}

// standingLanding reads the teardown's landing seam; a Teardown without one reads none standing.
func (t *Teardown) standingLanding(slug string) (bool, error) {
	if t.landingStands == nil {
		return false, nil
	}
	return t.landingStands(slug)
}

// refuseWhileLandingStands refuses a teardown of slug while its landing stands.
// retry is the command the operator runs again, without quoting.
func (t *Teardown) refuseWhileLandingStands(slug, retry string) error {
	stands, err := t.standingLanding(slug)
	if err != nil || !stands {
		return err
	}
	return fmt.Errorf("%w: a landing push that %q carries or holds is still owed, so nothing is removed; way forward: wait for the pusher to push the parent branch, or run the step the parent's refused-push record names, then run `%s` again",
		ErrLandingStands, slug, retry)
}

// dropPairLedgerEntry removes the push ledger entry of the removed pair of slug, with the records commit it held back.
// An entry holding landings is kept, and a failure is logged since the pair is already gone.
func (t *Teardown) dropPairLedgerEntry(slug string) {
	key := pushledger.RecordsKey(fabricengine.RecordsWorktreePath(t.prime, slug))
	entries, err := pushledger.List(t.prime.HubPath)
	if err == nil {
		if _, found := entries[key]; !found {
			return
		}
		err = pushledger.Update(t.prime.HubPath, key, time.Now(), func(entry pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
			return entry, len(entry.Landings) > 0, nil
		})
	}
	if err != nil {
		logger.Warn("pairteardown: the push ledger entry of a removed pair was not removed", "slug", slug, "error", err)
	}
}
