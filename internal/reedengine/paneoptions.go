// paneoptions.go sets the display-only tmux user options that identify strands: @strand and @strand_color on each strand pane, @lyx_strands on the strand window.
// It also names @lyx_console, the marker of the operator console pane that the key bindings read.
// No Go decision reads them back; the status bar and the key bindings do.

package reedengine

import (
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// consolePaneOption is the pane option marking the operator console pane, set to 1 by markSelvagePaneLocked.
// The key bindings read it so the console joins the Alt+Up/Alt+Down cycle and takes Alt+z; no Go decision reads it.
const consolePaneOption = "@lyx_console"

// strandLabel returns the role segment of a strand name, or the full name when it does not parse.
func strandLabel(name string) string {
	parsed, err := agentname.Parse(name)
	if err != nil {
		return name
	}
	return parsed.Role
}

// setStrandPaneOptionsLocked sets @strand on a strand's pane, and @strand_color when the strand's segment resolves to a color, else unsets it.
// A failure is logged and never fails the step.
func (e *Engine) setStrandPaneOptionsLocked(paneID string, s Strand) {
	if err := e.tmux.run("set-option", "-p", "-t", paneID, "@strand", strandLabel(s.Name)); err != nil {
		logger.Warn("reed: could not set @strand", "strand", s.GUID, "pane", paneID, "err", err)
	}
	if color, ok := e.segmentColor(s.Segment); ok {
		if tmuxColor, ok := segmentcolor.TmuxColor(color); ok {
			if err := e.tmux.run("set-option", "-p", "-t", paneID, "@strand_color", tmuxColor); err != nil {
				logger.Warn("reed: could not set @strand_color", "strand", s.GUID, "pane", paneID, "err", err)
			}
			return
		}
	}
	if err := e.tmux.run("set-option", "-p", "-u", "-t", paneID, "@strand_color"); err != nil {
		logger.Warn("reed: could not unset @strand_color", "strand", s.GUID, "pane", paneID, "err", err)
	}
}

// reassertStrandPaneOptionsLocked sets the pane options of every strand bound to an alive pane in live.
func (e *Engine) reassertStrandPaneOptionsLocked(st *ReedState, live []LivePane) {
	alive := aliveIDSet(live)
	for _, s := range st.Strands {
		if s.PaneID != "" && alive[s.PaneID] {
			e.setStrandPaneOptionsLocked(s.PaneID, s)
		}
	}
}

// markStrandWindowLocked sets @lyx_strands on the strand window.
// A failure is logged and never fails the step.
func (e *Engine) markStrandWindowLocked(windowTarget string) {
	if err := e.tmux.run("set-option", "-w", "-t", windowTarget, "@lyx_strands", "1"); err != nil {
		logger.Warn("reed: could not set @lyx_strands", "window", windowTarget, "err", err)
	}
}

// markResolvedStrandWindowLocked marks the strand window once Selvage exists and the window resolves, then checks the shell window, which is how a fresh boot names its one window shell.
// An unresolved window is logged and skips the mark and the check.
func (e *Engine) markResolvedStrandWindowLocked(st *ReedState) {
	windowTarget, err := e.strandWindowTargetFor(st)
	if err != nil {
		logger.Warn("reed: could not resolve the strand window to mark it", "err", err)
		return
	}
	e.markStrandWindowLocked(windowTarget)
	e.ensureShellWindowLocked(st)
}

// withColor returns s with Color filled from its segment's configured color.
func (e *Engine) withColor(s Strand) Strand {
	s.Color, _ = e.segmentColor(s.Segment)
	return s
}
