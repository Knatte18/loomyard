// io.go implements the pane-transport engine ops shuttle drives directly:
// SendText/SendKey write into a strand's live pane, and CapturePane reads its
// current screen contents back. None of these three reconciles, re-renders,
// or persists — they are pure transport/query wrapped around the same
// resolveLivePaneID lookup every one of them shares, matching the
// dumb-carrier contract the rest of the package follows: mux moves bytes in
// and out of a pane, it never interprets them.

package muxengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

// resolveLivePaneID looks up guid in st.Strands and returns its bound pane
// id, or an error naming guid when the strand is unknown, still
// anchor:hidden (never launched, so it has no pane), or otherwise carries an
// empty PaneID (registered but not yet realized into a pane). Every
// pane-transport op (SendText, SendKey, CapturePane) shares this single
// resolution so their unknown/hidden/unbound error messages stay identical.
func resolveLivePaneID(st *MuxState, guid string) (string, error) {
	strand, ok := strandByGUID(st.Strands, guid)
	if !ok {
		return "", fmt.Errorf("unknown strand %q", guid)
	}
	if strand.Display.Anchor == render.AnchorHidden {
		return "", fmt.Errorf("strand %q is hidden; no pane to target", guid)
	}
	if strand.PaneID == "" {
		return "", fmt.Errorf("strand %q has no live pane", guid)
	}
	return strand.PaneID, nil
}

// SendText types text into guid's live pane as a literal string (never
// reinterpreted as psmux flags or key names) and, when submit is true,
// follows it with a separate Enter — the exact two-step send-keys pattern
// launchStrandLocked uses to run a strand's launch command. SendText is pure
// transport: it does not reconcile, re-render, or persist, so a caller
// driving many sends in a tight loop pays no layout-apply cost per call. The
// whole lookup-then-send sequence runs under the op lock, the same
// discipline every other public op follows, so a concurrent mutation can
// never resolve a pane id that a racing remove then invalidates before the
// send lands.
func (e *Engine) SendText(guid, text string, submit bool) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		paneID, err := resolveLivePaneID(st, guid)
		if err != nil {
			return err
		}

		if err := e.psmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(text)); err != nil {
			return fmt.Errorf("send text: %w", err)
		}
		if submit {
			if err := e.psmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
				return fmt.Errorf("submit text: %w", err)
			}
		}
		return nil
	})
}

// SendKey sends a single named key (e.g. "Enter", "Escape") into guid's live
// pane WITHOUT the -l literal flag, so psmux interprets it as a key name
// rather than typing it verbatim — the opposite of SendText's literal
// transport. Like SendText, it is pure transport (no reconcile, re-render,
// or persist) and runs its lookup-then-send sequence under the op lock.
func (e *Engine) SendKey(guid, key string) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		paneID, err := resolveLivePaneID(st, guid)
		if err != nil {
			return err
		}

		if err := e.psmux.run("send-keys", "-t", paneID, key); err != nil {
			return fmt.Errorf("send key %q: %w", key, err)
		}
		return nil
	})
}
