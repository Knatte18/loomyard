// run.go implements (*Shed).Run, the six-step loop that is this task's entire deliverable: read
// the status file, look up the current producer, check pause/cancellation, call the producer,
// append-and-persist, and route on the outcome. Everything a producer does past its own Call
// return value is invisible to this loop.

package shedengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// findProducer looks up name in producers, returning the matching definition and whether it was
// found. It never guesses: a caller that gets found == false must hard-error rather than
// fabricate a fallback, exactly as the loop's own lookup step does.
func findProducer(producers []ProducerDef, name string) (ProducerDef, bool) {
	for _, p := range producers {
		if p.Name == name {
			return p, true
		}
	}
	return ProducerDef{}, false
}

// Run walks the whole six-step loop in one call, from wherever the status file's current_producer
// currently sits, until it hits a stopping condition: pause/cancellation, blocked, done, or an
// error.
// Result is meaningless unless the returned error is nil -- every hard-error path below returns an
// unpopulated Result alongside its error, and a caller must check error before reading Outcome.
// The producer list itself carries zero routing meaning once Done routes by OnDone: it is
// storage, plus validate's iteration order, plus cosmetic display order, nothing else.
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
		def, ok := findProducer(s.Producers, st.CurrentProducer)
		if !ok {
			return Result{}, fmt.Errorf("shedengine: current_producer %q in %q names no producer in the list; the producer list has changed since the file was last written", st.CurrentProducer, s.StatusPath)
		}

		// Step 3, the pause and cancellation check. The two conditions are treated identically
		// on purpose -- an operator's Ctrl-C or a parent deadline is an operational stop, not a
		// failure, exactly as resumable as an explicit pause request.
		if st.PauseRequested || ctx.Err() != nil {
			// Clearing the flag in the same persist is what stops the next Run re-pausing
			// forever on the flag it is resuming from; the durable record of "this run is
			// paused" is state, not the flag.
			if pauseErr := s.persist(st.CurrentProducer, StatePaused, "", st.History, true); pauseErr != nil {
				return Result{}, pauseErr
			}
			return Result{
				Outcome:        RunPaused,
				HaltedProducer: st.CurrentProducer,
				History:        st.History,
			}, nil
		}

		// Step 3b, the resume write. A run resumed from paused, blocked, or failed reaches this
		// point about to call a producer, while the status file still says paused, blocked, or
		// failed -- and for every LLM row that is minutes, during which the file, the status strand,
		// and "lyx loom status" all describe a run that is in fact already spawning. The stale
		// error text goes with it: Activity.Wait is composed from it, so the pane would otherwise
		// keep asserting a specific failure reason the loop is at that moment retrying past.
		//
		// This is the only write in the loop that is not the record of a producer's verdict, and it
		// is deliberately conditional: on the ordinary running-to-running path it never fires, so
		// the loop's one-persist-per-iteration shape is unchanged for every step after the first.
		if st.State != StateRunning {
			if err := s.persist(st.CurrentProducer, StateRunning, "", st.History, false); err != nil {
				return Result{}, err
			}
		}

		// Step 4: call the looked-up definition's producer.
		outcome, output, callErr := def.Producer.Call(ctx)

		// Steps 5 and 6 are computed entirely in memory first, then committed with exactly one
		// persist call for this iteration. Written as two writes, a crash between them leaves
		// current_producer still naming the producer that just finished, so the next Run
		// re-calls it and appends a duplicate history entry -- defeating the exact
		// crash-safety property step 5 exists to provide.
		//
		// Appended to a copy of the history read at step 1, never mutating the read slice in
		// place.
		appendHistory := func() []HistoryEntry {
			next := make([]HistoryEntry, len(st.History), len(st.History)+1)
			copy(next, st.History)
			return append(next, HistoryEntry{
				Producer: def.Name,
				Outcome:  outcome,
				Output:   output.Path,
				At:       nowRFC3339(),
			})
		}

		switch {
		case callErr != nil && ctx.Err() != nil:
			// Non-nil error with a cancelled context: the pause exit, exactly as step 3 takes
			// it. The predicate is ctx.Err() != nil, not an errors.Is check against a
			// cancellation sentinel -- the context's own state is ground truth about whether
			// an operator stopped the run and stays correct even if a producer wraps or
			// discards the sentinel, whereas a producer whose own internal derived context
			// times out returns a deadline error while the parent context is perfectly
			// healthy: a genuine producer failure, not an operator stop.
			//
			// No history entry is appended: the producer never reached a verdict, so there is
			// nothing to record, and leaving current_producer put means the next Run simply
			// re-calls it, the same semantics as a crash before the persist. The accepted
			// trade: a producer returning a genuine, unrelated error in the same instant an
			// operator cancels is reported as a pause, which is harmless because the producer
			// is re-called on resume and the real error surfaces again then.
			if err := s.persist(st.CurrentProducer, StatePaused, "", st.History, true); err != nil {
				return Result{}, err
			}
			return Result{
				Outcome:        RunPaused,
				HaltedProducer: st.CurrentProducer,
				History:        st.History,
			}, nil

		case callErr != nil:
			// Non-nil error with a healthy context: an engine-level failure, never a producer
			// verdict, so it is never routed anywhere -- a human resolves it. No further
			// producer is called.
			nextHistory := appendHistory()
			if err := s.persist(st.CurrentProducer, StateFailed, callErr.Error(), nextHistory, false); err != nil {
				return Result{}, err
			}
			return Result{}, callErr

		case outcome == Stuck:
			nextHistory := appendHistory()
			switch {
			case def.OnStuck == "":
				reason := "stuck with no OnStuck target"
				if err := s.persist(st.CurrentProducer, StateBlocked, reason, nextHistory, false); err != nil {
					return Result{}, err
				}
				return Result{
					Outcome:        RunBlocked,
					HaltedProducer: st.CurrentProducer,
					Reason:         reason,
					History:        nextHistory,
				}, nil
			// The count argument is st.History, the slice read at step 1, and never
			// nextHistory: a post-append read shifts the boundary by one and would look
			// like an off-by-one bug rather than the semantic change it would actually be.
			case episodeStuckCount(st.History, def.Name) >= effectiveMaxBounces(def, s.MaxBounces):
				// The boundary is pinned exactly, restated per-producer: a budget of three
				// performs three bounce-backs and blocks on the fourth Stuck.
				reason := "bounce budget exhausted"
				if err := s.persist(st.CurrentProducer, StateBlocked, reason, nextHistory, false); err != nil {
					return Result{}, err
				}
				return Result{
					Outcome:        RunBlocked,
					HaltedProducer: st.CurrentProducer,
					Reason:         reason,
					History:        nextHistory,
				}, nil
			default:
				if err := s.persist(def.OnStuck, StateRunning, "", nextHistory, false); err != nil {
					return Result{}, err
				}
				continue
			}

		case outcome == Done:
			nextHistory := appendHistory()
			if def.OnDone == "" {
				// current_producer keeps this producer's own name -- never the empty string --
				// because activity.now is defined as current_producer's name and
				// HaltedProducer as the producer current_producer named when Run returned, so
				// an empty value would leave both fields meaningless at the happy-path
				// terminal a reader of a finished status file most wants to understand. The
				// terminal is now chosen by an empty OnDone, not by list position.
				if err := s.persist(def.Name, StateDone, "", nextHistory, false); err != nil {
					return Result{}, err
				}
				return Result{
					Outcome:        RunDone,
					HaltedProducer: def.Name,
					History:        nextHistory,
				}, nil
			}
			// A non-empty OnDone needs no lookup here: validate has already rejected an OnDone
			// naming no producer in the list, so the name is persisted as-is and resolved by
			// step 2's lookup on the next iteration.
			if err := s.persist(def.OnDone, StateRunning, "", nextHistory, false); err != nil {
				return Result{}, err
			}
			continue

		default:
			// An Outcome that is neither Done nor Stuck, returned with a nil error, is an
			// engine-level failure: Outcome is a string type and therefore open, so the
			// routing would otherwise have an undefined fourth case, and coercing an unknown
			// value to Stuck would consume bounce budget for a broken adapter while coercing
			// it to Done would advance past a producer that may not have done its work.
			nextHistory := appendHistory()
			failErr := fmt.Errorf("shedengine: producer %q returned an unrecognised outcome %q", def.Name, outcome)
			if err := s.persist(st.CurrentProducer, StateFailed, failErr.Error(), nextHistory, false); err != nil {
				return Result{}, err
			}
			return Result{}, failErr
		}
	}
}

