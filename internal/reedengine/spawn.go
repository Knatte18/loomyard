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
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shell"
)

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

// launchStrandLocked reconciles the session's panes first, then creates a fresh pane for s and runs launchCmd in it.
// The pane is a split of window 0, or a detached window of its own for an own-window strand, and both run the same tail:
// the title lock, the pane options, the prelude, the launch script and the send-keys.
// It validates the split created a new pane and sets only s.PaneID; Live is
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
//
// The prelude that resolves this strand pane's `lyx` to the binary that spawned it is owned by panebin.go and composed here, at the one launch every strand-realizing path funnels through — see composePaneLaunchLine.
// The same composition exports LYX_STRAND_NAME and LYX_PARENT beside LYX_BIN: the name is the strand's stored birth attribute and the parent is re-told from the geometry, so every launch and resume path sees the same values.
// The split issued below still carries no trailing shell-command argument,
// so the pane remains tmux's own default-shell started as a login shell (pane-start-mode-is-untouched Shared Decision).
// The prelude and launchCmd reach that shell afterward through the strand's launch script, which the send-keys line typed into the pane sources in the shell's own scope, rather than by changing how the pane's shell itself starts;
// a script write failure degrades to typing the composed line itself.
// The script is written on every launch, so a relaunch with a different command or a different spawning executable always regenerates it.
func (e *Engine) launchStrandLocked(st *ReedState, s *Strand, launchCmd string) error {
	live, windows, err := e.listStrandAndWindowPanes(st)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}

	killed, err := e.reconcileLocked(st, live, windows)
	if err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	if len(killed) > 0 {
		// Order matters: kill untracked/dead -> re-enumerate live -> plan.
		// The kill-pane calls above mutate the pane set planPaneTarget below
		// must choose from, so enumeration must follow them (same ordering
		// reconcileApplyPersistLocked already treats as load-bearing).
		live, err = e.listStrandPanes(st)
		if err != nil {
			return fmt.Errorf("list panes after reconcile: %w", err)
		}
	}

	var paneID string
	previousPaneID := s.PaneID
	if isOwnWindow(s.Display) {
		paneID, err = e.createOwnWindowPaneLocked(st, s)
	} else {
		paneID, err = e.splitWindowZeroPaneLocked(st, live, s)
	}
	if err != nil {
		return err
	}

	s.PaneID = paneID
	if isOwnWindow(s.Display) {
		e.killReplacedOwnWindowPaneLocked(previousPaneID, paneID)
	}
	// Both title commands are display mirrors: allow-set-title off stops the agent inside from overwriting the name,
	// and no lookup or delivery ever resolves through a title, so a failure of either costs display only and never fails the launch.
	if err := e.tmux.run("set-option", "-p", "-t", paneID, "allow-set-title", "off"); err != nil {
		logger.Warn("reed: could not turn off allow-set-title, the pane title may drift", "strand", s.GUID, "pane", paneID, "err", err)
	}
	if err := e.tmux.run("select-pane", "-t", paneID, "-T", s.Name); err != nil {
		logger.Warn("reed: could not set the pane title", "strand", s.GUID, "pane", paneID, "name", s.Name, "err", err)
	}
	e.setStrandPaneOptionsLocked(paneID, *s)
	// composePaneLaunchLine joins the pane-binary prelude (panebin.go) onto launchCmd, on the same shell.ForGOOS() dialect the launch command itself was built with,
	// so the two never disagree about which shell is typed into.
	// stageLaunchScript writes that composed line to the strand's launch script and returns the dialect's source statement for it, which is what the pane shows;
	// a write failure returns the composed line itself, so the pane still launches.
	// The payload is sent as a literal string (-l) so tmux never reinterprets any part of it as a key name (e.g. "Enter", "C-c") or splits it on an embedded ';' — the launch command is opaque and shuttle builds arbitrary PowerShell command chains.
	// A separate Enter then submits it.
	sh := shell.ForGOOS()
	composedLine, exe := composePaneLaunchLine(sh, launchCmd, s.GUID, s.Name, e.geom.ParentName)
	s.LyxBin = exe
	payload := stageLaunchScript(sh, e.stateDir(), s.GUID, composedLine)
	if err := e.tmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(payload)); err != nil {
		return fmt.Errorf("send launch command: %w", err)
	}
	if err := e.tmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
		return fmt.Errorf("submit launch command: %w", err)
	}
	return nil
}

