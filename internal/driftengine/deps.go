// deps.go — the told seams the Drift producers share, and the portable config of one Drift row.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// Deps holds the seams the Drift producers read, so a test substitutes each one.
// A producer fails loudly, naming the field, when a seam it reads is nil.
type Deps struct {
	// Paths are the drift paths of the task's anchor root.
	Paths Paths
	// WorktreeRoot is the task worktree.
	WorktreeRoot string
	// Config loads the hub's drift config.
	Config func() (Config, error)
	// Status reads the run's status.
	Status func() (shedengine.Status, error)
	// Rows is the run's recipe row order, which the past-row check reads against the status row.
	Rows []string
	// Head, BranchPoint and DirtyPaths read the code checkout's HEAD, its branch point and its tracked dirty paths.
	Head        func() (string, error)
	BranchPoint func() (string, error)
	DirtyPaths  func() ([]string, error)
	// Commit commits the told anchor-relative paths under the durable drift directory with the run's records.
	Commit func(relPaths []string) error
	// CommitWithTrailers commits the told anchor-relative paths as one records commit.
	// The told trailers follow the commit's own code-head trailer.
	CommitWithTrailers func(relPaths []string, trailers []fabricengine.Trailer) error
	// TrailerCommit finds the newest records commit carrying the trailer key when newest is true, and the oldest otherwise; found is false when none carries it.
	TrailerCommit func(key string, newest bool) (fabricengine.TrailerCommit, bool, error)
	// FileAt reads an anchor-relative file as stored in the records commit rev; found is false when the commit lacks it.
	FileAt func(rev, relPath string) ([]byte, bool, error)
	// Seats and Now are the seat runner and the clock the registry entry fills from the recipe's environment.
	Seats shedadapters.SeatRunner
	Now   func() time.Time
	// CommitsSince lists the code commits after base, which Drift-Expect names when the code moved after the baseline was captured.
	CommitsSince func(base string) ([]string, error)
	// ExpectTable builds the Drift-Expect agent's table over the listing file the producer wrote.
	ExpectTable func(listing string) (seatengine.Table, error)
	// JudgeTable builds the judge's table over the request file Drift-Verify wrote before the spawn.
	JudgeTable func(request string) (seatengine.Table, error)
	// Checkouts reads the state of the code checkout and of the records checkout.
	Checkouts func() (code, records fabricengine.CheckoutState, err error)
	// RecordsChangedSince lists the records paths committed since the revision rev.
	RecordsChangedSince func(rev string) ([]string, error)
	// EndRoles ends every live strand of this worktree whose role is exactly one of the told roles, and none of any other role.
	EndRoles func(roles []string) error
	// JudgeSnapshotDir is the directory the snapshot of both checkouts taken before the judge runs is kept in.
	JudgeSnapshotDir string
	// LoomRecordsDirs are the records paths loom itself writes during a row, relative to the records checkout's root:
	// each a directory or, ending in `-`, a path prefix.
	LoomRecordsDirs []string
	// StartCapture starts the detached capture and returns once its record exists.
	StartCapture func() error
	// RunCapture runs the capture in the calling process and returns its final record.
	RunCapture func(ctx context.Context) (CaptureRecord, error)
	// CaptureCurrent runs the capture command into a fresh scratch directory of the current code.
	// It waits for the capture lock and returns how the capture ended.
	CaptureCurrent func(ctx context.Context) (CurrentCapture, error)
	// Sleep waits d or until ctx ends; it is the clock of the capture wait.
	Sleep func(ctx context.Context, d time.Duration) error
	// Compare runs the mechanical compare against the baseline over the scope's skip list and returns how it ended.
	Compare func(ctx context.Context) (CompareResult, error)
	// GateFindingsFile is the file an exhausted gate entry's findings are kept in; empty keeps none.
	GateFindingsFile string
}

// now returns the told clock's time, or the system clock's when none was told.
func (d Deps) now() time.Time {
	if d.Now == nil {
		return time.Now()
	}
	return d.Now()
}

// Row is the portable config of one Drift row.
type Row struct {
	// Subject is SubjectDecisionRecord or SubjectBoardEntry.
	Subject string
}

// commitOnExit commits relPaths through commit after a producer pass and returns the pass's result.
// A failed commit is returned beside the pass's own error.
// It replaces the outcome, since the drift records must reach git.
func commitOnExit(name string, commit func([]string) error, relPaths []string, outcome shedengine.Outcome, pointer shedengine.OutputPointer, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if commitErr := commit(relPaths); commitErr != nil {
		return "", shedengine.OutputPointer{}, errors.Join(err, fmt.Errorf("drift: %s: commit the drift records: %w", name, commitErr))
	}
	return outcome, pointer, err
}

// passWhenSwitchedOff returns a Done naming the switch, and stop true, when the hub's drift config has enabled off.
// The entry gate reads drift.enabled only when the run first enters a row,
// so this check lets a resume pass a row the run already entered once the switch went off, in any layer.
// A config that fails to load is a Stuck naming its error and lym loom resume.
func passWhenSwitchedOff(config func() (Config, error)) (outcome shedengine.Outcome, pointer shedengine.OutputPointer, stop bool) {
	cfg, err := config()
	if err != nil {
		return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf("%v; then lym loom resume", err)}, true
	}
	if !cfg.Enabled {
		return shedengine.Done, shedengine.OutputPointer{Reason: "drift is switched off (drift.enabled = false), so the row passes"}, true
	}
	return "", shedengine.OutputPointer{}, false
}

// seam names one Deps field and whether the producer was told it.
type seam struct {
	name string
	set  bool
}

// requireSeams returns the wiring error naming every seam that is not set, or nil.
func requireSeams(producer string, seams ...seam) error {
	var missing []string
	for _, s := range seams {
		if !s.set {
			missing = append(missing, s.name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("drift: %s is wired without Deps.%s", producer, strings.Join(missing, ", Deps."))
}
