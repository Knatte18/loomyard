// parse.go implements the pure, I/O-free parser the tmux overlay (overlay.go) calls after shelling
// out: pane-list parsing.
// Keeping it free of subprocess I/O means it is unit-testable without a running tmux server,
// matching the module's hermetic-by-default testing posture.

package reedengine

import (
	"fmt"
	"strconv"
	"strings"
)

// LivePane represents the state of a single tmux pane from list-panes.
type LivePane struct {
	ID     string `json:"id"`
	Dead   bool   `json:"dead"`
	Top    int    `json:"top"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	PID    int    `json:"pid"`
	// Title is the pane title, a display mirror of the strand's full name; empty when unset.
	Title string `json:"title,omitempty"`
	// CurrentCommand is the name of the pane's foreground process group leader; empty when the listing did not carry it.
	CurrentCommand string `json:"current_command,omitempty"`
	// StartCommand is the command tmux froze into the pane at spawn; empty when the session held no default-command pin.
	StartCommand string `json:"start_command,omitempty"`
}

// paneTailSeparator joins the title, the current command and the start command in the last field of a pane listing line.
// Each of the three can hold spaces, and tmux passes the ASCII unit separator through a format verbatim.
const paneTailSeparator = "\x1f"

// splitPaneTail splits the last field of a pane listing line into the title, the current command and the start command.
// A part the tail does not carry reads empty, so a tail holding only a title parses with both commands empty.
func splitPaneTail(tail string) (title, current, start string) {
	parts := strings.SplitN(tail, paneTailSeparator, 3)
	title = parts[0]
	if len(parts) > 1 {
		current = parts[1]
	}
	if len(parts) > 2 {
		start = parts[2]
	}
	return title, current, start
}

// parsePaneList parses list-panes output into LivePane values.
// Returns nil, nil when output is empty (normal for uncreated session).
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

		// The tail field may hold spaces, so everything after the sixth field is the tail.
		parts := strings.SplitN(line, " ", 7)
		if len(parts) < 6 {
			return nil, fmt.Errorf("invalid pane format: %q", line)
		}
		tail := ""
		if len(parts) == 7 {
			tail = parts[6]
		}
		title, currentCommand, startCommand := splitPaneTail(tail)

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
			ID:             parts[0],
			Dead:           dead,
			Top:            top,
			Width:          width,
			Height:         height,
			PID:            pid,
			Title:          title,
			CurrentCommand: currentCommand,
			StartCommand:   startCommand,
		})
	}

	return panes, nil
}
