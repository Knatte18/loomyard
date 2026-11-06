// loadturn.go runs one skill-load turn of a starting run and holds it through an API error.
// An API error says nothing about the skills, so such a turn is no skipped skill: the load runs again once a back-off has passed, and the prompt waits behind it.
// The typed load line is the probe, since a session has no other way of asking whether its API answers.

package shuttleengine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// loadHoldBackoffCeiling is the longest wait between two load turns of a hold; the wait starts at apiErrorRetryBackoff and doubles with each API error.
	loadHoldBackoffCeiling = 5 * time.Minute
	// loadHoldNoticeErrors is the API-error count at which the parent is told of the hold, so a passing error sends no notice.
	loadHoldNoticeErrors = 2
)

// loadModelRejectedError ends a start whose load turn the provider ended on a model it does not know, which no further turn changes.
type loadModelRejectedError struct {
	// text is the provider's error text.
	text string
}

func (e *loadModelRejectedError) Error() string {
	return "shuttle: the provider rejected the session's model in a skill-load turn: " + e.text
}

// loadDeadlineError ends a start whose load turn was still open when the run's deadline passed.
type loadDeadlineError struct{}

func (e *loadDeadlineError) Error() string {
	return "shuttle: a skill-load turn was still open at the run's deadline"
}

// loadTurnWatch is what one load turn keeps across its polls.
type loadTurnWatch struct {
	// began is when the turn being awaited began: the call's entry, or the latest turn after an API error.
	began time.Time
	// open is true while a turn start read has no turn end after it.
	open bool
	// turnStart is the newest turn start signal read, which an engine reads an interrupt marker of.
	turnStart SessionSignal
	// errors counts the API-error turn ends read, and since is when the first was read.
	errors int
	since  time.Time
	// pending is true while the newest turn signal read is an API-error turn end, whose load runs again at retypeAt.
	pending  bool
	retypeAt time.Time
	// backoff is the wait the next API error gets.
	backoff time.Duration
	// rejected is true when the pending turn end is one the provider marked ModelRejected, and text is the newest error's text.
	rejected bool
	text     string
	// noticed is true once the parent was told of the hold.
	noticed bool
}

// heldDeadline returns deadline moved later by the hold that preceded the turn being awaited, which is the time from the first API error to that turn's start.
func (w *loadTurnWatch) heldDeadline(deadline time.Time) time.Time {
	if w.errors == 0 {
		return deadline
	}
	return deadline.Add(w.began.Sub(w.since))
}

// apiErrorRead records an API-error turn end read at now and schedules the load's next turn.
func (w *loadTurnWatch) apiErrorRead(text string, rejected bool, now time.Time) {
	if w.errors == 0 {
		w.since, w.backoff = now, apiErrorRetryBackoff
	}
	w.errors++
	w.open, w.pending, w.rejected, w.text = false, true, rejected, text
	w.retypeAt = now.Add(w.backoff)
	w.backoff = min(2*w.backoff, loadHoldBackoffCeiling)
}

// turnStartRead records a turn start read at now, which ends a pending API error: the operator, a revive or a typed load line started the turn awaited from now on.
func (w *loadTurnWatch) turnStartRead(signal SessionSignal, now time.Time) {
	w.open, w.pending, w.turnStart = true, false, signal
	if w.errors > 0 {
		w.began = now
	}
}

