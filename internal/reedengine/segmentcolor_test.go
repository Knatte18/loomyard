// segmentcolor_test.go pins segmentColor's resolution of a segment to a palette color.

package reedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

func TestSegmentColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		colors  map[string]string
		segment string
		want    segmentcolor.Color
		wantOK  bool
	}{
		{name: "Configured", colors: map[string]string{"review": "orange"}, segment: "review", want: segmentcolor.Orange, wantOK: true},
		{name: "EmptySegment", colors: map[string]string{"review": "orange"}, segment: ""},
		{name: "NoKey", colors: map[string]string{"review": "orange"}, segment: "plan"},
		{name: "NonPaletteValue", colors: map[string]string{"review": "crimson"}, segment: "review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := &Engine{cfg: Config{SegmentColors: tt.colors}}
			got, ok := e.segmentColor(tt.segment)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("segmentColor(%q) = (%q, %v), want (%q, %v)", tt.segment, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
