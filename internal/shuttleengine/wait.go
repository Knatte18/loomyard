// wait.go implements Run.Wait: the poll loop that reads a run's
// events.jsonl, classifies its terminal outcome (done/asking/died/timeout),
// probes the startup window for a trust-dialog dismissal or a fast-failing
// dead pane, and runs the done-outcome cleanup (strand removal + run dir
// deletion). Wait is the only place in the run loop that sleeps — the clock
// seam defined here lets tests replay a whole poll sequence instantly.

package shuttleengine

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// clock abstracts time.Now/time.Sleep so Wait's poll loop runs instantly
// under test: a fake clock's Sleep advances a virtual "now" rather than
// blocking, and a same-package test overrides a Run's unexported clock
// field directly before calling Wait.
type clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// realClock is the production clock: real wall-clock time, real sleeping.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// maxEventsReadRetries bounds how many consecutive tick failures to read
// events.jsonl Wait tolerates before reporting a mechanism failure — a
// transient share-violation or a file mid-rename should not abort a run
// outright, but a persistently unreadable events file leaves no
// classifiable outcome at all (the run loop cannot see turn-end signals).
const maxEventsReadRetries = 3

// maxStatusRetries bounds how many CONSECUTIVE mux.Status failures Wait
// tolerates before reporting a mechanism failure, per the Shared Decision
// wording ("Status error twice consecutively").
const maxStatusRetries = 2

// Wait blocks until run reaches a terminal outcome, polling at
// cfg.PollIntervalMS via run.clock (real time in production, instant in
// tests). Each tick: reads and parses any new events.jsonl bytes,
// classifying done/asking on any new Stop event; every
// cfg.LivenessEveryNPolls-th tick, checks the strand's liveness via mux
// and, during the startup window, probes the pane for a trust prompt or a
// still-pending fast-fail; and checks spec.Timeout's deadline. A terminal
// classification always returns error == nil — Wait's error return is
// reserved for mechanism failures (events.jsonl unreadable after retries,
// mux.Status failing twice consecutively) that leave no classifiable
// outcome at all.
func (run *Run) Wait() (Result, error) {
	cfg := run.runner.cfg
	interval := time.Duration(cfg.PollIntervalMS) * time.Millisecond
	livenessEvery := cfg.LivenessEveryNPolls
	if livenessEvery <= 0 {
		livenessEvery = 1
	}
	startupTimeout := time.Duration(cfg.StartupTimeoutS) * time.Second
	startupDeadline := run.clock.Now().Add(startupTimeout)

	started := false
	eventsFailures := 0
	statusFailures := 0

	for tick := 1; ; tick++ {
		outcome, message, err := run.pollEventsTick()
		if err != nil {
			eventsFailures++
			if eventsFailures >= maxEventsReadRetries {
				return Result{}, fmt.Errorf("shuttle: events file unreadable after %d attempts: %w", maxEventsReadRetries, err)
			}
		} else {
			eventsFailures = 0
			if outcome != "" {
				return run.finalize(outcome, message)
			}
		}

		if tick%livenessEvery == 0 {
			livenessOutcome, err := run.checkLivenessTick(&started, startupDeadline)
			if err != nil {
				statusFailures++
				if statusFailures >= maxStatusRetries {
					return Result{}, fmt.Errorf("shuttle: mux status failed %d times consecutively: %w", maxStatusRetries, err)
				}
			} else {
				statusFailures = 0
				if livenessOutcome != "" {
					return run.finalize(livenessOutcome, "")
				}
			}
		}

		if run.clock.Now().After(run.deadline) {
			return run.finalize(OutcomeTimeout, "")
		}

		run.clock.Sleep(interval)
	}
}

// pollEventsTick reads any events.jsonl bytes appended since run.offset,
// parses them via the engine, and advances run.offset past whatever it
// consumed. It classifies OutcomeDone/OutcomeAsking from the LAST StopEvent
// among the newly parsed ones (a batch containing more than one Stop — e.g.
// an interrupted turn immediately followed by a resumed one — is classified
// by its most recent turn-end, and every consumed byte still counts, so
// none of the earlier events in the same batch is ever reprocessed).
// Returns outcome == "" when there is nothing new to classify yet.
func (run *Run) pollEventsTick() (Outcome, string, error) {
	data, newOffset, err := readEventsFrom(run.state.EventsPath, run.offset)
	if err != nil {
		return "", "", err
	}
	run.offset = newOffset
	if len(data) == 0 {
		return "", "", nil
	}

	events, err := run.runner.engine.ParseEvents(data)
	if err != nil {
		return "", "", err
	}
	if len(events) == 0 {
		return "", "", nil
	}

	last := events[len(events)-1]
	if allOutputFilesExist(run.spec.OutputFiles) {
		return OutcomeDone, "", nil
	}
	return OutcomeAsking, last.LastAssistantMessage, nil
}

