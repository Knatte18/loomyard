// selvagepane.go holds the Selvage-specific code of this package:
// the pane's create and heal lifecycle, the reconcile exemption policy and the strand-claim seed.
// Other files in the package call into these seams rather than carrying Selvage logic of their own.

package reedengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// reapPolicy answers the Selvage questions of a reconcile pass.
// It reports whether a pane is exempt from the dead-pane kill and whether it is exempt from the untracked-pane reap.
// It also reports whether the Selvage pane's own state authorizes the untracked reap to run at all.
// The zero value has an empty pane id and reports not-alive, matching a state with no Selvage pane recorded.
type reapPolicy struct {
	paneID string
	alive  bool
}

// newReapPolicy builds the reap policy for a reconcile pass.
// It reads st's recorded Selvage pane id and checks it against the current live pane set.
func newReapPolicy(st *ReedState, live []LivePane) reapPolicy {
	policy := reapPolicy{paneID: st.SelvagePaneID}
	if policy.paneID == "" {
		return policy
	}
	for _, p := range live {
		if p.ID == policy.paneID && !p.Dead {
			policy.alive = true
			break
		}
	}
	return policy
}

// exemptFromDeadKill reports whether paneID is Selvage's own pane, which the dead-pane kill loop must never schedule.
// Nothing outside up/resume rebuilds Selvage.
// Killing a pane_dead=1 Selvage here would leave the session without its always-on operator console.
// It would leave a stale SelvagePaneID in place until the next up/resume.
// A kept Selvage corpse stays enumerable.
// ensureSelvagePaneLocked heals it on the next up/resume by killing and re-splitting it.
// The non-empty guard is explicit rather than relying on a live pane id never being the empty string.
func (p reapPolicy) exemptFromDeadKill(paneID string) bool {
	return p.paneID != "" && p.paneID == paneID
}

// exemptFromUntrackedReap reports whether paneID is Selvage's own pane, which the untracked reap must never kill.
// This holds whether or not Selvage is alive: presence alone exempts it, a corpse included.
// Whether killing anything else is allowed is a separate question, answered by the aliveness of the Selvage pane.
// The two must not be folded together.
// The non-empty guard is explicit rather than relying on a live pane id never being the empty string.
func (p reapPolicy) exemptFromUntrackedReap(paneID string) bool {
	return p.paneID != "" && p.paneID == paneID
}

// authorizesReap reports whether an alive Selvage pane authorizes the untracked-pane reap to run.
// Killing an alive pane at worst corpses it under remain-on-exit,
// so the surviving Selvage always keeps the session alive.
// The reap fires from the strand add and update paths, and neither of those ever calls ensureSelvagePaneLocked.
// A dead-but-present Selvage must therefore not authorize reaping the session's only alive pane.
func (p reapPolicy) authorizesReap() bool {
	return p.paneID != "" && p.alive
}

// seedSelvageClaim adds st's Selvage pane id to claimed when it is non-empty.
// It encodes the rule that a strand may never own Selvage's pane: Selvage's own binding always seeds
// the claimed set before any strand's pane id is considered.
func seedSelvageClaim(st *ReedState, claimed map[string]bool) {
	if st.SelvagePaneID != "" {
		claimed[st.SelvagePaneID] = true
	}
}

// clearSelvagePaneBinding clears st's Selvage pane binding.
// It is the single writer of that clear.
// Every caller clears it for the same reason:
// a reborn or foreign session incarnation's reused pane id must never be mistaken for the still-live Selvage pane.
func clearSelvagePaneBinding(st *ReedState) {
	st.SelvagePaneID = ""
}

// ensureSelvagePaneLocked ensures Selvage exists and is alive.
// It (re)creates Selvage when it is missing, dead, or gone.
// Selvage is separate from strands and must land physically bottom-most.
// The (re)creation itself is done by splitSelvagePaneAtBottomLocked.
func (e *Engine) ensureSelvagePaneLocked(st *ReedState) error {
	session := e.SessionName()
	live, err := e.listStrandPanes(st)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}

	if len(live) == 0 {
		// A zero-pane husk cannot host a split: report it as an error, not a panic on the empty slice below.
		// ensureServerAndSessionLocked kills husks before this runs.
		// Reaching this means the session emptied between the two probes: an error, not an invariant.
		return fmt.Errorf("session %s has no panes to split Selvage from", session)
	}

	if st.SelvagePaneID != "" && aliveIDSet(live)[st.SelvagePaneID] {
		// Present and alive: an idempotent no-op across up and resume.
		// Aliveness, not mere presence, is the check.
		// A dead-but-present Selvage corpse must be healed here, not mistaken for a working Selvage.
		return nil
	}

	// A dead-but-present Selvage corpse is killed before the replacement is split,
	// so its bottom row is freed and the bottommost target below is a real, usually alive, pane.
	// The exception is a corpse that is the session's only pane: killing it first would end the session,
	// so the corpse itself is the split target and is killed after the new pane exists.
	corpseID := ""
	if st.SelvagePaneID != "" && liveIDSet(live)[st.SelvagePaneID] {
		corpseID = st.SelvagePaneID
		if len(live) > 1 {
			if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
				return fmt.Errorf("kill dead Selvage pane %s: %w", corpseID, err)
			}
			corpseID = ""
			live, err = e.listStrandPanes(st)
			if err != nil {
				return fmt.Errorf("list panes after killing dead Selvage: %w", err)
			}
			if len(live) == 0 {
				return fmt.Errorf("session %s has no panes to split Selvage from", session)
			}
		}
	}

	// Selvage's split-window argument is e.cfg.Shell, the same command new-session uses for the session's first pane.
	// It is an ordinary interactive shell, never a re-exec of lym.
	paneID, err := e.splitSelvagePaneAtBottomLocked(live, e.cfg.Shell)
	if err != nil {
		return fmt.Errorf("split Selvage pane: %w", err)
	}

	if corpseID != "" {
		// The corpse is the one the new Selvage was split off of, when it was the session's only pane.
		// Now that a second pane exists, killing it cannot end the session.
		// This is best-effort: a corpse that already vanished is fine,
		// but the discard still gets a Debug line so the step is observable without upgrading routine cleanup to a Warn.
		if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
			logger.Debug("reed: best-effort kill of Selvage corpse pane failed", "socket", e.Socket(), "pane", corpseID, "err", err)
		}
	}

	st.SelvagePaneID = paneID
	e.markSelvagePaneLocked(st)
	if err := SaveState(e.stateDir(), st); err != nil {
		return fmt.Errorf("persist Selvage pane id: %w", err)
	}
	return nil
}

