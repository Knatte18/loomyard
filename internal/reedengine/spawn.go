// spawn.go implements the shared pane-launch helper every strand-realizing path composes: AddStrand
// launching a freshly added strand, UpdateStrand surfacing a hidden->visible strand, and Resume
// replaying a not-live strand all call launchStrandLocked to reconcile and then create a fresh tmux
// pane and run the strand's command in it (GAP A) — without this shared helper, add would register a
// record and re-render but never create a pane or run anything.
// This file also carries the two other small cross-file bootstrap helpers strand.go and
// lifecycle.go both need: loadOrInitStateLocked (fresh-worktree state bootstrap) and
// reconcileApplyPersistLocked (the reconcile-then-apply-then-persist tail every public op ends
// with).

package reedengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// planPaneTarget always yields a split target for the next strand realization
// — it never adopts an existing pane. The surviving rules are a pure function
// of live and headerPaneID: prefer the tallest alive non-header pane, fall
// back to any present non-header pane (a corpse) when none is alive, and fall
// back to live[0] (the header itself) when no non-header pane exists at all.
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
// untracked reap is authorized by an alive header (reconcile.go), the initial
// pane is disposed of like any other untracked pane before this function ever
// runs, so a fresh split — idle by construction — costs one kill-pane plus
// one split-window and buys correctness back.
func planPaneTarget(live []LivePane, headerPaneID string) (splitTargetID string, err error) {
	if len(live) == 0 {
		return "", fmt.Errorf("session has no panes to split")
	}

	splitTargetID = ""
	tallestAlive := -1
	for _, p := range live {
		if p.ID == headerPaneID || p.Dead {
			continue
		}
		if p.Height > tallestAlive {
			tallestAlive = p.Height
			splitTargetID = p.ID
		}
	}
	if splitTargetID == "" {
		// No alive non-header pane: fall back to any present non-header
		// pane (a dead corpse), mirroring the pre-header "every pane dead"
		// fallback.
		for _, p := range live {
			if p.ID != headerPaneID {
				splitTargetID = p.ID
				break
			}
		}
	}
	if splitTargetID == "" {
		// No non-header pane exists at all: every strand has been removed
		// and only the header remains. Split the header itself so this add
		// still has a pane to split.
		splitTargetID = live[0].ID
	}
	return splitTargetID, nil
}

// validateSplitCreatedNewPane returns an error unless paneID is genuinely
// new — absent from preSplitLive. psmux's split-window on a too-small pane
// fails silently and prints an existing pane's id, which would bind two
// owners to one pane.
func validateSplitCreatedNewPane(paneID string, preSplitLive []LivePane, target string) error {
	if paneID == "" || liveIDSet(preSplitLive)[paneID] {
		return fmt.Errorf("split-window created no new pane (got %q; target %s likely too small to split)", paneID, target)
	}
	return nil
}

// sendKeysLiteralArg returns the argument tmux send-keys -l must receive
// so text types verbatim. tmux parses dash-leading arguments as flags,
// silently dropping them, so a dash-leading cmd must be prefixed with space.
func sendKeysLiteralArg(text string) string {
	if strings.HasPrefix(text, "-") {
		return " " + text
	}
	return text
}

