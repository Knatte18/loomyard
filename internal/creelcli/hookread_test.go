//go:build tier0

// hookread_test.go pins the exit decision of the hidden hook-read verb, which only a hook-driven CLI run reaches in pieces.

package creelcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestHookWaitExit pins the exit code of hook-wait for the decisions the CLI rows reach only in part:
// a refused or lost confirmation after the lines were written still wakes the session, and an acknowledgement that was never sent does not.
func TestHookWaitExit(t *testing.T) {
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

// sendError wraps an error the way the client wraps a failed write of the acknowledgement, so the row reaches errors.Is through a chain.
type sendError struct{ err error }

func (e *sendError) Error() string { return "write failed: " + e.err.Error() }

func (e *sendError) Unwrap() error { return e.err }

// TestHookReadExit pins the exit code and whether the error is printed for the decisions no CLI row reaches:
// an acknowledgement failing after the block was written, a daemon failure before the write and a failed tool-boundary claim.
func TestHookReadExit(t *testing.T) {
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
