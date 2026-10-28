//go:build tier0

// hookread_test.go pins the exit decision of the hidden hook-read verb, which only a hook-driven CLI run reaches in pieces.

package creelcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

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
		{"StopWroteTheBlockAndTheAcknowledgementFailed", shuttleengine.HookEventStop, true, failure, 0, true},
		{"StopWroteTheBlock", shuttleengine.HookEventStop, true, nil, 0, false},
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