// launchStrandLocked reconciles the session's panes first, then always splits a fresh pane for s and
// runs launchCmd in it. It validates the split created a new pane and sets only s.PaneID; Live is
// derived from pane binding downstream.
//
// Reconciling here — rather than trusting the caller's last reconcile — is the chokepoint that makes
// "reap before allocate" hold on every strand-realizing path (AddStrand, UpdateStrand, Resume, and
// any future one) by construction, instead of by each call site remembering to do it. It is safe for
// the strand being launched itself, though the reason differs per path:
//   - On AddStrand and UpdateStrand, s reaches here with PaneID == "" (addStrandLocked builds a
//     fresh record; updateStrandLocked launches a strand that has never held a pane), so reconcile
//     can neither clear nor kill anything belonging to it.
//   - On Resume that is not universally true: planResumeLaunches selects strands whose pane is
//     absent from aliveIDSet, which includes a strand still bound to a dead-but-present pane. That is
//     harmless here too — that binding names a corpse, so reconcile either kills it as a dead pane
//     (clearing the binding) or spares it as the kept dead pane, and either way this function
//     overwrites s.PaneID with the freshly split pane a few lines below. Strands Resume's per-strand
//     loop already relaunched earlier in the same pass are bound to alive panes by the time this
//     reconcile runs for the next strand, so they are exempt from the untracked reap, not merely
//     lucky to survive it.
//
// This does not call SaveState: AddStrand and UpdateStrand only persist after this function returns
// nil, so a split-window or send-keys failure here returns with panes already killed in tmux while
// the corresponding binding clears live only in memory — reed.json itself is untouched. That window
// is accepted rather than closed, because it is self-healing (Status and toRenderStrands derive
// liveness from the live pane set, never the persisted binding, and the next mutating verb's own
// reconcile clears the now-stale bindings for real) and because closing it here would be worse:
// persisting inside this helper would write the half-added strand record on the add path, turning a
// clean failure into a phantom strand Resume would later try to launch. If this window ever needs
// closing, the right shape is reaping before the strand record is appended to st.Strands, not
// persisting a partial one from inside this helper.
func (e *Engine) launchStrandLocked(st *ReedState, s *Strand, launchCmd string) error {
	session := e.SessionName()

	live, err := e.tmux.listPanes(session)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}

	killed, err := e.reconcileLocked(st, live)
	if err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	if len(killed) > 0 {
		// Order matters: kill untracked/dead -> re-enumerate live -> plan.
		// The kill-pane calls above mutate the pane set planPaneTarget below
		// must choose from, so enumeration must follow them (same ordering
		// reconcileApplyPersistLocked already treats as load-bearing).
		live, err = e.tmux.listPanes(session)
		if err != nil {
			return fmt.Errorf("list panes after reconcile: %w", err)
		}
	}

	splitTargetID, err := planPaneTarget(live, st.HeaderPaneID)
	if err != nil {
		return err
	}

	// -c pins the new pane's cwd to Geometry.PaneCwd, exactly as
	// new-session and the header split (lifecycle.go) already do. Without
	// it tmux resolves the cwd from the invoking CLIENT — verified live
	// (tmux 3.6): a split issued from outside tmux lands in the calling
	// process's cwd, neither the target pane's cwd nor the session's. That
	// happens to be PaneCwd whenever lyx runs under lyxcwd.Resolve's
	// exact-equality cwd gate, and stops being it the moment a
	// caller injects a cwd through the RunCLIIn seam instead — at which
	// point every strand command would run against the wrong tree while
	// reed reported success.
	out, err := e.tmux.output("split-window", "-t", splitTargetID, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}")
	if err != nil {
		return fmt.Errorf("split window: %w", err)
	}
	paneID := strings.TrimSpace(out)
	if err := validateSplitCreatedNewPane(paneID, live, splitTargetID); err != nil {
		return err
	}

	s.PaneID = paneID
	// Send the command as a literal string (-l) so tmux never reinterprets
	// any part of the opaque launchCmd as a key name (e.g. "Enter", "C-c") or
	// splits it on an embedded ';' — the caller (shuttle) builds arbitrary
	// PowerShell command chains. A separate Enter then submits it.
	if err := e.tmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(launchCmd)); err != nil {
		return fmt.Errorf("send launch command: %w", err)
	}
	if err := e.tmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
		return fmt.Errorf("submit launch command: %w", err)
	}
	return nil
}

// loadOrInitStateLocked loads or initializes a ReedState stamped with
// the engine's server/socket/session identity for a fresh worktree.
// The two identity fields are re-stamped on every load, not only at init: nothing in production
// reads them back (every consumer uses the told Geometry via Socket()/SessionName()), so they exist
// purely as an on-disk forensic diagnostic — and a diagnostic recording an identity reed no longer
// drives (a renamed worktree carries its .lyx state along, but its session name changes with the
// directory) is worse than none (R3 review finding R3-F2). The caller persists via the op's normal
// SaveState tail, so no extra write happens here.
func (e *Engine) loadOrInitStateLocked() (*ReedState, error) {
	st, err := LoadState(e.stateDir())
	if err != nil {
		// Passed through bare: LoadState's corrupt-file error already names the file and both
		// remedies, and a "load state:" prefix in front of that would only bury the diagnosis.
		return nil, err
	}
	if st == nil {
		st = &ReedState{}
	}
	st.Socket = e.Socket()
	st.Session = e.SessionName()

	// Discard bindings minted against a session incarnation that is no longer the one running, and
	// refuse outright when the session they were minted against is still alive on this socket under
	// another name. Every call site reaches here with the told session already up — Up/Resume via
	// ensureServerAndSessionLocked, every other op via requireSessionLocked — so the generation
	// probe always has a session to ask about. See generation.go.
	if err := e.adoptPaneGenerationLocked(st); err != nil {
		return nil, err
	}

	// Repair a table whose pane bindings contradict each other before any caller reads it. This runs
	// on every load, not only after a rebirth, because the corrupt tables it exists for arrive
	// BETWEEN boots (a restored backup, a hand-edited file) and would otherwise be trusted all the
	// way into select-layout, where tmux answers the contradiction by destroying panes — see
	// clearConflictingPaneBindings for both observed shapes.
	if cleared := clearConflictingPaneBindings(st); len(cleared) > 0 {
		logger.Warn("reed: cleared strand pane bindings that named a pane another owner already claims",
			"socket", e.Socket(), "session", e.SessionName(), "strands", cleared)
	}
	return st, nil
}

// reconcileApplyPersistLocked fetches current panes, reconciles dead
// pane bindings, reapplies the layout, and persists the state. It is the
// shared tail every public op composes after mutation.
func (e *Engine) reconcileApplyPersistLocked(st *ReedState) ([]LivePane, error) {
	session := e.SessionName()
	live, err := e.tmux.listPanes(session)
	if err != nil {
		return nil, fmt.Errorf("list panes: %w", err)
	}

	killed, err := e.reconcileLocked(st, live)
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	if len(killed) > 0 {
		// Order matters: kill dead -> re-enumerate live -> compute layout
		// -> apply. The kill-pane calls above mutate the pane set the next
		// select-layout must enumerate, so enumeration must follow them.
		live, err = e.tmux.listPanes(session)
		if err != nil {
			return nil, fmt.Errorf("list panes after reconcile: %w", err)
		}
	}

	if err := e.applyLayoutLocked(st, live); err != nil {
		return nil, fmt.Errorf("apply layout: %w", err)
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	return live, nil
}
