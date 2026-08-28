// windowsize.go owns the live-window-size query and its fallback, the two geometry option pins
// (status off, window-size latest), and the two effective-value readbacks the attach path (batch 2)
// gates the chain on.
// Every tmux interaction here is non-fatal, per the Shared Decision
// geometry-tmux-failures-are-non-fatal-everywhere: a failure is logged via logger.Warn and answered
// with a safe fallback, never returned as an error.

package reedengine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
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
// render.Box anchored at the origin.
// On a round-trip error or a malformed answer, it logs via logger.Warn and falls back to the
// configured e.cfg.Width/e.cfg.Height — exactly today's pre-live-query value — so a degraded query
// never blocks a caller.
// Assumes the op lock is already held.
func (e *Engine) liveBoxLocked() render.Box {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_width} #{window_height}")
	if err != nil {
		logger.Warn("reed: failed to query live window size, falling back to configured box", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}
	}
	w, h, ok := parseWindowSize(out)
	if !ok {
		logger.Warn("reed: malformed live window size answer, falling back to configured box", "socket", e.Socket(), "session", e.SessionName(), "answer", out)
		return render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}
	}
	return render.Box{X: 0, Y: 0, W: w, H: h}
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

// pinGeometryOptionsLocked pins this session's window to "status off" and "window-size latest".
// Both pins are session/window-targeted rather than -g, because a session- or window-scoped value set
// from the operator's own ~/.tmux.conf silently wins over a global set while set-option still exits
// 0 — verified live. Each call's error is logged via logger.Warn and then ignored; the second pin is
// attempted even when the first failed, per the Shared Decision
// geometry-tmux-failures-are-non-fatal-everywhere.
// Assumes the op lock is already held.
func (e *Engine) pinGeometryOptionsLocked() {
	target := exactSessionWindowTarget(e.SessionName())
	if err := e.tmux.run("set-option", "-t", target, "status", "off"); err != nil {
		logger.Warn("reed: failed to pin status off", "socket", e.Socket(), "session", e.SessionName(), "option", "status", "err", err)
	}
	if err := e.tmux.run("set-option", "-w", "-t", target, "window-size", "latest"); err != nil {
		logger.Warn("reed: failed to pin window-size latest", "socket", e.Socket(), "session", e.SessionName(), "option", "window-size", "err", err)
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

// resizePinHookArgvs returns the full argv sequence rebuilding session's `window-resized` window-hook
// array for pins. It performs no I/O and no logging.
//
// The first returned argv is always the clear — {"set-hook", "-u", "-w", "-t",
// exactSessionWindowTarget(session), "window-resized"} — emitted even when pins is empty, per the
// Shared Decision the-clear-is-unconditional-including-zero-pins. Then one argv per pin, in pins
// order: {"set-hook", "-w", "-t", exactSessionWindowTarget(session), "window-resized", body} for the
// first pin and {"set-hook", "-a", "-w", "-t", exactSessionWindowTarget(session), "window-resized",
// body} for every subsequent pin, where body is the single string "resize-pane -t <pane> -y
// <height>".
//
// The body is one whole argv element; this function never emits a bare ";" element, because
// set-hook takes its body as a single argument and a separate ";" element would terminate the
// set-hook command itself. The array encoding — rather than one ";"-separated command string — exists
// for failure isolation: verified live on tmux 3.6, a resize-pane naming a destroyed pane aborts the
// rest of a single command list, while array entries are independent. The header is always pin index
// 0 so it fires before any strip pin can go wrong.
func resizePinHookArgvs(session string, pins []render.Pin) [][]string {
	target := exactSessionWindowTarget(session)
	argvs := make([][]string, 0, len(pins)+1)
	argvs = append(argvs, []string{"set-hook", "-u", "-w", "-t", target, "window-resized"})
	for i, pin := range pins {
		body := fmt.Sprintf("resize-pane -t %s -y %d", pin.PaneID, pin.Height)
		if i == 0 {
			argvs = append(argvs, []string{"set-hook", "-w", "-t", target, "window-resized", body})
		} else {
			argvs = append(argvs, []string{"set-hook", "-a", "-w", "-t", target, "window-resized", body})
		}
	}
	return argvs
}

// installResizePinsLocked rebuilds this session's `window-resized` window-hook array from pins,
// issuing each argv resizePinHookArgvs builds through e.tmux.run. It returns nothing.
//
// This follows the Shared Decision hook-failure-is-non-fatal-everywhere, which already governs
// pinGeometryOptionsLocked in this same file: each failure is logged via logger.Warn naming the
// socket, the session and the error, and then ignored, so a failed call never stops the calls after
// it — a failed clear still lets the rebuild proceed, since the first (non-"-a") set-hook overwrites
// the array from entry [0] regardless.
//
// The clear is unconditional because reaching a call site means reed has computed an opinion, and
// with zero pins that opinion is "nothing is pinned" (Shared Decision
// the-clear-is-unconditional-including-zero-pins). The whole array is a snapshot rebuilt on every
// successful apply rather than something recomputed at fire time.
//
// Known limitation: a clamp-derived pin is computed for the box at install time, so an operator who
// shrinks the terminal past a clamp threshold with no intervening reed op keeps a pre-shrink pin,
// bounded by tmux's own one-row floor and self-correcting on the next reed op.
//
// Assumes the op lock is already held, like every other Locked method in this file.
func (e *Engine) installResizePinsLocked(pins []render.Pin) {
	for _, argv := range resizePinHookArgvs(e.SessionName(), pins) {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to install resize-pane hook", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
	}
}
