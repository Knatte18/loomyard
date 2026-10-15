// follow.go holds the per-session follow state: the session option @lyx_follow and the read that turns it into a decision.
// The option is the one decision-bearing session option reed reads, unlike @strand, @strand_color, @lyx_unseen and the wait marks, which stay display only.

package reedengine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// followSessionOption is the session option holding the follow state.
// Follow is off only while it reads "off"; an unset option reads on.
const followSessionOption = "@lyx_follow"

// followingLocked reports whether the session's viewer follows the run, reading followSessionOption.
// Only the trimmed value "off" reads false, so a session booted by an older lyx, whose option is unset, follows.
// A failed read reports false and is logged at Warn, so a broken read never moves a viewer.
func (e *Engine) followingLocked() bool {
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{"+followSessionOption+"}")
	if err != nil {
		logger.Warn("reed: could not read the follow state, treating follow as off", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		return false
	}
	return strings.TrimSpace(out) != "off"
}

// FollowRun turns the session's follow state on and selects the relevant window: the newest live agent window, else the overview, else nothing.
// It returns the no-session and state-load errors; a failed option write is logged at Warn and the select still runs, and a failed listing or select is logged and never fails the call.
// It sets one session option and selects one window of the engine's own session, and persists no state.
func (e *Engine) FollowRun() error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}
		if err := e.tmux.run("set-option", "-t", exactSessionWindowTarget(e.SessionName()), followSessionOption, "on"); err != nil {
			logger.Warn("reed: could not turn the follow state on", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
		e.selectRelevantWindowLocked(st, nil)
		return nil
	})
}

// overviewWindowOption is the window option reed sets on an overview window beside ownWindowOption.
// It lets a caller told only a socket and a session tell the overview from an agent window without reed state.
const overviewWindowOption = "@lyx_overview"

// followTargetFormat is the session-wide listing format followTargetWindow reads: window id, window index, own flag, overview flag, then the pane's dead flag.
const followTargetFormat = "#{window_id} #{window_index} #{?#{@lyx_own},1,0} #{?#{@lyx_overview},1,0} #{pane_dead}"

// followTargetWindow returns the window a follow jump lands on, read from a followTargetFormat listing.
// It is the same rule as relevantWindow, read from window options instead of reed state:
// among the windows carrying the own option and not the overview one, the highest-index window holding a pane that is not dead, else the window carrying the overview option, else the empty string.
// An overview window opened by an older lyx carries no overview marker and so counts as an agent window, which it can win only when no other agent is live, the same outcome as the overview fallback.
// A line it cannot parse is skipped.
func followTargetWindow(listing string) string {
	best, bestIndex, overview := "", -1, ""
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 5 {
			continue
		}
		index, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		own, isOverview, dead := fields[2] == "1", fields[3] == "1", fields[4] == "1"
		switch {
		case isOverview:
			overview = fields[0]
		case own && !dead && index > bestIndex:
			best, bestIndex = fields[0], index
		}
	}
	if best != "" {
		return best
	}
	return overview
}

// FollowSession turns follow on for the told session of the server listening on socketPath and selects the session's relevant window.
// It reads no working directory, config or state: the window is chosen from tmux facts alone.
// It issues list-panes, set-option and, when a window is relevant, select-window on that socket, and nothing else.
// The error names an unknown session, a refused option write or a refused select.
func FollowSession(tmuxPath, socketPath, session string) error {
	return followSessionVia(newTmuxCmdForSocketPath(tmuxPath, socketPath), session)
}

// followSessionVia is FollowSession's implementation over an already-built TmuxCmd, so a test can drive it through the execHook seam.
// A failed listing, which is how an unknown session answers, returns before any write, and a failed option write returns before any select.
func followSessionVia(cmd TmuxCmd, session string) error {
	listing, err := cmd.output("list-panes", "-s", "-t", exactSessionTarget(session), "-F", followTargetFormat)
	if err != nil {
		return fmt.Errorf("list the panes of session %q: %w", session, err)
	}
	if err := cmd.run("set-option", "-t", exactSessionWindowTarget(session), followSessionOption, "on"); err != nil {
		return fmt.Errorf("turn follow on for session %q: %w", session, err)
	}
	window := followTargetWindow(listing)
	if window == "" {
		return nil
	}
	if err := cmd.run("select-window", "-t", window); err != nil {
		return fmt.Errorf("select window %s of session %q: %w", window, session, err)
	}
	return nil
}
