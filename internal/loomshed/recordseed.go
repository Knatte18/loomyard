// recordseed.go implements the Record-Seed producer, which is Record-Write in a stored-only mode.
// It seeds a loom run's decision record from its board entry's stored record and never from the entry's body.

package loomshed

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ReseedValues are the values the re-seed of a run with no Discussion row is written with.
type ReseedValues struct {
	// Recipe is the recipe the task runs again under, one that discusses it first.
	Recipe string
	// Driver is the driver the run's seed records.
	Driver string
	// Parent is the parent branch the run's seed records.
	Parent string
}

// RecordSeedDeps are the injected seams and absolute paths of the Record-Seed producer.
type RecordSeedDeps struct {
	// RecordPath is the absolute path of the decision record.
	RecordPath string
	// SupportLogPath is the absolute path of the support log.
	SupportLogPath string
	// DiscussionDir is the absolute path of the directory holding both files.
	DiscussionDir string
	// ReadStored returns the stored record; found is true only when the entry is flagged discussed and its file exists.
	ReadStored func() (record []byte, found bool, err error)
	// Commit commits the discussion directory.
	// It is a no-op on a directory with no uncommitted change.
	Commit func() error
	// DiscussionRow is the row of the run's recipe that runs Discussion, empty when the recipe holds none.
	// A run with such a row goes back to it when there is no record to seed.
	DiscussionRow string
	// Slug is the run's board entry slug, which the way forward of a run with no Discussion row names.
	Slug string
	// SeedPath is the absolute path of the run's seed file.
	SeedPath string
	// ReadSeed returns the values the re-seed of a run with no Discussion row is written with.
	ReadSeed func() (ReseedValues, error)
}

// SeedParentParam is the seed parameter that records a loom run's parent branch.
const SeedParentParam = "parent"

// noStoredRecordReason opens the Stuck reason of a run with no record to seed.
const noStoredRecordReason = "the decision record is missing and the board entry holds no stored decision record, so there is no record to seed; "

// NewRecordSeed returns the producer of the Record-Seed row, identified as name.
// Its Call removes the torn temp files a crash left in the discussion directory.
// It then writes whichever of the record and the support log is missing, and commits the pair.
// A missing record is the board entry's stored record, written verbatim;
// a missing support log is the stored-record log when the entry has a stored record.
// With no stored record, the missing log is one that makes no claim about the record's source.
// A file that exists is never overwritten, so an approved record and every entry added to it survive.
// With the record missing and no stored record, the row is Stuck with nothing written.
// The reason's way forward takes a run with a Discussion row back to it.
// For a run with none it names the re-run as tapestry.
// It gives the steps for a pair batten made and for any other pair, with the seed's path, driver and parent filled in.
func NewRecordSeed(name string, deps RecordSeedDeps) shedengine.ShedProducer {
	return &recordWrite{
		name: name,
		deps: RecordWriteDeps{
			RecordPath:     deps.RecordPath,
			SupportLogPath: deps.SupportLogPath,
			DiscussionDir:  deps.DiscussionDir,
			ReadStored:     deps.ReadStored,
			Commit:         deps.Commit,
		},
		noStoredReason: func() (string, error) { return recordSeedStuckReason(deps) },
	}
}

// recordSeedStuckReason renders the Stuck reason of a run whose record is missing and whose entry holds no stored record.
func recordSeedStuckReason(deps RecordSeedDeps) (string, error) {
	if deps.DiscussionRow != "" {
		return fmt.Sprintf("%sway forward: \"lym loom goto --to %s\", then \"lym loom resume\"", noStoredRecordReason, deps.DiscussionRow), nil
	}

	seed, err := deps.ReadSeed()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%sway forward: run the task as %s, which discusses it first. "+
		"For a pair batten made, from the hub's prime worktree: \"lym batten cancel %s --remove\", "+
		"\"lym board upsert '{\"slug\":\"%s\",\"recipe\":\"%s\"}'\", "+
		"\"lym board set-status '{\"slug\":\"%s\",\"status\":null}'\", then \"lym batten run %s\". "+
		"For any other pair, from its task worktree: \"rm '%s'\", "+
		"\"lym shed seed self --recipe %s --driver %s --param %s=%s\", "+
		"\"lym loom goto --to %s\", \"lym loom start\", then \"lym loom resume\"",
		noStoredRecordReason, seed.Recipe,
		deps.Slug, deps.Slug, seed.Recipe, deps.Slug, deps.Slug,
		deps.SeedPath, seed.Recipe, seed.Driver, SeedParentParam, seed.Parent, NameDiscussionWrite), nil
}
