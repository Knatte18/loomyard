// hookread_test.go pins the exit decisions of the hidden hook-read and hook-wait verbs.
// It also pins hook-wait's wake, which only a hook-driven CLI run reaches in pieces.

package creelcli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHookWaitExit pins the exit code of hook-wait for the decisions the CLI rows reach only in part:
// a refused or lost confirmation after the lines were written still wakes the session,
// and an acknowledgement that was never sent does not.
func TestHookWaitExit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	failure := errors.New("creel: the daemon refused the confirmation")
	unsent := &sendError{creelengine.ErrAckUnsent}
	tests := []struct {
		name       string
		wroteLines bool
		err        error
		want       int
	}{
		{"LinesWrittenAndConfirmed", true, nil, 2},
		{"LinesWrittenAndTheConfirmationRefusedOrLost", true, failure, 2},
		{"LinesWrittenAndTheAckUnsent", true, unsent, 0},
		{"LinesNotWritten", false, failure, 0},
		{"NothingWrittenAndNoError", false, nil, 0},
		{"TheAckUnsentWithNoLinesWritten", false, unsent, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hookWaitExit(2, tt.wroteLines, tt.err); got != tt.want {
				t.Errorf("hookWaitExit(2, %v, %v) = %d; want %d", tt.wroteLines, tt.err, got, tt.want)
			}
		})
	}
}

// rewakeRenderer is an engine whose idle waiter wakes the session by writing the text to stderr and exiting 2.
type rewakeRenderer struct{}

func (rewakeRenderer) HookInput([]byte) (shuttleengine.HookInput, error) {
	return shuttleengine.HookInput{}, nil
}

func (rewakeRenderer) HookOutput(shuttleengine.HookEvent, string, string) ([]byte, error) {
	return nil, nil
}

func (rewakeRenderer) RewakeOutput(text string) ([]byte, int) { return []byte(text + "\n"), 2 }

// queueingRenderer is an engine whose idle waiter starts the woken turn itself;
// it records what it was asked to wake with and answers err.
type queueingRenderer struct {
	rewakeRenderer
	err     error
	payload string
	text    string
}

func (r *queueingRenderer) WakeIdle(payload []byte, text string) error {
	r.payload, r.text = string(payload), text
	return r.err
}

// TestWakeSession pins hook-wait's wake of a rung session.
// An engine woken by a hook's exit gets its wake text on stderr and its wake code.
// An engine that wakes the session itself is handed the hook payload and the mail and ends the hook with 0.
// Its failure is returned so the mail stays unread.
func TestWakeSession(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	payload, mail := []byte(`{"session_id":"thread-1"}`), "[creel] #1\n[creel] #2"
	failure := errors.New("codex queue failed")
	tests := []struct {
		name       string
		renderer   shuttleengine.ToolBoundaryContextRenderer
		wantCode   int
		wantStderr string
		wantErr    error
		wantWaked  bool
	}{
		{name: "WokenByTheHookExit", renderer: rewakeRenderer{}, wantCode: 2, wantStderr: mail + "\n"},
		{name: "WakesItself", renderer: &queueingRenderer{}, wantWaked: true},
		{name: "WakeFails", renderer: &queueingRenderer{err: failure}, wantErr: failure, wantWaked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			code, err := wakeSession(tt.renderer, payload, mail, &stderr)
			if code != tt.wantCode || !errors.Is(err, tt.wantErr) || stderr.String() != tt.wantStderr {
				t.Errorf("wakeSession() = %d, %v with stderr %q; want %d, %v with stderr %q", code, err, stderr.String(), tt.wantCode, tt.wantErr, tt.wantStderr)
			}
			if queueing, ok := tt.renderer.(*queueingRenderer); ok && (queueing.payload != string(payload) || queueing.text != mail) {
				t.Errorf("WakeIdle got (%q, %q); want (%q, %q)", queueing.payload, queueing.text, payload, mail)
			}
		})
	}
}

// sendError wraps a failed write of the acknowledgement.
// Unwrap exposes the wrapped error, so errors.Is matches it through the chain.
type sendError struct{ err error }

func (e *sendError) Error() string { return "write failed: " + e.err.Error() }

func (e *sendError) Unwrap() error { return e.err }

// TestHookReadExit pins the exit code and whether the error is printed for the decisions no CLI row reaches:
// an acknowledgement failing after the block was written, a daemon failure before the write;
// and a failed tool-boundary claim.
func TestHookReadExit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	failure := errors.New("creel: the daemon failed")
	tests := []struct {
		name      string
		event     shuttleengine.HookEvent
		wrote     bool
		err       error
		wantCode  int
		wantPrint bool
	}{
		{"StopWroteTheMailAndTheAcknowledgementFailed", shuttleengine.HookEventStop, true, failure, 0, true},
		{"StopWroteTheMail", shuttleengine.HookEventStop, true, nil, 0, false},
		{"StopFailedBeforeTheWrite", shuttleengine.HookEventStop, false, failure, 1, true},
		{"StopClaimedNothing", shuttleengine.HookEventStop, false, nil, 1, false},
		{"ToolBoundaryClaimFailed", shuttleengine.HookEventToolBoundary, false, failure, 1, true},
		{"ToolBoundaryClaimedNothing", shuttleengine.HookEventToolBoundary, false, nil, 0, false},
		{"RefusedInput", "", false, failure, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, printErr := hookReadExit(tt.event, tt.wrote, tt.err)
			if code != tt.wantCode || printErr != tt.wantPrint {
				t.Errorf("hookReadExit(%q, %v, %v) = (%d, %v); want (%d, %v)", tt.event, tt.wrote, tt.err, code, printErr, tt.wantCode, tt.wantPrint)
			}
		})
	}
}
