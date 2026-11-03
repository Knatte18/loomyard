// deps.go — the told seams the Drift producers share, and the portable config of one Drift row.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	// Head, BranchPoint and DirtyPaths read the code checkout's HEAD, its branch point and its tracked dirty paths.
	Head        func() (string, error)
	BranchPoint func() (string, error)
	DirtyPaths  func() ([]string, error)
	// Commit commits the told anchor-relative paths under the durable drift directory with the run's records.
	Commit func(relPaths []string) error
	// Seats and Now are the seat runner and the clock the registry entry fills from the recipe's environment.
	Seats shedadapters.SeatRunner
	Now   func() time.Time
	// BaselineTable builds the Drift-Baseline agent's table; scopeNote is non-empty when the scope is already recorded.
	BaselineTable func(scopeNote string) (seatengine.Table, error)
	// StartCapture starts the detached capture and returns once its record exists.
	StartCapture func() error
	// RunCapture runs the capture in the calling process and returns its final record.
	RunCapture func(ctx context.Context) (CaptureRecord, error)
	// Sleep waits d or until ctx ends; it is the clock of the capture wait.
	Sleep func(ctx context.Context, d time.Duration) error
	// VerifyTable builds the Drift-Verify agent's table for the failing compare's log and the baseline's commit.
	VerifyTable func(told VerifyTold) (seatengine.Table, error)
	// Compare runs the mechanical compare against the baseline over the scope's skip list and returns how it ended.
	Compare func(ctx context.Context) (CompareResult, error)
	// CaptureAccepted captures the accepted profile on the current code, leaving out the scope's skipped tests alone.
	CaptureAccepted func(ctx context.Context) (AcceptedCapture, error)
	// CompareAccepted runs the compare against the accepted profile, leaving out the scope's skipped tests alone, so the accepted tests are compared too.
	CompareAccepted func(ctx context.Context) (CompareResult, error)
	// GateFindingsFile is the file an exhausted gate entry's findings are kept in; empty keeps none.
	GateFindingsFile string
}

// Row is the portable config of one Drift row.
type Row struct {
	// Subject is SubjectDecisionRecord or SubjectBoardEntry.
	Subject string
}

// commitOnExit commits relPaths through commit after a producer pass and returns the pass's result.
// A failed commit is returned beside the pass's own error and replaces the outcome, since the drift records must reach git.
func commitOnExit(name string, commit func([]string) error, relPaths []string, outcome shedengine.Outcome, pointer shedengine.OutputPointer, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if commitErr := commit(relPaths); commitErr != nil {
		return "", shedengine.OutputPointer{}, errors.Join(err, fmt.Errorf("drift: %s: commit the drift records: %w", name, commitErr))
	}
	return outcome, pointer, err
}

// passWhenSwitchedOff returns a Done naming the switch, and stop true, when the hub's drift config has enabled off.
// The entry gate reads drift.enabled only when the run first enters a row, so this check lets a resume pass a row the run already entered once the switch went off, in any layer.
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
