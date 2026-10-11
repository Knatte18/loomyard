// color.go maps lyx's palette to Claude Code's /color names and builds the key choreography that sets a session's prompt-bar color.

package claudeengine

import (
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// colorNames maps each palette color to the name Claude Code's /color command takes.
// This is the one place that mapping lives.
var colorNames = map[segmentcolor.Color]string{
	segmentcolor.Blue:   "blue",
	segmentcolor.Purple: "purple",
	segmentcolor.Cyan:   "cyan",
	segmentcolor.Green:  "green",
	segmentcolor.Orange: "orange",
	segmentcolor.Yellow: "yellow",
	segmentcolor.Pink:   "pink",
	segmentcolor.Red:    "red",
}

// ColorSequence returns the key choreography that sets a live claude session's prompt-bar color: the `/color <name>` slash command.
// A color outside the palette answers no inputs.
// It sends no leading Escape and no pause between the text and the Enter: the paste path submits them in one tmux invocation.
func (c *Claude) ColorSequence(color segmentcolor.Color) []shuttleengine.PaneInput {
	name, ok := colorNames[color]
	if !ok {
		return nil
	}
	return []shuttleengine.PaneInput{
		{Text: "/color " + name},
		{Key: "Enter"},
	}
}
