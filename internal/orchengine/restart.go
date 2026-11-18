// restart.go holds the watcher's restart of the orch's session:
// onto another account when a usage limit stops it, and fresh when a resume was refused.

package orchengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// OrchRestart holds the two restarts a watcher may run over the orch's session.
// Each is given the watcher's current state and returns the state it saved on the new strand.
// A restart removes the old strand before it launches, so a failed one leaves no strand to go on with.
type OrchRestart struct {
	// Resume relaunches the orch's recorded session, under the account the worktree now resolves to.
	Resume func(State) (State, error)
	// Fresh launches the orch fresh, with no recorded session to resume.
	Fresh func(State) (State, error)
}

// WatchRestart tells the watcher the restarts it may run.
// A watcher never told one keeps the usage-limit wait and ignores a refused resume.
func (w *Watcher) WatchRestart(restart OrchRestart) {
	w.restart = &restart
}

// restartOnTurnEnd restarts the orch's session when its newest turn end calls for it.
// A usage-limited turn end restarts through Resume when the session's worktree switched to another account.
// Any other turn end restarts through Fresh when the session reads it as a refused resume's.
// A failed answer from the session is logged and restarts nothing.
// It reports whether a restart ran, and whether the watcher ends because the restart failed.
// A failed restart records the exit reason in st, which it saves.
func (w *Watcher) restartOnTurnEnd(st State, limit shuttleengine.UsageLimit, turnEnd shuttleengine.Event) (restarted, ended bool, err error) {
	run, mode := w.restart.Fresh, "fresh"
	if limit.Limited {
		run, mode = w.restart.Resume, "resume"
	}
	if run == nil {
		return false, false, nil
	}
	var wanted bool
	var askErr error
	if limit.Limited {
		wanted, askErr = w.session.SwitchAccountOnLimit(st.Strand, turnEnd)
	} else {
		wanted, askErr = w.session.RefusedResume(st.Strand, turnEnd)
	}
	if askErr != nil {
		logger.Warn("orch: asking whether the orch's session must restart failed", "strandGUID", st.Strand, "mode", mode, "cause", askErr)
		return false, false, nil
	}
	if !wanted {
		return false, false, nil
	}
	next, err := run(st)
	if err != nil {
		logger.Warn("orch: restarting the orch's session failed after its old strand was removed; the watcher ends; way forward: run `lym beam start`",
			"strandGUID", st.Strand, "mode", mode, "cause", err)
		st.WatcherExit = fmt.Sprintf("restart failed: %v", err)
		return true, true, w.save(st)
	}
	logger.Info("orch: restarted the orch's session", "mode", mode, "previousStrandGUID", st.Strand, "strandGUID", next.Strand)
	return true, false, nil
}
