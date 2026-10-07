// modelswitch_test.go pins ModelSwitchSequence's exact choreography shape: the `/model <name>`
// command typed and submitted, with the model name passed through verbatim and — load-bearing — NO
// leading Escape key, since the sequence is injected while a foreground tool call runs in the
// target pane and Escape there is claude's interrupt-running-tool key (it killed the injecting
// begin-batch subprocess live on 2.1.205).

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestModelSwitchSequence_ShapeAndVerbatimModel proves the returned sequence is exactly ["/model <name>"+submit], for several model name shapes (including ones containing characters that must NOT be escaped or altered).
// The expected step carries no Key, so the equality below also proves the sequence sends no key press (Escape included): it is injected mid-tool-call, where Escape interrupts the running tool and aborts the target session's turn, a corruption mode confirmed live.
func TestModelSwitchSequence_ShapeAndVerbatimModel(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			c := New()
			got := c.ModelSwitchSequence(tt.model)

			want := []shuttleengine.PaneInput{
				{Text: "/model " + tt.model, SettleMS: defaultSubmitSettleMS},
				{Key: "Enter"},
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