// nowRFC3339 returns the current UTC time formatted as RFC3339, the format every history[].at
// value is written in. No injectable clock field exists on Shed -- adding one would add a field to
// the struct shape the design pins, for a value tests assert structurally instead of by literal.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// episodeStuckCount walks history backward from the end and counts the Stuck entries authored by
// name within its current episode: the run of entries since name's own most recent Done. It
// returns immediately at the first entry whose Producer equals name and whose Outcome is Done, and
// otherwise counts the entries whose Producer equals name and whose Outcome is Stuck; entries
// authored by other producers are skipped and never terminate the scan, so a Done by some other
// producer does not end this producer's episode.
// A done entry written by the hard-failure arm also terminates the scan, and that is accepted
// rather than special-cased: the engine records the verdict a producer actually returned, and
// state: "failed" halts the run, so every continuation past it is a fresh human-initiated act.
func episodeStuckCount(history []HistoryEntry, name string) int {
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]
		if entry.Producer != name {
			continue
		}
		if entry.Outcome == Done {
			return count
		}
		if entry.Outcome == Stuck {
			count++
		}
	}
	return count
}

// effectiveMaxBounces resolves def's own bounce budget, inheriting at two levels: def.MaxBounces
// when it is greater than zero, else shedMax when that is greater than zero, else
// defaultMaxBounces. A zero value never means "no bounces allowed" at either level.
func effectiveMaxBounces(def ProducerDef, shedMax int) int {
	if def.MaxBounces > 0 {
		return def.MaxBounces
	}
	if shedMax > 0 {
		return shedMax
	}
	return defaultMaxBounces
}

