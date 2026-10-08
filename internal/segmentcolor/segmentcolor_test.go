// segmentcolor_test.go pins the palette, its tmux mapping, color parsing and the closed segment and palette sets.

package segmentcolor

import (
	"slices"
	"strings"
	"testing"
)

func TestTmuxColor_MapsEveryPaletteColor(t *testing.T) {
	t.Parallel()
	rows := []struct {
		color Color
		want  string
		ok    bool
	}{
		{Blue, "blue", true},
		{Purple, "magenta", true},
		{Cyan, "cyan", true},
		{Green, "green", true},
		{Orange, "colour208", true},
		{Yellow, "yellow", true},
		{Pink, "colour205", true},
		{Red, "red", true},
		{Color("teal"), "", false},
	}
	for _, row := range rows {
		got, ok := TmuxColor(row.color)
		if got != row.want || ok != row.ok {
			t.Errorf("TmuxColor(%q) = (%q, %v), want (%q, %v)", row.color, got, ok, row.want, row.ok)
		}
	}
}

func TestParseColor(t *testing.T) {
	t.Parallel()
	for _, c := range Palette() {
		got, err := ParseColor(string(c))
		if err != nil || got != c {
			t.Errorf("ParseColor(%q) = (%q, %v), want (%q, nil)", c, got, err, c)
		}
	}
	for _, raw := range []string{"teal", "Blue", ""} {
		_, err := ParseColor(raw)
		if err == nil {
			t.Fatalf("ParseColor(%q) succeeded, want an error", raw)
		}
		for _, want := range append([]string{`"` + raw + `"`}, "blue", "purple", "cyan", "green", "orange", "yellow", "pink", "red") {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ParseColor(%q) error %q does not contain %q", raw, err, want)
			}
		}
	}
}

func TestClosedSets(t *testing.T) {
	t.Parallel()
	wantSegments := []Segment{"coordinator", "discussion", "plan", "webster", "review", "describe", "landing"}
	if got := Segments(); !slices.Equal(got, wantSegments) {
		t.Errorf("Segments() = %v, want %v", got, wantSegments)
	}
	wantPalette := []Color{"blue", "purple", "cyan", "green", "orange", "yellow", "pink", "red"}
	if got := Palette(); !slices.Equal(got, wantPalette) {
		t.Errorf("Palette() = %v, want %v", got, wantPalette)
	}
}
