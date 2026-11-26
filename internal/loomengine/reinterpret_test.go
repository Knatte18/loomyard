// reinterpret_test.go covers FlagCodeAutoAccept over hub loom.yaml bytes.

package loomengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestFlagCodeAutoAccept pins the outcomes of the check.
// A file that sets code.auto_accept to true is flagged with the way forward and left unchanged.
// A file that sets none, and an absent file, are flagged too, since they take the template default of true.
// A file that sets it to false is neither changed nor flagged.
// A file that is no mapping, or sets a value that is no bool, fails.
func TestFlagCodeAutoAccept(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const wayForward = "escalates to the run's parent with cause budget"
	tests := []struct {
		name  string
		input string
		// absent passes nil bytes, as the walk does for a hub with no loom.yaml.
		absent      bool
		wantFlagged bool
		wantError   bool
	}{
		{name: "SetOn", input: "model: sonnet\ncode: {auto_accept: true}\n", wantFlagged: true},
		{name: "SetOff", input: "code: {auto_accept: false}\n"},
		{name: "AreaWithoutTheKey", input: "code: {max_bounces: 2}\n", wantFlagged: true},
		{name: "OtherKeysOnly", input: "model: sonnet\n", wantFlagged: true},
		{name: "HeaderOnly", input: "# tuned for this hub\n", wantFlagged: true},
		{name: "AbsentFile", absent: true, wantFlagged: true},
		{name: "NotABool", input: "code: {auto_accept: maybe}\n", wantError: true},
		{name: "NotAMapping", input: "- a\n- b\n", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var existing []byte
			if !tt.absent {
				existing = []byte(tt.input)
			}
			rewritten, rewrote, flagged, err := loomengine.FlagCodeAutoAccept(existing, nil)
			if tt.wantError {
				if err == nil {
					t.Errorf("FlagCodeAutoAccept(%q) = nil error; want a parse error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("FlagCodeAutoAccept(%q): %v", tt.input, err)
			}
			if string(rewritten) != tt.input {
				t.Errorf("FlagCodeAutoAccept(%q) rewritten = %q; want the input unchanged", tt.input, rewritten)
			}
			if len(rewrote) != 0 {
				t.Errorf("FlagCodeAutoAccept(%q) rewrote = %v; want none", tt.input, rewrote)
			}
			switch {
			case tt.wantFlagged && (len(flagged) != 1 || !strings.Contains(flagged[0], wayForward)):
				t.Errorf("FlagCodeAutoAccept(%q) flagged = %v; want one line naming %q", tt.input, flagged, wayForward)
			case !tt.wantFlagged && len(flagged) != 0:
				t.Errorf("FlagCodeAutoAccept(%q) flagged = %v; want none", tt.input, flagged)
			}
		})
	}
}
