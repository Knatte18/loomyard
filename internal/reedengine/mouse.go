// mouse.go implements the mouse config value validator: mapping the mouse config key's raw string to the canonical "on"/"off" tmux option value the boot-time set-option call needs.
// It is a pure planning helper (no filesystem or process I/O);
// the caller (lifecycle.go) performs the actual tmux set-option round trip.

package reedengine

import (
	"fmt"
	"strings"
)

// mouseOption validates and normalizes a mouse config value to "on" or "off".
// Trimmed and lowercased; empty string errors (template handles the default).
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
