// selvagepane.go owns every piece of Selvage-specific code in this package: the pane's create/heal
// lifecycle, the reconcile exemption policy, the strand-claim seed, the split-target decision, and
// the render-params mapping. lifecycle.go, reconcile.go, spawn.go and apply.go each call into this
// file's narrow seams rather than carrying their own Selvage logic inline.

package reedengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// reapPolicy answers the three Selvage-related questions planReconcile asks: whether a given pane is
// exempt from the dead-pane kill, whether it is exempt from the untracked-pane reap, and whether the
// Selvage pane's own state authorizes the untracked reap to run at all.
// The zero value has an empty pane id and reports not-alive, matching a state with no Selvage pane
// recorded.
type reapPolicy struct {
	paneID string
	alive  bool
}

// newReapPolicy builds the reap policy planReconcile consults from st's recorded Selvage pane id and
// the current live pane set.
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

// exemptFromDeadKill reports whether paneID is Selvage's own pane and so must never be scheduled by
// the dead-pane kill loop: nothing outside up/resume ever rebuilds Selvage, so killing a
// pane_dead=1 Selvage here would leave the session without its always-on operator console with a
// stale SelvagePaneID until the next up/resume — and, before the planLayout presence filter existed,
// that stale id was still emitted as a layout cell, which a real tmux ACCEPTS (exit 0) and assigns
// positionally, scrambling every strand's height (observed live, tmux 3.6). A kept Selvage corpse
// instead stays enumerable, keeps the cell/pane count consistent, and is healed — killed and
// re-split — by ensureSelvagePaneLocked on the next up/resume.
// The non-empty guard is explicit rather than relying on a live pane id never being the empty string.
func (p reapPolicy) exemptFromDeadKill(paneID string) bool {
	return p.paneID != "" && p.paneID == paneID
}

// exemptFromUntrackedReap reports whether paneID is Selvage's own pane and so must never be reaped as
// an untracked pane, regardless of whether Selvage itself is alive: Selvage stays exempt from being
// killed by mere presence (a Selvage corpse is still never killed), distinct from authorizesReap,
// which governs only whether killing anything else is allowed at all. Presence-exemption and
// aliveness-authorization must not be folded together — see authorizesReap's doc comment for why.
// The non-empty guard is explicit rather than relying on a live pane id never being the empty string.
func (p reapPolicy) exemptFromUntrackedReap(paneID string) bool {
	return p.paneID != "" && p.paneID == paneID
}

// authorizesReap reports whether an alive Selvage pane authorizes the untracked-pane reap to run:
// killing an alive pane at worst corpses it under remain-on-exit, so the surviving Selvage always
// keeps the session alive. This is a separate question from exemptFromUntrackedReap/exemptFromDeadKill
// and must never be folded into them: Selvage stays exempt from being killed by mere presence (a
// Selvage corpse is still never killed), while only an ALIVE Selvage may authorize killing anything
// else. This disjunct exists because this reap fires from AddStrand/UpdateStrand once the
// reap-before-allocate chokepoint lands, and neither of those paths ever calls
// ensureSelvagePaneLocked — so a dead-but-present Selvage must not be allowed to authorize reaping the
// session's only alive pane.
func (p reapPolicy) authorizesReap() bool {
	return p.paneID != "" && p.alive
}

