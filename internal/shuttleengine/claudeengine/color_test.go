// color_test.go pins the exact choreography of `ColorSequence`.
// It types and submits `/color <name>` for each palette color, and sends no input for a color outside the palette.

package claudeengine

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestColorSequence_TypesTheColorCommandForEveryPaletteColor covers every palette color plus an unknown one.
// The expected steps carry no Escape, since the command may be typed while a turn runs and Escape there interrupts it.
func TestColorSequence_TypesTheColorCommandForEveryPaletteColor(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type row struct {
		color segmentcolor.Color
		want  []shuttleengine.PaneInput
	}
	var rows []row
	for _, color := range segmentcolor.Palette() {
		rows = append(rows, row{color, []shuttleengine.PaneInput{
			{Text: "/color " + string(color)},
			{Key: "Enter"},
		}})
	}
	rows = append(rows, row{"crimson", nil}, row{"", nil})

	for _, tt := range rows {
		t.Run(string(tt.color), func(t *testing.T) {
			t.Parallel()

			got := New().ColorSequence(tt.color)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ColorSequence(%q) = %+v; want %+v", tt.color, got, tt.want)
			}
		})
	}
}
