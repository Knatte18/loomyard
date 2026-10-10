// windowsize.go owns the live-window-size query and its fallback and the geometry option pins (the two-line status bar, the pane-border title and window-size latest).
// It also owns the effective-value readbacks the attach path gates the chain on and the title-row readback the pins are adjusted by,
// and the whole write side of the `window-resized` hook array: both the resize-pane pins and the watchdog's own resize-signal entry, which are one array and are therefore installed by one function (installResizePinsLocked).
// The array's READ side is reapply.go's hookInstalledLocked.
// Every tmux interaction here is non-fatal, per the Shared Decision geometry-tmux-failures-are-non-fatal-everywhere:
// a failure is logged via logger.Warn and answered with a safe fallback, never returned as an error.

package reedengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"slices"
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
// windowTarget is the strands' window, as resolved by the window seam.
// Assumes the op lock is already held.
func (e *Engine) liveBoxLocked(windowTarget string) (render.Box, bool) {
	out, err := e.tmux.output("display-message", "-p", "-t", windowTarget, "#{window_width} #{window_height}")
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

// resolveShellPath is the absolute path of the configured shell, found through the executable search path.
// An unresolvable shell is an error naming the `shell` key and its value, so the operator can set a shell that exists.
func resolveShellPath(shell string) (string, error) {
	path, err := exec.LookPath(shell)
	if err != nil {
		return "", fmt.Errorf("reed: the configured shell %q (key shell) is not an executable on PATH: %w; set shell in reed.yaml to a shell that exists", shell, err)
	}
	return path, nil
}

// shellOptionArgvs is the two session-scoped pins that make tmux start a pane's shell without the login flag: `default-shell` and an equal non-empty `default-command`.
// They are never global, because one server serves every worktree of the hub.
// An empty shellPath yields no pins.
func shellOptionArgvs(target, shellPath string) [][]string {
	if shellPath == "" {
		return nil
	}
	return [][]string{
		{"set-option", "-t", target, "default-shell", shellPath},
		{"set-option", "-t", target, "default-command", shellPath},
	}
}

// pinGeometryOptionsLocked pins the two-line status bar and the strand pane-border title (bar.go), pins this
// session's window to "window-size latest", and owns the UNSET half of the window-resized hook's
// lifecycle — the install half belongs to installResizePinsLocked at the bottom of this file, which
// rebuilds the whole array (pins plus the watchdog's signal entry) from scratch on every successful
// apply.
//
// It issues, in order: "status-format[0]" and "status-format[1]" with -g, because tmux's status formats are one array and a per-session index would drop the global lines for that session;
// "status-style" with -g, like them, so the gaps between the buttons are dark grey instead of tmux's default green;
// "status" "2" and "status-position" "bottom" on the session;
// and, window-targeted with -w on the strands' window only, "pane-border-status" "top" and "pane-border-format", so a batten window gets no border title.
// The reserved status rows are read back by readStatusRowsLocked, so the layout box shrinks by two rows with no planner change.
// After the option pins it issues the navigation bindings (bindings.go), so a server booted by an older lyx gets them at the next attach.
//
// Each call's error is logged via logger.Warn and then ignored.
// Every later step, including the hook block, is attempted even when an earlier one failed, per the Shared Decision geometry-tmux-failures-are-non-fatal-everywhere.
// No runtime.GOOS == "windows" branch guards any of the set-option calls: per the Shared
// Decision windows-status-line-is-an-unbranched-accepted-degrade they are attempted on every platform,
// and psmux may refuse some or all of them.
//
// This function is the right home for the unset because it already runs both at boot (lifecycle.go)
// and in the attach pre-flight (attach.go), so an operator who flips watchdog: off gets the hook torn
// down on the very next attach rather than at a manual down + up. The install half rides the same two
// paths for the mirror-image reason: the attach pre-flight calls installResizePinsLocked a few
// statements later, which is what lets a session booted by an older lyx pick the hook up on the
// operator's next attach.
// watchdog: off must reach the hook as well as the loop, because a kill-switch that leaves the hook
// installed keeps spawning run-shell on every resize to write a signal file nobody reads — and it
// clears the pins alongside it here on purpose, since the very next installResizePinsLocked rebuilds
// them.
// target is the strands' window, as resolved by the window seam;
// the session-wide options ride the window's session.
// After the option pins it issues shellOptionArgvs for shellPath, each non-fatal like them; the empty string skips both.
// Assumes the op lock is already held.
func (e *Engine) pinGeometryOptionsLocked(target, shellPath string) {
	pins := []struct {
		option string
		argv   []string
	}{
		{"status-format[0]", []string{"set-option", "-g", "status-format[0]", statusFormatButtons}},
		{"status-format[1]", []string{"set-option", "-g", "status-format[1]", statusFormatSessions}},
		{"status-style", []string{"set-option", "-g", "status-style", statusBarStyle}},
		{"status", []string{"set-option", "-t", target, "status", "2"}},
		{"status-position", []string{"set-option", "-t", target, "status-position", "bottom"}},
		{"pane-border-status", []string{"set-option", "-w", "-t", target, "pane-border-status", "top"}},
		{"pane-border-format", []string{"set-option", "-w", "-t", target, "pane-border-format", paneBorderFormat}},
	}
	for _, pin := range pins {
		if err := e.tmux.run(pin.argv...); err != nil {
			logger.Warn("reed: failed to pin a status bar option", "socket", e.Socket(), "session", e.SessionName(), "option", pin.option, "err", err)
		}
	}

	if err := e.tmux.run("set-option", "-w", "-t", target, "window-size", "latest"); err != nil {
		logger.Warn("reed: failed to pin window-size latest", "socket", e.Socket(), "session", e.SessionName(), "option", "window-size", "err", err)
	}

	for _, argv := range shellOptionArgvs(target, shellPath) {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to pin a session shell option", "socket", e.Socket(), "session", e.SessionName(), "option", argv[3], "err", err)
		}
	}

	e.pinBindingsLocked()

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

	if !enabled {
		// set-hook -u is idempotent and exits 0 whether or not a hook was set (verified live).
		// This clears both old-style watchdog hooks (from prior sessions) and new-style resize-pin
		// hooks (installed by the current code path), ensuring a fresh state for either mechanism.
		if err := e.tmux.run("set-hook", "-u", "-t", target, windowResizedHookName); err != nil {
			logger.Warn("reed: failed to unset window-resized hook", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
		e.removeResizeSignalFileLocked()
	}
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
func (e *Engine) readStatusRowsLocked(windowTarget string) (rows int, ok bool) {
	out, err := e.tmux.output("display-message", "-p", "-t", windowTarget, "#{status}")
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
func (e *Engine) readWindowSizeLatestLocked(windowTarget string) bool {
	out, err := e.tmux.output("display-message", "-p", "-t", windowTarget, "#{window-size}")
	if err != nil {
		logger.Warn("reed: failed to read back window-size option", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	return windowSizeAllowsChain(out)
}

// readBorderTitleRowLocked reports whether the strands' window draws a pane-border title row at window row 0.
// Only an answer of exactly `top` means a title row; `off`, `bottom`, an empty answer and a failed readback (logged) all mean none.
// Assumes the op lock is already held.
func (e *Engine) readBorderTitleRowLocked(windowTarget string) bool {
	out, err := e.tmux.output("display-message", "-p", "-t", windowTarget, "#{pane-border-status}")
	if err != nil {
		logger.Warn("reed: failed to read back pane-border-status, assuming no title row", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	return strings.TrimSpace(out) == "top"
}

// pinsForContent adjusts pins, which carry planned cell heights, to the content heights `resize-pane -y` sizes.
// A pane's content equals its cell everywhere except at window row 0, where a title row takes the top row of the cell:
// with titleRow, the pin on rowZeroPaneID is its planned cell height minus one, floored at one content row, and every other pin keeps its cell height.
// The floor is the one case where a cell ends one row taller than planned, the row coming from the cells below it.
// Without a title row the pins pass through unchanged; a row-0 pane with no pin gets none.
// The input is not modified.
func pinsForContent(pins []render.Pin, rowZeroPaneID string, titleRow bool) []render.Pin {
	adjusted := slices.Clone(pins)
	if !titleRow {
		return adjusted
	}
	for i, pin := range adjusted {
		if pin.PaneID == rowZeroPaneID {
			adjusted[i].Height = max(pin.Height-1, 1)
		}
	}
	return adjusted
}

// contentPinsLocked returns the fixed-height pins of st against live within box, adjusted for the strands' window's title row.
// The row-0 pane is the first of the physical pane order the layout was built from, never a pin's emission order.
// Assumes the op lock is already held.
func (e *Engine) contentPinsLocked(windowTarget string, st *ReedState, live []LivePane, box render.Box) []render.Pin {
	var rowZeroPaneID string
	if order := paneIDsByTop(live); len(order) > 0 {
		rowZeroPaneID = order[0]
	}
	return pinsForContent(e.fixedHeightPins(st, live, box), rowZeroPaneID, e.readBorderTitleRowLocked(windowTarget))
}

// runResizePinsLocked issues one `resize-pane -t <pane> -y <height>` per pin, so a layout just applied holds its heights before any resize fires the hook.
// Each failure is logged via logger.Warn and ignored, like the hook install; the hook's signal entry is never run from here.
// Assumes the op lock is already held.
func (e *Engine) runResizePinsLocked(pins []render.Pin) {
	for _, pin := range pins {
		if err := e.tmux.run("resize-pane", "-t", pin.PaneID, "-y", strconv.Itoa(pin.Height)); err != nil {
			logger.Warn("reed: failed to pin a pane height after the layout apply", "socket", e.Socket(), "session", e.SessionName(), "pane", pin.PaneID, "height", pin.Height, "err", err)
		}
	}
}

// resizePinHookArgvs returns the full argv sequence rebuilding the strands' window's `window-resized` window-hook
// array from pins and signalCommand. It performs no I/O and no logging.
//
// The first returned argv is always the clear, {"set-hook", "-u", "-w", "-t", windowTarget, "window-resized"}, emitted even when there is nothing at all to install behind it, per the Shared Decision the-clear-is-unconditional-including-zero-pins.
// Then one argv per array entry, in entry order:
// {"set-hook", "-w", "-t", windowTarget, "window-resized", body} for the entry that establishes the array at index 0 (a plain set-hook replaces),
// and {"set-hook", "-a", "-w", "-t", windowTarget, "window-resized", body} for every entry after it (-a appends).
//
// The entries are the pins, in pins order, each with the body "resize-pane -t <pane> -y <height>".
// When there is any pin, the zoom record entry comes first and the zoom restore entry follows the last pin (zoom.go),
// so a zoomed strand is unzoomed while the pins run and zoomed again behind them.
// Then — when signalCommand is non-empty — signalCommand itself, verbatim, is the array's LAST
// entry. Ordering the signal entry last is what makes the watcher's re-apply plan against a window
// tmux has already finished fixing up: the signal says "this resize is handled as far as the server
// itself can handle it", so the watcher's own corrective apply starts from the pinned state rather
// than racing the pins. Putting it last is safe because array entries fire independently (see the
// failure-isolation note below), so a pin naming a destroyed pane cannot swallow the touch behind it —
// live-verified on tmux 3.6 with a deliberately bogus "resize-pane -t %99" at index 0.
//
// signalCommand is emitted even when pins is empty, in which case it becomes the array's sole entry
// and is installed by the plain (non-"-a") set-hook. The touch is not a property of there being
// something to pin: it is how the watcher learns a resize happened at all, and gating it on a
// non-empty pin set would silently pin such a session's watcher into poll mode forever. An empty
// signalCommand emits no entry, which is how the caller says "watchdog off, or a platform where the
// hook is never installed" — see resizeSignalHookCommand.
//
// The body is one whole argv element; this function never emits a bare ";" element, because
// set-hook takes its body as a single argument and a separate ";" element would terminate the
// set-hook command itself. The array encoding — rather than one ";"-separated command string — exists
// for failure isolation: verified live on tmux 3.6, a resize-pane naming a destroyed pane aborts the
// rest of a single command list, while array entries are independent. The Selvage pin is always pin
// index 0 so it fires before any strip pin can go wrong.
func resizePinHookArgvs(target string, pins []render.Pin, signalCommand string) [][]string {
	argvs := make([][]string, 0, len(pins)+4)
	argvs = append(argvs, []string{"set-hook", "-u", "-w", "-t", target, "window-resized"})
	appendEntry := func(body string) {
		// len(argvs) == 1 means only the clear has been emitted so far, so this entry is the one that
		// establishes the array at index 0 and must use the replacing form.
		if len(argvs) == 1 {
			argvs = append(argvs, []string{"set-hook", "-w", "-t", target, "window-resized", body})
			return
		}
		argvs = append(argvs, []string{"set-hook", "-a", "-w", "-t", target, "window-resized", body})
	}
	if len(pins) > 0 {
		appendEntry(zoomRecordHookBody)
	}
	for _, pin := range pins {
		appendEntry(fmt.Sprintf("resize-pane -t %s -y %d", pin.PaneID, pin.Height))
	}
	if len(pins) > 0 {
		appendEntry(zoomRestoreHookBody)
	}
	if signalCommand != "" {
		appendEntry(signalCommand)
	}
	return argvs
}

// resizeSignalHookCommand returns the run-shell body the window-resized array must end with so that a
// live resize touches this worktree's resize signal file, or "" when no such entry belongs in the
// array at all. It performs no tmux I/O.
//
// "" is returned on exactly the two paths pinGeometryOptionsLocked's own hook block already treats as
// "no hook here": Windows, where the hook is never installed because set-hook/run-shell are absent
// from requiredSubcommands and psmux's support for them is unverified (hookInstalledLocked answers
// (false, false) there unconditionally for the same reason), and watchdog: off, where an installed
// touch entry would spawn a run-shell per resize to write a file nobody reads.
// An invalid watchdog value takes the off side here rather than propagating, mirroring
// pinGeometryOptionsLocked: this function has no error channel, and the boot path
// (ensureServerAndSessionLocked) is where an invalid value is loud.
func (e *Engine) resizeSignalHookCommand() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	enabled, err := watchdogOption(e.cfg.Watchdog)
	if err != nil {
		logger.Warn("reed: invalid watchdog value, installing no resize-signal hook entry", "socket", e.Socket(), "session", e.SessionName(), "watchdog", e.cfg.Watchdog, "err", err)
		return ""
	}
	if !enabled {
		return ""
	}
	return resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
}

// installResizePinsLocked rebuilds this session's `window-resized` window-hook array from pins and
// this worktree's resize-signal entry, issuing each argv resizePinHookArgvs builds through
// e.tmux.run. It returns nothing.
//
// This is the ONLY function that writes the watchdog's signal entry, and it is one deliberately, because
// the array is a whole-snapshot rebuild: any second writer would have to either clear the pins this
// one just installed or accumulate a duplicate touch per attach.
// The boot installs the signal entry for every session, as a zero-pin rebuild,
// so a session the apply guards skip (fewer than two panes, or no strand owning a present pane) still has it;
// a later apply or successful attach rebuilds the array with its pins.
//
// This follows the Shared Decision hook-failure-is-non-fatal-everywhere, which already governs
// pinGeometryOptionsLocked in this same file: each failure is logged via logger.Warn naming the
// socket, the session and the error, and then ignored, so a failed call never stops the calls after
// it — a failed clear still lets the rebuild proceed, since the first (non-"-a") set-hook overwrites
// the array from entry [0] regardless.
//
// The clear is unconditional because reaching a call site means reed has computed an opinion, and
// with zero pins that opinion is "nothing is pinned" (Shared Decision
// the-clear-is-unconditional-including-zero-pins) — which is a statement about the PINS alone: a
// zero-pin rebuild with the watchdog on still installs the signal entry, since "nothing is pinned"
// and "nobody wants to hear about a resize" are different opinions. The whole array is a snapshot
// rebuilt on every successful apply rather than something recomputed at fire time.
//
// Known limitation: a clamp-derived pin is computed for the box at install time, so an operator who
// shrinks the terminal past a clamp threshold with no intervening reed op keeps a pre-shrink pin,
// bounded by tmux's own one-row floor and self-correcting on the next reed op.
//
// No forced-repaint entry ships in this array. A repaint mechanism was measured against the
// dot-fill render artifact and not shipped — see the Measurement record (repaint candidates) block
// in internal/reedengine/doc.go's package doc comment for which candidates were tried and which
// criterion each failed. With no repaint entry installed, the artifact's remaining duration on a
// live resize is the latency of the watchdog's own round trip (signal entry, watch loop wake, and
// re-apply), when the watchdog is on; with the watchdog off, the artifact stands until the next
// reed operation.
//
// Assumes the op lock is already held, like every other Locked method in this file.
func (e *Engine) installResizePinsLocked(windowTarget string, pins []render.Pin) {
	for _, argv := range resizePinHookArgvs(windowTarget, pins, e.resizeSignalHookCommand()) {
		if err := e.tmux.run(argv...); err != nil {
			// One message for every argv in the rebuild — the clear, each resize-pane pin, and the
			// resize-signal entry — since they are one array's install and each of them is equally
			// non-fatal; the argv itself is what says which one failed.
			logger.Warn("reed: failed to install window-resized hook entry", "socket", e.Socket(), "session", e.SessionName(), "argv", argv, "err", err)
		}
	}
}