// planPaneTarget always yields a split target for the next strand realization
// — it never adopts an existing pane. The surviving rules are a pure function
// of st's Selvage pane id and live: prefer the tallest alive non-Selvage pane, fall
// back to any present non-Selvage pane (a corpse) when none is alive, and fall
// back to live[0] (Selvage itself) when no non-Selvage pane exists at all.
//
// Adoption used to give a fresh session's initial pane a use rather than
// splitting a needless second one, but the seam it required — deciding
// whether a candidate pane was reed's own idle initial pane or a foreign one
// — could not be made safely, and produced two live findings: R4-F5 (after
// .lyx/reed.json was scrubbed from a running session, adoption picked the
// previous header pane — still running "lyx reed header --blocking" — and the
// strand's command was typed onto its screen and never ran, with status
// reporting live:true and no such process on the box) and M16 (adoption
// claimed an operator's own manually-created split-window pane). Once the
// untracked reap is authorized by an alive Selvage (reconcile.go), the initial
// pane is disposed of like any other untracked pane before this function ever
// runs, so a fresh split — idle by construction — costs one kill-pane plus
// one split-window and buys correctness back.
//
// insertAbove is true only when the third tier fires — no non-Selvage pane exists at all, so Selvage
// itself is the chosen target — and false for the tallest-alive and present-corpse tiers. This is
// exactly equivalent to the condition launchStrandLocked used to compute at its call site: it
// appended -b when the chosen target equalled the Selvage pane id, and that can only hold when tier
// three fired, because tiers one and two both exclude the Selvage pane by construction and tier three
// is reachable only when every present pane IS the Selvage pane. When tier three fires, splitting
// below Selvage — tmux's default — would insert the new strand pane AFTER Selvage in tmux's own
// physical pane order, the same "cells apply positionally, not by pane id" hazard
// splitSelvagePaneAtBottomLocked's doc comment describes for Selvage's own split; -b keeps the new
// strand pane physically above Selvage instead, preserving the bottom-most invariant on exactly the
// one path that would otherwise violate it. Every other split target is a strand, and inserting below
// another strand never touches Selvage's position.
func planPaneTarget(st *ReedState, live []LivePane) (splitTargetID string, insertAbove bool, err error) {
	if len(live) == 0 {
		return "", false, fmt.Errorf("session has no panes to split")
	}
	selvagePaneID := st.SelvagePaneID

	splitTargetID = ""
	tallestAlive := -1
	for _, p := range live {
		if p.ID == selvagePaneID || p.Dead {
			continue
		}
		if p.Height > tallestAlive {
			tallestAlive = p.Height
			splitTargetID = p.ID
		}
	}
	if splitTargetID == "" {
		// No alive non-Selvage pane: fall back to any present non-Selvage
		// pane (a dead corpse), mirroring the pre-Selvage "every pane dead"
		// fallback.
		for _, p := range live {
			if p.ID != selvagePaneID {
				splitTargetID = p.ID
				break
			}
		}
	}
	if splitTargetID == "" {
		// No non-Selvage pane exists at all: every strand has been removed
		// and only Selvage remains. Split Selvage itself so this add
		// still has a pane to split.
		splitTargetID = live[0].ID
		insertAbove = true
	}
	return splitTargetID, insertAbove, nil
}

