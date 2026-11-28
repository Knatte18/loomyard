// apierrorretry.go re-sends the turn of a run whose provider's API ended a turn on an error, for a spec that sets APIErrorRetries.
// It ends the run OutcomeAPIError once the re-sends are spent, or at once when the provider rejected the session's model.
// It reads the session signals itself, since an API-error turn end is no event the run loop classifies.

package shuttleengine

import (
	"cmp"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// apiErrorRetryBackoff is how long after the first API-error turn end is read its turn is re-sent; the wait doubles with each later API-error turn end.
// It is long enough for a provider's passing server error to clear and short enough to leave a settler most of its deadline.
const apiErrorRetryBackoff = 30 * time.Second

// apiErrorRetryLine is the one line a run whose turn ended on an API error is given to carry on.
const apiErrorRetryLine = "[lym] Your last turn ended on an API error. Carry on with your task where you stopped."

// apiErrorUnknownText is the error text a run ends on when the API-error turn end read carried none.
const apiErrorUnknownText = "unknown error"

// apiErrorWatch is what one Wait keeps to re-send a turn the provider's API ended.
type apiErrorWatch struct {
	// cursor is the events-file offset the next signal read starts from; it starts at the prompt offset, so an attach counts the run's earlier API errors.
	cursor int64
	// errors counts the API-error turn ends read past the prompt offset.
	errors int
	// pending is true while the newest turn signal read is an API-error turn end.
	pending bool
	// rejected is true when the pending turn end is one the provider marked ModelRejected.
	rejected bool
	// text is the pending turn end's error text, and readAt when Wait read it.
	text   string
	readAt time.Time
}

// retryAPIError acts on an API-error turn end of a run, and reports whether Wait finishes with the returned Result and error.
// A turn end on an API error stays pending until a turn start, a turn end or a session start is read after it.
// While one the engine marks ModelRejected is pending and no pause covers the run, the run ends at once through classifyDeadlineExpiry, whatever the spec's APIErrorRetries, since no re-send changes the model.
// For any other, the spec must set APIErrorRetries.
// While one is pending and no pause covers the run, the n-th such turn end of the run is re-sent with apiErrorRetryLine once apiErrorRetryBackoff times 2^(n-1) has passed since it was read.
// The turn end past the last re-send, or one that leaves every output file on disk, ends the run through classifyDeadlineExpiry: done when the files exist, else OutcomeAPIError with Result.APIError naming the error.
// A send that fails is logged once per episode and tried again on a later tick, bounded by the run deadline.
// A session the provider's usage limit stopped is re-sent and handed on like any other API error.
// An engine without SessionSignalParser and an unreadable events file change nothing.
func (run *Run) retryAPIError() (Result, bool, error) {
	parser, ok := run.engine.(SessionSignalParser)
	if !ok {
		return Result{}, false, nil
	}
	watch := &run.apiErrors
	data, _, err := readEventsFrom(run.state.EventsPath, watch.cursor)
	if err != nil {
		return Result{}, false, nil
	}
	if len(data) > 0 {
		signals, consumed := parser.ParseSessionSignals(data)
		watch.cursor += int64(consumed)
		for _, signal := range signals {
			switch signal.Kind {
			case SessionSignalAPIErrorTurnEnd:
				watch.errors++
				watch.pending, watch.rejected, watch.text, watch.readAt = true, signal.ModelRejected, signal.ErrorText, run.clock.Now()
			case SessionSignalTurnStart, SessionSignalTurnEnd, SessionSignalSessionStart:
				watch.pending, watch.rejected = false, false
			}
		}
	}
	if !watch.pending || run.covered {
		return Result{}, false, nil
	}
	outcome := run.classifyDeadlineExpiry(OutcomeAPIError)
	if watch.rejected {
		return run.endOnAPIError(outcome, "shuttle: run ended at once on a model the provider rejected")
	}
	if run.spec.APIErrorRetries <= 0 {
		return Result{}, false, nil
	}
	if watch.errors > run.spec.APIErrorRetries || outcome == OutcomeDone {
		return run.endOnAPIError(outcome, "shuttle: run ended on API errors its re-sends did not clear")
	}
	if run.clock.Now().Sub(watch.readAt) < apiErrorRetryBackoff<<(watch.errors-1) {
		return Result{}, false, nil
	}
	if err := run.sendWithin(apiErrorRetryLine); err != nil {
		run.failures.Fail("api-error re-send", "shuttle: re-send after an API error not delivered, retrying on a later tick", "strandGUID", run.state.StrandGUID, "apiErrors", watch.errors, "error", err)
		return Result{}, false, nil
	}
	run.failures.Recover("api-error re-send", "shuttle: re-send after an API error delivered again", "strandGUID", run.state.StrandGUID)
	watch.pending = false
	logger.Info("shuttle: re-sent a turn the provider's API ended on an error", "strandGUID", run.state.StrandGUID, "apiErrors", watch.errors, "retries", run.spec.APIErrorRetries, "error", watch.text)
	return Result{}, false, nil
}

// endOnAPIError finalizes the run with outcome and, for OutcomeAPIError, logs warning at Warn and names the pending error on the Result, or apiErrorUnknownText when the turn end carried none.
// An OutcomeAPIError end also removes the run's own-window strand, since its pane is still live and no caller is guaranteed to stop it.
func (run *Run) endOnAPIError(outcome Outcome, warning string) (Result, bool, error) {
	text := cmp.Or(run.apiErrors.text, apiErrorUnknownText)
	if outcome == OutcomeAPIError {
		logger.Warn(warning, "strandGUID", run.state.StrandGUID, "apiErrors", run.apiErrors.errors, "error", text)
	}
	result, err := run.finalize(outcome)
	if result.Outcome == OutcomeAPIError {
		result.APIError = text
		run.removeEndedOwnWindowStrand()
	}
	return result, true, err
}
