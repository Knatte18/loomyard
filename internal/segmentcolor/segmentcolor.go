// segmentcolor.go declares the loom segments, the color palette and the palette's tmux mapping.

package segmentcolor

import (
	"fmt"
	"strings"
)

// Segment is the loom phase a spawned strand belongs to.
type Segment string

const (
	Coordinator Segment = "coordinator"
	Discussion  Segment = "discussion"
	Plan        Segment = "plan"
	Webster     Segment = "webster"
	Review      Segment = "review"
	Describe    Segment = "describe"
	Landing     Segment = "landing"
	Darn        Segment = "darn"
)

// Segments returns every segment in declaration order.
func Segments() []Segment {
	return []Segment{Coordinator, Discussion, Plan, Webster, Review, Describe, Landing, Darn}
}

// Color is one entry of lyx's palette.
type Color string

const (
	Blue   Color = "blue"
	Purple Color = "purple"
	Cyan   Color = "cyan"
	Green  Color = "green"
	Orange Color = "orange"
	Yellow Color = "yellow"
	Pink   Color = "pink"
	Red    Color = "red"
)

// Palette returns every palette color in declaration order.
func Palette() []Color {
	return []Color{Blue, Purple, Cyan, Green, Orange, Yellow, Pink, Red}
}

var tmuxColors = map[Color]string{
	Blue:   "blue",
	Purple: "magenta",
	Cyan:   "cyan",
	Green:  "green",
	Orange: "colour208",
	Yellow: "yellow",
	Pink:   "colour205",
	Red:    "red",
}

// TmuxColor returns the tmux color name for a palette color, and false for a color outside the palette.
func TmuxColor(c Color) (string, bool) {
	name, ok := tmuxColors[c]
	return name, ok
}

// ParseColor returns the palette color spelled exactly as raw.
// The error names the value and lists the palette.
func ParseColor(raw string) (Color, error) {
	for _, c := range Palette() {
		if string(c) == raw {
			return c, nil
		}
	}
	names := make([]string, 0, len(Palette()))
	for _, c := range Palette() {
		names = append(names, string(c))
	}
	return "", fmt.Errorf("color %q is not in the palette; use one of: %s", raw, strings.Join(names, ", "))
}