// killReplacedOwnWindowPaneLocked kills previousPaneID, the pane a relaunched own-window strand held before newPaneID replaced it, when the session still lists it in an own window.
// It runs after the new pane exists, so a shared window that held only the old pane is never ended mid-launch.
// A pane that is empty, unchanged or no longer listed is left alone; a failure is logged and never fails the launch.
func (e *Engine) killReplacedOwnWindowPaneLocked(previousPaneID, newPaneID string) {
	if previousPaneID == "" || previousPaneID == newPaneID {
		return
	}
	windows, err := e.tmux.listWindowPanes(e.SessionName())
	if err != nil {
		logger.Warn("reed: could not list panes to find the replaced pane", "socket", e.Socket(), "session", e.SessionName(), "pane", previousPaneID, "err", err)
		return
	}
	for _, w := range windows {
		if w.PaneID != previousPaneID || !w.Own {
			continue
		}
		if err := e.tmux.run("kill-pane", "-t", previousPaneID); err != nil {
			logger.Warn("reed: could not kill the replaced own-window pane", "socket", e.Socket(), "session", e.SessionName(), "pane", previousPaneID, "err", err)
		}
		return
	}
}

// splitWindowZeroPaneLocked splits a fresh pane for s into window 0 at the target planPaneTarget picks from live and returns its id.
func (e *Engine) splitWindowZeroPaneLocked(st *ReedState, live []LivePane, s *Strand) (string, error) {
	split, err := planPaneTarget(st, live, s.Display)
	if err != nil {
		return "", err
	}
	splitTargetID := split.target

	// -c pins the new pane's cwd to Geometry.PaneCwd, exactly as
	// new-session and Selvage's own split (lifecycle.go) already do. Without
	// it tmux resolves the cwd from the invoking CLIENT — verified live
	// (tmux 3.6): a split issued from outside tmux lands in the calling
	// process's cwd, neither the target pane's cwd nor the session's. That
	// happens to be PaneCwd whenever lyx runs under lyxcwd.Resolve's
	// exact-equality cwd gate, and stops being it the moment a
	// caller injects a cwd through the RunCLIIn seam instead — at which
	// point every strand command would run against the wrong tree while
	// reed reported success.
	//
	// -b is the exception to "a strand split always lands below its target";
	// see planPaneTarget's doc comment for when it is set.
	// -f, set with -b for a top-anchored strand, makes the split full-width and takes its rows from the whole window.
	argv := []string{"split-window"}
	if split.above {
		argv = append(argv, "-b")
	}
	if split.full {
		argv = append(argv, "-f")
	}
	argv = append(argv, "-t", splitTargetID, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}")
	out, err := e.tmux.output(argv...)
	if err != nil {
		return "", fmt.Errorf("split window: %w", err)
	}
	paneID := strings.TrimSpace(out)
	if err := validateSplitCreatedNewPane(paneID, live, splitTargetID); err != nil {
		return "", err
	}
	return paneID, nil
}

// createOwnWindowPaneLocked creates the pane of an own-window strand and returns its id.
// A strand with no window group gets a window of its own named by its role.
// A grouped strand follows planOwnWindowPlacement: it opens the group's shared window, joins it, or gets a window of its own.
// The group record the plan returns is stored in st, which the caller persists with the strand.
// A refusal returns before anything is created.
// A shared window is laid out evenly in the window_split direction after a new pane joins or opens it.
func (e *Engine) createOwnWindowPaneLocked(st *ReedState, s *Strand) (string, error) {
	group := s.Display.Window
	if group == "" {
		return e.openOwnWindowLocked(st, s, strandLabel(s.Name), "")
	}

	// Listed here, after any head reconcile that ended an own window, so the plan and the join read the current windows.
	windows, err := e.tmux.listWindowPanes(e.SessionName())
	if err != nil {
		return "", fmt.Errorf("list windows: %w", err)
	}
	count, groupWindow := groupLiveCount(st.Strands, group, s.GUID, windows)
	placement, record, err := planOwnWindowPlacement(st.WindowGroups, s.Display, e.cfg.WindowShareMax, count, groupWindow)
	if err != nil {
		return "", err
	}
	if st.WindowGroups == nil {
		st.WindowGroups = make(map[string]WindowGroup)
	}
	st.WindowGroups[group] = record

	if placement == placeApart {
		return e.openOwnWindowLocked(st, s, strandLabel(s.Name), "")
	}

	splitFlag, layout, err := windowSplitLayout(e.cfg.WindowSplit)
	if err != nil {
		return "", err
	}
	var paneID string
	if placement == placeJoinShared {
		paneID, err = e.joinGroupWindowLocked(groupWindow, windows, splitFlag)
	} else {
		paneID, err = e.openOwnWindowLocked(st, s, group, group)
	}
	if err != nil {
		return "", err
	}
	if err := e.tmux.run("select-layout", "-t", paneID, layout); err != nil {
		logger.Warn("reed: could not lay out the shared window", "strand", s.GUID, "pane", paneID, "layout", layout, "err", err)
	}
	return paneID, nil
}

