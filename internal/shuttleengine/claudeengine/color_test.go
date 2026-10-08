// color_test.go pins ColorSequence's exact choreography: `/color <name>` typed and submitted for each palette color, and no inputs for a color outside it.

package claudeengine

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// TestColorSequence_TypesTheColorCommandForEveryPaletteColor covers every palette color plus an unknown one.
// The expected steps carry no Escape, since the command may be typed while a turn runs and Escape there interrupts it.
func TestColorSequence_TypesTheColorCommandForEveryPaletteColor(t *testing.T) {
	t.Parallel()

	type row struct {
		color segmentcolor.Color
		want  []shuttleengine.PaneInput
	}
	var rows []row
	for _, color := range segmentcolor.Palette() {
		rows = append(rows, row{color, []shuttleengine.PaneInput{
			{Text: "/color " + string(color), SettleMS: defaultSubmitSettleMS},
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
