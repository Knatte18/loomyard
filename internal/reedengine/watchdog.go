// watchdog.go owns the resize watchdog's pure surface: the config validator, the loop's fixed
// timings, the signal file's location, and the window-resized hook command string.
// Everything here is I/O-free apart from resizeSignalPath's filepath.Join — no tmux round trip, no
// lock, no goroutine. Those live in batches 2 and 3.

package reedengine

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
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

// The watch loop's fixed, non-configurable timings.
const (
	// watchdogDebounceQuiet is how long the loop waits for resize signals to stop arriving before
	// treating a burst as one settled event.
	watchdogDebounceQuiet = 200 * time.Millisecond
	// watchdogSignalTick is how often the loop polls for the signal file's presence.
	watchdogSignalTick = 100 * time.Millisecond
	// watchdogPollCycle is the poll-mode reconcile cadence, used only where the signal-file
	// mechanism is unavailable.
	watchdogPollCycle = 2 * time.Second
	// watchdogRetryBaseDelay is the base of one debounced event's escalating retry delay
	// (watchdogRetryBaseDelay << (attempt-1)).
	watchdogRetryBaseDelay = 200 * time.Millisecond
	// watchdogMaxAttempts caps one debounced event's retries, never the watcher itself.
	watchdogMaxAttempts = 3
	// watchdogDormantCycle is the cadence a watcher runs at once it is told its worktree root is
	// provably gone. It exists so a session abandoned by `down` costs one log line and a
	// minute-scale poll rather than a warning every two seconds forever.
	watchdogDormantCycle = 60 * time.Second
)

// resizeSignalFileName is the resize signal file's name inside stateDir().
// The file's existence alone is the signal — the watcher consumes it by removing it, so no
// timestamp comparison is ever involved.
const resizeSignalFileName = "reed-resize.signal"

// windowResizedHookName is the tmux hook option name the watchdog installs, unsets, and reads
// back. Declared once so the three call sites cannot drift.
const windowResizedHookName = "window-resized"

// resizeSignalPath returns the resize signal file's path for this engine's worktree.
// stateDir() is the only permitted route to this path under the Durable-vs-Ephemeral State
// Invariant, and one signal file per worktree is what keeps sibling worktrees on the shared
// per-hub tmux server from colliding.
func (e *Engine) resizeSignalPath() string {
	return filepath.Join(e.stateDir(), resizeSignalFileName)
}

// tmuxQuoteValue wraps s in tmux double quotes, backslash-escaping any \, ", or $ in s first.
// tmux parses a hook's value as a tmux command line with its own word splitting, so the shell
// fragment inside it has to survive as one tmux word; $ is escaped because tmux expands it inside
// double quotes.
func tmuxQuoteValue(s string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`$`, `\$`,
	).Replace(s)
	return `"` + escaped + `"`
}

// resizeHookCommand builds the window-resized hook's value: the tmux run-shell invocation that
// touches signalPath via sh.
//
// run-shell MUST carry -b, because without it the tmux SERVER blocks while the command runs
// (live-verified). This exact string round-trips byte-identically through show-options -v, which
// is what makes batch 2's exact-match availability probe viable (also live-verified).
//
// run-shell is executed by the tmux server's own shell rather than the pane shell internal/shell
// otherwise models, so shell.ForGOOS() is the closest available approximation for dialect
// selection, and only the POSIX dialect is ever executed in practice — Windows runs poll-only
// (see batch 2).
func resizeHookCommand(sh shell.Shell, signalPath string) string {
	return "run-shell -b " + tmuxQuoteValue(sh.Touch(signalPath))
}
