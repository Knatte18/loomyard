// await.go implements the bounded wait on a fixer strand: done once its reply file exists, dead when its turn ends without one, its timeout passes or its strand is gone.

package gatefixer

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// pollTick is how often Await re-reads the fixer's state.
const pollTick = time.Second

// Result is one bounded wait's outcome.
type Result struct {
	// Status is StatusRunning when the wait elapsed first, else StatusDone or StatusDead.
	Status string
	// DeadReason is set only when Status is StatusDead.
	DeadReason string
	// Reply is the reply file's content, set only when Status is StatusDone.
	Reply string
	// ElapsedS is the seconds since the spawn.
	ElapsedS int
	// Warnings are cleanup failures that change no status.
	Warnings []string
}

// Await blocks up to wait for rec's fixer to reach a terminal classification, polling once a second on deps.Clock.
// The fixer is done once rec.ReplyPath exists.
// It is dead when its newest turn ended without one (DeadReasonAsking), when timeout has passed since the spawn (DeadReasonTimeout), or when its strand is gone (DeadReasonDied), checked in that order.
// A wait that elapses first returns StatusRunning.
// Every terminal result stops the strand; only done also reads the reply and removes the run directory.
// A failed stop, reply read or removal is a warning.
// Await never marks rec terminal: the caller does, through its store.
func Await(deps Deps, rec *Record, timeout, wait time.Duration) (*Result, error) {
	spawnedAt, err := time.Parse(time.RFC3339, rec.SpawnedAt)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: parse the fixer's recorded spawnedAt %q: %w", rec.SpawnedAt, err)
	}
	engine, err := deps.Engines.Lookup(rec.Engine)
	if err != nil {
		return nil, fmt.Errorf("gatefixer: resolve the engine %q the fixer ran under: %w", rec.Engine, err)
	}
	clock := deps.Clock

	classify := func() (status, deadReason string, terminal bool, err error) {
		if _, err := os.Stat(rec.ReplyPath); err == nil {
			return StatusDone, "", true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", "", false, fmt.Errorf("gatefixer: stat fixer reply %s: %w", rec.ReplyPath, err)
		}
		turnEnded, err := TurnEndedAfter(rec.EventsPath, rec.EventsOffset, engine, TurnEndRead{Now: clock.Now()})
		if err != nil {
			return "", "", false, err
		}
		if turnEnded {
			return StatusDead, DeadReasonAsking, true, nil
		}
		if clock.Now().Sub(spawnedAt) > timeout {
			return StatusDead, DeadReasonTimeout, true, nil
		}
		live, err := StrandLive(deps.Reed, rec.StrandGUID)
		if err != nil {
			return "", "", false, err
		}
		if !live {
			return StatusDead, DeadReasonDied, true, nil
		}
		return StatusRunning, "", false, nil
	}

	deadline := clock.Now().Add(wait)
	var status, deadReason string
	for {
		var terminal bool
		status, deadReason, terminal, err = classify()
		if err != nil {
			return nil, err
		}
		if terminal || clock.Now().After(deadline) {
			break
		}
		clock.Sleep(pollTick)
	}

	result := &Result{Status: status, DeadReason: deadReason, ElapsedS: int(clock.Now().Sub(spawnedAt).Seconds())}
	if status == StatusRunning {
		return result, nil
	}

	if err := deps.Stopper.StopStrand(rec.StrandGUID); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("stop strand %s: %v", rec.StrandGUID, err))
	}
	if status == StatusDone {
		reply, err := os.ReadFile(rec.ReplyPath)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("read reply %s: %v", rec.ReplyPath, err))
		}
		result.Reply = string(reply)
		if rec.ShuttleRunDir != "" {
			if err := os.RemoveAll(rec.ShuttleRunDir); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("remove run dir %s: %v", rec.ShuttleRunDir, err))
			}
		}
	}
	return result, nil
}
