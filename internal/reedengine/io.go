// io.go implements the pane-transport engine ops shuttle drives directly: SendText/SendKey/PasteSubmit write
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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// The pane user options a wait mark is stored in.
const (
	waitOption      = "@lyx_wait"
	waitStartOption = "@lyx_wait_start"
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

	live, err := e.listStrandPanes(st)
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

// PasteSubmit pastes text into guid's live pane as one bracketed paste and, when submit is true, sends Enter right behind it.
// The paste and the Enter reach the pane in one tmux invocation, so nothing the operator types lands between them.
// The text, of any length, is loaded from standard input.
// It does not reconcile, re-render, or persist — pure transport.
func (e *Engine) PasteSubmit(guid, text string, submit bool) error {
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

		return e.pasteSubmitLocked(paneID, text, submit)
	})
}

// pasteSubmitLocked pastes text into paneID through a fresh named tmux buffer, so the operator's automatic paste-buffer stack is untouched.
// The caller holds the op lock and has resolved paneID.
// A failed invocation stops before the paste's own delete, so the buffer is then deleted by a best-effort command of its own, whose error is dropped.
func (e *Engine) pasteSubmitLocked(paneID, text string, submit bool) error {
	buffer, err := injectBufferName()
	if err != nil {
		return err
	}
	if err := e.tmux.runStdin([]byte(text), pasteSubmitArgv(paneID, buffer, submit)...); err != nil {
		_ = e.tmux.run("delete-buffer", "-b", buffer)
		return fmt.Errorf("paste text: %w", err)
	}
	return nil
}

// pasteSubmitArgv returns the tmux command list that fills buffer from standard input, pastes it into paneID and deletes the buffer, then sends Enter when submit is true.
// The commands are joined by the literal one-character ";" element chainedAttachArgv documents.
// The text arrives on standard input and so never adds, ends or alters a command in the list; -p makes tmux send a bracketed paste when the pane's application enabled it.
func pasteSubmitArgv(paneID, buffer string, submit bool) []string {
	argv := []string{
		"load-buffer", "-b", buffer, "-",
		";", "paste-buffer", "-p", "-d", "-b", buffer, "-t", paneID,
	}
	if submit {
		argv = append(argv, ";", "send-keys", "-t", paneID, "Enter")
	}
	return argv
}

// injectBufferName returns a fresh tmux buffer name, "lyx-inject-" plus 8 lowercase hex characters.
func injectBufferName() (string, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("name paste buffer: %w", err)
	}
	return "lyx-inject-" + hex.EncodeToString(raw[:]), nil
}

// OperatorActive reports whether an attached client has been active on guid's live pane within the configured quiet window, and that window.
// It is read-only: no reconcile, re-render, or persist.
// A failed client listing is returned as the error, and a caller decides whether to hold its write on it.
func (e *Engine) OperatorActive(guid string) (active bool, quiet time.Duration, err error) {
	err = e.withOpLock(func() error {
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

		active, quiet, err = e.operatorActiveLocked(paneID)
		return err
	})
	return active, quiet, err
}

// operatorActiveLocked is OperatorActive over an already-resolved paneID, for a caller that holds the op lock.
func (e *Engine) operatorActiveLocked(paneID string) (active bool, quiet time.Duration, err error) {
	quiet = operatorQuiet(e.cfg)
	out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", "#{pane_id} #{client_activity}")
	if err != nil {
		return false, quiet, fmt.Errorf("list clients: %w", err)
	}
	return activeClientOnPane(out, paneID, time.Now(), quiet), quiet, nil
}

// activeClientOnPane reports whether any line of clients, one "<pane_id> <client_activity epoch seconds>" per client, names paneID with an activity later than now minus quiet.
// A malformed line is skipped.
func activeClientOnPane(clients, paneID string, now time.Time, quiet time.Duration) bool {
	threshold := now.Add(-quiet).Unix()
	for _, line := range strings.Split(clients, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != paneID {
			continue
		}
		activity, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		if activity > threshold {
			return true
		}
	}
	return false
}

// defaultOperatorQuietS is the quiet window, in seconds, a Config without a positive operator_quiet_s takes.
const defaultOperatorQuietS = 5

// operatorQuiet returns cfg's operator quiet window, floored to the default for a zero or negative value.
func operatorQuiet(cfg Config) time.Duration {
	if cfg.OperatorQuietS <= 0 {
		return defaultOperatorQuietS * time.Second
	}
	return time.Duration(cfg.OperatorQuietS) * time.Second
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

// SetWaitMark sets guid's pane wait mark to label, started at start, or clears it when label is empty.
// The mark is two pane user options, @lyx_wait (the label) and @lyx_wait_start (epoch seconds),
// which the status bar renders on the strand's button and which die with the pane.
// It is display only: nothing is persisted,
// and a failure is returned for the caller to log.
func (e *Engine) SetWaitMark(guid, label string, start time.Time) error {
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

		if label == "" {
			for _, option := range []string{waitOption, waitStartOption} {
				if err := e.tmux.run("set-option", "-p", "-u", "-t", paneID, option); err != nil {
					return fmt.Errorf("clear wait mark: %w", err)
				}
			}
			return nil
		}
		if err := e.tmux.run("set-option", "-p", "-t", paneID, waitOption, label); err != nil {
			return fmt.Errorf("set wait mark: %w", err)
		}
		if err := e.tmux.run("set-option", "-p", "-t", paneID, waitStartOption, strconv.FormatInt(start.Unix(), 10)); err != nil {
			return fmt.Errorf("set wait mark start: %w", err)
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
