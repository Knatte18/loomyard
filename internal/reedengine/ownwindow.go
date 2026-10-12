// ownwindow.go holds the own-window placement.
// That is the session-wide window listing, the presence of own-window panes, and the windows' creation, grouping and removal.
// An own-window strand lives outside window 0, so window 0's layout never places it; its pane is read through the session listing instead.

package reedengine

import (
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// ownWindowOption is the window option reed sets on every own window it opens.
// Unlike the display-only options, Go decides on it: reconcile ends only a window carrying it.
const ownWindowOption = "@lyx_own"

// groupWindowOption is the window option naming the group a shared window holds.
// Go decides on it: a group window is found by it, never by window name.
const groupWindowOption = "@lyx_group"

// windowPane is one pane of the session-wide listing.
type windowPane struct {
	WindowID string
	// Own reports that the pane's window carries ownWindowOption.
	Own bool
	// Group is the window's group, empty when the window carries none.
	Group  string
	PaneID string
	Dead   bool
	PID    int
	// Title is the pane title, which may hold spaces.
	Title string
}

// livePane maps the pane to a LivePane; the geometry fields stay zero, since no layout reads an own-window pane.
func (w windowPane) livePane() LivePane {
	return LivePane{ID: w.PaneID, Dead: w.Dead, PID: w.PID, Title: w.Title}
}

// windowPaneFormat is the session-wide listing format of windowPane: window id, own flag, group (or "-"), pane id, dead flag, pid, then the pane title.
const windowPaneFormat = "#{window_id} #{?#{@lyx_own},1,0} #{?#{@lyx_group},#{@lyx_group},-} #{pane_id} #{pane_dead} #{pane_pid} #{pane_title}"

// parseWindowPanes parses a windowPaneFormat listing, skipping a line it cannot parse.
func parseWindowPanes(out string) []windowPane {
	var panes []windowPane
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 7)
		if len(fields) < 6 {
			continue
		}
		pid, err := strconv.Atoi(fields[5])
		if err != nil {
			continue
		}
		pane := windowPane{
			WindowID: fields[0],
			Own:      fields[1] == "1",
			PaneID:   fields[3],
			Dead:     fields[4] == "1",
			PID:      pid,
		}
		if fields[2] != "-" {
			pane.Group = fields[2]
		}
		if len(fields) == 7 {
			pane.Title = fields[6]
		}
		panes = append(panes, pane)
	}
	return panes
}

// listWindowPanes returns every pane of every window of session.
// The exact-match target keeps sibling sessions out.
func (p TmuxCmd) listWindowPanes(session string) ([]windowPane, error) {
	out, err := p.output("list-panes", "-s", "-t", exactSessionTarget(session), "-F", windowPaneFormat)
	if err != nil {
		return nil, err
	}
	return parseWindowPanes(out), nil
}

// unseenWindowOption is the display-only window option marking an own window the operator has not selected since it opened.
const unseenWindowOption = "@lyx_unseen"

// isOwnWindow reports whether d places a strand outside window 0.
func isOwnWindow(d render.Display) bool {
	return d.Anchor == render.AnchorOwnWindow
}

// presentPanes returns live plus the pane of every listed window pane that is bound to an own-window strand of strands and that live does not already hold.
// It is the pane set every liveness read uses, so an own-window pane counts as present, alive or dead, and a pane the operator killed by hand reads absent.
func presentPanes(strands []Strand, live []LivePane, windows []windowPane) []LivePane {
	held := make(map[string]bool, len(live))
	for _, p := range live {
		held[p.ID] = true
	}
	ownBound := make(map[string]bool)
	for _, s := range strands {
		if isOwnWindow(s.Display) && s.PaneID != "" {
			ownBound[s.PaneID] = true
		}
	}
	present := append([]LivePane(nil), live...)
	for _, w := range windows {
		if ownBound[w.PaneID] && !held[w.PaneID] {
			present = append(present, w.livePane())
			held[w.PaneID] = true
		}
	}
	return present
}

// listStrandAndWindowPanes returns the panes of the strands' window and the panes of the whole session.
func (e *Engine) listStrandAndWindowPanes(st *ReedState) ([]LivePane, []windowPane, error) {
	live, err := e.listStrandPanes(st)
	if err != nil {
		return nil, nil, err
	}
	windows, err := e.tmux.listWindowPanes(e.SessionName())
	if err != nil {
		return nil, nil, err
	}
	return live, windows, nil
}

// selectStrandsWindowIfCurrentLocked selects the strands' window when the session's current window is one of ending, so a viewer on a window about to end lands on window 0 and a viewer elsewhere is never moved.
// Every failure is logged and never fails the step.
func (e *Engine) selectStrandsWindowIfCurrentLocked(st *ReedState, ending []string) {
	if len(ending) == 0 {
		return
	}
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_id}")
	if err != nil {
		logger.Warn("reed: could not read the current window, a viewer may be left on a window that ends", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return
	}
	current := strings.TrimSpace(out)
	for _, id := range ending {
		if id != current {
			continue
		}
		target, err := e.strandWindowTargetFor(st)
		if err != nil {
			logger.Warn("reed: could not resolve the strands' window, a viewer may be left on a window that ends", "socket", e.Socket(), "session", e.SessionName(), "err", err)
			return
		}
		if err := e.tmux.run("select-window", "-t", target); err != nil {
			logger.Warn("reed: could not select the strands' window", "socket", e.Socket(), "session", e.SessionName(), "window", target, "err", err)
		}
		return
	}
}
