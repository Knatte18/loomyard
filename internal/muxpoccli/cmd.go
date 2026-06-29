// cmd.go — low-level psmux command helpers.
//
// PsmuxCmd wraps low-level psmux operations: run (discarding output), output
// (capturing stdout), hasSession (checking existence), and listPanes (parsing pane
// format). LivePane represents a single pane state. expandTpl is a template helper
// for launch/resume commands. (Config is defined in cli.go.)

package muxpoccli

import (
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// PsmuxCmd wraps low-level psmux operations.
type PsmuxCmd struct {
	cfg Config
}

// NewPsmuxCmd creates a new PsmuxCmd with the given config.
func NewPsmuxCmd(cfg Config) PsmuxCmd {
	return PsmuxCmd{cfg: cfg}
}

// run builds an exec.Command with -L <socket> prepended and runs it,
// discarding stdout and stderr. Returns cmd.Run() error.
func (p PsmuxCmd) run(args ...string) error {
	fullArgs := append([]string{"-L", socketName(p.cfg.WorktreeRoot)}, args...)
	cmd := exec.Command(p.cfg.PsmuxPath, fullArgs...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// output builds an exec.Command with -L <socket> prepended and runs it,
// capturing stdout. Returns (stdout string, error).
func (p PsmuxCmd) output(args ...string) (string, error) {
	fullArgs := append([]string{"-L", socketName(p.cfg.WorktreeRoot)}, args...)
	cmd := exec.Command(p.cfg.PsmuxPath, fullArgs...)
	out, err := cmd.Output()
	return string(out), err
}

// hasSession checks whether the named session exists. Returns (true, nil) on
// exit 0, (false, nil) on exit 1 (session absent — normal, not an error).
// Returns (false, err) on any other error.
func (p PsmuxCmd) hasSession(name string) (bool, error) {
	err := p.run("has-session", "-t", name)
	if err == nil {
		return true, nil
	}

	// Check if it's an ExitError with code 1 (session absent)
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// LivePane represents the state of a single pane.
type LivePane struct {
	ID     string `json:"id"`
	Dead   bool   `json:"dead"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// listPanes returns all panes in the session. Parses output from
// list-panes -F "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}".
// Returns nil, nil if output is empty (no panes).
func (p PsmuxCmd) listPanes(session string) ([]LivePane, error) {
	out, err := p.output("list-panes", "-t", session, "-F", "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}")
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}

// parsePaneList parses the output of
// list-panes -F "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}" into
// LivePane values. Returns nil, nil when the output is empty (no panes). Kept
// free of I/O so it can be unit-tested without psmux.
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

// activePaneID returns the pane id (e.g. "%5") of the active pane in session.
// After split-window the new pane becomes active, so this reports the freshly
// created pane; in a single-pane session it reports that pane.
func (p PsmuxCmd) activePaneID(session string) (string, error) {
	out, err := p.output("display-message", "-p", "-t", session, "#{pane_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// windowSize returns the (width, height) of the session's window.
func (p PsmuxCmd) windowSize(session string) (int, int, error) {
	out, err := p.output("display-message", "-p", "-t", session, "#{window_width}x#{window_height}")
	if err != nil {
		return 0, 0, err
	}
	return parseWindowSize(out)
}

// parseWindowSize parses a "WIDTHxHEIGHT" string (the rendered
// "#{window_width}x#{window_height}" format). Kept free of I/O for unit testing.
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

// paneIDsTopToBottom returns the session's pane ids (e.g. "%1") ordered by
// vertical position, top first. Used to drive the column layout.
func (p PsmuxCmd) paneIDsTopToBottom(session string) ([]string, error) {
	out, err := p.output("list-panes", "-t", session, "-F", "#{pane_top} #{pane_id}")
	if err != nil {
		return nil, err
	}
	return parsePaneOrder(out)
}

// parsePaneOrder parses "#{pane_top} #{pane_id}" lines and returns the pane ids
// ordered by vertical position, top first. Kept free of I/O for unit testing.
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

// activePaneShare is the fraction of window height given to the bottom (active)
// pane by applyColumnLayout. The deepest child is the only running session; its
// ancestors are blocked waiting on it, so they collapse to compact strips while
// the active pane dominates. Kept above 50% per the muxpoc layout model.
const activePaneShare = 55

// applyColumnLayout arranges the session's panes as a vertical column in which
// the bottom (active) pane receives ~activePaneShare% of the height and the
// ancestor panes above share the remainder equally. psmux honours a hand-built
// window-layout string (with a computed checksum) via select-layout; preset
// layouts like even-vertical cannot express "bottom pane dominant".
func (p PsmuxCmd) applyColumnLayout(session string) error {
	ids, err := p.paneIDsTopToBottom(session)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}
	if len(ids) < 2 {
		return nil // a single pane already fills the window
	}

	w, h, err := p.windowSize(session)
	if err != nil {
		return fmt.Errorf("window size: %w", err)
	}

	if err := p.run("select-layout", "-t", session, buildColumnLayout(w, h, ids)); err != nil {
		return fmt.Errorf("select-layout: %w", err)
	}
	// Focus the active (bottom) pane so input lands where work happens.
	if err := p.run("select-pane", "-t", ids[len(ids)-1]); err != nil {
		return fmt.Errorf("select-pane: %w", err)
	}
	return nil
}

// buildColumnLayout returns a checksum-prefixed psmux window-layout string for a
// vertical column of panes (ids ordered top to bottom) in a w×h window: the
// bottom (active) pane gets ~activePaneShare% of the height and the ancestors
// above share the remainder equally. Callers must pass at least two ids. Pure
// (no I/O) so the height math and checksum are unit-testable.
func buildColumnLayout(w, h int, ids []string) string {
	n := len(ids)
	usable := h - (n - 1) // one row per divider between panes
	ancestorH := usable * (100 - activePaneShare) / (100 * (n - 1))
	if ancestorH < 1 {
		ancestorH = 1
	}
	bottomH := usable - ancestorH*(n-1)

	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d,0,0[", w, h)
	y := 0
	for i, id := range ids {
		paneH := ancestorH
		if i == n-1 {
			paneH = bottomH
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%dx%d,0,%d,%s", w, paneH, y, strings.TrimPrefix(id, "%"))
		y += paneH + 1 // advance past this pane and its divider
	}
	b.WriteByte(']')

	layout := b.String()
	return layoutChecksum(layout) + "," + layout
}

// layoutChecksum computes the tmux window-layout checksum for s (the layout
// string following the leading "csum," field), returned as four lowercase hex
// digits. Matches tmux's layout_checksum: a 16-bit rotate-right accumulator.
func layoutChecksum(s string) string {
	var csum uint16
	for i := 0; i < len(s); i++ {
		csum = (csum >> 1) | ((csum & 1) << 15)
		csum += uint16(s[i])
	}
	return fmt.Sprintf("%04x", csum)
}

// expandTpl replaces %SID% with sid and %TASK% with task in tpl.
// Used by up.go and daemon.go to build claude launch/resume commands.
func expandTpl(tpl, sid, task string) string {
	result := strings.ReplaceAll(tpl, "%SID%", sid)
	result = strings.ReplaceAll(result, "%TASK%", task)
	return result
}
