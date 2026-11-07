// spawn.go holds the pane-launch helper that every path realizing a strand shares.
// launchStrandLocked creates a fresh tmux pane for the strand and runs its command in it.
// It also holds two bootstrap helpers that strand.go and lifecycle.go both need.
// loadOrInitStateLocked initializes state for a fresh worktree.
// reconcileApplyPersistLocked runs the reconcile, apply and persist tail that every public operation ends with.

package reedengine

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shell"
)

// validateSplitCreatedNewPane returns an error unless paneID is new, absent from preSplitLive.
// psmux's split-window fails silently on a pane too small to split and prints an existing pane's id.
// Accepting that id would bind two owners to one pane.
func validateSplitCreatedNewPane(paneID string, preSplitLive []LivePane, target string) error {
	if paneID == "" || liveIDSet(preSplitLive)[paneID] {
		return fmt.Errorf("split-window created no new pane (got %q; target %s likely too small to split)", paneID, target)
	}
	return nil
}

// sendKeysLiteralArg returns the argument that tmux send-keys -l must receive so that text types verbatim.
// tmux parses a dash-leading argument as a flag and silently drops it, so a dash-leading cmd gets a leading space.
func sendKeysLiteralArg(text string) string {
	if strings.HasPrefix(text, "-") {
		return " " + text
	}
	return text
}

// launchStrandLocked reconciles the session's panes, creates a fresh pane for s, and runs launchCmd in it.
// The pane goes in a window of its own, or in its group's shared window.
// The launch then sets the title lock and the pane options.
// It then types the prelude, the launch script and the send-keys into the pane.
// It sets only s.PaneID; Live is derived from pane binding downstream.
//
// Reconciling here, rather than trusting the caller's last reconcile, makes "reap before allocate" hold on every
// strand-realizing path by construction.
// Reconcile is safe for the strand being launched, though each path reaches it in a different state.
//   - On AddStrand and UpdateStrand, s has PaneID == "": addStrandLocked builds a fresh record, and updateStrandLocked
//     launches a strand that has never held a pane. Reconcile can neither clear nor kill anything belonging to it.
//   - On Resume, s may still be bound to a pane absent from engineAliveIDSet, such as a dead pane that is still present
//     or a shell-only pane past the launch grace. That binding names a dead pane, so reconcile kills it as a dead pane,
//     spares it as the kept dead pane, or leaves the shell-only pane for the own-window kill. Either way this function
//     overwrites s.PaneID with the new pane.
//     Strands that Resume already relaunched earlier in the same pass hold alive panes by then.
//     So the untracked reap exempts them.
//
// This does not call SaveState. AddStrand and UpdateStrand persist only after this function returns nil.
// A split-window or send-keys failure here therefore leaves panes killed in tmux.
// The cleared binding exists only in memory, and reed.json is untouched.
// That window is accepted rather than closed, because it self-heals.
// Status derives liveness from the live pane set, never from the persisted binding.
// The next mutating verb's reconcile then clears the stale bindings for real.
// Closing it here would be worse.
// Persisting inside this helper would write the half-added strand record on the add path.
// That turns a clean failure into a phantom strand that Resume would later try to launch.
// If the window ever needs closing, reap before the strand record is appended to st.Strands.
// Do not persist a partial record from inside this helper.
//
// The prelude that resolves this strand pane's `lym` to the spawning binary is owned by panebin.go.
// composePaneLaunchLine composes it here, at the one launch every strand-realizing path funnels through.
// The same composition exports LYM_STRAND_NAME and LYM_PARENT beside LYM_BIN.
// The name is the strand's stored birth attribute, and the parent is re-told from the geometry.
// Every launch and resume path therefore sees the same values.
// The variables that the geometry's PaneEnv supplies are exported ahead of launchCmd, read afresh at each launch.
// The window created here carries no trailing shell-command argument,
// so the pane runs tmux's default shell, started as a login shell.
// The prelude and launchCmd reach that shell through the strand's launch script.
// The send-keys line types the script's source statement into the pane.
// The shell sources that statement in its own scope, rather than the pane's shell starting differently.
// A script write failure degrades to typing the composed line itself.
// The script is written on every launch.
// A relaunch with a different command or a different spawning executable therefore always regenerates it.
func (e *Engine) launchStrandLocked(st *ReedState, s *Strand, launchCmd string) error {
	live, windows, err := e.listReconcilePanesLocked(st)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}

	if _, err := e.reconcileLocked(st, live, windows); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	previousPaneID := s.PaneID
	paneID, err := e.createOwnWindowPaneLocked(st, s)
	if err != nil {
		return err
	}

	s.PaneID = paneID
	e.killReplacedOwnWindowPaneLocked(previousPaneID, paneID)
	// Both title commands only mirror the display: allow-set-title off stops the agent inside from overwriting the name.
	// No lookup or delivery resolves through a title, so a failure of either costs display only and never fails the launch.
	// A tmux that predates allow-set-title skips the option, and the version probe has already logged that fallback once.
	if !e.tmux.lacksAllowSetTitle() {
		if err := e.tmux.run("set-option", "-p", "-t", paneID, "allow-set-title", "off"); err != nil {
			logger.Warn("reed: could not turn off allow-set-title, the pane title may drift", "strand", s.GUID, "pane", paneID, "err", err)
		}
	}
	if err := e.tmux.run("select-pane", "-t", paneID, "-T", s.Name); err != nil {
		logger.Warn("reed: could not set the pane title", "strand", s.GUID, "pane", paneID, "name", s.Name, "err", err)
	}
	e.setStrandPaneOptionsLocked(paneID, *s)
	// composePaneLaunchLine joins the pane-binary prelude (panebin.go) onto launchCmd in the shell.ForGOOS() dialect.
	// The launch command was built in that same dialect, so the two never disagree about which shell is typed into.
	// stageLaunchScript writes the composed line to the strand's launch script.
	// It returns the dialect's source statement for that script, which is what the pane shows.
	// A write failure returns the composed line itself, so the pane still launches.
	// The payload goes as a literal string (-l), so tmux reads no part of it as a key name such as "Enter" or "C-c".
	// It also never splits the payload on an embedded ';'.
	// The launch command is opaque to tmux: shuttle builds arbitrary PowerShell command chains.
	// A separate Enter then submits it.
	// The dialect's discard-line key goes first.
	// Input that reaches the booting pane before the payload would otherwise prefix the launch line and break it.
	// Such input includes a viewer's keystrokes or a terminal reply that arrives while a window switch lands on the pane.
	sh := shell.ForGOOS()
	launchCmd = sh.Chain(e.paneEnvExports(sh, s.GUID), launchCmd)
	composedLine, exe := composePaneLaunchLine(sh, e.executablePath, launchCmd, s.GUID, s.Name, e.geom.ParentName)
	s.LymBin = exe
	payload := stageLaunchScript(sh, e.stateDir(), s.GUID, composedLine)
	if key := sh.DiscardLineKey(); key != "" {
		if err := e.tmux.run("send-keys", "-t", paneID, key); err != nil {
			return fmt.Errorf("discard the pane's pending input: %w", err)
		}
	}
	if err := e.tmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(payload)); err != nil {
		return fmt.Errorf("send launch command: %w", err)
	}
	if err := e.tmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
		return fmt.Errorf("submit launch command: %w", err)
	}
	return nil
}

