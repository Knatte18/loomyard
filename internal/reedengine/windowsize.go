// windowsize.go owns the live-window-size query and its fallback, the two geometry option pins
// (status off, window-size latest), and the two effective-value readbacks the attach path (batch 2)
// gates the chain on.
// Every tmux interaction here is non-fatal, per the Shared Decision
// geometry-tmux-failures-are-non-fatal-everywhere: a failure is logged via logger.Warn and answered
// with a safe fallback, never returned as an error.

package reedengine

import (
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// parseWindowSize parses a `display-message -p '#{window_width} #{window_height}'` answer into a
// width/height pair.
// It requires exactly two whitespace-separated fields, both parsing as strictly positive integers;
// any other shape — empty, one field, three or more fields, non-numeric, zero, or negative — reports
// ok == false. No I/O.
func parseWindowSize(out string) (w, h int, ok bool) {
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(fields[0])
	h, errH := strconv.Atoi(fields[1])
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// liveBoxLocked queries the live tmux window size for this engine's session and returns it as a
// render.Box anchored at the origin, plus whether that box was a real observation.
// On a round-trip error or a malformed answer, it logs via logger.Warn and falls back to the
// configured e.cfg.Width/e.cfg.Height — exactly today's pre-live-query value — so a degraded query
// never blocks a caller; the second return value is false on both fallback paths and true only when
// the parse succeeded.
// This method never reports failure through its box: a degraded query returns the configured
// cfg.Width/cfg.Height pair, a perfectly plausible-looking box, so a caller comparing boxes across
// calls must be told whether the box was an observation at all.
// Assumes the op lock is already held.
func (e *Engine) liveBoxLocked() (render.Box, bool) {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_width} #{window_height}")
	if err != nil {
		logger.Warn("reed: failed to query live window size, falling back to configured box", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}, false
	}
	w, h, ok := parseWindowSize(out)
	if !ok {
		logger.Warn("reed: malformed live window size answer, falling back to configured box", "socket", e.Socket(), "session", e.SessionName(), "answer", out)
		return render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}, false
	}
	return render.Box{X: 0, Y: 0, W: w, H: h}, true
}

// reservedRowsFromStatus maps a `#{status}` readback to the number of window rows tmux's status line
// consumes: "off" yields 0, "on" yields 1, and a non-negative integer string yields that integer
// verbatim.
// Every other value — including the empty string and a negative integer — reports ok == false.
// Trimmed and lowercased before matching. No I/O.
func reservedRowsFromStatus(raw string) (rows int, ok bool) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "off":
		return 0, true
	case "on":
		return 1, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// windowSizeAllowsChain reports whether a `#{window-size}` readback permits the attach chain: true
// only for the exact value "latest" (trimmed and lowercased), false for every other value including
// "manual", "largest", "smallest" and the empty string. No I/O.
func windowSizeAllowsChain(raw string) bool {
	return strings.ToLower(strings.TrimSpace(raw)) == "latest"
}

// pinGeometryOptionsLocked pins this session's window to "status off" and "window-size latest", and
// owns the window-resized hook's whole install/unset lifecycle.
// Both geometry pins are session/window-targeted rather than -g, because a session- or window-scoped
// value set from the operator's own ~/.tmux.conf silently wins over a global set while set-option
// still exits 0 — verified live. Each call's error is logged via logger.Warn and then ignored; every
// later step, including the hook block, is attempted even when an earlier one failed, per the Shared
// Decision geometry-tmux-failures-are-non-fatal-everywhere.
//
// This function is the right home for the hook lifecycle because it already runs both at boot
// (lifecycle.go) and in the attach pre-flight (attach.go) — which is what lets a session booted by an
// older lyx pick the hook up on the operator's next attach rather than staying unhealed until a manual
// down + up. watchdog: off must reach the hook as well as the loop, because a kill-switch that leaves
// the hook installed keeps spawning run-shell on every resize to write a signal file nobody reads.
// Assumes the op lock is already held.
func (e *Engine) pinGeometryOptionsLocked() {
	target := exactSessionWindowTarget(e.SessionName())
	if err := e.tmux.run("set-option", "-t", target, "status", "off"); err != nil {
		logger.Warn("reed: failed to pin status off", "socket", e.Socket(), "session", e.SessionName(), "option", "status", "err", err)
	}
	if err := e.tmux.run("set-option", "-w", "-t", target, "window-size", "latest"); err != nil {
		logger.Warn("reed: failed to pin window-size latest", "socket", e.Socket(), "session", e.SessionName(), "option", "window-size", "err", err)
	}

	// watchdogOption returns nothing and is all-non-fatal by contract, so an invalid value takes the
	// unset side here rather than propagating; the boot path (ensureServerAndSessionLocked) is where
	// an invalid value is loud.
	enabled, err := watchdogOption(e.cfg.Watchdog)
	if err != nil {
		logger.Warn("reed: invalid watchdog value, treating the watchdog as off", "socket", e.Socket(), "session", e.SessionName(), "watchdog", e.cfg.Watchdog, "err", err)
		enabled = false
	}

	if runtime.GOOS == "windows" {
		// The hook is never installed on Windows — set-hook/run-shell are absent from
		// requiredSubcommands and psmux's support for them is unverified — but the signal file is
		// still removed when the watchdog is off, since nothing else will clean it up.
		if !enabled {
			e.removeResizeSignalFileLocked()
		}
		return
	}

	if enabled {
		// The plain, REPLACING set-hook form is mandatory and -a must never appear: verified live,
		// four identical plain installs yield exactly one fire per resize while three additional -a
		// appends yield four, and this function runs on every AttachArgv pre-flight as well as at
		// boot, so the append form would cost N run-shell spawns per resize after N attaches.
		if err := e.tmux.run("set-hook", "-t", target, windowResizedHookName, resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())); err != nil {
			logger.Warn("reed: failed to install window-resized hook", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
		return
	}

	// set-hook -u is idempotent and exits 0 whether or not a hook was set (verified live).
	if err := e.tmux.run("set-hook", "-u", "-t", target, windowResizedHookName); err != nil {
		logger.Warn("reed: failed to unset window-resized hook", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}
	e.removeResizeSignalFileLocked()
}

// removeResizeSignalFileLocked removes this worktree's resize signal file, if present.
// An absent file is silent (errors.Is(err, fs.ErrNotExist)); any other error is logger.Warn-ed and
// ignored, since this is always called from a non-fatal context.
func (e *Engine) removeResizeSignalFileLocked() {
	if err := os.Remove(e.resizeSignalPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("reed: failed to remove resize signal file", "socket", e.Socket(), "session", e.SessionName(), "path", e.resizeSignalPath(), "err", err)
	}
}

// readStatusRowsLocked reads back this session's effective `#{status}` value and reports the number
// of rows it reserves, per reservedRowsFromStatus.
// A round-trip error is logged via logger.Warn and reported as (0, false).
// Assumes the op lock is already held.
func (e *Engine) readStatusRowsLocked() (rows int, ok bool) {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{status}")
	if err != nil {
		logger.Warn("reed: failed to read back status option", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return 0, false
	}
	return reservedRowsFromStatus(out)
}

// readWindowSizeLatestLocked reads back this session's effective `#{window-size}` value and reports
// whether it permits the attach chain, per windowSizeAllowsChain.
// A round-trip error is logged via logger.Warn and reported as false.
// Assumes the op lock is already held.
func (e *Engine) readWindowSizeLatestLocked() bool {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window-size}")
	if err != nil {
		logger.Warn("reed: failed to read back window-size option", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	return windowSizeAllowsChain(out)
}
