// modelswitch_test.go pins ModelSwitchSequence's exact choreography shape:
// the `/model <name>` command typed and submitted, with the model name passed
// through verbatim and — load-bearing — NO leading Escape key, since the
// sequence is injected while a foreground tool call runs in the target pane
// and Escape there is claude's interrupt-running-tool key (it killed the
// injecting begin-batch subprocess live on 2.1.205).

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestModelSwitchSequence_ShapeAndVerbatimModel proves the returned sequence
// is exactly ["/model <name>"+submit], for several model name shapes
// (including ones containing characters that must NOT be escaped or altered).
func TestModelSwitchSequence_ShapeAndVerbatimModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"simple", "opus"},
		{"versioned", "claude-sonnet-4-5-20250929"},
		{"withSpaces", "sonnet 4.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			got := c.ModelSwitchSequence(tt.model)

			want := []shuttleengine.PaneInput{
				{Text: "/model " + tt.model, Submit: true},
			}
			if len(got) != len(want) {
				t.Fatalf("ModelSwitchSequence(%q) = %d steps; want %d", tt.model, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("ModelSwitchSequence(%q)[%d] = %+v; want %+v", tt.model, i, got[i], want[i])
				}
			}
		})
	}
}

// TestModelSwitchSequence_NoKeyPresses proves no step in the sequence sends a
// bare key press (Escape included): the sequence is injected mid-tool-call,
// where Escape interrupts the running tool and aborts the target session's
// turn — the W2b corruption mode webster's hardening round confirmed live.
func TestModelSwitchSequence_NoKeyPresses(t *testing.T) {
	c := New()
	got := c.ModelSwitchSequence("opus")

	if len(got) == 0 {
		t.Fatal("ModelSwitchSequence() returned no steps")
	}
	for i, step := range got {
		if step.Key != "" {
			t.Errorf("ModelSwitchSequence()[%d].Key = %q; want no key presses anywhere in the sequence", i, step.Key)
		}
	}
}
