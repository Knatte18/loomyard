// modelswitch_test.go pins ModelSwitchSequence's exact choreography shape:
// Escape first with its settle pause, then the `/model <name>` command typed
// and submitted, with the model name passed through verbatim.

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestModelSwitchSequence_ShapeAndVerbatimModel proves the returned sequence
// is exactly [Escape+settle, "/model <name>"+submit], for several model
// name shapes (including ones containing characters that must NOT be
// escaped or altered).
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
				{Key: "Escape", SettleMS: composeSendSettleMS},
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

// TestModelSwitchSequence_EscapeCarriesNoText proves the leading Escape step
// carries no Text/Submit — it is a pure key press, exactly like ComposeSend's
// leading Escape.
func TestModelSwitchSequence_EscapeCarriesNoText(t *testing.T) {
	c := New()
	got := c.ModelSwitchSequence("opus")

	if len(got) == 0 {
		t.Fatal("ModelSwitchSequence() returned no steps")
	}
	first := got[0]
	if first.Key != "Escape" || first.Text != "" || first.Submit {
		t.Errorf("ModelSwitchSequence()[0] = %+v; want a pure Escape key press", first)
	}
	if first.SettleMS != composeSendSettleMS {
		t.Errorf("ModelSwitchSequence()[0].SettleMS = %d; want %d (same settle gap as ComposeSend)", first.SettleMS, composeSendSettleMS)
	}
}