// killReplacedOwnWindowPaneLocked kills the pane that a relaunched own-window strand held before newPaneID replaced it.
// It kills only when the session still lists that pane in an own window.
// It runs after the new pane exists, so a shared window that held only the old pane is never ended mid-launch.
// An empty or unchanged previous pane, or one no longer listed, is left alone.
// A failure is logged and never fails the launch.
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

// createOwnWindowPaneLocked creates the pane of an own-window strand and returns its id.
// A strand with no window group gets a window of its own named by its role.
// A grouped strand's placement opens the group's shared window, joins it, or gives it a window of its own.
// The group record the placement returns is stored in st, which the caller persists with the strand.
// A refusal returns before anything is created.
// A shared window is laid out evenly in the window_split direction after a new pane joins or opens it.
func (e *Engine) createOwnWindowPaneLocked(st *ReedState, s *Strand) (string, error) {
	group := s.Display.Window
	if group == "" {
		return e.openOwnWindowLocked(st, s, strandLabel(s.Name), "")
	}

	// Listed here, after any head reconcile that ended an own window,
	// so the plan and the join read the current windows.
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

// joinGroupWindowLocked splits the first listed pane of the group window groupWindow in the direction splitFlag.
// It returns the new pane's id.
// The split is detached, so the group window's active pane stays where it is,
// and a joiner never takes focus from an operator viewing the window.
// A split that reuses an existing pane is refused.
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
// A placed new-window the multiplexer refuses on Windows logs at Debug and is retried once in the plain form,
// so the order is best-effort there;
// on every other platform the error is returned.
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
// The window is created with -d, so the client's current window and active pane stay where they are.
// An own-window strand's window is selected only when it takes the viewer:
// a Focus display, or, unless it sets KeepViewer, a session whose follow state reads on.
// A window selected here carries no @lym_unseen.
// The window it opens never changes the follow state.
// An overview window is never selected here.
// It is placed behind every earlier agent window and before the shell window,
// and a relaunch keeps the place of the window its old pane was in.
// It carries no command, so the pane is the default login shell like every strand pane.
// An overview strand's window is named by its display's Title, or by the name "overview" when that is empty.
// It opens after the last overview window whose order key is not higher than its own.
// Failing that, it opens before the lowest-index window, hidden ones included.
// It carries the own marker and the overview marker, but neither @strand_color nor @lym_unseen.
// A non-empty group is recorded on the window, which is how a group's shared window is found later.
// A window option that fails to set is logged and never fails the open.
// Reconcile never ends a window that lacks the own marker.
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
		name = overviewWindowTitle(s.Display)
		placement = overviewWindowInsertArgs(windows, overviewWindowKeys(st.Strands, windowByPane), s.Display.Order)
	} else {
		selectNew = e.newAgentWindowTakesViewerLocked(s.Display)
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

	e.markOwnWindowLocked(s, windowID, overview, !selectNew, group)
	if selectNew {
		if err := e.tmux.run("select-window", "-t", windowID); err != nil {
			logger.Warn("reed: could not select the new agent window", "strand", s.GUID, "window", windowID, "err", err)
		}
	}

	logger.Info("reed: opened own window", "socket", e.Socket(), "session", e.SessionName(), "strand", s.GUID, "window", windowID, "pane", paneID)
	return paneID, nil
}

