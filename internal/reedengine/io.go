// io.go implements the pane-transport engine ops shuttle drives directly: SendText/SendKey write
// into a strand's live pane,
// and CapturePane reads its current screen contents back.
// None of these three reconciles, re-renders, or persists — they are pure transport/query wrapped
// around the same resolvePaneInThisSessionLocked lookup every one of them shares, matching the
// dumb-carrier contract the rest of the package follows: reed moves bytes in and out of a pane, it
// never interprets them.
// That shared lookup does make one read-only tmux query, to confirm the persisted pane id names a
// pane of THIS worktree's session — see resolvePaneInThisSessionLocked for why a pane id alone is
// not a safe target on a socket every worktree in the hub shares.
//
// CapturePane in particular follows Status's read-only discipline: a query must never move input
// focus or mutate persisted state as a side effect of being asked a question.

package reedengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// resolveLivePaneID looks up guid in st.Strands and returns its bound pane
// id, or an error naming guid when the strand is unknown, still
// anchor:hidden (never launched, so it has no pane), or otherwise carries an
// empty PaneID (registered but not yet realized into a pane). Every
// pane-transport op (SendText, SendKey, CapturePane) shares this single
// resolution so their unknown/hidden/unbound error messages stay identical.
func resolveLivePaneID(st *ReedState, guid string) (string, error) {
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

// resolvePaneInThisSessionLocked resolves guid's bound pane id and refuses one that is not present
// in THIS worktree's session, which is the only pane set this engine may address.
//
// The refusal is not defensive tidiness. The tmux socket is per HUB, shared by every worktree in it,
// and tmux pane ids are server-global — so a pane id a stale or copied reed.json carries is very
// often a VALID, addressable id belonging to a sibling worktree's live session, and send-keys /
// capture-pane against it succeed. R5 review finding R5-F4 reproduced the destructive twin of this
// at the remove path (a `lyx reed remove` in one worktree killed a sibling's strand pane and its
// process, reporting ok:true); the transport ops are the same exposure with a quieter symptom —
// one agent's input typed into another agent's pane.
//
// The pane-generation guard (generation.go) already discards such bindings at load, so in practice
// this check is reached only when that guard failed open (a tmux probe hiccup, or a state file
// written before the stamp existed). It costs one list-panes round trip per transport op, which is
// the right price for a target this consequential to be wrong about, and it keeps these ops
// read-only: it confirms membership, it does not reconcile, re-render, or persist.
func (e *Engine) resolvePaneInThisSessionLocked(st *ReedState, guid string) (string, error) {
	paneID, err := resolveLivePaneID(st, guid)
	if err != nil {
		return "", err
	}

	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		return "", fmt.Errorf("list panes: %w", err)
	}
	if !liveIDSet(live)[paneID] {
		return "", fmt.Errorf(
			"strand %q is bound to pane %s, which is not a pane of this worktree's session %q — reed state is stale (pane ids are shared across every worktree on this hub's tmux server); run \"lyx reed resume\" to rebind it",
			guid, paneID, e.SessionName())
	}
	return paneID, nil
}

// SendText types text into guid's live pane and optionally submits it with Enter.
// It does not reconcile, re-render, or persist — pure transport.
func (e *Engine) SendText(guid, text string, submit bool) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		paneID, err := e.resolvePaneInThisSessionLocked(st, guid)
		if err != nil {
			return err
		}

		if err := e.tmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(text)); err != nil {
			return fmt.Errorf("send text: %w", err)
		}
		if submit {
			if err := e.tmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
				return fmt.Errorf("submit text: %w", err)
			}
		}
		return nil
	})
}

// SendKey sends a named key (e.g. "Enter") into guid's live pane.
// Like SendText, it is pure transport with no reconcile, re-render, or persist.
func (e *Engine) SendKey(guid, key string) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		paneID, err := e.resolvePaneInThisSessionLocked(st, guid)
		if err != nil {
			return err
		}

		if err := e.tmux.run("send-keys", "-t", paneID, key); err != nil {
			return fmt.Errorf("send key %q: %w", key, err)
		}
		return nil
	})
}

// CapturePane returns guid's live pane's current screen contents.
// It is read-only: no reconcile, re-apply, or persist.
func (e *Engine) CapturePane(guid string) (string, error) {
	var captured string
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		paneID, err := e.resolvePaneInThisSessionLocked(st, guid)
		if err != nil {
			return err
		}

		out, err := e.tmux.output("capture-pane", "-p", "-t", paneID)
		if err != nil {
			return fmt.Errorf("capture pane: %w", err)
		}
		captured = out
		return nil
	})
	return captured, err
}
