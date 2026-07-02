// parse.go implements the pure, I/O-free parsers the psmux overlay (overlay.go)
// calls after shelling out: pane-list parsing, window-size parsing, and
// pane-order parsing. Keeping these free of subprocess I/O means they are
// unit-testable without a running psmux server, matching the module's
// hermetic-by-default testing posture.

package muxengine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LivePane represents the state of a single psmux pane as reported by
// list-panes: its id, whether it is dead (present but its command has
// exited — psmux keeps a dead pane around under `remain-on-exit on` until
// something explicitly kills it), and its current width/height.
type LivePane struct {
	ID     string `json:"id"`
	Dead   bool   `json:"dead"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// parsePaneList parses the output of
// list-panes -F "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}" into
// LivePane values. Returns nil, nil when the output is empty (no panes) —
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
		if len(parts) < 4 {
			return nil, fmt.Errorf("invalid pane format: %q", line)
		}

		// psmux reports pane_dead as "1"/"0"; remain-on-exit keeps the pane
		// entry around after its command exits, which is exactly the case
		// this flag exists to distinguish from a live pane.
		dead := parts[1] == "1"
		width, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid pane width: %s", parts[2])
		}
		height, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid pane height: %s", parts[3])
		}

		panes = append(panes, LivePane{
			ID:     parts[0],
			Dead:   dead,
			Width:  width,
			Height: height,
		})
	}

	return panes, nil
}

// parseWindowSize parses a "WIDTHxHEIGHT" string (the rendered
// "#{window_width}x#{window_height}" format) into its two integer parts.
func parseWindowSize(out string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(out), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected window size: %q", out)
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse width: %w", err)
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse height: %w", err)
	}
	return w, h, nil
}

// parsePaneOrder parses "#{pane_top} #{pane_id}" lines and returns the pane
// ids ordered by vertical position, top first. The caller uses this to know
// which pane is which ancestor/descendant when composing the column layout.
func parsePaneOrder(out string) ([]string, error) {
	type pane struct {
		top int
		id  string
	}
	var panes []pane
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return nil, fmt.Errorf("invalid pane line: %q", line)
		}
		top, err := strconv.Atoi(f[0])
		if err != nil {
			return nil, fmt.Errorf("parse pane_top: %w", err)
		}
		panes = append(panes, pane{top: top, id: f[1]})
	}
	sort.Slice(panes, func(i, j int) bool { return panes[i].top < panes[j].top })

	ids := make([]string, len(panes))
	for i, pn := range panes {
		ids[i] = pn.id
	}
	return ids, nil
}
