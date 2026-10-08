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

// strandWindowTarget returns the target of the window holding session's strands, as a window id ("@<n>").
// It resolves, in order, the window of the recorded Selvage pane (alive or a dead corpse), then the window of any recorded strand pane, and only with neither present the session's current window.
// A recorded pane id counts as present only when the session's own listing carries it, because pane ids are server-wide and a stale id may name another session's pane.
// A nil st or a state with no recorded pane ids resolves the current window without a round trip.
// A failed listing is returned as an error.
func (p TmuxCmd) strandWindowTarget(session string, st *ReedState) (string, error) {
	recorded := recordedPaneIDs(st)
	if len(recorded) == 0 {
		return exactSessionWindowTarget(session), nil
	}
	out, err := p.output("list-panes", "-s", "-t", exactSessionTarget(session), "-F", sessionPaneWindowFormat)
	if err != nil {
		return "", err
	}
	windowByPane := parseSessionPaneWindows(out)
	for _, paneID := range recorded {
		if windowID, ok := windowByPane[paneID]; ok {
			return windowID, nil
		}
	}
	return exactSessionWindowTarget(session), nil
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
