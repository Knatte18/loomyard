// parentlock.go wires the seams of Run-Shed's parent lock backstop: the lock record of the child's run, its release, the child's agent states and the landing-in-flight read.

package battencli

import (
	"errors"
	"io/fs"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// childParentLock builds the parent lock seams of the task pair for slug, over the lock store under boardDir.
// bound is landing's idle bound for the lock.
// The task pair is located at call time, so a pair that is not there yet fails the seam and not the wiring.
func childParentLock(slug string, locateTask func() (*lymcwd.Location, error), boardDir string, bound time.Duration) *battenshed.ChildParentLock {
	store := &parentlock.Store{Dir: parentlock.Dir(boardDir)}
	return &battenshed.ChildParentLock{
		Read: func() (parentlock.Record, bool, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return parentlock.Record{}, false, err
			}
			return runLockRecord(store, slug, taskLocation.WorktreePath())
		},
		Release: func(record parentlock.Record) (bool, error) {
			return store.Release(record.Branch, record.Owner)
		},
		Agents: func() ([]parentlock.AgentState, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return nil, err
			}
			return readAgentStates(taskLocation)
		},
		IdleBound: bound,
		KeepsLock: func() (bool, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return false, err
			}
			return landingInFlight(taskLocation)
		},
	}
}

// landingInFlight reports whether the task pair's run is at Finalize and running, or a verify marker of the pair is live, which holds the run's parent lock.
// A status directory that does not exist yet reads as no status; any other failed read of the status file or the marker returns its error.
func landingInFlight(taskLocation *lymcwd.Location) (bool, error) {
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](shedrun.StatusFile(taskLocation, shedrun.SelfRunID), shedrun.StatusLock(taskLocation, shedrun.SelfRunID))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if found && status.CurrentProducer == loomshed.NameFinalize && status.State == shedengine.StateRunning {
		return true, nil
	}
	_, live, err := verifytree.ReadMarker(verifytree.NewPaths(taskLocation.WorktreePath(), verifytree.Dir(taskLocation.AnchorPath())).Marker)
	return live, err
}

// lockIdleBound returns landing's idle bound for the parent lock of the hub of prime.
// A landing config that cannot be read logs a Warn and leaves the zero config, whose bound is the default.
func lockIdleBound(prime *lymcwd.Location) time.Duration {
	cfg, err := landingshed.LoadConfig(fabricengine.ConfigDirs(prime), "landing")
	if err != nil {
		logger.Warn("battencli: landing config unreadable; the parent lock's idle bound falls back to its default", "error", err)
	}
	return cfg.IdleRelease()
}

// runLockRecord returns the lock record in store whose owner is the run for slug in the pair at pairPath, and false when none is.
// A record that cannot be read is skipped: it names no run.
func runLockRecord(store *parentlock.Store, slug, pairPath string) (parentlock.Record, bool, error) {
	entries, err := store.List()
	if err != nil {
		return parentlock.Record{}, false, err
	}
	for _, entry := range entries {
		if entry.Err == "" && entry.Record.Owner.Slug == slug && entry.Record.Owner.Pair == pairPath {
			return entry.Record, true, nil
		}
	}
	return parentlock.Record{}, false, nil
}

// readAgentStates returns the agent states of the live runs of the task pair.
func readAgentStates(taskLocation *lymcwd.Location) ([]parentlock.AgentState, error) {
	anchor := taskLocation.AnchorPath()
	cfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "shuttle")
	if err != nil {
		return nil, err
	}
	engines, err := shuttleengines.New(cfg, fabricengine.ConfigDirs(taskLocation))
	if err != nil {
		return nil, err
	}
	waits, err := shuttleengine.ReadAgentWaits(cfg, anchor, engines, time.Now())
	if err != nil {
		return nil, err
	}
	states := make([]parentlock.AgentState, 0, len(waits))
	for _, wait := range waits {
		states = append(states, parentlock.AgentState{Name: wait.StrandName, Busy: wait.Busy, Waiting: wait.Waiting})
	}
	return states, nil
}
