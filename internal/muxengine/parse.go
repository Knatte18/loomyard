// parse.go implements the pure, I/O-free parser the tmux overlay (overlay.go)
// calls after shelling out: pane-list parsing. Keeping it free of subprocess
// I/O means it is unit-testable without a running tmux server, matching the
// module's hermetic-by-default testing posture.

package muxengine

import (
	"fmt"
	"strconv"
	"strings"
)

// LivePane represents the state of a single tmux pane as reported by
// list-panes: its id, whether it is dead (present but its command has
// exited — tmux keeps a dead pane around under `remain-on-exit on` until
// something explicitly kills it), its vertical position (pane_top, the row
// its first line occupies — the key for deriving the window's actual
// top-to-bottom pane order, which select-layout applies cells against), its
// current width/height, and the OS pid of the pane's immediate child
// process (#{pane_pid} — the launcher process, not the deeper descendant
// that actually holds the worktree directory; see
// paneProcessTreePIDsLocked).
type LivePane struct {
	ID     string `json:"id"`
	Dead   bool   `json:"dead"`
	Top    int    `json:"top"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	PID    int    `json:"pid"`
}

// parsePaneList parses the output of
// list-panes -F "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}"
// into LivePane values. Returns nil, nil when the output is empty (no panes) —
// this is the normal shape for a not-yet-created session, not an error.
func parsePaneList(out string) ([]LivePane, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}

	var panes []LivePane
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 6 {
			return nil, fmt.Errorf("invalid pane format: %q", line)
		}

		// tmux reports pane_dead as "1"/"0"; remain-on-exit keeps the pane
		// entry around after its command exits, which is exactly the case
		// this flag exists to distinguish from a live pane.
		dead := parts[1] == "1"
		top, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid pane top: %s", parts[2])
		}
		width, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid pane width: %s", parts[3])
		}
		height, err := strconv.Atoi(parts[4])
		if err != nil {
			return nil, fmt.Errorf("invalid pane height: %s", parts[4])
		}
		pid, err := strconv.Atoi(parts[5])
		if err != nil {
			return nil, fmt.Errorf("invalid pane pid: %s", parts[5])
		}

		panes = append(panes, LivePane{
			ID:     parts[0],
			Dead:   dead,
			Top:    top,
			Width:  width,
			Height: height,
			PID:    pid,
		})
	}

	return panes, nil
}
