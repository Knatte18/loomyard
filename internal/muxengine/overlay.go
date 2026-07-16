// overlay.go implements the tmux subprocess overlay: TmuxCmd wraps the raw
// `tmux -L <socket> ...` invocation and exposes the typed helpers the
// lifecycle layer (batch 5) composes into Add/Remove/reconcile/apply/up.
// Every invocation is traced via logger.Debug so that -vv reveals the exact
// tmux command line for diagnosis, while a normal run (default Warn
// threshold) stays silent. This file is domain-free: it knows nothing about
// Claude, review panes, or any caller vocabulary, only tmux session/pane
// primitives.

package muxengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// TmuxCmd wraps low-level tmux operations for one resolved tmux binary
// and one -L socket. It carries no caller-specific configuration — width,
// height, launch templates, and similar tuning knobs live in Config
// (config.go), not here.
type TmuxCmd struct {
	tmuxPath string
	socket   string
}

// NewTmuxCmd builds a TmuxCmd bound to the given tmux binary path and -L
// socket name. Every run/output call this TmuxCmd makes prepends
// "-L <socket>" automatically, so callers never repeat the socket flag.
func NewTmuxCmd(tmuxPath, socket string) TmuxCmd {
	return TmuxCmd{tmuxPath: tmuxPath, socket: socket}
}

// run builds an exec.Command with "-L <socket>" prepended and runs it,
// discarding stdout but folding tmux's stderr into the returned error —
// a bare "exit status 1" is undiagnosable, while tmux's own message
// ("can't find session: …") names the actual failure. It traces the full
// argument list at Debug level before exec so a -vv run can see exactly
// what tmux was told to do.
func (p TmuxCmd) run(args ...string) error {
	fullArgs := append([]string{"-L", p.socket}, args...)
	logger.Debug("tmux", "args", fullArgs)
	cmd := exec.Command(p.tmuxPath, fullArgs...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	err := cmd.Run()
	return wrapTmuxError(err, stderr.Bytes())
}

// output builds an exec.Command with "-L <socket>" prepended and runs it,
// capturing stdout and folding tmux's stderr into the returned error,
// matching run's tracing and error shape.
func (p TmuxCmd) output(args ...string) (string, error) {
	fullArgs := append([]string{"-L", p.socket}, args...)
	logger.Debug("tmux", "args", fullArgs)
	cmd := exec.Command(p.tmuxPath, fullArgs...)
	out, err := cmd.Output()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), wrapTmuxError(err, exitErr.Stderr)
	}
	return string(out), err
}

// wrapTmuxError attaches tmux's trimmed stderr text to err so failures
// surface with tmux's own diagnosis attached. The original err stays the
// wrapped cause, so callers matching on *exec.ExitError (hasSession's
// absent-vs-error split) must unwrap via errors.As, never a direct type
// assertion.
func wrapTmuxError(err error, stderr []byte) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
}

// exactSessionTarget returns session as a tmux exact-match session target
// ("=<name>"). tmux resolves a bare -t session name by exact match first
// but falls back to PREFIX matching when no exact match exists, so a bare
// name can silently address a SIBLING worktree's session whose name shares
// this one's prefix (verified live on tmux 3.6: with only "repo2" present,
// `has-session -t repo` exits 0 and `kill-session -t repo` kills repo2 —
// worktree basenames such as mill task slugs share prefixes routinely).
// The "=" prefix pins exact matching. Subcommands whose -t argument is a
// WINDOW or PANE target need exactSessionWindowTarget instead — their
// target parser rejects the bare "=<name>" form.
func exactSessionTarget(session string) string {
	return "=" + session
}

// exactSessionWindowTarget returns session as an exact-match target for
// subcommands whose -t argument is a window/pane target (list-panes,
// select-layout, display-message). Their target parser rejects a bare
// "=<name>" ("can't find pane: =<name>", verified live on tmux 3.6) but
// accepts the "=<name>:" session-qualified form, which pins exact session
// matching and selects that session's current window.
func exactSessionWindowTarget(session string) string {
	return "=" + session + ":"
}

// hasSession reports whether the named session exists — by exact name,
// never tmux's prefix fallback (see exactSessionTarget). tmux exits 0 when
// the session is present and exits 1 when it is absent — exit 1 is the
// normal "not there yet" case, not an error, so only other failures surface
// as an error.
func (p TmuxCmd) hasSession(name string) (bool, error) {
	err := p.run("has-session", "-t", exactSessionTarget(name))
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// listPanes returns all panes in the session — targeted by exact session
// name, never tmux's prefix fallback (see exactSessionWindowTarget) —
// parsed from
// list-panes -F "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}".
// pane_top rides along so callers can derive the window's actual top-to-bottom
// pane order, and pane_pid so pane-destroying ops can snapshot a pane's
// process subtree, without a second round trip.
func (p TmuxCmd) listPanes(session string) ([]LivePane, error) {
	out, err := p.output("list-panes", "-t", exactSessionWindowTarget(session), "-F", "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}")
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}