// joinGroupWindowLocked splits the first listed pane of the group window groupWindow in the direction splitFlag and returns the new pane's id.
// The split is detached, so the group window's active pane stays where it is and a joiner never takes focus from an operator viewing the window.
// A split that reuses an existing pane is refused as in window 0.
func (e *Engine) joinGroupWindowLocked(groupWindow string, windows []windowPane, splitFlag string) (string, error) {
	var members []LivePane
	for _, w := range windows {
		if w.WindowID == groupWindow {
			members = append(members, w.livePane())
		}
	}
	if len(members) == 0 {
		return "", fmt.Errorf("group window %s lists no pane to split", groupWindow)
	}
	target := members[0].ID
	out, err := e.tmux.output("split-window", "-d", splitFlag, "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}")
	if err != nil {
		return "", fmt.Errorf("split group window: %w", err)
	}
	paneID := strings.TrimSpace(out)
	if err := validateSplitCreatedNewPane(paneID, members, target); err != nil {
		return "", err
	}
	return paneID, nil
}

// newWindowLocked creates a detached window and returns new-window's output.
// The command is new-window -d, then the placement arguments, then argv, which carries no -t of its own.
// With an empty placement it adds -t on the exact session target, so every new-window carries exactly one -t.
// A placed new-window the multiplexer refuses on Windows logs at Debug and is retried once in the plain form, so the order is best-effort there; on every other platform the error is returned.
// It is the one place a window is created.
func (e *Engine) newWindowLocked(placement []string, argv []string) (string, error) {
	plain := append([]string{"new-window", "-d", "-t", exactSessionWindowTarget(e.SessionName())}, argv...)
	if len(placement) == 0 {
		return e.tmux.output(plain...)
	}
	placed := append(append([]string{"new-window", "-d"}, placement...), argv...)
	out, err := e.tmux.output(placed...)
	if err != nil && runtime.GOOS == "windows" {
		logger.Debug("reed: the multiplexer refused a placed new-window, opening it in the plain form", "socket", e.Socket(), "session", e.SessionName(), "placement", placement, "err", err)
		return e.tmux.output(plain...)
	}
	return out, err
}

