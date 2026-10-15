// ownwindow.go holds the own-window placement.
// That is the session-wide window listing, the presence of own-window panes, and the windows' creation, grouping and removal.
// An own-window strand lives outside window 0, so window 0's layout never places it; its pane is read through the session listing instead.

package reedengine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
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

// isOwnWindow reports whether d places a strand in a window of its own, outside window 0: the own-window anchor or the overview anchor.
func isOwnWindow(d render.Display) bool {
	return d.Anchor == render.AnchorOwnWindow || isOverview(d)
}

// isOverview reports whether d places a strand in the session's overview window.
func isOverview(d render.Display) bool {
	return d.Anchor == render.AnchorOverview
}

// WindowGroup is the record of one window group.
// It is reed's own, written by the group's first strand and dropped with its last.
type WindowGroup struct {
	// Shared reports that the group's first strand chose one shared window for it.
	Shared bool `json:"shared"`
	// Size is the group size the first strand declared, 0 read as 1.
	Size int `json:"size"`
}

// ownWindowPlacement is where an own-window strand's pane goes.
type ownWindowPlacement int

const (
	// placeApart gives the strand a window to itself, named by its role.
	placeApart ownWindowPlacement = iota
	// placeNewShared opens the group's shared window with this strand.
	placeNewShared
	// placeJoinShared splits this strand's pane into the group's existing shared window.
	placeJoinShared
)

// planOwnWindowPlacement decides where a strand with display d goes, and returns the group record to store.
// An ungrouped strand is placed apart with no record.
// A group's first strand records whether the group shares one window, which is its declared size being at most shareMax;
// a later strand follows the record, so a changed shareMax never splits a group across shared and separate windows.
// A strand declaring another size than the record's is refused.
// A shared group joins its window while liveCount, the strands already holding a pane of the group, is below the group's size;
// the strand past that is placed apart.
// groupWindow is the id of the group's existing shared window, empty when it has none.
func planOwnWindowPlacement(groups map[string]WindowGroup, d render.Display, shareMax, liveCount int, groupWindow string) (ownWindowPlacement, WindowGroup, error) {
	if d.Window == "" {
		return placeApart, WindowGroup{}, nil
	}
	size := max(d.WindowSize, 1)
	group, recorded := groups[d.Window]
	if recorded {
		if recordedSize := max(group.Size, 1); recordedSize != size {
			return placeApart, WindowGroup{}, fmt.Errorf("window group %q is live with size %d, and this strand declares size %d; way forward: give the strand the group's size, or another group name", d.Window, recordedSize, size)
		}
	} else {
		group = WindowGroup{Shared: size <= shareMax, Size: size}
	}
	switch {
	case !group.Shared, liveCount >= max(group.Size, 1):
		return placeApart, group, nil
	case groupWindow != "":
		return placeJoinShared, group, nil
	}
	return placeNewShared, group, nil
}

// groupLiveCount counts the strands of group other than self whose binding names a listed pane, alive or dead, since a dead pane still occupies the shared window.
// It also returns the id of the window carrying the group's @lyx_group option and the own marker, empty when none is listed.
func groupLiveCount(strands []Strand, group, self string, windows []windowPane) (count int, groupWindow string) {
	listed := make(map[string]bool, len(windows))
	for _, w := range windows {
		listed[w.PaneID] = true
		if groupWindow == "" && w.Own && w.Group == group {
			groupWindow = w.WindowID
		}
	}
	for _, s := range strands {
		if s.GUID != self && s.Display.Window == group && s.PaneID != "" && listed[s.PaneID] {
			count++
		}
	}
	return count, groupWindow
}

// ownWindowsLosingPanes sorts the own windows that hold a pane of kill, in listing order and once each.
// A window all of whose listed panes are in kill ends with them, and a window that keeps at least one pane is left.
// A window without the own marker is in neither list.
func ownWindowsLosingPanes(windows []windowPane, kill []string) (ending, left []string) {
	killing := make(map[string]bool, len(kill))
	for _, id := range kill {
		killing[id] = true
	}
	type tally struct{ killed, kept int }
	counts := make(map[string]*tally)
	var order []string
	for _, w := range windows {
		if !w.Own {
			continue
		}
		c, seen := counts[w.WindowID]
		if !seen {
			c = &tally{}
			counts[w.WindowID] = c
			order = append(order, w.WindowID)
		}
		if killing[w.PaneID] {
			c.killed++
		} else {
			c.kept++
		}
	}
	for _, id := range order {
		switch c := counts[id]; {
		case c.killed == 0:
		case c.kept == 0:
			ending = append(ending, id)
		default:
			left = append(left, id)
		}
	}
	return ending, left
}

