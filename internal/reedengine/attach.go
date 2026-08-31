// attach.go owns the attach argv both internal/reedcli and internal/loomcli build their terminal
// handover from: AttachArgv composes the whole told-geometry pre-flight (the two option pins, both
// effective-value readbacks, the state read, the pane list, the layout guards, and the layout plan)
// behind a single withOpLock acquisition, and degrades to today's bare attach-session argv on every
// failure. The builder never refuses: attach is the operator's escape hatch into a session, including
// a broken one, so no engine-side failure here may ever block the handover.

package reedengine

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// errAttachChainSuppressed names a benign, expected degradation: some precondition for chaining
// select-layout onto the attach argv was not met, so AttachArgv falls back to the bare argv. It is
// never returned to a caller outside this file — withOpLock's closure returns it purely so the single
// warn-and-degrade tail at the bottom of AttachArgv has one place to log from.
var errAttachChainSuppressed = errors.New("attach chain suppressed")

// bareAttachArgv returns the five-element tmux argv both internal/reedcli and internal/loomcli build
// today for an in-place attach: "-L", socket, "attach-session", "-t", the exact session target.
// A fresh slice is returned on every call — never a shared package-level slice — since a caller may
// append to it.
func bareAttachArgv(socket, session string) []string {
	return []string{"-L", socket, "attach-session", "-t", exactSessionTarget(session)}
}

// chainedAttachArgv returns the ten-element argv that chains a client-sized select-layout onto the
// bare attach: the five elements of bareAttachArgv, then the literal one-character element ";", then
// "select-layout", "-t", the exact session/window target, and layout.
//
// The separator is a literal single-character ";" argv element, never "\\;" — exec.Command passes
// argv directly and never sees a shell, so a backslash would be passed through as a literal
// backslash-semicolon and tmux would not read it as a command separator.
//
// The chained select-layout carries its own explicit -t "=<session>:" target rather than relying on
// whichever window the new client lands in, matching the exact-target discipline every other reed
// call site follows.
func chainedAttachArgv(socket, session, layout string) []string {
	bare := bareAttachArgv(socket, session)
	out := make([]string, 0, len(bare)+5)
	out = append(out, bare...)
	out = append(out, ";", "select-layout", "-t", exactSessionWindowTarget(session), layout)
	return out
}

