// color.go defines the window colour palette, the pickers for a beam's colour and a worktree's activity-bar colour,
// and the text colour a background takes by its luminance.
// Green is reserved for the prime.

package vscode

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ErrUnsupported is returned when vscode launch is attempted on an unsupported platform.
var ErrUnsupported = errors.New("vscode launch unsupported on this platform")

// PrimeColor is the green the prime takes on its title bar and activity bar, and every task under the prime on its title bar.
const PrimeColor = "#1b5e20"

// palette lists the window colours in pick order, the prime's green first.
var palette = []string{
	PrimeColor,
	"#7d2d6b",
	"#2d4f7d",
	"#7d5c2d",
	"#6b2d2d",
	"#2d6b6b",
	"#4a2d7d",
	"#7d462d",
}

// PickBeamColor returns the first palette colour other than green that no colour in held names, compared case-insensitively.
// It is the colour a new beam records, held being the colours the other open beams hold.
// When every non-green colour is held it returns the first non-green one.
func PickBeamColor(held []string) string {
	return firstFree(PrimeColor, "", held)
}

// PickWorktreeColor returns a worktree's activity-bar colour under a parent whose project colour is projectColor:
// current when it is a palette colour other than projectColor that no colour in siblings names, else the first such palette colour, else, with every one taken, the first palette colour other than projectColor.
// Colours compare case-insensitively.
func PickWorktreeColor(projectColor, current string, siblings []string) string {
	return firstFree(projectColor, current, siblings)
}

// firstFree returns preferred when it is a palette colour other than excluded and absent from taken, else the first palette colour that is neither, else the first palette colour other than excluded.
func firstFree(excluded, preferred string, taken []string) string {
	free := func(color string) bool {
		return !strings.EqualFold(color, excluded) && !slices.ContainsFunc(taken, func(t string) bool { return strings.EqualFold(t, color) })
	}
	if slices.Contains(palette, strings.ToLower(preferred)) && free(strings.ToLower(preferred)) {
		return strings.ToLower(preferred)
	}
	fallback := ""
	for _, color := range palette {
		if strings.EqualFold(color, excluded) {
			continue
		}
		if fallback == "" {
			fallback = color
		}
		if free(color) {
			return color
		}
	}
	return fallback
}

// SiblingWorktreeColors returns the activity-bar colours of the hub's worktrees, other than the one named self, whose title bar holds projectColor, read from each worktree's .vscode/settings.json at l's anchor.
// They are the colours a new worktree under the same parent avoids.
// A worktree with no readable or decodable settings, or no activity-bar colour, adds nothing, and an unreadable hub yields nil.
func SiblingWorktreeColors(l *lymcwd.Location, projectColor, self string) []string {
	entries, err := os.ReadDir(l.HubPath)
	if err != nil {
		return nil
	}
	var colors []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == self {
			continue
		}
		title, activity := barColors(filepath.Join(l.HubPath, entry.Name(), l.AnchorRel))
		if activity != "" && strings.EqualFold(title, projectColor) {
			colors = append(colors, strings.ToLower(activity))
		}
	}
	return colors
}

// luminanceThreshold is the relative luminance at which black and white text give a background equal contrast.
const luminanceThreshold = 0.179

// Foreground returns the text colour for background, a #rrggbb colour: black when its WCAG relative luminance is above the point where black and white give equal contrast, white otherwise.
// A value that is not #rrggbb reads as dark and takes white.
func Foreground(background string) string {
	if relativeLuminance(background) > luminanceThreshold {
		return "#000000"
	}
	return "#ffffff"
}

// relativeLuminance returns the WCAG relative luminance of a #rrggbb colour, 0 for any other value.
func relativeLuminance(color string) float64 {
	hex, ok := strings.CutPrefix(color, "#")
	if !ok || len(hex) != 6 {
		return 0
	}
	value, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0
	}
	linear := func(channel uint64) float64 {
		c := float64(channel) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(value>>16&0xff) + 0.7152*linear(value>>8&0xff) + 0.0722*linear(value&0xff)
}

// ActivityBarColor returns the activity-bar colour <dir>/.vscode/settings.json holds, lower-cased, empty when it holds none or cannot be read or decoded.
func ActivityBarColor(dir string) string {
	_, activity := barColors(dir)
	return strings.ToLower(activity)
}

// barColors returns the title-bar and activity-bar background colours <dir>/.vscode/settings.json holds, each empty when it holds none or cannot be read or decoded.
func barColors(dir string) (title, activity string) {
	content, err := os.ReadFile(filepath.Join(dir, ".vscode", "settings.json"))
	if err != nil {
		return "", ""
	}
	var settings map[string]any
	if err := json.Unmarshal(stripJSONC(content), &settings); err != nil {
		return "", ""
	}
	customizations, _ := settings[colorCustomizationsKey].(map[string]any)
	title, _ = customizations[titleBarActiveBackground].(string)
	activity, _ = customizations[activityBarBackground].(string)
	return title, activity
}
