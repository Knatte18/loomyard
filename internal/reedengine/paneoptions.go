// paneoptions.go sets the tmux user options that identify strands: @strand, @strand_color and @lym_strand_name on each strand pane, @lym_strands on the strand window.
// It also names @lym_console, the marker of the operator console pane that the key bindings read.
// No Go decision reads them back but the button click, which reads @lym_strand_name for {strand}; the status bar and the key bindings read the rest.

package reedengine

import (
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// consolePaneOption is the pane option marking the operator console pane, set to 1 by markSelvagePaneLocked.
// The key bindings read it so the console joins the Alt+Up/Alt+Down cycle; no Go decision reads it.
const consolePaneOption = "@lym_console"

// strandLabel returns the role segment of a strand name, or the full name when it does not parse.
func strandLabel(name string) string {
	parsed, err := agentname.Parse(name)
	if err != nil {
		return name
	}
	return parsed.Role
}

// strandNamePaneOption is the pane option holding a strand's full name, which a button click reads to fill {strand}; @strand holds only the role.
const strandNamePaneOption = "@lym_strand_name"

// setStrandPaneOptionsLocked sets @strand and strandNamePaneOption on a strand's pane, and @strand_color when the strand's segment resolves to a color, else unsets it.
// An empty strand name unsets strandNamePaneOption, so a stale name never outlives a relaunch under no name.
// A failure is logged and never fails the step.
func (e *Engine) setStrandPaneOptionsLocked(paneID string, s Strand) {
	if err := e.tmux.run("set-option", "-p", "-t", paneID, "@strand", strandLabel(s.Name)); err != nil {
		logger.Warn("reed: could not set @strand", "strand", s.GUID, "pane", paneID, "err", err)
	}
	nameArgv := []string{"set-option", "-p", "-t", paneID, strandNamePaneOption, s.Name}
	if s.Name == "" {
		nameArgv = []string{"set-option", "-p", "-u", "-t", paneID, strandNamePaneOption}
	}
	if err := e.tmux.run(nameArgv...); err != nil {
		logger.Warn("reed: could not set the strand name option", "option", strandNamePaneOption, "strand", s.GUID, "pane", paneID, "err", err)
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

// markStrandWindowLocked sets @lym_strands on the strand window.
// A failure is logged and never fails the step.
func (e *Engine) markStrandWindowLocked(windowTarget string) {
	if err := e.tmux.run("set-option", "-w", "-t", windowTarget, "@lym_strands", "1"); err != nil {
		logger.Warn("reed: could not set @lym_strands", "window", windowTarget, "err", err)
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
