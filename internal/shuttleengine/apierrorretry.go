// apierrorretry.go re-sends the turn of a run whose provider's API ended a turn on an error, for a spec that sets APIErrorRetries.
// It ends the run OutcomeAPIError once the re-sends are spent, or at once when the provider rejected the session's model.
// It reads the session signals itself, since an API-error turn end is no event the run loop classifies.

package shuttleengine

import (
	"bytes"
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
	// handledOffset is the events-file offset past the last turn end an account switch handled; no read starts before it.
	// Such a turn end is neither counted nor re-sent.
	handledOffset int64
	// turnEnds counts the turn ends, API-error ones included, past the record's launch offset.
	// It is seeded with those before the cursor, so a load turn's end counts as the first.
	// Only the first can be a refused resume's.
	turnEnds int
	// turnEndsSeeded is true once turnEnds holds the turn ends between the launch offset and the cursor.
	turnEndsSeeded bool
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
// An account switch that handled its turn end is the exception: that turn end is neither counted nor re-sent.
// A refused resume is the other exception.
// It is the first turn end past the record's launch offset.
// It must be an API error no usage limit caused.
// The launch must have resumed a session whose transcript it copied.
// Such a turn end is neither counted nor re-sent, whatever the spec's APIErrorRetries.
// The session starts fresh in the same run, and a finished relaunch ends Wait with its result.
// An engine without SessionSignalParser and an unreadable events file change nothing.
func (run *Run) retryAPIError() (Result, bool, error) {
	parser, ok := run.engine.(SessionSignalParser)
	if !ok {
		return Result{}, false, nil
	}
	watch := &run.apiErrors
	if !watch.turnEndsSeeded {
		watch.turnEndsSeeded = true
		watch.turnEnds = run.turnEndsBetween(parser, run.state.LaunchOffset, watch.cursor)
	}
	watch.cursor = max(watch.cursor, watch.handledOffset)
	data, _, err := readEventsFrom(run.state.EventsPath, watch.cursor)
	if err != nil {
		return Result{}, false, nil
	}
	if len(data) > 0 {
		signals, consumed := parser.ParseSessionSignals(data)
		ends := signalEndOffsets(data, watch.cursor, signals)
		watch.cursor += int64(consumed)
		for i, signal := range signals {
			first := false
			isTurnEnd := signal.Kind == SessionSignalAPIErrorTurnEnd || signal.Kind == SessionSignalTurnEnd
			if isTurnEnd && ends[i] > run.state.LaunchOffset {
				watch.turnEnds++
				first = watch.turnEnds == 1
			}
			if signal.Kind == SessionSignalAPIErrorTurnEnd && refusedResume(run.state, signal, first, run.usageLimited(&heldTurnEnd{raw: signal.Raw})) {
				if result, finished, handled, err := run.startFreshAfterRefusedResume(); handled {
					return result, finished, err
				}
			}
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

// turnEndsBetween counts the turn ends, API-error ones included, in the events-file bytes from start up to end.
// An unreadable events file counts none.
func (run *Run) turnEndsBetween(parser SessionSignalParser, start, end int64) int {
	if end <= start {
		return 0
	}
	data, _, err := readEventsFrom(run.state.EventsPath, start)
	if err != nil {
		return 0
	}
	data = data[:min(len(data), int(end-start))]
	signals, _ := parser.ParseSessionSignals(data)
	count := 0
	for _, signal := range signals {
		if signal.Kind == SessionSignalAPIErrorTurnEnd || signal.Kind == SessionSignalTurnEnd {
			count++
		}
	}
	return count
}

// refusedResume reports whether signal is a refused resume's.
// The provider then ended the first turn of a resumed session on an API error.
// It holds when rs records that its latest launch copied the session's transcript.
// It also needs signal to be an API-error turn end that is the first past the record's launch offset,
// and no usage limit to have caused it.
// firstTurnEnd and usageLimited are the caller's readings of those two facts.
func refusedResume(rs RunState, signal SessionSignal, firstTurnEnd, usageLimited bool) bool {
	return rs.TranscriptCopied && signal.Kind == SessionSignalAPIErrorTurnEnd && firstTurnEnd && !usageLimited
}

// signalEndOffsets returns the events-file offset just past the line of each of signals, in order.
// data is the slice of the file that begins at startOffset, and signals were parsed from it.
// A signal whose line is not found in data resolves to the end of data.
func signalEndOffsets(data []byte, startOffset int64, signals []SessionSignal) []int64 {
	ends := make([]int64, len(signals))
	searchFrom := 0
	for i, signal := range signals {
		ends[i] = startOffset + int64(len(data))
		at := bytes.Index(data[searchFrom:], signal.Raw)
		if at < 0 || len(signal.Raw) == 0 {
			continue
		}
		end := searchFrom + at + len(signal.Raw)
		if newline := bytes.IndexByte(data[end:], '\n'); newline >= 0 {
			end += newline + 1
		}
		searchFrom = end
		ends[i] = startOffset + int64(end)
	}
	return ends
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
