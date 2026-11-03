// dismisspicker_test.go covers the guards and the settle of dismissPicker over a guarded fake engine and a fake pane.

package shuttleengine

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDismissPicker_Guards pins that a picker shown over a running turn gets no key and records no try, so a later poll can try again,
// and that a picker over a quiet pane gets one Escape, records the try and leaves the pane alone for the engine's interrupt settle.
func TestDismissPicker_Guards(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const settle = 2 * time.Second
	tests := []struct {
		name        string
		capture     string
		wantHandled bool
		wantEscapes int
		wantSlept   time.Duration
	}{
		{name: "a picker over a running turn", capture: "COMPACT-PICKER RUNNING"},
		{name: "a picker over a quiet pane", capture: "COMPACT-PICKER", wantHandled: true, wantEscapes: 1, wantSlept: settle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			engine := &guardedDismissPickerEngine{
				dismissPickerEngine: dismissPickerEngine{readinessEngine: readinessEngine{idleEngine: idleEngine{sessionFakeEngine: sessionFakeEngine{liveness: LivenessAlive}}}},
				settle:              settle,
			}
			rig := newGateRig(t, Config{SendReadyTimeoutS: 1}, "STOP:turn1\n", withGateEngine(engine))
			rig.reed.picker = tt.capture
			picker := &pickerDismissal{}

			_, handled := dismissPicker(rig.run.newSendContext(), engine, &turnStartHold{}, &strayClear{}, picker, tt.capture)

			if handled != tt.wantHandled {
				t.Errorf("handled = %v, want %v", handled, tt.wantHandled)
			}
			if picker.tried != tt.wantHandled {
				t.Errorf("picker.tried = %v, want %v", picker.tried, tt.wantHandled)
			}
			escapes := 0
			for _, call := range rig.reed.SendKeyCalls {
				if call.Key == "Escape" {
					escapes++
				}
			}
			if escapes != tt.wantEscapes {
				t.Errorf("Escape keys = %d, want %d", escapes, tt.wantEscapes)
			}
			if slept := rig.clock.Now().Sub(rig.start); slept != tt.wantSlept {
				t.Errorf("clock advanced %v, want %v", slept, tt.wantSlept)
			}
		})
	}
}
