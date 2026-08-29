// reapply.go owns the watchdog's single re-apply op — the only place the watch loop ever reaches
// tmux. reapplyLayout is structurally Status()'s lock-then-load-then-list shape (lifecycle.go) with
// the layout apply chained onto the end, plus the hook-availability probe batch 3's mode transition
// reads.

package reedengine

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// ReapplyResult reports one reapplyLayout call's outcome.
type ReapplyResult struct {
	// Deferred is true when the op lock was held by someone else, so nothing ran.
	Deferred bool
	// Applied is true only when select-layout was actually issued.
	Applied bool
	// Box is the box the layout was planned against. Meaningful only when BoxIsLive.
	Box render.Box
	// BoxIsLive reports whether Box was a real observation rather than
	// liveBoxLocked's configured fallback, or no query at all.
	BoxIsLive bool
	// HookInstalled reports whether this session's window-resized hook array
	// carries, as one whole entry, exactly reed's own command string for this
	// worktree's signal path.
	HookInstalled bool
	// HookKnown reports whether HookInstalled was decided at all this call.
	HookKnown bool
}

// hookArrayEntries splits one `show-options -v <hook-name>` answer into the hook array's entries.
//
// tmux 3.6 prints EVERY entry of an array option under -v, one per line, in index order, with the
// "<name>[N]" prefix that plain show-options would carry suppressed (live-verified) — so the answer
// for a window-resized array holding two pins and reed's touch is three lines, not one. A trailing
// newline yields a trailing empty entry, which no caller can match against a non-empty command
// string, so it is left in place rather than special-cased.
func hookArrayEntries(raw string) []string {
	return strings.Split(raw, "\n")
}

// hookInstalledLocked reports whether one entry of this session's window-resized hook array is
// exactly reed's own command string for this worktree's signal path (installed), and whether that was
// decided at all this call (known). It is called only when a caller of reapplyLayout asks for a probe
// (see reapplyLayout's probeHook parameter).
//
// On runtime.GOOS == "windows" it returns (false, false) immediately, issuing NO round trip — Windows
// is poll-only unconditionally, because set-hook/run-shell are absent from requiredSubcommands and
// psmux's support for them is unverified, and a hook that installs but never fires would pin the
// watcher in signal mode forever with zero self-heal.
//
// Otherwise it reads back with `show-options -v`, never `show-hooks`: hooks are options in tmux 3.6,
// and show-hooks prints nothing for a session-scoped hook that demonstrably fires (live-verified).
// show-options is absent from requiredSubcommands, and that is acceptable precisely because every
// failure shape here yields known == false and therefore poll mode, so no capability-probe change is
// needed and no psmux risk is taken.
//
// The match against resizeHookCommand is exact against reed's own command string for THIS worktree's
// signal path, never merely "some window-resized hook exists" (also live-verified as necessary): a
// foreign hook or a sibling worktree's signal path would deliver nothing this watcher can consume.
// It is exact PER ENTRY rather than against the whole answer, because reed's own array normally holds
// one resize-pane pin per fixed-height pane ahead of the touch (windowsize.go's resizePinHookArgvs) —
// an equality test against the whole multi-line answer therefore reports "absent" on every healthy
// session with anything pinned, which is exactly the bug this probe shipped with. Exactness stays
// per-entry all the same: a substring search over the raw answer would accept a foreign entry that
// merely embeds reed's command string.
//
// Entry POSITION is deliberately not part of the match. installResizePinsLocked always writes the
// touch last, but what this probe has to answer is only "will a resize touch THIS worktree's signal
// file", and a foreign entry appended after reed's own does not change that answer — while demanding
// the last position would silently drop such a session into poll mode forever.
func (e *Engine) hookInstalledLocked() (installed bool, known bool) {
	if runtime.GOOS == "windows" {
		return false, false
	}
	out, err := e.tmux.output("show-options", "-v", "-t", exactSessionWindowTarget(e.SessionName()), windowResizedHookName)
	if err != nil {
		logger.Debug("reed: failed to read back window-resized hook", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false, false
	}
	own := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	for _, entry := range hookArrayEntries(out) {
		if entry == own {
			return true, true
		}
	}
	return false, true
}

// reapplyLayout is the watchdog's single re-apply op: it tries the (non-blocking) op lock, and, if
// acquired, loads state, lists live panes, optionally probes the window-resized hook, and re-applies
// the layout with the focus half suppressed and a box-equality guard against lastApplied.
//
// probeHook is what keeps the design's "signal-driven mode never re-probes" rule literally true.
// Without it the show-options round trip would fire on EVERY re-apply in both modes, so a signal-mode
// watcher would re-probe once per resize — suppressing only the mode TRANSITION, not the round trip,
// which is not what the rule says and not what it is for. probeHook == false yields
// HookInstalled == false AND HookKnown == false, meaning "not asked" rather than "asked and absent" —
// the same undecided shape a deferral produces, which is exactly right, since a caller that did not
// ask has learned nothing.
//
// When probeHook is true, the probe runs AFTER listPanes and BEFORE the apply, so that a session the
// apply guards skip (fewer than two panes, or no strand owning a present pane) still decides the
// mode — otherwise a watcher on such a session could never promote out of poll mode.
//
// reapplyLayout persists nothing: it never calls SaveState and never writes reed.json. It inherits
// applyLayoutLockedOpts's two session-survival guards rather than re-deriving them — which matters
// more here than anywhere else, since the watcher fires unattended with no operator watching the
// envelope — and it owns the box-equality guard itself so the comparison happens under the same lock
// as the query that produced the box.
//
// reapplyLayout is never exported, never acquires a second lock, and never queries geometry outside
// applyLayoutLockedOpts.
func (e *Engine) reapplyLayout(lastApplied render.Box, probeHook bool) (ReapplyResult, error) {
	var result ReapplyResult
	acquired, err := e.withTryOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}
		live, err := e.tmux.listPanes(e.SessionName())
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}

		if probeHook {
			installed, known := e.hookInstalledLocked()
			result.HookInstalled = installed
			result.HookKnown = known
		}

		applyRes, err := e.applyLayoutLockedOpts(st, live, applyOpts{SkipFocus: true, SkipWhenBoxEquals: &lastApplied})
		result.Applied = applyRes.Applied
		result.Box = applyRes.Box
		result.BoxIsLive = applyRes.BoxIsLive
		return err
	})
	if !acquired && err == nil {
		return ReapplyResult{Deferred: true}, nil
	}
	if err != nil {
		return result, err
	}
	return result, nil
}