// selvageRenderParams builds the render.Selvage value toRenderInputs assembles into render.Params:
// the pane id taken from st, blanked to the empty string when presentIDs does not hold it, and
// HeightRows read from e.cfg.Selvage.HeightRows. This is the package's only render.Selvage
// construction and, outside config.go, its only e.cfg.Selvage read.
func (e *Engine) selvageRenderParams(st *ReedState, presentIDs map[string]bool) render.Selvage {
	paneID := st.SelvagePaneID
	if !presentIDs[paneID] {
		paneID = ""
	}
	return render.Selvage{PaneID: paneID, HeightRows: e.cfg.Selvage.HeightRows}
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
// It is the single writer of that clear; its three callers are upLocked's and Resume's own
// server-rebirth handling (lifecycle.go) and adoptPaneGenerationLocked's pane-generation mismatch
// handling (generation.go) — all three clear it for the identical reason: a reborn or foreign session
// incarnation's reused pane id must never be mistaken for the still-live Selvage pane.
func clearSelvagePaneBinding(st *ReedState) {
	st.SelvagePaneID = ""
}

// ensureSelvagePaneLocked ensures Selvage exists and is alive.
// (Re)creates it when missing, dead, or gone. Selvage is separate from
// strands and must land physically bottom-most so layout heights stay
// correct. The (re)creation itself is splitSelvagePaneAtBottomLocked's job,
// including the even-vertical retry that keeps a stale or lost
// SelvagePaneID from wedging the worktree — see that function for why a
// split against the bottom pane can fail at all.
func (e *Engine) ensureSelvagePaneLocked(st *ReedState) error {
	session := e.SessionName()
	live, err := e.tmux.listPanes(session)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}

	if len(live) == 0 {
		// A zero-pane husk cannot host a split; surface it rather than
		// panicking on an empty slice below. ensureServerAndSessionLocked
		// kills husks before this runs, so reaching this means the session
		// emptied between the two probes — an error, not an invariant.
		return fmt.Errorf("session %s has no panes to split Selvage from", session)
	}

	if st.SelvagePaneID != "" && aliveIDSet(live)[st.SelvagePaneID] {
		// Present AND alive: idempotent no-op across up/resume. Aliveness,
		// not mere presence, is the check — a dead-but-present Selvage
		// corpse (kept enumerable by reconcile's deliberate exemption) must
		// be healed here, not mistaken for a working Selvage.
		return nil
	}

	// A dead-but-present Selvage corpse is killed before the replacement is
	// split, so its bottom row is freed and the bottommost target below is a
	// real (usually alive) pane — unless the corpse is the session's SOLE
	// pane, where killing first would end the session; then the corpse
	// itself is the split target and it is killed after the new pane
	// exists.
	corpseID := ""
	if st.SelvagePaneID != "" && liveIDSet(live)[st.SelvagePaneID] {
		corpseID = st.SelvagePaneID
		if len(live) > 1 {
			if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
				return fmt.Errorf("kill dead Selvage pane %s: %w", corpseID, err)
			}
			corpseID = ""
			live, err = e.tmux.listPanes(session)
			if err != nil {
				return fmt.Errorf("list panes after killing dead Selvage: %w", err)
			}
			if len(live) == 0 {
				return fmt.Errorf("session %s has no panes to split Selvage from", session)
			}
		}
	}

	// Selvage's trailing split-window argument is e.cfg.Shell, the same way
	// new-session already launches the session's first pane — an ordinary
	// interactive shell, never a re-exec of lyx.
	paneID, err := e.splitSelvagePaneAtBottomLocked(session, live, e.cfg.Shell)
	if err != nil {
		return fmt.Errorf("split Selvage pane: %w", err)
	}

	if corpseID != "" {
		// The sole-pane corpse the new Selvage was split off of: now that a
		// second pane exists, killing it can no longer end the session.
		// Best-effort — a corpse that somehow vanished already is fine; the
		// discard is still worth a Debug line so the step is observable at
		// the trace level without upgrading routine cleanup to a Warn.
		if err := e.tmux.run("kill-pane", "-t", corpseID); err != nil {
			logger.Debug("reed: best-effort kill of Selvage corpse pane failed", "socket", e.Socket(), "pane", corpseID, "err", err)
		}
	}

	st.SelvagePaneID = paneID
	if err := SaveState(e.stateDir(), st); err != nil {
		return fmt.Errorf("persist Selvage pane id: %w", err)
	}
	return nil
}

// bottommostPaneID returns the id of the pane sitting physically lowest in the window — the largest
// pane_top — which is the only place Selvage may be split in.
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

