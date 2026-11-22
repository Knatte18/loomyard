// reinterpret_test.go covers FlagRemoteControlDefault over hub orch.yaml bytes.

package orchengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestFlagRemoteControlDefault pins the outcomes of the check.
// A file that sets no remote_control is flagged with the step that turns it on and left unchanged.
// A file that sets it, on or off, is neither changed nor flagged.
// A file that is no mapping fails to parse.
func TestFlagRemoteControlDefault(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const wayForward = "`lym config orch --set remote_control=true`"
	tests := []struct {
		name        string
		input       string
		wantFlagged bool
		wantError   bool
	}{
		{name: "OtherKeysOnly", input: "model: sonnet\n", wantFlagged: true},
		{name: "HeaderOnly", input: "# tuned for this hub\n", wantFlagged: true},
		{name: "SetOn", input: "remote_control: true\n"},
		{name: "SetOff", input: "model: sonnet\nremote_control: false\n"},
		{name: "NotAMapping", input: "- a\n- b\n", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rewritten, rewrote, flagged, err := orchengine.FlagRemoteControlDefault([]byte(tt.input), nil)
			if tt.wantError {
				if err == nil {
					t.Errorf("FlagRemoteControlDefault(%q) = nil error; want a parse error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("FlagRemoteControlDefault(%q): %v", tt.input, err)
			}
			if string(rewritten) != tt.input {
				t.Errorf("FlagRemoteControlDefault(%q) rewritten = %q; want the input unchanged", tt.input, rewritten)
			}
			if len(rewrote) != 0 {
				t.Errorf("FlagRemoteControlDefault(%q) rewrote = %v; want none", tt.input, rewrote)
			}
			switch {
			case tt.wantFlagged && (len(flagged) != 1 || !strings.Contains(flagged[0], wayForward)):
				t.Errorf("FlagRemoteControlDefault(%q) flagged = %v; want one line naming %s", tt.input, flagged, wayForward)
			case !tt.wantFlagged && len(flagged) != 0:
				t.Errorf("FlagRemoteControlDefault(%q) flagged = %v; want none", tt.input, flagged)
			}
		})
	}
}
