// mouse.go implements the mouse config value validator: mapping the mouse
// config key's raw string to the canonical "on"/"off" tmux option
// value the boot-time set-option call needs. It is a pure planning helper (no
// filesystem or process I/O); the caller (lifecycle.go) performs the actual
// tmux set-option round trip.

package muxengine

import (
	"fmt"
	"strings"
)

// mouseOption validates and normalizes a mouse config value to the exact
// string the tmux "set-option -g mouse" invocation expects: "on" or
// "off". raw is trimmed of surrounding whitespace and lowercased before
// comparison, so a template-sourced value like " ON " resolves the same as
// "on". Every other value — the empty string included — is a misconfiguration
// and is reported as an error rather than silently defaulted; an empty string
// is never treated as "off" here; the template's
// "${env:LYX_MUX_MOUSE:-off}" default is what supplies "off" when the env
// var is unset, so a well-formed config never reaches this helper empty.
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