// markSelvagePaneLocked sets the display-only console marker (consolePaneOption) on the recorded Selvage pane.
// It does nothing when no Selvage pane is recorded.
// A failure is logged and never fails the step.
func (e *Engine) markSelvagePaneLocked(st *ReedState) {
	if st.SelvagePaneID == "" {
		return
	}
	if err := e.tmux.run("set-option", "-p", "-t", st.SelvagePaneID, consolePaneOption, "1"); err != nil {
		logger.Warn("reed: could not set the console marker", "pane", st.SelvagePaneID, "err", err)
	}
}

// bottommostPaneID returns the id of the physically lowest pane in the window, the one with the largest pane_top.
// That is the only place Selvage may be split in.
// live must be non-empty.
func bottommostPaneID(live []LivePane) string {
	bottommost := live[0]
	for _, p := range live[1:] {
		if p.Top > bottommost.Top {
			bottommost = p
		}
	}
	return bottommost.ID
}

// splitSelvagePaneAtBottomLocked splits a new pane in below the physically bottom-most pane of live and returns its id.
//
// Selvage must land physically bottom-most, so the split targets the bottom-most pane.
// A lost or stale ReedState.SelvagePaneID can leave that pane an untracked one-row pane an older binary left behind.
// The split is full-width and takes its rows from the whole window, so such a short pane cannot refuse it.
// A split that still fails returns its error, which the next up or resume retries.
func (e *Engine) splitSelvagePaneAtBottomLocked(live []LivePane, launchCmd string) (string, error) {
	return e.splitPaneBelowLocked(bottommostPaneID(live), live, launchCmd)
}

// splitPaneBelowLocked splits a new pane below target and returns its id.
// It refuses an id that was already present in preSplitLive.
//
// The split carries -f, a full-width split taking its rows from the whole window.
// A short target pane therefore cannot make it fail.
// tmux's default split direction is vertical with the new pane below, which is where Selvage must land.
//
// The check that the new pane is genuinely new guards against psmux's silent too-small-to-split failure.
// That failure prints an existing pane's id with exit 0.
// Recording that id as Selvage would bind Selvage to a strand's pane.
func (e *Engine) splitPaneBelowLocked(target string, preSplitLive []LivePane, launchCmd string) (string, error) {
	argv := []string{"split-window", "-f", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}"}
	if launchCmd != "" {
		// The launch command is one trailing argument, as in an interactive `tmux split-window`.
		// The pane runs launchCmd directly instead of an interactive shell.
		// Nothing types the command, echoes it, or reads a shell rc file for it.
		argv = append(argv, launchCmd)
	}
	out, err := e.tmux.output(argv...)
	if err != nil {
		return "", err
	}
	paneID := strings.TrimSpace(out)
	if err := validateSplitCreatedNewPane(paneID, preSplitLive, target); err != nil {
		return "", err
	}
	return paneID, nil
}

// shellWindowOf returns the id of the window that holds the recorded console pane, alive or dead.
// It reads the window from windowByPane, a pane-to-window listing of the session.
// It reports false when st records no console pane or the listing does not carry it.
// It never falls back to another window: a session with no recorded console pane has no shell window.
func shellWindowOf(st *ReedState, windowByPane map[string]string) (string, bool) {
	if st == nil || st.SelvagePaneID == "" {
		return "", false
	}
	windowID, ok := windowByPane[st.SelvagePaneID]
	return windowID, ok
}

// recordedPaneIDs lists the pane ids st records in the shell window: the recorded Selvage pane alone.
// A strand's pane is not listed, since every strand lives in a window of its own.
func recordedPaneIDs(st *ReedState) []string {
	if st == nil || st.SelvagePaneID == "" {
		return nil
	}
	return []string{st.SelvagePaneID}
}

// planShellWindowMoves returns the ids of the shellPanes that a strand of st is bound to, alive or dead.
// The ids come in listing order.
// The recorded Selvage pane and any unbound pane are never returned.
// When shellPanes lacks the Selvage pane, the last bound pane stays.
// Breaking out a window's only pane would take the whole window with it.
func planShellWindowMoves(st *ReedState, shellPanes []LivePane) []string {
	bound := make(map[string]bool, len(st.Strands))
	for _, s := range st.Strands {
		if s.PaneID != "" {
			bound[s.PaneID] = true
		}
	}
	selvagePresent := false
	var moves []string
	for _, p := range shellPanes {
		switch {
		case st.SelvagePaneID != "" && p.ID == st.SelvagePaneID:
			selvagePresent = true
		case bound[p.ID]:
			moves = append(moves, p.ID)
		}
	}
	if !selvagePresent && len(moves) > 0 {
		moves = moves[:len(moves)-1]
	}
	return moves
}
