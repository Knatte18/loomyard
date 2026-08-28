// attach.go owns the attach argv both internal/reedcli and internal/loomcli build their terminal
// handover from: AttachArgv composes the whole told-geometry pre-flight (the two option pins, both
// effective-value readbacks, the state read, the pane list, the layout guards, and the layout plan)
// behind a single withOpLock acquisition, and degrades to today's bare attach-session argv on every
// failure. The builder never refuses: attach is the operator's escape hatch into a session, including
// a broken one, so no engine-side failure here may ever block the handover.

package reedengine

import (
	"errors"

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