// loadSkillTurn is one load turn: when typed, it sends the engine's load message for skills through the verified send path; otherwise the launch line already carried the message.
// It then polls the events file until the first turn end past the run's offset, which it returns, the pane dies, or the wait ends as described below.
// timeout is measured from the call's entry and ends only a turn no turn start was read for, such as one of an engine that reads no turn starts.
// A provider retries a failing API inside a turn for minutes before it ends the turn on the error, so a turn start read with no turn end after it is awaited until its end, the pane's death, the run's deadline, which fails with a *loadDeadlineError, or the engine's reading of it as interrupted.
// ended is false on a timeout and on an interrupted turn.
// died is true when the strand's pane is no longer live.
// The run's offset moves past every event read.
//
// A turn that ends on an API error, by the engine's signal or by the transcript's marker on a turn end, returns nothing.
// The poll sleeps out a back-off from apiErrorRetryBackoff doubling to loadHoldBackoffCeiling, and the load message goes out again through the verified send path, as many times as the API fails, with no bound of its own: it ends when a turn ends, the pane dies or a send fails.
// A turn start read meanwhile takes the place of that line, since the operator or a revive may have carried the session on, and a busy session only moves the line to the next back-off.
// A turn end the engine marks ModelRejected fails with a *loadModelRejectedError, since no further turn changes the model.
// The hold moves the run's deadline later by its length, as a pause does.
func (run *Run) loadSkillTurn(loader SkillLoader, skills []string, timeout time.Duration, typed bool) (turnEnd Event, ended, died bool, err error) {
	reed := run.runner.reed
	guid := run.state.StrandGUID
	if typed {
		if err := run.sendThroughPause(run.newSendContext(), loader.SkillLoadMessage(skills)); err != nil {
			return Event{}, false, false, fmt.Errorf("shuttle: load skills %v: %w", skills, err)
		}
	}
	watch := loadTurnWatch{began: run.clock.Now()}
	defer run.endLoadHold(&watch)
	interval := pollInterval(run.runner.cfg)
	for {
		if turnEnd, read := run.readLoadTurn(&watch, skills); read {
			return turnEnd, true, false, nil
		}
		if watch.rejected {
			return Event{}, false, false, &loadModelRejectedError{text: watch.text}
		}
		run.noticeLoadHold(&watch)

		if status, serr := reed.Status(); serr == nil {
			if strand, tracked := strandStatusByGUID(status.Strands, guid); !tracked || !strand.Live {
				return Event{}, false, true, nil
			}
		}
		now := run.clock.Now()
		if watch.pending {
			if now.Before(watch.retypeAt) {
				run.clock.Sleep(watch.retypeAt.Sub(now))
				continue
			}
			err := run.sendThroughPause(run.newSendContext(), loader.SkillLoadMessage(skills))
			switch {
			case errors.Is(err, ErrSessionBusy):
				logger.Info("shuttle: load turn after an API error not typed, session busy; trying again after the back-off", "strandGUID", guid, "error", err)
				watch.retypeAt = run.clock.Now().Add(watch.backoff)
				continue
			case err != nil:
				return Event{}, false, false, fmt.Errorf("shuttle: load skills %v after an API error: %w", skills, err)
			}
			logger.Info("shuttle: load turn typed again after an API error", "strandGUID", guid, "skills", skills, "apiErrors", watch.errors)
			watch.pending, watch.began = false, run.clock.Now()
		} else if watch.open {
			if run.loadTurnInterrupted(&watch) {
				return Event{}, false, false, nil
			}
			if !now.Before(watch.heldDeadline(run.deadline)) {
				return Event{}, false, false, &loadDeadlineError{}
			}
		} else if !now.Before(watch.began.Add(timeout)) {
			return Event{}, false, false, nil
		}
		run.clock.Sleep(interval)
	}
}

