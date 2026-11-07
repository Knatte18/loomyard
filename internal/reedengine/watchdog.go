// watchdog.go holds the watchdog's pure helpers.
// They cover the config validator, the discover signal file's name, and the daemons' wake cadence.
// They also cover the window-resized hook's name.
// None of them does I/O: no tmux round trip, no lock, no goroutine.

package reedengine

import (
	"fmt"
	"strings"
	"time"
)

// watchdogOption validates a watchdog config value and normalizes it to a boolean enable.
// The value is trimmed and lowercased.
// "on" yields true and "off" yields false.
// Any other value, including the empty string, is an error naming the offending value.
// It returns a bool because the value is never handed to tmux,
// and every consumer wants a plain enable or disable decision.
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

// DiscoverSignalFileName is the name of the discover signal file inside the hub scratch directory.
// A cold session boot touches the file, and the engine writes the stop marker into it when it shuts the server down.
// The watchdog daemon's discovery loop wakes on it.
const DiscoverSignalFileName = "reed-discover.signal"

// DiscoverStopMarker is the fixed content of the discover signal file that asks the watchdog daemon to stop.
// The writer and the daemon share it so the spelling lives in one place.
const DiscoverStopMarker = "stop"

// NextWakeCadence is the backoff rule the daemon loops share for the wait after a cycle.
// It returns base when the cycle changed something; otherwise it returns twice current, capped at ceiling.
func NextWakeCadence(current, base, ceiling time.Duration, changed bool) time.Duration {
	if changed {
		return base
	}
	return min(2*current, ceiling)
}

// windowResizedHookName is the name of the tmux window-resized hook option.
// The window-size code removes that hook to unpin the shell window.
const windowResizedHookName = "window-resized"
