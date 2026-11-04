// apierrorretry_test.go covers how Wait re-sends a turn the provider's API ended on an error, and how the run ends once the re-sends are spent.

package shuttleengine

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// reactingReed is an echoReed that runs react after each line typed into its pane, as the agent's answer to it.
// Its first failSends sends fail before any line is typed.
type reactingReed struct {
	*echoReed

	react     func()
	failSends int
}

func (r *reactingReed) SendText(guid, text string, submit bool) error {
	if r.failSends > 0 {
		r.failSends--
		return errors.New("pane busy")
	}
	if err := r.echoReed.SendText(guid, text, submit); err != nil {
		return err
	}
	r.react()
	return nil
}

func TestWait_APIErrorRetry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const defaultEvents = "START\nAPIERR:server_error\n"
	tests := []struct {
		name    string
		retries int
		// events is the events file the run starts over, defaultEvents when empty.
		events string
		// react is what the agent appends to its events file after each line typed into its pane, and writeOutput makes it write the output file first.
		react       []string
		writeOutput bool
		// outputFirst writes the output file before Wait starts.
		outputFirst bool
		// covered has a pause cover the run until the first tick has passed.
		covered bool
		// failSends is how many sends fail before one is delivered.
		failSends int
		// promptOffset is the run's prompt offset: the API errors before it are not the run's.
		promptOffset int64
		timeout      time.Duration
		wantOutcome  Outcome
		// wantGaps is the least run-clock time before each send, from the API-error turn end it re-sends.
		wantGaps     []time.Duration
		wantAPIError string
		// ownWindow makes the run's strand an own-window one, and wantRemoved is how many strand removals the run makes, asserted only then.
		ownWindow   bool
		wantRemoved int
	}{
		{
			name: "a re-send after the back-off lets the agent finish", retries: 2,
			react: []string{"START", "STOP:done"}, writeOutput: true,
			wantOutcome: OutcomeDone, wantGaps: []time.Duration{apiErrorRetryBackoff},
		},
		{
			name: "an API error past the re-sends ends the run naming the error and removes its strand", retries: 2, ownWindow: true,
			react:       []string{"START", "APIERR:server_error"},
			wantOutcome: OutcomeAPIError, wantGaps: []time.Duration{apiErrorRetryBackoff, 2 * apiErrorRetryBackoff}, wantAPIError: "server_error", wantRemoved: 1,
		},
		{
			name: "an API error with no text ends the run naming an unknown error", retries: 1, events: "START\nAPIERR\n",
			react:       []string{"START", "APIERR"},
			wantOutcome: OutcomeAPIError, wantGaps: []time.Duration{apiErrorRetryBackoff}, wantAPIError: apiErrorUnknownText,
		},
		{
			name: "a run with no retries re-sends nothing and waits out its deadline", retries: 0, timeout: 2 * time.Minute,
			wantOutcome: OutcomeTimeout,
		},
		{
			name: "an API error with every output file on disk ends the run done with no re-send", retries: 2, outputFirst: true,
			wantOutcome: OutcomeDone,
		},
		{
			name: "a later turn start and turn end clear the API error, so nothing is re-sent", retries: 2, timeout: 2 * time.Minute,
			events:      "START\nAPIERR:server_error\nSTART\nSTOP:recovered\n",
			wantOutcome: OutcomeTimeout,
		},
		{
			name: "a pause covering the run holds the end until it lifts", retries: 1, covered: true,
			events:      "START\nAPIERR:first\nAPIERR:second\n",
			wantOutcome: OutcomeAPIError, wantAPIError: "second",
		},
		{
			name: "a send that fails is tried again on a later tick", retries: 2, failSends: 1,
			react: []string{"START", "STOP:done"}, writeOutput: true,
			wantOutcome: OutcomeDone, wantGaps: []time.Duration{apiErrorRetryBackoff},
		},
		{
			name: "an API error before the prompt offset is not counted against the re-sends", retries: 1,
			events: "START\nAPIERR:old\nSTART\nAPIERR:new\n", promptOffset: int64(len("START\nAPIERR:old\n")),
			react: []string{"START", "STOP:done"}, writeOutput: true,
			wantOutcome: OutcomeDone, wantGaps: []time.Duration{apiErrorRetryBackoff},
		},
		{
			name: "a rejected model ends a run with no retries at once and removes its strand", retries: 0, ownWindow: true,
			events:      "START\nREJECT:model_not_found\n",
			wantOutcome: OutcomeAPIError, wantAPIError: "model_not_found", wantRemoved: 1,
		},
		{
			name: "a rejected model ends the run at once, spending no re-send", retries: 2, ownWindow: true,
			events:      "START\nREJECT:model_not_found\n",
			wantOutcome: OutcomeAPIError, wantAPIError: "model_not_found", wantRemoved: 1,
		},
		{
			name: "a rejected model with every output file on disk ends the run done", retries: 0, outputFirst: true, ownWindow: true,
			events:      "START\nREJECT:model_not_found\n",
			wantOutcome: OutcomeDone, wantRemoved: 1,
		},
		{
			name: "a pause covering the run holds the rejected-model end until it lifts", retries: 0, covered: true, ownWindow: true,
			events:      "START\nREJECT:model_not_found\n",
			wantOutcome: OutcomeAPIError, wantAPIError: "model_not_found", wantRemoved: 1,
		},
		{
			name: "earlier API errors in the events file count against the re-sends", retries: 2,
			events:      "START\nAPIERR:first\nAPIERR:second\nAPIERR:third\n",
			wantOutcome: OutcomeAPIError, wantAPIError: "third",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine := &sessionFakeEngine{waitingEngine: waitingEngine{fakeEngine: fakeEngine{StartupScript: []StartupState{StartupReady}}}, liveness: LivenessAlive}
			fc := newFakeClock(time.Now())
			start := fc.Now()
			output := filepath.Join(t.TempDir(), "out.md")
			status := liveStrandStatus(true)
			if tt.ownWindow {
				status[0].Strands[0].OwnWindow, status[0].Strands[0].Present = true, true
			}
			pane := &reactingReed{echoReed: &echoReed{fakeReed: &fakeReed{StatusQueue: status}, pane: "❯ "}, failSends: tt.failSends}
			fx := newFixture(t, pane, engine, withConfig(gateConfig))
			var clk Clock = fc
			lifted := false
			if tt.covered {
				geometry, resume := coverHub(t, fc.Now())
				fx.Runner.SetPause(geometry)
				clk = &multiStepClock{fakeClock: fc, steps: []func(){func() {
					resume(fc.Now())
					lifted = true
				}}}
			}
			timeout := cmp.Or(tt.timeout, time.Hour)
			run := fx.newRun(Spec{OutputFiles: []string{output}, Timeout: timeout, APIErrorRetries: tt.retries},
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", PromptOffset: tt.promptOffset}),
				withRunEvents(cmp.Or(tt.events, defaultEvents)),
				withRunOffset(tt.promptOffset),
				withRunClock(clk, start.Add(timeout)),
			)
			var sentAt []time.Time
			pane.react = func() {
				sentAt = append(sentAt, fc.Now())
				if tt.writeOutput {
					if err := os.WriteFile(output, []byte("done"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				for _, line := range tt.react {
					appendEventsLine(t, run.state.EventsPath, line)
				}
			}
			if tt.outputFirst {
				if err := os.WriteFile(output, []byte("done"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			result, err := run.Wait()

			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != tt.wantOutcome || result.APIError != tt.wantAPIError {
				t.Errorf("Wait() outcome, API error = %q, %q; want %q, %q", result.Outcome, result.APIError, tt.wantOutcome, tt.wantAPIError)
			}
			if pane.failSends != 0 {
				t.Errorf("failSends left = %d; want 0, every failing send attempted", pane.failSends)
			}
			if tt.ownWindow && len(pane.RemoveStrandCalls) != tt.wantRemoved {
				t.Errorf("RemoveStrand calls = %d; want %d", len(pane.RemoveStrandCalls), tt.wantRemoved)
			}
			if tt.covered && !lifted {
				t.Error("Wait() ended the run while a pause covered it; want it held until the pause lifted")
			}
			if len(pane.typed) != len(tt.wantGaps) {
				t.Fatalf("typed = %q; want %d re-sends", pane.typed, len(tt.wantGaps))
			}
			errorAt := start
			for i, text := range pane.typed {
				if text != apiErrorRetryLine {
					t.Errorf("typed[%d] = %q; want %q", i, text, apiErrorRetryLine)
				}
				if gap := sentAt[i].Sub(errorAt); gap < tt.wantGaps[i] || gap > tt.wantGaps[i]+10*time.Second {
					t.Errorf("re-send %d came %v after its API error; want the back-off %v, within a few ticks", i+1, gap, tt.wantGaps[i])
				}
				errorAt = sentAt[i]
			}
		})
	}
}
