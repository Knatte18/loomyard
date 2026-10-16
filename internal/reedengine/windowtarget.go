// windowtarget.go is the one seam that names the tmux window holding a session's strands.
// Every op that enumerates, lays out or measures the strands' window goes through it,
// so a second window in the session (a batten window, say) is never read as the strands' window.

package reedengine

import (
	"fmt"
	"strings"
)

// sessionPaneWindowFormat is the session-wide listing format the window seam parses: pane id, then window id.
const sessionPaneWindowFormat = "#{pane_id} #{window_id}"

// strandsMarkerFormat is the session-wide window listing format the marker fallback parses: window id, then 1 when the window carries @lyx_strands and 0 otherwise.
const strandsMarkerFormat = "#{window_id} #{?#{@lyx_strands},1,0}"

// strandWindowTarget returns the target of the window holding session's strands, as a window id ("@<n>").
// It is the shell window when shellWindow resolves one, and otherwise the session's current window.
// A failed listing is returned as an error.
func (p TmuxCmd) strandWindowTarget(session string, st *ReedState) (string, error) {
	window, ok, err := p.shellWindow(session, st)
	if err != nil {
		return "", err
	}
	if ok {
		return window, nil
	}
	return exactSessionWindowTarget(session), nil
}

// shellWindow resolves the window holding session's shell pane, as a window id ("@<n>"), and reports whether it resolved one.
// It resolves the window of the recorded Selvage pane (alive or a dead corpse) first.
// A recorded pane id counts as present only when the session's own listing carries it, because pane ids are server-wide and a stale id may name another session's pane.
// With no recorded Selvage pane listed it resolves the window carrying @lyx_strands from the session's window listing, so a shell window whose Selvage pane is gone is still found.
// A nil st resolves nothing without a round trip, and a failed listing is returned as an error.
func (p TmuxCmd) shellWindow(session string, st *ReedState) (string, bool, error) {
	if st == nil {
		return "", false, nil
	}
	if recorded := recordedPaneIDs(st); len(recorded) > 0 {
		out, err := p.output("list-panes", "-s", "-t", exactSessionTarget(session), "-F", sessionPaneWindowFormat)
		if err != nil {
			return "", false, err
		}
		windowByPane := parseSessionPaneWindows(out)
		for _, paneID := range recorded {
			if windowID, ok := windowByPane[paneID]; ok {
				return windowID, true, nil
			}
		}
	}
	out, err := p.output("list-windows", "-t", exactSessionTarget(session), "-F", strandsMarkerFormat)
	if err != nil {
		return "", false, err
	}
	window, ok := markedStrandsWindow(out)
	return window, ok, nil
}

// markedStrandsWindow returns the window id of the first line of a strandsMarkerFormat listing whose marker field is 1.
func markedStrandsWindow(out string) (string, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "1" {
			return fields[0], true
		}
	}
	return "", false
}

// parseSessionPaneWindows maps each pane id of a sessionPaneWindowFormat listing to its window id.
func parseSessionPaneWindows(out string) map[string]string {
	windowByPane := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		windowByPane[fields[0]] = fields[1]
	}
	return windowByPane
}

// strandWindowTargetFor resolves the strands' window of this engine's session against st.
func (e *Engine) strandWindowTargetFor(st *ReedState) (string, error) {
	return e.tmux.strandWindowTarget(e.SessionName(), st)
}

// storedStrandWindowTarget resolves the strands' window against the persisted state, for a caller that holds no state of its own.
func (e *Engine) storedStrandWindowTarget() (string, error) {
	st, err := LoadState(e.stateDir())
	if err != nil {
		return "", fmt.Errorf("load state: %w", err)
	}
	return e.strandWindowTargetFor(st)
}

// listStrandPanes returns the panes of the strands' window, resolved against st.
func (e *Engine) listStrandPanes(st *ReedState) ([]LivePane, error) {
	target, err := e.strandWindowTargetFor(st)
	if err != nil {
		return nil, err
	}
	return e.tmux.listPanes(target)
}

// listStoredStrandPanes returns the panes of the strands' window, resolved against the persisted state.
func (e *Engine) listStoredStrandPanes() ([]LivePane, error) {
	target, err := e.storedStrandWindowTarget()
	if err != nil {
		return nil, err
	}
	return e.tmux.listPanes(target)
}
