// overlay.go implements the tmux subprocess overlay: TmuxCmd wraps the raw `tmux -L <socket> ...`
// invocation and exposes the typed helpers the lifecycle layer (batch 5) composes into
// Add/Remove/reconcile/apply/up.
// Every invocation is traced via logger.Debug so that -vv reveals the exact tmux command line for
// diagnosis, while a normal run (default Warn threshold) stays silent.
// This file is domain-free: it knows nothing about Claude, review panes, or any caller vocabulary,
// only tmux session/pane primitives.

package reedengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// TmuxCmd wraps low-level tmux operations for one binary and -L socket.
type TmuxCmd struct {
	tmuxPath string
	socket   string
	// execHook, when non-nil, replaces the real subprocess exec for BOTH run
	// and output — the single white-box seam a test can stub to drive a
	// composed engine call site (e.g. ensureHeaderPaneLocked's header-rebuild
	// split) against a scripted tmux response WITHOUT a live server. It is the
	// only way to exercise the psmux-only silent-split failure shape (exit 0
	// with an EXISTING pane id printed) that native tmux cannot produce — the
	// exact shape validateSplitCreatedNewPane guards each split site against.
	// capture mirrors output's stdout-capturing call (true) vs run's
	// discard-stdout call (false); args is the tmux subcommand argv WITHOUT the
	// leading "-L <socket>", so a hook matches on args[0] (the subcommand)
	// directly. Production never sets it; a set hook is a test-only override.
	execHook func(capture bool, args ...string) (string, error)
}

// NewTmuxCmd builds a TmuxCmd bound to the given binary and -L socket.
func NewTmuxCmd(tmuxPath, socket string) TmuxCmd {
	return TmuxCmd{tmuxPath: tmuxPath, socket: socket}
}

// run builds and runs a command with "-L <socket>" prepended,
// folding stderr into the returned error.
func (p TmuxCmd) run(args ...string) error {
	if p.execHook != nil {
		_, err := p.execHook(false, args...)
		return err
	}
	fullArgs := append([]string{"-L", p.socket}, args...)
	logger.Debug("tmux", "args", fullArgs)
	cmd := exec.Command(p.tmuxPath, fullArgs...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	err := cmd.Run()
	return wrapTmuxError(err, stderr.Bytes())
}

// output builds and runs a command with "-L <socket>" prepended,
// capturing stdout and folding stderr into the error.
func (p TmuxCmd) output(args ...string) (string, error) {
	if p.execHook != nil {
		return p.execHook(true, args...)
	}
	fullArgs := append([]string{"-L", p.socket}, args...)
	logger.Debug("tmux", "args", fullArgs)
	cmd := exec.Command(p.tmuxPath, fullArgs...)
	out, err := cmd.Output()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), wrapTmuxError(err, exitErr.Stderr)
	}
	return string(out), err
}

// wrapTmuxError attaches tmux's stderr to err so failures surface
// with tmux's own diagnosis. Wrapped as a cause for errors.As unwrapping.
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

// exactSessionTarget returns session as an exact-match target ("=<name>").
// Without "=" prefix, tmux may prefix-match a sibling worktree's session.
func exactSessionTarget(session string) string {
	return "=" + session
}

// exactSessionWindowTarget returns session as an exact-match window/pane target
// ("=<name>:"). Window/pane parsers reject bare "=<name>"; they need the ":".
func exactSessionWindowTarget(session string) string {
	return "=" + session + ":"
}

// ListSessions returns every session name live on the -L tmuxPath socket named socketKey,
// or an error if the round trip itself failed.
//
// It is the one engine-less, exported function in this package: every other exported method
// hangs off *Engine and is bound to one session, but the watchdog daemon (internal/reedcli) must
// enumerate a hub socket's sessions BEFORE it has built any Engine for any of them — there is
// nothing yet to bind a method call to. TmuxCmd.run and TmuxCmd.output stay unexported; this is
// the one seam this package opens for that discovery, built on the identical
// `tmux -L <socket> list-sessions -F '#{session_name}'` invocation five call sites in this
// package already issue.
//
// Telling ListSessions the binary rather than having it load a config keeps the Told-Geometry
// Invariant intact from this side too: every session on one hub socket shares one tmux binary, so
// naming it explicitly is the only coherent answer for an enumeration that spans sessions.
//
// The three outcomes a caller can distinguish are: a non-empty slice (sessions live), an empty
// slice with a nil error (exit 0, no sessions — a normal empty hub), and a non-nil error (the
// round trip itself failed — no server, unreachable socket, or another tmux failure). The error
// is returned unwrapped beyond output's own wrapTmuxError.
func ListSessions(tmuxPath, socketKey string) ([]string, error) {
	return listSessionsVia(NewTmuxCmd(tmuxPath, socketKey))
}

// listSessionsVia is ListSessions' implementation, taking an already-built TmuxCmd rather than raw
// binary/socket strings — the split exists so a test can drive the parsing half through TmuxCmd's
// execHook seam directly, without a live server.
func listSessionsVia(cmd TmuxCmd) ([]string, error) {
	out, err := cmd.output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

// hasSession reports whether the named session exists (by exact match, not prefix).
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

// listPanes returns all panes in the session (by exact match).
func (p TmuxCmd) listPanes(session string) ([]LivePane, error) {
	out, err := p.output("list-panes", "-t", exactSessionWindowTarget(session), "-F", "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}")
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}
