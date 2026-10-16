// attach.go owns the attach argv both internal/reedcli and internal/loomcli build their terminal handover from.
// AttachArgv runs the attach pre-flight (the option pins, the strand-window marks, the state read and the window selection) behind a single withOpLock acquisition.
// It always returns the bare attach-session argv with the window selection appended.
// The builder never refuses: attach is the operator's escape hatch into a session, including
// a broken one, so no engine-side failure here may ever block the handover.
// internal/reedcli's attach pre-flight now boots the session (EnsureSession) before this builder ever
// runs, so AttachArgv's own requireSessionLocked call below ordinarily finds a session already up —
// but this builder does no booting of its own: it returns no error by contract, and it stays read-only
// with respect to .lyx/reed.json.

package reedengine

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// bareAttachArgv returns the five-element tmux argv both internal/reedcli and internal/loomcli build
// today for an in-place attach: "-L", socket, "attach-session", "-t", the exact session target.
// A fresh slice is returned on every call — never a shared package-level slice — since a caller may
// append to it.
func bareAttachArgv(socket, session string) []string {
	return []string{"-L", socket, "attach-session", "-t", exactSessionTarget(session)}
}

// AttachArgv builds the tmux argv for an in-place attach: the bare attach-session argv with the window selection appended.
//
// AttachArgv returns no error, by contract: attach is the operator's escape hatch into a session,
// including a broken one, so no engine-side failure may ever block the handover. Every precondition
// this builder checks — the session existing, the persisted state and the geometry option pins — degrades to
// the bare attach-session argv on failure, logged via logger.Warn rather than surfaced as an error.
// The pre-flight also lists the session's currently attached clients, but that listing is the one
// step that is not a precondition at all: it gates nothing, so it
// belongs in this sentence only as the pre-flight's complete inventory, not as another degrade path.
//
// cols and rows are the attaching client's own terminal size, in columns and rows; the pre-flight compares them with every other attached client's size and warns on a mismatch.
// A non-positive value means no client size is known, and no mismatch is warned about.
//
// Right after the shell window check, the pre-flight resolves the attach's window selection: a trailing `; select-window -t <relevant window>`, the newest live agent window, else the overview, never the shell.
// The selection is appended to every argv this returns past the state read.
// When another client is attached, or the client listing fails, the selection is kept only while the session's follow state reads on, and dropped while it reads off or its read fails; it is dropped too when no window is relevant.
// Failing to take the lock, read the session or the state still drops it, each logged at Warn.
// A race with the operator's own window change costs at most one switch.
//
// The pre-flight re-pins the geometry options, which also clears the `window-resized` hook an older binary pinned on the shell window.
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

		// A session booted by an older lyx gets its shell window named and placed at the next attach.
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

		// Strands and sessions spawned by an older lyx get their display options at the next attach.
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

// attachedClient is one line of a `list-clients -F '#{client_name} #{client_width}
// #{client_height}'` answer: a tmux client attached to this session and the terminal size it is
// currently attached at.
type attachedClient struct {
	Name   string
	Width  int
	Height int
}

// parseClientList parses a `list-clients -F '#{client_name} #{client_width} #{client_height}'`
// answer into one attachedClient per well-formed line. It performs no I/O and no logging.
//
// Per line: a line yields a client only when it
// has exactly three whitespace-separated fields and both size fields parse as strictly positive
// integers. Every other line shape — blank, one field, two fields, four or more, non-numeric, zero,
// or negative — is skipped rather than reported, so one malformed line among several well-formed
// ones never discards them. The whole answer is trimmed before splitting on lines, so a trailing
// newline yields no phantom entry.
//
// The returned slice is empty, never nil, when no line parses — never a nil-versus-empty
// ambiguity a caller would have to special-case.
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

// warnMismatchedClientsLocked lists this session's currently attached clients and logs one logger.Warn per client whose size differs from cols/rows, the size this attach was told.
// With no client size known it warns about no mismatch.
// It reports whether another client is attached, a failed listing counting as one, which AttachArgv's selection gate reads.
// It never blocks the attach: tmux is correctly painting real estate no other client's window can cover,
// and the warning's whole job is to name the specific other terminal an operator should go look at, not to repair anything.
//
// It is called deliberately ahead of every in-closure degrade return in AttachArgv, before the state read and everything after it, so the warning still fires on an attach whose pre-flight later fails.
//
// A round-trip error is logged via logger.Warn, naming the socket, the session, and the error, and warnMismatchedClientsLocked emits no per-client line.
// That is exactly the Shared Decision geometry-tmux-failures-are-non-fatal-everywhere that already governs the rest of this package's tmux calls.
// It never returns an error and never introduces a degrade path.
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
