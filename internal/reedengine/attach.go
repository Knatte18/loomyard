// attach.go builds the attach argv that internal/reedcli and internal/loomcli hand over to the terminal.
// AttachArgv runs the attach pre-flight behind a single withOpLock acquisition.
// It returns the bare attach-session argv with the window selection appended.
// The builder never refuses: attach is the operator's escape hatch into a session, including a broken one,
// so no engine-side failure here may block the handover.
// The attach pre-flight in internal/reedcli boots the session before this builder runs,
// so the session check in AttachArgv ordinarily finds it up.
// This builder boots nothing itself, returns no error by contract, and stays read-only with respect to .lym/reed.json.

package reedengine

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// bareAttachArgv returns the five-element tmux argv for an in-place attach:
// "-L", socket, "attach-session", "-t", and the exact session target.
// It returns a fresh slice on every call, since a caller may append to it.
func bareAttachArgv(socket, session string) []string {
	return []string{"-L", socket, "attach-session", "-t", exactSessionTarget(session)}
}

// AttachArgv builds the tmux argv for an in-place attach: the bare attach-session argv plus the window selection.
//
// AttachArgv returns no error, by contract.
// Attach is the operator's escape hatch into a session, even a broken one,
// so no engine-side failure may ever block the handover.
// The pre-flight checks the session, the persisted state and the geometry option pins.
// A failed check degrades to the bare attach-session argv, logged via logger.Warn rather than surfaced as an error.
// The pre-flight also lists the session's attached clients; that listing gates nothing and is no degrade path.
//
// cols and rows are the attaching client's terminal size, in columns and rows.
// The pre-flight compares them with every other attached client's size and warns on a mismatch.
// A non-positive value means no client size is known, and no mismatch is warned about.
//
// The pre-flight resolves the window selection right after the shell window check.
// The selection is a trailing `; select-window -t <window>` that names the newest live agent window.
// If there is none, it names the lowest-index overview window, never the shell.
// It is appended to every argv this returns past the state read.
// When another client is attached, or the client listing fails, the selection depends on the follow state.
// It is kept only while the session's follow state reads on, and dropped while it reads off or its read fails.
// The selection is also dropped when no window is relevant.
// Failing to take the lock, or to read the session or the state, drops it too, each logged at Warn.
// A race with the operator's own window change costs at most one switch.
//
// The pre-flight re-pins the geometry options, which also unpins the `window-resized` hook.
func (e *Engine) AttachArgv(cols, rows int) []string {
	bare := bareAttachArgv(e.Socket(), e.SessionName())

	var selection []string
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		anotherClient := e.warnMismatchedClientsLocked(cols, rows)

		// Read-only with respect to reed.json: this builder never calls SaveState.
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		// Sessions booted by an older lym get their shell window named and placed on attach.
		// This runs before every step that can fail, and also when no client size is known.
		e.ensureShellWindowLocked(st)

		// The window the attaching client lands on is resolved ahead of the steps below, so every argv past this point carries it.
		// Beside another attached client the selection is kept only while follow reads on; the follow state is read only then, and attach never writes it.
		if !anotherClient || e.followingLocked() {
			selection = e.attachWindowSelectionLocked(st)
		}

		windowTarget, err := e.strandWindowTargetFor(st)
		if err != nil {
			return err
		}

		// Strands and sessions spawned by an older lym get their display options re-asserted on attach.
		e.markStrandWindowLocked(windowTarget)
		if windowPanes, err := e.tmux.listPanes(windowTarget); err != nil {
			logger.Warn("reed: could not list the strand window's panes to re-assert strand options", "window", windowTarget, "err", err)
		} else {
			e.reassertStrandPaneOptionsLocked(st, windowPanes)
			e.markSelvagePaneLocked(st)
		}

		shellPath := ""
		if runtime.GOOS != "windows" {
			var err error
			if shellPath, err = resolveShellPath(e.cfg.Shell); err != nil {
				logger.Warn("reed: cannot resolve the configured shell, leaving the session's shell pins as they are", "key", "shell", "shell", e.cfg.Shell, "err", err)
			}
		}
		e.pinGeometryOptionsLocked(windowTarget, shellPath)
		return nil
	})
	if err != nil {
		logger.Warn("reed: attach pre-flight failed, attaching without the full pre-flight", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}
	return append(bare, selection...)
}

// attachedClient is one line of a `list-clients -F '#{client_name} #{client_width} #{client_height}'` answer:
// a tmux client attached to this session and the terminal size it is attached at.
type attachedClient struct {
	Name   string
	Width  int
	Height int
}

// parseClientList parses a `list-clients -F '#{client_name} #{client_width} #{client_height}'` answer.
// It yields one attachedClient per well-formed line, and performs no I/O and no logging.
//
// A line yields a client only when it has exactly three whitespace-separated fields.
// Its two size fields must parse as strictly positive integers.
// Every other line is skipped rather than reported, so one malformed line never discards the well-formed ones.
// The whole answer is trimmed before splitting on lines, so a trailing newline yields no phantom entry.
//
// The returned slice is empty, never nil, when no line parses, so callers need no nil check.
func parseClientList(out string) []attachedClient {
	clients := []attachedClient{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		width, errW := strconv.Atoi(fields[1])
		height, errH := strconv.Atoi(fields[2])
		if errW != nil || errH != nil || width <= 0 || height <= 0 {
			continue
		}
		clients = append(clients, attachedClient{Name: fields[0], Width: width, Height: height})
	}
	return clients
}

// warnMismatchedClientsLocked logs one logger.Warn per attached client whose size differs from cols and rows.
// Those are the size this attach was told.
// With no client size known it warns about no mismatch.
// It reports whether another client is attached, a failed listing counting as one.
// It never blocks the attach: tmux paints the real estate no other client's window can cover.
// The warning names the other terminal an operator should look at and repairs nothing.
//
// It runs before the state read in AttachArgv, so the warning still fires on an attach whose pre-flight later fails.
//
// A round-trip error is logged via logger.Warn with the socket, session and error; no per-client line follows.
// It never returns an error and never adds a degrade path, since tmux failures are non-fatal throughout this package.
func (e *Engine) warnMismatchedClientsLocked(cols, rows int) (anotherClient bool) {
	out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", "#{client_name} #{client_width} #{client_height}")
	if err != nil {
		logger.Warn("reed: failed to list attached clients, skipping the multi-client size check", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return true
	}
	sizeKnown := cols > 0 && rows > 0
	clients := parseClientList(out)
	for _, client := range clients {
		if !sizeKnown || (client.Width == cols && client.Height == rows) {
			continue
		}
		logger.Warn("reed: another client is attached at a different size; tmux must pick one window size, so the mismatched client shows tmux padding until it is resized or detached", "socket", e.Socket(), "session", e.SessionName(), "client", client.Name, "clientWidth", client.Width, "clientHeight", client.Height, "toldCols", cols, "toldRows", rows)
	}
	return len(clients) > 0
}

// ClientAttached reports whether any client is attached to this worktree's reed session.
// A missing session reads false with no error, and any other multiplexer failure is returned.
func (e *Engine) ClientAttached() (bool, error) {
	up, err := e.tmux.hasSession(e.SessionName())
	if err != nil {
		return false, fmt.Errorf("check session: %w", err)
	}
	if !up {
		return false, nil
	}
	out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", "#{client_name} #{client_width} #{client_height}")
	if err != nil {
		return false, fmt.Errorf("list clients: %w", err)
	}
	return len(parseClientList(out)) > 0, nil
}
