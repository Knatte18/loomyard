// ownwindow.go holds the own-window placement.
// That is the session-wide window listing, the presence of own-window panes, and the windows' creation, grouping and removal.
// An own-window strand lives outside window 0, so window 0's layout never places it; its pane is read through the session listing instead.

package reedengine

import (
	"fmt"
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
	placeNewShared ownWindowPlacement = 1
	// placeJoinShared splits this strand's pane into the group's existing shared window.
	placeJoinShared ownWindowPlacement = 2
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
