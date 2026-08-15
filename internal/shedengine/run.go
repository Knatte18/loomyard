// run.go implements (*Shed).Run, the six-step loop that is this task's entire deliverable: read
// the status file, look up the current producer, check pause/cancellation, call the producer,
// append-and-persist, and route on the outcome. Everything a producer does past its own Call
// return value is invisible to this loop.

package shedengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// findProducer looks up name in producers, returning the matching definition, its index, and
// whether it was found. It never guesses: a caller that gets found == false must hard-error
// rather than fabricate a fallback, exactly as the loop's own lookup step does.
func findProducer(producers []ProducerDef, name string) (ProducerDef, int, bool) {
	for i, p := range producers {
		if p.Name == name {
			return p, i, true
		}
	}
	return ProducerDef{}, 0, false
}

// Run walks the whole six-step loop in one call, from wherever the status file's current_producer
// currently sits, until it hits a stopping condition: pause/cancellation, blocked, done, or an
// error.
// Result is meaningless unless the returned error is nil -- every hard-error path below returns an
// unpopulated Result alongside its error, and a caller must check error before reading Outcome.
func (s *Shed) Run(ctx context.Context) (Result, error) {
	if err := s.validate(); err != nil {
		return Result{}, err
	}

	// internal/lock opens a lock file with O_CREATE but never creates its parent directory, which
	// is why both internal/loomengine/preflight.go and internal/treadleengine/run.go MkdirAll
	// before acquiring. This is not path derivation -- the paths are still told, Shed only ensures
	// the told path is usable.
	if err := os.MkdirAll(filepath.Dir(s.LockPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("shedengine: create run lock parent dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.StatusLockPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("shedengine: create status lock parent dir: %w", err)
	}

	// An OS advisory lock is reclaimed on process death, so a killed run never bricks a later
	// resume. internal/state's own per-write lock does not substitute for this one: it is held
	// only for the duration of one write, never across a whole Call, so two concurrent runs could
	// otherwise both read the same current_producer and both spawn it.
	runLock, locked, err := lock.TryAcquireWriteLock(s.LockPath)
	if err != nil {
		return Result{}, fmt.Errorf("shedengine: acquire run lock %q: %w", s.LockPath, err)
	}
	if !locked {
		return Result{}, fmt.Errorf("%w: %q", ErrShedBusy, s.LockPath)
	}
	defer runLock.Release()

	// The bounce counter is per-Run-call and in-memory by design -- deliberately unpersisted, so a
	// crash-restart or a human-resumed blocked run starts again with the full budget, because
	// every event that resets it is a new human-initiated invocation, which is exactly the outcome
	// the budget exists to force.
	bouncesRemaining := s.MaxBounces
	if bouncesRemaining == 0 {
		bouncesRemaining = defaultMaxBounces
	}

	for {
		// Step 1, the read gate. A found of false is a hard error: Shed never seeds a status
		// file.
		st, found, err := state.ReadJSONStrict[Status](s.StatusPath, s.StatusLockPath)
		if err != nil {
			return Result{}, fmt.Errorf("shedengine: read status file %q: %w", s.StatusPath, err)
		}
		if !found {
			return Result{}, fmt.Errorf("shedengine: status file %q does not exist; Shed never seeds one", s.StatusPath)
		}
		if !st.State.valid() {
			return Result{}, fmt.Errorf("shedengine: status file %q carries an invalid state %q", s.StatusPath, st.State)
		}

		// The already-done short-circuit, positioned after step 1's read and before step 2's
		// lookup: a done file whose current_producer is no longer in the list returns cleanly
		// and does not hard-error, because a finished task must not become un-queryable because
		// someone later edited the producer list. Filling HaltedProducer and History from the
		// file, rather than returning a bare RunDone, makes a re-run's Result identical to the
		// original completing run's.
		if st.State == StateDone {
			return Result{
				Outcome:        RunDone,
				HaltedProducer: st.CurrentProducer,
				History:        st.History,
			}, nil
		}
		// StateBlocked and StateFailed deliberately do not short-circuit -- the loop proceeds
		// and re-calls current_producer, which is how a human resumes after fixing whatever
		// caused the halt.

		// Step 2, the lookup. Not found is a hard error that changes nothing on disk: Shed
		// never guesses, neither restarting from the first producer nor advancing to the
		// nearest match, because both fabricate a status nobody confirmed.
		def, _, ok := findProducer(s.Producers, st.CurrentProducer)
		if !ok {
			return Result{}, fmt.Errorf("shedengine: current_producer %q in %q names no producer in the list; the producer list has changed since the file was last written", st.CurrentProducer, s.StatusPath)
		}

		// Step 3, the pause and cancellation check. The two conditions are treated identically
		// on purpose -- an operator's Ctrl-C or a parent deadline is an operational stop, not a
		// failure, exactly as resumable as an explicit pause request.
		if st.PauseRequested || ctx.Err() != nil {
			// Implemented inline here rather than via a persist method, which does not exist
			// until card 8 refactors this branch into one. Clearing the flag in the same
			// persist is what stops the next Run re-pausing forever on the flag it is
			// resuming from; the durable record of "this run is paused" is state, not the
			// flag.
			pauseErr := state.UpdateJSON(s.StatusPath, s.StatusLockPath, func(cur Status, found bool) (Status, error) {
				if !found {
					return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-run; Shed refuses to create one", s.StatusPath)
				}
				cur.CurrentProducer = st.CurrentProducer
				cur.State = StatePaused
				cur.Error = ""
				cur.History = st.History
				cur.Activity = composeActivity(st.CurrentProducer, st.History, StatePaused, "")
				cur.PauseRequested = false
				return cur, nil
			})
			if pauseErr != nil {
				return Result{}, pauseErr
			}
			return Result{
				Outcome:        RunPaused,
				HaltedProducer: st.CurrentProducer,
				History:        st.History,
			}, nil
		}

		// Steps 4 through 6 are completed by card 9; this placeholder keeps the package
		// building until that card lands.
		_ = def
		_ = bouncesRemaining
		return Result{}, errors.New("shedengine: Run's loop body is incomplete")
	}
}
