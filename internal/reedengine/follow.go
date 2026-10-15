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