// AttachArgv builds the tmux argv for an in-place attach, chaining a client-sized select-layout onto
// it when the current session geometry safely supports one.
//
// AttachArgv returns no error, by contract: attach is the operator's escape hatch into a session,
// including a broken one, so no engine-side failure may ever block the handover. Every precondition
// this builder checks — the session existing, the geometry option pins and their readbacks, the
// persisted state, the live pane list, both layout guards, and the layout plan itself — degrades to
// the bare attach-session argv on failure, logged via logger.Warn rather than surfaced as an error.
// The pre-flight also lists the session's currently attached clients, but that listing is the one
// step that is not a precondition at all: it gates nothing and can never suppress the chain, so it
// belongs in this sentence only as the pre-flight's complete inventory, not as another degrade path.
//
// cols and rows are the attaching client's own terminal size, in columns and rows; a non-positive
// value means no client size is known, and AttachArgv returns the bare argv immediately without
// taking the lock.
//
// The pre-flight also refreshes the session's window-resized resize-pin hook, computed against the
// same told box the chained layout is. This is what corrects a later client resize, and — on a
// session whose earlier apply already installed the hook — a degraded bare attach too. A degrade
// return installs nothing: the uncovered window is a session between "up" and its first placed
// strand, which has nothing to pin anyway because a lone header pane takes render.Rules' sole-cell
// branch.
func (e *Engine) AttachArgv(cols, rows int) []string {
	bare := bareAttachArgv(e.Socket(), e.SessionName())

	if cols <= 0 || rows <= 0 {
		logger.Warn("reed: no client terminal size available, attaching without a chained layout", "socket", e.Socket(), "session", e.SessionName(), "cols", cols, "rows", rows)
		return bare
	}

	var chained []string
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		e.warnMismatchedClientsLocked(cols, rows)

		// The pins are made here, by the builder itself, not by a second exported call a CLI must
		// remember. The ordering is load-bearing: the told box is only correct once "status off" has
		// landed, since that is what makes the post-attach window equal the client's rows rather than
		// rows - 1.
		e.pinGeometryOptionsLocked()

		if !e.readWindowSizeLatestLocked() {
			// Anything other than "latest" means the post-attach window will not become the client's
			// size, so the told box's whole premise has failed and chaining would hand tmux a wrong-height
			// string to rescale — worse than not chaining.
			return errAttachChainSuppressed
		}

		reserved, ok := e.readStatusRowsLocked()
		if !ok {
			// A #{status} that reads back as something other than "off" does NOT suppress the chain: the
			// reserved-row count is simply taken from that value instead. Only an unrecognized value
			// (readStatusRowsLocked's ok == false) suppresses it.
			return errAttachChainSuppressed
		}

		// Read-only with respect to reed.json: this builder never calls SaveState.
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		live, err := e.tmux.listPanes(e.SessionName())
		if err != nil {
			return err
		}

		// Reproduce applyLayoutLocked's two skip guards exactly. These are load-bearing, not stylistic —
		// a layout string enumerating zero panes is accepted by tmux (exit 0) and answered by destroying
		// every pane in the session, and an attach that wipes the session it is attaching to would be a
		// far worse bug than the one being fixed.
		if len(live) < 2 {
			return errAttachChainSuppressed
		}
		if !anyPlacedStrand(st.Strands, liveIDSet(live)) {
			return errAttachChainSuppressed
		}

		// Floor reserved at rows-1 so the box height is never non-positive: an oversized #{status}
		// readback (e.g. a multi-line status bar) must not hand planLayout/render.Rules a zero or
		// negative height. render.Rules already floors every cell at 1, but that degrade must be a
		// deliberate one-row-remaining box here, not an accidental non-positive input.
		if reserved > rows-1 {
			reserved = rows - 1
		}

		// The box is the TOLD client box; liveBoxLocked must not be called anywhere on this path,
		// because at argv-build time the live window is still the pre-attach size and would be exactly
		// the wrong answer. The focus target is deliberately discarded: the chain carries select-layout
		// only, never select-pane.
		box := render.Box{X: 0, Y: 0, W: cols, H: rows - reserved}
		layout, _, err := e.planLayout(st, live, box)
		if err != nil {
			return err
		}

		e.installResizePinsLocked(e.fixedHeightPins(st, live, box))

		chained = chainedAttachArgv(e.Socket(), e.SessionName(), layout)
		return nil
	})
	if err != nil {
		if errors.Is(err, errAttachChainSuppressed) {
			logger.Warn("reed: attach chain suppressed, attaching without a chained layout", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		} else {
			logger.Warn("reed: attach pre-flight failed, attaching without a chained layout", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
		return bare
	}

	return chained
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
// Per line, following parseWindowSize's strictness discipline: a line yields a client only when it
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

// warnMismatchedClientsLocked lists this session's currently attached clients and logs one
// logger.Warn per client whose size differs from cols/rows, the size this attach was told. It
// returns nothing and never blocks the chain: this is the uncovered subset of the resize-dot-fill
// root-cause model (see the discussion's root-cause-model decision) — tmux is correctly painting
// real estate no other client's window can cover, and the warning's whole job is to name the
// specific other terminal an operator should go look at, not to repair anything.
//
// It is called deliberately ahead of every in-closure degrade return in AttachArgv — before
// pinGeometryOptionsLocked and everything after it — so the warning still fires on an attach whose
// chain is later suppressed. That is precisely the attach an operator is most likely to be confused
// by: a bare (unchained) attach gives no other hint that a second client is holding the window at a
// different size.
//
// A round-trip error is logged via logger.Warn, naming the socket, the session, and the error, and
// warnMismatchedClientsLocked returns without emitting any per-client line — exactly the
// Shared Decision geometry-tmux-failures-are-non-fatal-everywhere already governs the rest of this
// package's tmux calls. It never returns an error and never introduces a degrade path.
func (e *Engine) warnMismatchedClientsLocked(cols, rows int) {
	out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", "#{client_name} #{client_width} #{client_height}")
	if err != nil {
		logger.Warn("reed: failed to list attached clients, skipping the multi-client size check", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return
	}
	for _, client := range parseClientList(out) {
		if client.Width == cols && client.Height == rows {
			continue
		}
		logger.Warn("reed: another client is attached at a different size; tmux must pick one window size, so the mismatched client shows tmux padding until it is resized or detached", "socket", e.Socket(), "session", e.SessionName(), "client", client.Name, "clientWidth", client.Width, "clientHeight", client.Height, "toldCols", cols, "toldRows", rows)
	}
}