// splitSelvagePaneAtBottomLocked splits a new pane in below the physically bottom-most pane of
// session and returns its id, retrying once behind an even-vertical re-tile when the first attempt
// has no room.
//
// The retry is what keeps a lost or stale ReedState.SelvagePaneID from wedging a worktree
// permanently (R4 review finding R4-F4). The Selvage band is one row by default, and tmux cannot
// split a one-row pane at all — so the moment SelvagePaneID stops naming the pane at the bottom,
// the bottommost split target IS an untracked one-row band and every later up/resume fails with
// "no space for new pane", forever, while status keeps reporting the session healthy and the only
// escape ("lyx reed down", then up) is named nowhere. Two ordinary routes reach that state:
// scrubbing .lyx/reed.json, a never-tracked machine-local tree the Durable-vs-Ephemeral State
// Invariant makes disposable (a plain `git clean -xdf` in the worktree does it), and a process
// death in the window between the split above and the SaveState that records its id.
//
// The physical-position requirement is symmetric to the header's former top-placement requirement:
// render.Rules emits the band cell LAST and paneIDsByTop resequences by pane_top, so a Selvage pane
// that is not physically bottom-most would invert cell heights on the very first select-layout.
//
// select-layout even-vertical evens every pane's height using tmux's own built-in layout — no reed
// layout string is computed or applied here, so anyPlacedStrand's empty-layout hazard (apply.go) is
// not in play — after which the same split has room again; the even-vertical re-tile retry survives
// this flip verbatim, because tmux cannot split a one-row pane at all, which is just as true at the
// bottom as it was at the top. The op's normal reconcileApplyPersistLocked tail then restores reed's
// real geometry and reaps the untracked band; an op that fails before reaching that tail leaves the
// window evenly tiled, a cosmetic state the next successful op corrects.
// Both subcommands are already in requiredSubcommands, so the multiplexer capability contract is
// unchanged.
//
// On a failed retry the FIRST error is returned, not the retry's: it describes the state the
// operator actually has, and the re-tile is an internal repair attempt rather than something they
// asked for.
func (e *Engine) splitSelvagePaneAtBottomLocked(session string, live []LivePane, launchCmd string) (string, error) {
	paneID, firstErr := e.splitPaneBelowLocked(bottommostPaneID(live), live, launchCmd)
	if firstErr == nil {
		return paneID, nil
	}
	logger.Warn("reed: failed to split Selvage pane, retrying behind an even-vertical re-tile", "socket", e.Socket(), "session", session, "err", firstErr)

	if err := e.tmux.run("select-layout", "-t", exactSessionWindowTarget(session), "even-vertical"); err != nil {
		logger.Warn("reed: even-vertical re-tile failed, Selvage split not retried", "socket", e.Socket(), "session", session, "err", err)
		return "", firstErr
	}
	retiled, err := e.tmux.listPanes(session)
	if err != nil || len(retiled) == 0 {
		logger.Warn("reed: could not re-enumerate panes after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
		return "", firstErr
	}
	// The retry carries launchCmd too — a retried Selvage must never boot commandless, or it would
	// be left hosting an interactive shell exactly like the noise this batch removes.
	paneID, err = e.splitPaneBelowLocked(bottommostPaneID(retiled), retiled, launchCmd)
	if err != nil {
		logger.Warn("reed: Selvage split still had no room after the even-vertical re-tile", "socket", e.Socket(), "session", session, "err", err)
		return "", firstErr
	}
	logger.Info("reed: Selvage split recovered by an even-vertical re-tile", "socket", e.Socket(), "session", session, "pane", paneID)
	return paneID, nil
}

// splitPaneBelowLocked splits a new pane in directly below target and returns its id, refusing an
// id that was already present in preSplitLive.
//
// No -b flag is needed here: tmux's default split direction is vertical with the new pane below,
// which is exactly where Selvage must land now that render.Rules emits the band cell LAST rather
// than first. Selvage is still the only split in the whole engine that must land at a specific
// physical edge — every strand split (spawn.go) always targets a non-Selvage pane and inserts
// below it too, but strands have no positional requirement of their own the way Selvage does.
//
// The genuinely-new-pane guard is the same one launchStrandLocked runs: psmux's silent
// too-small-to-split failure prints an EXISTING pane's id with exit 0, and recording that id as
// Selvage would bind Selvage to a strand's pane — the next layout string would then carry a
// duplicate pane number, destroying the session's panes wholesale (see validateSplitCreatedNewPane).
func (e *Engine) splitPaneBelowLocked(target string, preSplitLive []LivePane, launchCmd string) (string, error) {
	argv := []string{"split-window", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}"}
	if launchCmd != "" {
		// A single trailing shell-command argument, exactly like an interactive `tmux split-window`
		// invocation's own trailing-command syntax: the pane then runs launchCmd directly rather than
		// an interactive shell, so nothing types it, echoes it, or reads a shell rc file for it.
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
