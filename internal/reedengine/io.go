// io.go holds the pane-transport ops that shuttle drives directly:
// SendText, SendKey, PasteSubmit, OperatorActive, and CapturePane.
// Each writes into or reads from a strand's live pane, and none reconciles, re-renders, or persists state.

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
	waitOption      = "@lym_wait"
	waitStartOption = "@lym_wait_start"
)

// resolveLivePaneID returns guid's bound pane id.
// It returns an error naming guid when the strand is unknown or its anchor is hidden.
// A hidden strand was never launched, so it has no pane.
// A strand with an empty PaneID is registered but not yet realized into a pane, and that is an error too.
// The pane-transport ops share this resolution, so their error messages stay identical.
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

// resolvePaneInThisSessionLocked returns guid's bound pane id and refuses one outside this worktree's session.
// The tmux socket is shared by every worktree on a hub, and tmux pane ids are server-global.
// A stale or copied reed.json can carry a pane id that belongs to a sibling worktree's live session.
// Send-keys or capture-pane against that pane then succeeds, so one agent's input lands in another agent's pane.
// The pane-generation guard in generation.go discards such bindings at load.
// This check is therefore reached only when that guard fails open:
// a tmux probe hiccup, or a state file written before the stamp existed.
// It costs one list-panes round trip per op.
// The check only confirms membership: it does not reconcile, re-render, or persist.
func (e *Engine) resolvePaneInThisSessionLocked(st *ReedState, guid string) (string, error) {
	paneID, err := resolveLivePaneID(st, guid)
	if err != nil {
		return "", err
	}

	live, windows, err := e.listStrandAndWindowPanes(st)
	if err != nil {
		return "", fmt.Errorf("list panes: %w", err)
	}
	if !liveIDSet(presentPanes(st.Strands, live, windows))[paneID] {
		return "", fmt.Errorf(
			"strand %q is bound to pane %s, which is not a pane of this worktree's session %q — reed state is stale (pane ids are shared across every worktree on this hub's tmux server); run \"lym reed resume\" to rebind it",
			guid, paneID, e.SessionName())
	}
	return paneID, nil
}

// SendText types text into guid's live pane and optionally submits it with Enter.
// It does not reconcile, re-render, or persist state.
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

// PasteSubmit pastes text into guid's live pane as one bracketed paste.
// When submit is true, it sends Enter right behind the paste.
// The paste and the Enter reach the pane in one tmux invocation, so nothing the operator types lands between them.
// The text, of any length, is loaded from standard input.
// It does not reconcile, re-render, or persist state.
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

// pasteSubmitLocked pastes text into paneID through a fresh named tmux buffer.
// The named buffer leaves the operator's automatic paste-buffer stack untouched.
// The caller holds the op lock and has resolved paneID.
// When the invocation fails, it can stop before the paste deletes the buffer,
// so a best-effort delete-buffer runs next and its error is dropped.
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

// pasteSubmitArgv returns the tmux command list that loads buffer from standard input and pastes it into paneID.
// The paste deletes the buffer, and an Enter follows when submit is true.
// The commands are joined by a literal ";" element, which tmux reads as a command separator.
// The text arrives on standard input, so it never adds, ends, or alters a command in the list.
// The -p flag makes tmux send a bracketed paste when the pane's application enabled it.
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

// injectBufferName returns a fresh tmux buffer name, "lym-inject-" plus 8 lowercase hex characters.
func injectBufferName() (string, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("name paste buffer: %w", err)
	}
	return "lym-inject-" + hex.EncodeToString(raw[:]), nil
}

// OperatorActive reports whether an attached client has been active on guid's live pane within the quiet window.
// It also returns that quiet window.
// It is read-only: it does not reconcile, re-render, or persist.
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

// operatorActiveLocked reports whether an attached client has been active on paneID within the quiet window.
// The caller holds the op lock and has resolved paneID.
func (e *Engine) operatorActiveLocked(paneID string) (active bool, quiet time.Duration, err error) {
	quiet = operatorQuiet(e.cfg)
	out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", "#{pane_id} #{client_activity}")
	if err != nil {
		return false, quiet, fmt.Errorf("list clients: %w", err)
	}
	return activeClientOnPane(out, paneID, time.Now(), quiet), quiet, nil
}

// activeClientOnPane reports whether a client on paneID has shown activity later than now minus quiet.
// clients holds one "<pane_id> <client_activity epoch seconds>" line per client.
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
// It does not reconcile, re-render, or persist state.
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
// The mark is two pane user options, `@lym_wait` (the label) and `@lym_wait_start` (epoch seconds).
// The status bar renders them on the strand's button, and they die with the pane.
// Nothing is persisted: a failed call returns an error for the caller to log.
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
	return e.capturePane(guid, false)
}

// CapturePaneStyled returns guid's live pane's current screen contents with their SGR attributes.
// The output is tmux's `capture-pane -e` form, returned verbatim; reed interprets none of the escapes.
// It is read-only: no reconcile, re-apply, or persist.
func (e *Engine) CapturePaneStyled(guid string) (string, error) {
	return e.capturePane(guid, true)
}

// capturePane reads guid's live pane, with its SGR attributes when styled.
func (e *Engine) capturePane(guid string, styled bool) (string, error) {
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

		args := []string{"capture-pane", "-p", "-t", paneID}
		if styled {
			args = []string{"capture-pane", "-e", "-p", "-t", paneID}
		}
		out, err := e.tmux.output(args...)
		if err != nil {
			return fmt.Errorf("capture pane: %w", err)
		}
		captured = out
		return nil
	})
	return captured, err
}