// readLoadTurn reads the events past the run's offset into watch, and returns the first turn end that is the load turn's own.
// A turn end the transcript marks an API error counts as an API-error turn end.
// An unreadable file and a failed parse read nothing and leave the offset where it was.
func (run *Run) readLoadTurn(watch *loadTurnWatch, skills []string) (Event, bool) {
	data, newOffset, err := readEventsFrom(run.state.EventsPath, run.offset)
	if err != nil || len(data) == 0 {
		return Event{}, false
	}
	events, err := run.engine.ParseEvents(data)
	if err != nil {
		return Event{}, false
	}
	run.offset = newOffset
	errorsBefore := watch.errors
	now := run.clock.Now()
	if parser, ok := run.engine.(SessionSignalParser); ok {
		signals, _ := parser.ParseSessionSignals(data)
		for _, signal := range signals {
			switch signal.Kind {
			case SessionSignalTurnStart:
				watch.turnStartRead(signal, now)
			case SessionSignalTurnEnd, SessionSignalSessionStart:
				watch.open = false
			case SessionSignalAPIErrorTurnEnd:
				watch.apiErrorRead(signal.ErrorText, signal.ModelRejected, now)
			}
		}
	}
	reader, canRead := run.engine.(ActivityReader)
	var turnEnd Event
	found := false
	for _, event := range events {
		if event.Kind != EventStop && event.Kind != EventWaiting {
			continue
		}
		if canRead {
			if activity := reader.TurnEndActivity(event); activity.APIError {
				watch.apiErrorRead(activity.APIErrorText, false, now)
				continue
			}
		}
		turnEnd, found = event, true
		break
	}
	if watch.errors > errorsBefore && !found {
		logger.Warn("shuttle: load turn ended on an API error; the prompt waits until the load runs through", "strandGUID", run.state.StrandGUID, "skills", skills, "apiErrors", watch.errors, "error", watch.text, "retypeAt", watch.retypeAt)
	}
	return turnEnd, found
}

// loadTurnInterrupted reports whether the engine reads the open load turn as interrupted by the operator, a turn that no turn end ever follows.
// An engine that cannot read an interrupt reads every open turn as running.
func (run *Run) loadTurnInterrupted(watch *loadTurnWatch) bool {
	prober, ok := run.engine.(SessionProber)
	if !ok {
		return false
	}
	_, interrupted := prober.TurnStartInterrupt(watch.turnStart)
	return interrupted
}

// noticeLoadHold tells the parent once that the API keeps failing the load, for an autonomous run on a runner with a notifier, since a start held on a lasting error shows nowhere else.
// A notifier error is only logged.
func (run *Run) noticeLoadHold(watch *loadTurnWatch) {
	notify := run.runner.notifier
	if watch.noticed || watch.errors < loadHoldNoticeErrors || notify == nil || run.spec.Interactive {
		return
	}
	watch.noticed = true
	strandName := run.state.StrandName
	if strandName == "" {
		strandName = run.state.StrandGUID
	}
	line := fmt.Sprintf("agent %s is held at its skill-load turn: the provider's API keeps failing it, so its prompt is not typed yet. The load runs again by itself and the prompt follows once the API answers, so nothing is needed unless the error is one that will not pass.", strandName)
	if err := notify(line); err != nil {
		logger.Warn("shuttle: load hold notice failed", "strandGUID", run.state.StrandGUID, "error", err)
	}
}

// endLoadHold moves the run's deadline later by the length of the hold watch kept, so the deadline counts no time spent waiting on the API.
func (run *Run) endLoadHold(watch *loadTurnWatch) {
	if watch.errors == 0 {
		return
	}
	held := run.clock.Now().Sub(watch.since)
	run.deadline = run.deadline.Add(held)
	logger.Info("shuttle: load turn hold over", "strandGUID", run.state.StrandGUID, "apiErrors", watch.errors, "held", held)
}

// abandonLoad ends a start whose load turn failed with err.
// A rejected model ends it OutcomeAPIError with Result.APIError naming the error, and a turn still open at the run's deadline ends it OutcomeTimeout;
// either returns the run instead when every output file exists, as a start past its gates.
// Any other failure ends it OutcomeDied.
func (run *Run) abandonLoad(err error) (Result, error) {
	var rejected *loadModelRejectedError
	var expired *loadDeadlineError
	var expiredOutcome Outcome
	switch {
	case errors.As(err, &rejected):
		expiredOutcome = OutcomeAPIError
	case errors.As(err, &expired):
		expiredOutcome = OutcomeTimeout
	default:
		return run.abandonStartup(OutcomeDied, err)
	}
	outcome := run.classifyDeadlineExpiry(expiredOutcome)
	if outcome == OutcomeDone {
		return Result{}, nil
	}
	result, startErr := run.abandonStartup(outcome, err)
	if rejected != nil {
		result.APIError = rejected.text
	}
	return result, startErr
}
