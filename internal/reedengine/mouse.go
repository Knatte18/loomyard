// mouse.go validates the mouse config value and normalizes it to the canonical "on" or "off" tmux option value.
// It is a pure planning helper with no filesystem or process I/O.
// The caller performs the tmux set-option round trip at boot.

package reedengine

import (
	"fmt"
	"strings"
)

// mouseOption validates and normalizes a mouse config value to "on" or "off".
// It trims surrounding space and lowercases the value.
// An empty value is an error; the config template supplies the default.
func mouseOption(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on":
		return "on", nil
	case "off":
		return "off", nil
	default:
		return "", fmt.Errorf("invalid mouse value %q: want \"on\" or \"off\"", raw)
	}
}