// relayoutSharedWindowsLocked lays out each of windows evenly in the window_split direction and returns "".
// An invalid window config skips every layout and returns the config error's text, so a removal still completes and only the layout is skipped.
// A layout that fails is logged and never fails the step.
func (e *Engine) relayoutSharedWindowsLocked(windows []string) string {
	if err := validateWindowConfig(e.cfg); err != nil {
		return err.Error()
	}
	_, layout, _ := windowSplitLayout(e.cfg.WindowSplit)
	for _, window := range windows {
		if err := e.tmux.run("select-layout", "-t", window, layout); err != nil {
			logger.Warn("reed: could not lay out a shared window after a strand left it", "window", window, "layout", layout, "err", err)
		}
	}
	return ""
}

// pruneWindowGroups deletes every group record that no strand of st names any more, so a later instance under the same name records afresh.
func pruneWindowGroups(st *ReedState) {
	named := make(map[string]bool, len(st.Strands))
	for _, s := range st.Strands {
		if s.Display.Window != "" {
			named[s.Display.Window] = true
		}
	}
	for name := range st.WindowGroups {
		if !named[name] {
			delete(st.WindowGroups, name)
		}
	}
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

// agentWindowPanes sorts the listed panes bound to strands of st by window: live holds each window with a bound own-window pane that is not dead, bound each window with any such pane, and overview the window of the overview strand's pane.
// Panes in a window of skip are left out.
func agentWindowPanes(st *ReedState, panes []windowPane, skip map[string]bool) (live, bound map[string]bool, overview string) {
	displayByPane := make(map[string]render.Display, len(st.Strands))
	for _, s := range st.Strands {
		if s.PaneID != "" {
			displayByPane[s.PaneID] = s.Display
		}
	}
	live, bound = make(map[string]bool), make(map[string]bool)
	for _, p := range panes {
		display, held := displayByPane[p.PaneID]
		if !held || skip[p.WindowID] {
			continue
		}
		switch {
		case isOverview(display):
			overview = p.WindowID
		case display.Anchor == render.AnchorOwnWindow:
			bound[p.WindowID] = true
			if !p.Dead {
				live[p.WindowID] = true
			}
		}
	}
	return live, bound, overview
}

// relevantWindow returns the window an automatic switch lands on, among the listed windows not in ending.
// That is the highest-index agent window holding a pane that is not dead, else the overview strand's window, else the empty string.
// An agent window is a window holding a pane bound to an own-window strand, so the overview, the shell and hidden windows are never one.
// By start order the highest index is the newest live agent.
func relevantWindow(st *ReedState, windows []sessionWindow, panes []windowPane, ending map[string]bool) string {
	live, _, overview := agentWindowPanes(st, panes, ending)
	best, bestIndex := "", -1
	for _, w := range windows {
		if live[w.ID] && w.Index > bestIndex {
			best, bestIndex = w.ID, w.Index
		}
	}
	if best != "" {
		return best
	}
	if _, listed := findSessionWindow(windows, overview); listed {
		return overview
	}
	return ""
}

// newAgentWindowSelected reports whether a new agent window takes the viewer who is on window current, deciding from the windows and panes listed before it opens.
// A strand whose display sets Focus always does, and one that sets only KeepViewer never does.
// Any other takes it from the overview window, from an agent window whose every pane is dead, from the relevant window and from a window holding a live driver pane, so a viewer following the run follows the newest agent.
// The shell, an older live agent window and any other window keep the viewer, since an operator moved there by hand.
func newAgentWindowSelected(st *ReedState, windows []sessionWindow, panes []windowPane, current string, display render.Display) bool {
	if display.Focus {
		return true
	}
	if display.KeepViewer || current == "" {
		return false
	}
	live, bound, overview := agentWindowPanes(st, panes, nil)
	return current == overview || (bound[current] && !live[current]) ||
		current == relevantWindow(st, windows, panes, nil) || driverWindows(st, panes)[current]
}

// driverWindows returns each agent window holding a pane that is not dead and is bound to an own-window strand whose role is the driver role.
// A driver placed in the strands' window yields none, so the shell window never passes the viewer on.
func driverWindows(st *ReedState, panes []windowPane) map[string]bool {
	driverPanes := make(map[string]bool)
	for _, s := range st.Strands {
		if s.PaneID == "" || s.Display.Anchor != render.AnchorOwnWindow {
			continue
		}
		if parsed, err := agentname.Parse(s.Name); err == nil && agentname.MatchesRole(parsed.Role, agentname.RoleDriver) {
			driverPanes[s.PaneID] = true
		}
	}
	windows := make(map[string]bool)
	for _, p := range panes {
		if driverPanes[p.PaneID] && !p.Dead {
			windows[p.WindowID] = true
		}
	}
	return windows
}

// newAgentWindowTakesViewerLocked reports whether a new agent window opened now takes the viewer, reading the session's current window, windows and panes before the window exists.
// A display setting Focus or KeepViewer needs no read.
// A failed read keeps the viewer and is logged at Warn.
func (e *Engine) newAgentWindowTakesViewerLocked(st *ReedState, display render.Display) bool {
	if display.Focus || display.KeepViewer {
		return newAgentWindowSelected(st, nil, nil, "", display)
	}
	current, err := e.currentWindowLocked()
	if err != nil {
		logger.Warn("reed: could not read the current window, a new agent window keeps the viewer where it is", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	windows, err := e.tmux.listSessionWindows(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list the session's windows, a new agent window keeps the viewer where it is", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	panes, err := e.tmux.listWindowPanes(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list the session's panes, a new agent window keeps the viewer where it is", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	return newAgentWindowSelected(st, windows, panes, current, display)
}

// currentWindowLocked returns the id of the session's current window.
func (e *Engine) currentWindowLocked() (string, error) {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// currentWindowIsAmongLocked reports whether the session's current window is one of windows.
// A failed read of the current window is logged and reports false.
func (e *Engine) currentWindowIsAmongLocked(windows []string) bool {
	current, err := e.currentWindowLocked()
	if err != nil {
		logger.Warn("reed: could not read the current window, a viewer may be left on a window that ends", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	return slices.Contains(windows, current)
}

// selectRelevantWindowIfCurrentLocked selects relevantWindow among the windows left when the session's current window is one of ending, so a viewer on a window about to end lands on the newest live agent or the overview, never the shell.
// A viewer elsewhere is never moved, and with no window left to select it selects nothing and leaves the choice to tmux.
// Every failure is logged and never fails the step.
func (e *Engine) selectRelevantWindowIfCurrentLocked(st *ReedState, ending []string) {
	if len(ending) == 0 || !e.currentWindowIsAmongLocked(ending) {
		return
	}
	e.selectRelevantWindowLocked(st, ending)
}

// selectRelevantWindowLocked selects relevantWindow among the session's windows not in ending, for a viewer already known to be on one of them.
// Every failure is logged and never fails the step.
func (e *Engine) selectRelevantWindowLocked(st *ReedState, ending []string) {
	windows, err := e.tmux.listSessionWindows(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list the session's windows, a viewer may be left on a window that ends", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return
	}
	panes, err := e.tmux.listWindowPanes(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list the session's panes, a viewer may be left on a window that ends", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return
	}
	endingSet := make(map[string]bool, len(ending))
	for _, id := range ending {
		endingSet[id] = true
	}
	target := relevantWindow(st, windows, panes, endingSet)
	if target == "" {
		return
	}
	if err := e.tmux.run("select-window", "-t", target); err != nil {
		logger.Warn("reed: could not select the relevant window", "socket", e.Socket(), "session", e.SessionName(), "window", target, "err", err)
	}
}

// attachWindowSelectionLocked returns the argv tail that selects relevantWindow for an attaching client: ";", "select-window", "-t", the window.
// It returns nothing when no window is relevant or a listing fails, which is logged.
func (e *Engine) attachWindowSelectionLocked(st *ReedState) []string {
	windows, err := e.tmux.listSessionWindows(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list the session's windows, attaching without a window selection", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return nil
	}
	panes, err := e.tmux.listWindowPanes(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list the session's panes, attaching without a window selection", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return nil
	}
	target := relevantWindow(st, windows, panes, nil)
	if target == "" {
		return nil
	}
	return []string{";", "select-window", "-t", target}
}

// selectStrandWindowLocked selects the window that holds paneID.
// A failed lookup or select is logged and never fails the step.
func (e *Engine) selectStrandWindowLocked(paneID string) {
	windowByPane, err := e.tmux.listSessionPaneWindows(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list the session's panes to select the new window", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return
	}
	window, ok := windowByPane[paneID]
	if !ok {
		logger.Warn("reed: the new strand's pane is not listed, leaving the viewer where it is", "socket", e.Socket(), "session", e.SessionName(), "pane", paneID)
		return
	}
	if err := e.tmux.run("select-window", "-t", window); err != nil {
		logger.Warn("reed: could not select the new window", "socket", e.Socket(), "session", e.SessionName(), "window", window, "err", err)
	}
}
