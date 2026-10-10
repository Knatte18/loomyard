// zoom.go keeps a zoomed strand zoomed across reed's own pane-changing steps.
// tmux clears a window's zoom on resize-pane -y, select-layout, split-window, select-pane without -Z and kill-pane of another pane,
// so every step that changes panes runs inside keepZoomLocked, and the two executions tmux runs after reed returns (the resize hook and the attach chain) carry the same bracket as tmux commands.

package reedengine

import (
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// zoomStateFormat is the display-message format that reads the strands' window's zoom flag and its active pane, which is the zoomed one.
const zoomStateFormat = "#{window_zoomed_flag} #{pane_id}"

// zoomRecordFlagOption is the window option that carries "this window was zoomed" from the run-time unzoom to the run-time re-zoom.
const zoomRecordFlagOption = "@lyx_rezoom"

// parseZoomState parses a zoomStateFormat answer into the zoomed pane's id.
// Only a flag of exactly "1" followed by a pane id means zoomed; every other answer, an unparseable one included, means not zoomed.
func parseZoomState(out string) (paneID string, zoomed bool) {
	fields := strings.Fields(out)
	if len(fields) != 2 || fields[0] != "1" {
		return "", false
	}
	return fields[1], true
}

// keepZoomLocked runs fn with the strands' window unzoomed and re-zooms it afterwards.
// A window that is not zoomed when the step starts, a missing session, a failed read and an unparseable answer all mean "not zoomed": fn runs plain.
// For a zoomed window the zoomed pane is unzoomed, fn runs, and the same pane is zoomed again, also when fn returned an error,
// but only when that pane is still in the window's listing; a pane fn removed leaves the window in the overview.
// A failed unzoom leaves the window as it was, so fn runs and nothing is re-zoomed.
// Every tmux failure here is non-fatal and logged; fn's error is returned unchanged.
// Assumes the op lock is already held.
func (e *Engine) keepZoomLocked(fn func() error) error {
	window, err := e.storedStrandWindowTarget()
	if err != nil {
		logger.Debug("reed: could not resolve the strands' window to read its zoom, running the step plain", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return fn()
	}
	out, err := e.tmux.output("display-message", "-p", "-t", window, zoomStateFormat)
	if err != nil {
		logger.Debug("reed: could not read the strands' window zoom, running the step plain", "socket", e.Socket(), "session", e.SessionName(), "window", window, "err", err)
		return fn()
	}
	paneID, zoomed := parseZoomState(out)
	if !zoomed {
		return fn()
	}

	if err := e.tmux.run("resize-pane", "-Z", "-t", paneID); err != nil {
		logger.Warn("reed: failed to unzoom the strands' window before a step, running it without a re-zoom", "socket", e.Socket(), "session", e.SessionName(), "window", window, "pane", paneID, "err", err)
		return fn()
	}
	stepErr := fn()

	panes, err := e.tmux.listPanes(window)
	if err != nil {
		logger.Warn("reed: failed to list the strands' window after a step, leaving it unzoomed", "socket", e.Socket(), "session", e.SessionName(), "window", window, "pane", paneID, "err", err)
		return stepErr
	}
	if !slices.ContainsFunc(panes, func(pane LivePane) bool { return pane.ID == paneID }) {
		logger.Debug("reed: the zoomed pane is gone after a step, leaving the window in the overview", "socket", e.Socket(), "session", e.SessionName(), "window", window, "pane", paneID)
		return stepErr
	}
	if err := e.tmux.run("resize-pane", "-Z", "-t", paneID); err != nil {
		logger.Warn("reed: failed to re-zoom the strands' window after a step", "socket", e.Socket(), "session", e.SessionName(), "window", window, "pane", paneID, "err", err)
	}
	return stepErr
}

// withOpLockKeepingZoom is withOpLock for a step that changes the strands' window's panes: fn runs inside keepZoomLocked, under the op lock.
func (e *Engine) withOpLockKeepingZoom(fn func() error) error {
	return e.withOpLock(func() error { return e.keepZoomLocked(fn) })
}

// withBootOpLockKeepingZoom is withOpLockKeepingZoom for an op that always boots: a boot-time config error, an over-long socket path or an unresolvable shell is refused before the bracket's first tmux read.
func (e *Engine) withBootOpLockKeepingZoom(fn func() error) error {
	return e.withOpLock(func() error {
		if _, _, err := e.validateBootConfig(); err != nil {
			return err
		}
		if _, err := e.preflightBootHost(); err != nil {
			return err
		}
		return e.keepZoomLocked(fn)
	})
}

// zoomRecordHookBody is the window-resized hook entry that records a zoomed window in zoomRecordFlagOption and unzooms it.
// It tests the zoom when the hook fires, not when the entry is built.
const zoomRecordHookBody = "if-shell -F '#{window_zoomed_flag}' 'set-option -w " + zoomRecordFlagOption + " 1 ; resize-pane -Z'"

// zoomRestoreHookBody is the window-resized hook entry that zooms the active pane again when zoomRecordHookBody recorded a zoom, and clears the record.
const zoomRestoreHookBody = "if-shell -F '#{" + zoomRecordFlagOption + "}' 'resize-pane -Z ; set-option -wu " + zoomRecordFlagOption + "'"

// zoomRecordChainArgv returns the attach-chain elements that record a zoomed windowTarget and unzoom it.
// The chain runs in the client's context, so every command names the window explicitly.
func zoomRecordChainArgv(windowTarget string) []string {
	return []string{"if-shell", "-F", "-t", windowTarget, "#{window_zoomed_flag}",
		"set-option -w -t " + windowTarget + " " + zoomRecordFlagOption + " 1 ; resize-pane -Z -t " + windowTarget}
}

// zoomRestoreChainArgv returns the attach-chain elements that zoom windowTarget's active pane again when zoomRecordChainArgv recorded a zoom, and clear the record.
func zoomRestoreChainArgv(windowTarget string) []string {
	return []string{"if-shell", "-F", "-t", windowTarget, "#{" + zoomRecordFlagOption + "}",
		"resize-pane -Z -t " + windowTarget + " ; set-option -wu -t " + windowTarget + " " + zoomRecordFlagOption}
}
