// followup.go words the step that settles a batten run's record once a removal of its pair leaves the record standing, and reads the two facts that wording needs.

package pairteardown

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The batten row names the follow-up wording reads.
// They are held here, not imported, because batten's recipe package reaches a package whose tests import this one, which would make an import cycle in those tests.
// TestBattenFollowUp pins both against the recipe package's values.
const (
	landRow             = "Land"
	worktreeTeardownRow = "Worktree-Teardown"
)

// BattenFollowUp words the step that settles slug's batten run record after `lym fabric remove` of the run's pair, from the record, whether one stands, and whether an owner pause covers the slug.
// A slug with no record, a record past its Land row, and a slug an owner pause covers give "".
// A record before Land names the cancel that clears the record and the task branch the next add would refuse.
// A record at Land names the batten run whose failed stop words the restore of each branch the removal took, and never a cancel, since the run's work already reached its parent.
func BattenFollowUp(slug string, record shedengine.Status, found, ownerHeld bool) string {
	switch {
	case !found, ownerHeld, record.State == shedengine.StateDone, record.CurrentProducer == worktreeTeardownRow:
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

// OwnerPauseHeld reports whether an owner pause covers slug in the hub's pause record.
func OwnerPauseHeld(prime *lymcwd.Location, slug string) (bool, error) {
	geometry, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return false, err
	}
	coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, slug, time.Now())
	if err != nil {
		return false, err
	}
	return coverage.OwnerHeld, nil
}

// FollowUpFor words BattenFollowUp for slug's batten run on the hub's prime.
// A failed read of the record or of the owner pause is logged and words nothing.
func FollowUpFor(prime *lymcwd.Location, slug string) string {
	return followUpOver(
		slug,
		func(slug string) (shedengine.Status, bool, error) { return ReadBattenRecord(prime, slug) },
		func(slug string) (bool, error) { return OwnerPauseHeld(prime, slug) },
	)
}

// followUpOver words BattenFollowUp over the two reads.
// A nil read stands for no record and no owner pause, and a read that fails is logged and words nothing.
func followUpOver(slug string, record func(slug string) (shedengine.Status, bool, error), ownerHeld func(slug string) (bool, error)) string {
	if record == nil || ownerHeld == nil {
		return ""
	}
	status, found, err := record(slug)
	if err != nil {
		logger.Warn("pairteardown: the batten record could not be read, so the removal names no follow-up", "slug", slug, "cause", err)
		return ""
	}
	held, err := ownerHeld(slug)
	if err != nil {
		logger.Warn("pairteardown: the owner pause could not be read, so the removal names no follow-up", "slug", slug, "cause", err)
		return ""
	}
	return BattenFollowUp(slug, status, found, held)
}