// persist is the single write path for the whole loop: one state.UpdateJSON call whose mutate
// overwrites exactly the Shed-owned fields -- current_producer, state, error, history, and
// activity (recomposed via composeActivity from the values being written) -- and, when
// consumePause is true, also writes pause_requested false; otherwise pause_requested is left
// exactly as re-read. persist never touches product.
//
// The merge exists rather than a whole-file rewrite from an in-memory copy because Shed is not the
// status file's only writer: a pause requested during a long producer call, and an external
// product update, must both survive. That safety is conditional, not unconditional -- internal/
// state's lock is advisory and keyed on the caller-supplied path, so the merge is safe against a
// concurrent external writer that takes the same StatusLockPath, and against no other.
//
// The found guard is not defensive noise: state.UpdateJSON treats a missing file as a non-error
// and writes whatever the mutate returns, so without the guard a status file deleted mid-run would
// be silently re-created from a zero value, contradicting the rule that Shed never seeds one.
//
// The accepted leniency asymmetry: UpdateJSON re-reads through a plain unmarshal with no
// unknown-field rejection, so strictness is the contract of the read gate only. Malformed JSON
// still fails loud on this path, but an unknown top-level key written by an external actor after
// the read gate passed is silently destroyed by the full-struct marshal -- not caught by a later
// strict read, because the merge strips it and the next read sees a clean file. This is accepted
// because product is the sanctioned channel for everything an external writer legitimately owns,
// and a key outside it is a mistake nothing here promises to preserve.
func (s *Shed) persist(nextCurrentProducer string, nextState State, nextError string, nextHistory []HistoryEntry, consumePause bool) error {
	return state.UpdateJSON(s.StatusPath, s.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-run; Shed refuses to create one", s.StatusPath)
		}
		cur.CurrentProducer = nextCurrentProducer
		cur.State = nextState
		cur.Error = nextError
		cur.History = nextHistory
		cur.Activity = composeActivity(nextCurrentProducer, nextHistory, nextState, nextError)
		if consumePause {
			cur.PauseRequested = false
		}
		return cur, nil
	})
}