// markOwnWindowLocked sets the own-window options on windowID, the window of s's pane:
// the own marker always, and then the overview marker for an overview window.
// Any other window gets the strand's @strand_color and, off Windows, @lym_unseen when unseen is set;
// a non-empty group is recorded on the window.
// A window option that fails to set is logged and never fails the step.
// Reconcile never ends a window that lacks the own marker.
func (e *Engine) markOwnWindowLocked(s *Strand, windowID string, overview, unseen bool, group string) {
	setOption := func(option, value string) {
		if err := e.tmux.run("set-option", "-w", "-t", windowID, option, value); err != nil {
			logger.Warn("reed: could not set an own-window option", "strand", s.GUID, "window", windowID, "option", option, "err", err)
		}
	}
	setOption(ownWindowOption, "1")
	if overview {
		setOption(overviewWindowOption, "1")
	} else {
		if color, ok := e.segmentColor(s.Segment); ok {
			if tmuxColor, ok := segmentcolor.TmuxColor(color); ok {
				setOption("@strand_color", tmuxColor)
			}
		}
		if runtime.GOOS != "windows" && unseen {
			setOption(unseenWindowOption, "1")
		}
	}
	if group != "" {
		setOption(groupWindowOption, group)
	}
}

// loadOrInitStateLocked loads or initializes a ReedState stamped with
// the engine's server/socket/session identity for a fresh worktree.
// The two identity fields are re-stamped on every load, not only at init.
// Nothing in production reads them back, so they exist only as an on-disk forensic diagnostic.
// A diagnostic that records an identity reed no longer drives is worse than none:
// a renamed worktree carries its .lym state along, but its session name changes with the directory.
// The caller persists the state through its normal save, so no extra write happens here.
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

	// Discard bindings minted against a session incarnation other than the running one.
	// Refuse outright when the session they were minted against is still alive on this socket under another name.
	// Callers reach here with the told session already up, so the generation probe always has a session to ask about.
	if err := e.adoptPaneGenerationLocked(st); err != nil {
		return nil, err
	}

	// Repair a table whose pane bindings contradict each other before any caller reads it.
	// This runs on every load, not only after a rebirth.
	// Corrupt tables arrive between boots, from a restored backup or a hand-edited file.
	// Unrepaired, they would reach select-layout, where tmux answers the contradiction by destroying panes.
	// See clearConflictingPaneBindings for the shapes it repairs.
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

// reconcileApplyPersistLocked fetches current panes, reconciles dead pane bindings, and persists the state.
// It is the shared tail every public op composes after mutation.
func (e *Engine) reconcileApplyPersistLocked(st *ReedState) ([]LivePane, error) {
	live, windows, err := e.listReconcilePanesLocked(st)
	if err != nil {
		return nil, fmt.Errorf("list panes: %w", err)
	}

	killed, err := e.reconcileLocked(st, live, windows)
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	if len(killed) > 0 {
		// The kill-pane calls above changed the pane set.
		// Callers read the returned panes, so enumeration must follow them.
		live, err = e.listStrandPanes(st)
		if err != nil {
			return nil, fmt.Errorf("list panes after reconcile: %w", err)
		}
	}

	if err := SaveState(e.stateDir(), st); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	return live, nil
}
