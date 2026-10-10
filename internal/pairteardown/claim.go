// claim.go settles a removed pair's board entry: a landed run's entry is marked done, and an abandoned run's claim is cleared.

package pairteardown

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// claimSettlement is what the teardown does to the board entry of a removed pair.
type claimSettlement string

const (
	// claimKeep writes nothing.
	claimKeep claimSettlement = "keep"
	// claimDone marks the entry done.
	claimDone claimSettlement = "done"
	// claimClear clears the entry's status.
	claimClear claimSettlement = "clear"
	// claimHeld writes nothing and warns: batten's run for the slug is still running.
	claimHeld claimSettlement = "held"
)

// settleClaim decides the settlement of a board entry with the given status.
// An entry with no status, a done status or a status that is not a run status is kept.
// A landed run is done.
// Otherwise a run batten never seeded, or one batten halted, is cleared; a run batten finished is done; a running one is held.
func settleClaim(status *string, landed, battenSeeded bool, battenState string) claimSettlement {
	if !boardengine.IsRunStatus(status) {
		return claimKeep
	}
	if landed {
		return claimDone
	}
	if !battenSeeded {
		return claimClear
	}
	switch shedengine.State(battenState) {
	case shedengine.StatePaused, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StateAwaiting:
		return claimClear
	case shedengine.StateDone:
		return claimDone
	case shedengine.StateRunning:
		return claimHeld
	}
	return claimKeep
}

// settleBoardClaim settles the removed pair's board entry once its removal succeeded.
// A board or status read failure warns and writes nothing, and a failed write warns.
func (t *Teardown) settleBoardClaim(slug string, landed bool) {
	status, found, err := t.readClaim(slug)
	if err != nil {
		logger.Warn("pairteardown: could not read the board entry, so its claim is left alone", "slug", slug, "cause", err)
		return
	}
	if !found {
		return
	}
	var battenSeeded bool
	var battenState string
	if !landed && boardengine.IsRunStatus(status) {
		battenSeeded, battenState, err = t.battenRun(slug)
		if err != nil {
			logger.Warn("pairteardown: could not read batten's run, so the board claim is left alone", "slug", slug, "cause", err)
			return
		}
	}

	var settled *string
	switch settlement := settleClaim(status, landed, battenSeeded, battenState); settlement {
	case claimKeep:
		return
	case claimHeld:
		logger.Warn("pairteardown: batten's run for the slug is still running, so the board claim is kept", "slug", slug,
			"wayForward", fmt.Sprintf("check it with `lyx batten status %s`, and turn a running run into an abandon with `lyx batten pause %s`", slug, slug))
		return
	case claimDone:
		done := "done"
		settled = &done
	}
	if err := t.writeClaim(slug, settled); err != nil {
		logger.Warn("pairteardown: could not settle the board claim", "slug", slug, "cause", err)
	}
}

// childLanded reports whether the task worktree's run finished a Finalize: its status history holds a Finalize with outcome Done.
func (t *Teardown) childLanded(slug string) (bool, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return false, err
	}
	status, found, err := readStatus(shedrun.StatusFile(task, shedrun.SelfRunID), shedrun.StatusLock(task, shedrun.SelfRunID))
	if err != nil || !found {
		return false, err
	}
	for _, entry := range status.History {
		if entry.Producer == loomshed.NameFinalize && entry.Outcome == shedengine.Done {
			return true, nil
		}
	}
	return false, nil
}

// battenRunState reports whether batten seeded a run for the slug on the prime, and the state its status file records.
// A seeded run with no status file has an empty state.
func (t *Teardown) battenRunState(slug string) (seeded bool, state string, err error) {
	_, seeded, err = shedrun.ReadSeed(t.prime, slug)
	if err != nil || !seeded {
		return false, "", err
	}
	status, found, err := readStatus(shedrun.StatusFile(t.prime, slug), shedrun.StatusLock(t.prime, slug))
	if err != nil || !found {
		return true, "", err
	}
	return true, string(status.State), nil
}

// readStatus reads the shed status file at path under its lock; found is false when the file is absent.
func readStatus(path, lockPath string) (shedengine.Status, bool, error) {
	return state.ReadJSONStrict[shedengine.Status](path, lockPath)
}

// readBoardClaim reads the status of the slug's board entry through the hub's board.
func (t *Teardown) readBoardClaim(slug string) (status *string, found bool, err error) {
	board, err := boardengine.OpenHub(t.prime.HubPath)
	if err != nil {
		return nil, false, err
	}
	task, found, err := board.GetTask(slug)
	if err != nil || !found {
		return nil, false, err
	}
	return task.Status, true, nil
}

// writeBoardClaim sets the status of the slug's board entry, or clears it with a nil status.
func (t *Teardown) writeBoardClaim(slug string, status *string) error {
	board, err := boardengine.OpenHub(t.prime.HubPath)
	if err != nil {
		return err
	}
	return board.SetStatus(slug, status)
}
