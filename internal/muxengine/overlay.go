// overlay.go implements the psmux subprocess overlay: PsmuxCmd wraps the raw
// `psmux -L <socket> ...` invocation and exposes the typed helpers the
// lifecycle layer (batch 5) composes into Add/Remove/reconcile/apply/up.
// Every invocation is traced via logger.Debug so that -vv reveals the exact
// psmux command line for diagnosis, while a normal run (default Warn
// threshold) stays silent. This file is domain-free: it knows nothing about
// Claude, review panes, or any caller vocabulary, only psmux session/pane
// primitives.

package muxengine

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// PsmuxCmd wraps low-level psmux operations for one resolved psmux binary
// and one -L socket. It carries no caller-specific configuration — width,
// height, launch templates, and similar tuning knobs live in Config
// (config.go), not here.
type PsmuxCmd struct {
	psmuxPath string
	socket    string
}

// NewPsmuxCmd builds a PsmuxCmd bound to the given psmux binary path and -L
// socket name. Every run/output call this PsmuxCmd makes prepends
// "-L <socket>" automatically, so callers never repeat the socket flag.
func NewPsmuxCmd(psmuxPath, socket string) PsmuxCmd {
	return PsmuxCmd{psmuxPath: psmuxPath, socket: socket}
}

// run builds an exec.Command with "-L <socket>" prepended and runs it,
// discarding stdout and stderr. It traces the full argument list at Debug
// level before exec so a -vv run can see exactly what psmux was told to do.
func (p PsmuxCmd) run(args ...string) error {
	fullArgs := append([]string{"-L", p.socket}, args...)
	logger.Debug("psmux", "args", fullArgs)
	cmd := exec.Command(p.psmuxPath, fullArgs...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// output builds an exec.Command with "-L <socket>" prepended and runs it,
// capturing stdout. It traces the full argument list at Debug level before
// exec, matching run's tracing.
func (p PsmuxCmd) output(args ...string) (string, error) {
	fullArgs := append([]string{"-L", p.socket}, args...)
	logger.Debug("psmux", "args", fullArgs)
	cmd := exec.Command(p.psmuxPath, fullArgs...)
	out, err := cmd.Output()
	return string(out), err
}

// hasSession reports whether the named session exists. psmux exits 0 when
// the session is present and exits 1 when it is absent — exit 1 is the
// normal "not there yet" case, not an error, so only other failures surface
// as an error.
func (p PsmuxCmd) hasSession(name string) (bool, error) {
	err := p.run("has-session", "-t", name)
	if err == nil {
		return true, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// listPanes returns all panes in the session, parsed from
// list-panes -F "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}".
func (p PsmuxCmd) listPanes(session string) ([]LivePane, error) {
	out, err := p.output("list-panes", "-t", session, "-F", "#{pane_id} #{pane_dead} #{pane_width} #{pane_height}")
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}

// activePaneID returns the pane id (e.g. "%5") of the active pane in
// session. After split-window the new pane becomes active, so this reports
// the freshly created pane; in a single-pane session it reports that pane.
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

// paneIDsTopToBottom returns the session's pane ids (e.g. "%1") ordered by
// vertical position, top first. The lifecycle/render layers use this to
// know which pane is which ancestor/descendant when composing a layout.
func (p PsmuxCmd) paneIDsTopToBottom(session string) ([]string, error) {
	out, err := p.output("list-panes", "-t", session, "-F", "#{pane_top} #{pane_id}")
	if err != nil {
		return nil, fmt.Errorf("list panes for %s: %w", session, err)
	}
	return parsePaneOrder(out)
}
