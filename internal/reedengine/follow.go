// follow.go holds the per-session follow state: the session option @lyx_follow and the read that turns it into a decision.
// The option is the one decision-bearing session option reed reads, unlike @strand, @strand_color, @lyx_unseen and the wait marks, which stay display only.

package reedengine

import (
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
		if err := e.tmux.run("set-option", "-t", exactSessionTarget(e.SessionName()), followSessionOption, "on"); err != nil {
			logger.Warn("reed: could not turn the follow state on", "socket", e.Socket(), "session", e.SessionName(), "err", err)
		}
		e.selectRelevantWindowLocked(st, nil)
		return nil
	})
}