// openOwnWindowLocked opens a detached window for s named name and returns the id of its pane.
// The window is created with -d, so the client's current window and active pane stay where they are; an own-window strand's window is then selected only when newAgentWindowSelected holds for the window the session showed before it opened, and carries no @lyx_unseen then.
// An overview window is never selected here.
// It is placed behind every earlier agent window and before the shell window, and a relaunch keeps the place of the window its old pane was in.
// It carries no command, so the pane is the default login shell like every strand pane.
// An overview strand's window is named overviewWindowName and opens before the lowest-index window, hidden ones included, so it holds the lowest index; it carries the own marker but neither @strand_color nor @lyx_unseen.
// A non-empty group is recorded on the window, which is how a group's shared window is found later.
// A window option that fails to set is logged and never fails the open; a window missing the own marker is only never ended by reconcile.
func (e *Engine) openOwnWindowLocked(st *ReedState, s *Strand, name, group string) (string, error) {
	e.ensureShellWindowLocked(st)
	var shellWindow, previousWindow string
	windowByPane, err := e.tmux.listSessionPaneWindows(e.SessionName())
	if err != nil {
		logger.Debug("reed: could not list the session's panes to place the new window", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	} else {
		shellWindow, _ = shellWindowOf(st, windowByPane)
		previousWindow = windowByPane[s.PaneID]
	}
	windows, err := e.tmux.listSessionWindows(e.SessionName())
	if err != nil {
		logger.Debug("reed: could not list the session's windows to place the new window", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}
	if _, listed := findSessionWindow(windows, shellWindow); !listed {
		logger.Debug("reed: no shell window is listed, placing the new window after the last visible one", "socket", e.Socket(), "session", e.SessionName())
	}
	overview := isOverview(s.Display)
	placement := ownWindowInsertArgs(windows, shellWindow, previousWindow)
	selectNew := false
	if overview {
		name = overviewWindowName
		placement = overviewWindowInsertArgs(windows)
	} else {
		selectNew = e.newAgentWindowTakesViewerLocked(st, s.Display.Focus)
	}
	out, err := e.newWindowLocked(placement,
		[]string{"-P", "-F", "#{pane_id} #{window_id}", "-n", name, "-c", e.geom.PaneCwd})
	if err != nil {
		return "", fmt.Errorf("new window: %w", err)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "", fmt.Errorf("new-window created no usable window (got %q)", strings.TrimSpace(out))
	}
	paneID, windowID := fields[0], fields[1]

	setOption := func(option, value string) {
		if err := e.tmux.run("set-option", "-w", "-t", windowID, option, value); err != nil {
			logger.Warn("reed: could not set an own-window option", "strand", s.GUID, "window", windowID, "option", option, "err", err)
		}
	}
	setOption(ownWindowOption, "1")
	if !overview {
		if color, ok := e.segmentColor(s.Segment); ok {
			if tmuxColor, ok := segmentcolor.TmuxColor(color); ok {
				setOption("@strand_color", tmuxColor)
			}
		}
		if runtime.GOOS != "windows" && !selectNew {
			setOption(unseenWindowOption, "1")
		}
	}
	if group != "" {
		setOption(groupWindowOption, group)
	}
	if selectNew {
		if err := e.tmux.run("select-window", "-t", windowID); err != nil {
			logger.Warn("reed: could not select the new agent window", "strand", s.GUID, "window", windowID, "err", err)
		}
	}

	logger.Info("reed: opened own window", "socket", e.Socket(), "session", e.SessionName(), "strand", s.GUID, "window", windowID, "pane", paneID)
	return paneID, nil
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
	// ensureServerAndSessionLocked, AddStrand via ensureSessionLocked's own self-heal, every other
	// op via requireSessionLocked — so the generation probe always has a session to ask about. See
	// generation.go.
	if err := e.adoptPaneGenerationLocked(st); err != nil {
		return nil, err
	}

	// Repair a table whose pane bindings contradict each other before any caller reads it. This runs
	// on every load, not only after a rebirth, because the corrupt tables it exists for arrive
	// BETWEEN boots (a restored backup, a hand-edited file) and would otherwise be trusted all the
	// way into select-layout, where tmux answers the contradiction by destroying panes — see
	// clearConflictingPaneBindings for both observed shapes.
	if cleared := clearConflictingPaneBindings(st); len(cleared) > 0 {
		// The clear is in memory only, so a repeat of the same cleared set logs at Debug.
		logClear := logger.Debug
		if e.stateLoadNotices.conflictClearIsNew(cleared) {
			logClear = logger.Warn
		}
		logClear("reed: cleared strand pane bindings that named a pane another owner already claims",
			"socket", e.Socket(), "session", e.SessionName(), "strands", cleared)
	} else {
		e.stateLoadNotices.forgetConflictClear()
	}
	return st, nil
}

// reconcileApplyPersistLocked fetches current panes, reconciles dead
// pane bindings, reapplies the layout, and persists the state. It is the
// shared tail every public op composes after mutation.
func (e *Engine) reconcileApplyPersistLocked(st *ReedState) ([]LivePane, error) {
	live, windows, err := e.listStrandAndWindowPanes(st)
	if err != nil {
		return nil, fmt.Errorf("list panes: %w", err)
	}

	killed, err := e.reconcileLocked(st, live, windows)
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	if len(killed) > 0 {
		// Order matters: kill dead -> re-enumerate live -> compute layout
		// -> apply. The kill-pane calls above mutate the pane set the next
		// select-layout must enumerate, so enumeration must follow them.
		live, err = e.listStrandPanes(st)
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