// readEventsFrom reads path from byte offset onward and returns only the
// bytes up to (and including) the last complete line — a trailing partial
// line (no terminating '\n' yet) is left unconsumed, so newOffset does not
// advance past it and the next tick re-reads and, hopefully, completes it.
// A not-yet-created events file (the engine has not appended its first
// line yet) is not an error: it returns (nil, offset, nil) unchanged.
func readEventsFrom(path string, offset int64) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, offset, nil
		}
		return nil, offset, fmt.Errorf("open events file: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seek events file: %w", err)
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, offset, fmt.Errorf("read events file: %w", err)
	}
	if len(buf) == 0 {
		return nil, offset, nil
	}

	lastNL := bytes.LastIndexByte(buf, '\n')
	if lastNL == -1 {
		// A partial append in progress: nothing complete to parse yet.
		return nil, offset, nil
	}

	consumed := buf[:lastNL+1]
	return consumed, offset + int64(len(consumed)), nil
}

// allOutputFilesExist reports whether every entry in files exists on disk —
// the file contract's "done" test.
func allOutputFilesExist(files []string) bool {
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return false
		}
	}
	return true
}

// checkLivenessTick checks the strand's liveness via mux.Status and, while
// still in the startup window (*started is false and startupDeadline has
// not passed), probes the pane for the engine's Startup classification: a
// trust prompt is dismissed with an Enter key press, a ready pane flips
// *started to true (ending the probe for the rest of this Wait call), and
// a still-booting pane is left pending unless startupDeadline has passed,
// which fast-fails the run as died rather than waiting out the full
// spec.Timeout on a pane that will never come up. Returns a non-nil error
// only for mux.Status itself failing — a mechanism failure Wait's caller
// tracks across consecutive ticks; every other failure along this path (a
// CapturePane error, a SendKey error dismissing the trust prompt) is logged
// and treated as "still pending" rather than propagated, since none of
// them is fatal to the run on its own.
func (run *Run) checkLivenessTick(started *bool, startupDeadline time.Time) (Outcome, error) {
	status, err := run.runner.mux.Status()
	if err != nil {
		return "", fmt.Errorf("mux status: %w", err)
	}

	live := false
	for _, s := range status.Strands {
		if s.GUID == run.state.StrandGUID {
			live = s.Live
			break
		}
	}
	if !live {
		return OutcomeDied, nil
	}

	if *started {
		return "", nil
	}

	capture, err := run.runner.mux.CapturePane(run.state.StrandGUID)
	if err != nil {
		log.Printf("shuttle: capture pane during startup probe (non-fatal, retrying): %v", err)
		return "", nil
	}

	switch run.runner.engine.Startup(capture) {
	case StartupReady:
		*started = true
	case StartupTrustPrompt:
		if err := run.runner.mux.SendKey(run.state.StrandGUID, "Enter"); err != nil {
			log.Printf("shuttle: dismiss trust prompt (non-fatal): %v", err)
		}
	case StartupPending:
		if run.clock.Now().After(startupDeadline) {
			return OutcomeDied, nil
		}
	}
	return "", nil
}

// finalize builds run's terminal Result and, for OutcomeDone without
// spec.KeepPane, removes the strand and the run directory — cleanup
// errors are logged, not fatal, since the classification itself already
// stands. Every other outcome keeps both the strand and the run directory
// for diagnosis/attach.
func (run *Run) finalize(outcome Outcome, message string) (Result, error) {
	result := Result{
		Outcome:              outcome,
		SessionID:            run.state.SessionID,
		StrandGUID:           run.state.StrandGUID,
		LastAssistantMessage: message,
		RunDir:               run.runDir,
	}

	if outcome == OutcomeDone && !run.spec.KeepPane {
		if _, err := run.runner.mux.RemoveStrand(run.state.StrandGUID, false); err != nil {
			log.Printf("shuttle: cleanup: remove strand %s (non-fatal): %v", run.state.StrandGUID, err)
		}
		if err := os.RemoveAll(run.runDir); err != nil {
			log.Printf("shuttle: cleanup: remove run dir %s (non-fatal): %v", run.runDir, err)
		}
	}

	return result, nil
}
