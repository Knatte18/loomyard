// add_test.go pins `lyx reed add`'s --anchor vocabulary refusal, which runs before the engine is called.

package reedcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestAddCmd_RefusesAnAnchorOutsideTheVocabulary drives addCmd directly with a nil engine,
// so a refusal that reached the engine would panic and the nil engine proves nothing was added.
func TestAddCmd_RefusesAnAnchorOutsideTheVocabulary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		anchor string
	}{
		{"BelowParent", "below-parent"},
		{"Top", "top"},
		{"Overview", "overview"},
		{"Sideways", "sideways"},
		{"Empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := (&reedCLI{}).addCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--cmd", "true", "--anchor=" + tt.anchor})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error: %v; output: %s", err, out.String())
			}

			env := envelope.Decode(t, out.String())
			if env.OK {
				t.Errorf("envelope ok = true; want false for --anchor=%q; output: %s", tt.anchor, out.String())
			}
			for _, want := range []string{`"` + tt.anchor + `"`, "own-window"} {
				if !strings.Contains(env.Error, want) {
					t.Errorf("error = %q; want it to contain %q", env.Error, want)
				}
			}
		})
	}
}
