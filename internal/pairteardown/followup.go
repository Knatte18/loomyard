// followup.go words the step that settles a batten run's record once a removal of its pair leaves the record standing.
// It also reads the fact that wording needs.

package pairteardown

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The batten row names the follow-up wording and ReadRun read.
// They are held here, not imported: batten's recipe package reaches a package whose tests import this one.
// Importing it would make an import cycle in those tests.
// TestReadRun pins runShedRow against the recipe package's value.
// It pins the rows ReadRun counts as started against the recipe too.
const (
	runShedRow          = "Run-Shed"
	landRow             = "Land"
	worktreeTeardownRow = "Worktree-Teardown"
)

// BattenFollowUp words the step that settles slug's batten run record after `lym fabric remove` of the run's pair.
// The wording depends on the record and on whether one stands.
// A slug with no record and a record past its Land row give "".
// A record before Land names the cancel that clears the record and the task branch the next add would refuse.
// A record at Land names the batten run whose failed stop words the restore of each branch the removal took.
// It never names a cancel, since the run's work already reached its parent.
func BattenFollowUp(slug string, record shedengine.Status, found bool) string {
	switch {
	case !found, record.State == shedengine.StateDone, record.CurrentProducer == worktreeTeardownRow:
		return ""
	case record.CurrentProducer == landRow:
		return fmt.Sprintf("the run's work already reached its parent, so never cancel it: run `lym batten run %s`, whose failed stop names the restore of each branch the removal took, run those steps, then `lym batten run %s` again", slug, slug)
	}
	return fmt.Sprintf("then run `lym batten cancel %s --remove`, which clears the run's record and the task branch the next run's add would refuse", slug)
}

// ReadBattenRecord reads the batten run record of slug on the hub's prime; found is false when the run has none.
func ReadBattenRecord(prime *lymcwd.Location, slug string) (shedengine.Status, bool, error) {
	return readStatus(shedrun.StatusFile(prime, slug), shedrun.StatusLock(prime, slug))
}

// removeCancelledRunRecord deletes slug's batten run record on the hub's prime when its state is cancelled.
// It reports whether it deleted the record.
// A cancelled run keeps its record while its pair stands, and nothing reads it once the pair is gone.
// The deletion is committed on the prime's records side when the records HEAD tracks the run, and its push is spawned.
// A push that fails to start is logged, since the next push carries the commit.
// A record in any other state, or none, is left alone.
func (t *Teardown) removeCancelledRunRecord(slug string) (bool, error) {
	record, found, err := ReadBattenRecord(t.prime, slug)
	if err != nil {
		return false, fmt.Errorf("read the batten run record of %s: %w", slug, err)
	}
	if !found || record.State != shedengine.StateCancelled {
		return false, nil
	}

	statusLock, err := lock.AcquireWriteLock(shedrun.StatusLock(t.prime, slug))
	if err != nil {
		return false, fmt.Errorf("take the status lock of %s: %w", slug, err)
	}
	defer func() {
		if err := statusLock.Release(); err != nil {
			logger.Warn("pairteardown: could not release the status lock after deleting a cancelled run record", "slug", slug, "cause", err)
		}
	}()

	if err := os.RemoveAll(shedrun.RunDir(t.prime, slug)); err != nil {
		return false, fmt.Errorf("remove the run directory of %s: %w", slug, err)
	}
	tracked, err := recordsTrackRun(t.prime, slug)
	if err != nil {
		return false, fmt.Errorf("read whether the records HEAD tracks the run of %s: %w", slug, err)
	}
	if !tracked {
		return true, nil
	}
	_, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), t.prime, []string{shedrun.RunDirRel(t.prime, slug)},
		"fabric remove: remove the cancelled run record of "+slug, fabricengine.EnvSyncOptions())
	if err != nil {
		return false, fmt.Errorf("commit the removal of the run record of %s: %w", slug, err)
	}
	if committed {
		if _, err := fabricengine.SpawnStatusPush(t.prime, ""); err != nil {
			logger.Warn("pairteardown: the push of a cancelled run record's removal did not start; the next records push carries it", "slug", slug, "cause", err)
		}
	}
	return true, nil
}

// recordsTrackRun reports whether the prime's records HEAD holds the status or the seed of slug's run.
func recordsTrackRun(prime *lymcwd.Location, slug string) (bool, error) {
	for _, rel := range []string{shedrun.StatusRel(prime, slug), shedrun.SeedRel(prime, slug)} {
		_, tracked, err := fabricengine.CommittedAnchoredFile(prime, rel)
		if err != nil || tracked {
			return tracked, err
		}
	}
	return false, nil
}

// FollowUpFor words the step that settles slug's batten run record on the hub's prime.
// A failed read of the record is logged and words nothing.
func FollowUpFor(prime *lymcwd.Location, slug string) string {
	return followUpOver(slug, func(slug string) (shedengine.Status, bool, error) { return ReadBattenRecord(prime, slug) })
}

// followUpOver words the step that settles slug's batten run record from the read it is given.
// A nil read, or a read that fails, words nothing; a failed read is also logged.
func followUpOver(slug string, record func(slug string) (shedengine.Status, bool, error)) string {
	if record == nil {
		return ""
	}
	status, found, err := record(slug)
	if err != nil {
		logger.Warn("pairteardown: the batten record could not be read, so the removal names no follow-up", "slug", slug, "cause", err)
		return ""
	}
	return BattenFollowUp(slug, status, found)
}
