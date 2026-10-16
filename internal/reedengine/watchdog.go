// watchdog.go owns the watchdog's pure surface: the config validator, the discover signal file's name, the daemons' wake cadence and the window-resized hook's name.
// Everything here is I/O-free — no tmux round trip, no lock, no goroutine.

package reedengine

import (
	"fmt"
	"strings"
	"time"
)

// watchdogOption validates and normalizes a watchdog config value to a boolean enable.
// It follows mouseOption's behaviour exactly: trimmed and lowercased, "on" yields true, "off"
// yields false, and every other value including the empty string errors naming the offending
// value.
// Unlike mouseOption, this returns bool rather than the normalized string: the watchdog value is
// never handed to tmux (there is no "set-option watchdog"), and every consumer wants a plain
// enable/disable decision rather than a tmux-ready string.
func watchdogOption(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid watchdog value %q: want \"on\" or \"off\"", raw)
	}
}

// DiscoverSignalFileName is the discover signal file's name inside the hub scratch directory.
// A cold session boot touches it, and the watchdog daemon's discovery loop wakes on it, so the teller and the daemon spell it once.
const DiscoverSignalFileName = "reed-discover.signal"

// NextWakeCadence is the backoff rule the daemon loops share: the wait after a cycle.
// It returns base when the cycle changed something, otherwise twice current capped at ceiling.
func NextWakeCadence(current, base, ceiling time.Duration, changed bool) time.Duration {
	if changed {
		return base
	}
	return min(2*current, ceiling)
}

// windowResizedHookName is the tmux hook option name boot and attach unpin from the shell window.
const windowResizedHookName = "window-resized"
