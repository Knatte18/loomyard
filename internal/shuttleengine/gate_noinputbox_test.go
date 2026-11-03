// gate_noinputbox_test.go covers how the gated wait loop counts re-prompt sends held by a pane with no input box:
// only holds in a row count, and a hold of another kind, a new arrival or a turn start after the boundary restarts the count, as does a delivered re-prompt followed by an arrival.

package shuttleengine

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// noInputBoxStep is one change a test makes to the pane and the events file once the clock reaches at.
// picker replaces the pane's picker, so an empty one shows the input box, and line is appended to the events file when non-empty.
type noInputBoxStep struct {
	at     time.Duration
	picker string
	line   string
}

// TestGate_NoInputBoxBoundCountsHoldsInARow pins that maxNoInputBoxReprompts counts consecutive holds by a pane with no input box.
// A hold of another kind, a new arrival and a turn start after the boundary each restart the count.
// A delivered re-prompt followed by an arrival is one row: the two resets cover for each other, so the row pins them together.
// It is not parallel: it captures the process-wide logger.
func TestGate_NoInputBoxBoundCountsHoldsInARow(t *testing.T) {
	tiergate.Require(t, 0)
	const (
		menu      = "some menu"
		boundWarn = "re-prompt held by a pane with no input box past the bound"
	)

	// A send waits out its one-second idle window and the loop sleeps one second between ticks, so a send starts every two seconds and ends one second later.
	var alternating []noInputBoxStep
	for at := 2 * time.Second; at < 30*time.Second; at += 2 * time.Second {
		picker := ""
		if (at/(2*time.Second))%2 == 0 {
			picker = menu
		}
		alternating = append(alternating, noInputBoxStep{at: at, picker: picker})
	}

	tests := []struct {
		name  string
		steps []noInputBoxStep
		// interrupted makes the engine report any turn start interrupted.
		interrupted    bool
		active         bool
		wantSends      int
		wantAttempts   int
		wantEndBefore  time.Duration
		wantEndAtLeast time.Duration
		wantBoundWarn  bool
	}{
		{
			name: "a hold of another kind between no-input-box holds restarts the count", steps: alternating, active: true,
			wantEndAtLeast: 30 * time.Second,
		},
		{
			name:  "a new arrival restarts the count",
			steps: []noInputBoxStep{{at: 8 * time.Second, picker: menu, line: "STOP:turn2"}},
			// Four holds precede the arrival at 8 s; five fresh ones end the loop at 17 s.
			wantEndAtLeast: 14 * time.Second, wantEndBefore: 30 * time.Second, wantBoundWarn: true,
		},
		{
			name:  "a delivered re-prompt followed by a new arrival restarts the count",
			steps: []noInputBoxStep{{at: 8 * time.Second}, {at: 10 * time.Second, picker: menu, line: "STOP:turn2"}},
			// The pane clears at 8 s and the re-prompt is delivered; the arrival at 10 s brings five fresh holds.
			wantSends: 1, wantAttempts: 1, wantEndAtLeast: 16 * time.Second, wantEndBefore: 30 * time.Second, wantBoundWarn: true,
		},
		{
			name:        "a turn start after the boundary restarts the count",
			steps:       []noInputBoxStep{{at: 8 * time.Second, picker: menu, line: "START"}},
			interrupted: true,
			// Four holds precede the turn start at 8 s; the boundary it clears is restored at once, and its next hold would be the fifth without the restart.
			wantEndAtLeast: 30 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)

			engine := &dismissPickerEngine{readinessEngine: readinessEngine{idleEngine: idleEngine{sessionFakeEngine: sessionFakeEngine{liveness: LivenessAlive}}}}
			engine.interrupted = tt.interrupted
			rig := newGateRig(t, Config{SendReadyTimeoutS: 1}, "STOP:turn1\n", withGateEngine(engine))
			rig.reed.picker, rig.reed.pickerSticky = menu, true
			rig.reed.ActiveQueue = []bool{tt.active}
			next := 0
			rig.clock.onSleep = func(now time.Time) {
				for next < len(tt.steps) && now.Sub(rig.start) >= tt.steps[next].at {
					step := tt.steps[next]
					next++
					rig.reed.picker = step.picker
					if step.line != "" {
						appendEventsLine(t, rig.eventsPath, step.line)
					}
				}
			}

			result, err := rig.run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if sends := len(rig.reed.SendTextCalls); sends != tt.wantSends {
				t.Errorf("SendText calls = %d, want %d", sends, tt.wantSends)
			}
			if result.Gate == nil || result.Gate.Attempts != tt.wantAttempts {
				t.Errorf("Gate = %+v, want %d attempts spent", result.Gate, tt.wantAttempts)
			}
			ended := rig.clock.Now().Sub(rig.start)
			if tt.wantEndBefore > 0 && ended >= tt.wantEndBefore {
				t.Errorf("ended %v after the start, want before %v", ended, tt.wantEndBefore)
			}
			if tt.wantEndAtLeast > 0 && ended < tt.wantEndAtLeast {
				t.Errorf("ended %v after the start, want at least %v", ended, tt.wantEndAtLeast)
			}
			wantWarns := 0
			if tt.wantBoundWarn {
				wantWarns = 1
			}
			if warns := countLevelLines(buf.String(), "WARN", boundWarn); warns != wantWarns {
				t.Errorf("bound WARN lines = %d, want %d; log: %s", warns, wantWarns, buf.String())
			}
		})
	}
}
