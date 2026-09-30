// handover.go decides and performs the terminal handover at the end of `start`.
// It is loomcli's decision, duplicated rather than imported because a <module>cli importing another <module>cli would couple two cobra seams.
// The attach and switch-client spawns are this command's interactive-handoff exception to the CLI/Cobra Invariant: every fallible step has reported on the envelope before they run.

package orchcli

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"golang.org/x/term"
)

// handover is what `start` does with the operator's terminal.
type handover int

const (
	// handoverAttach attaches the terminal to reed's tmux session.
	handoverAttach handover = iota
	// handoverEnvelope returns the success envelope: the caller is unattended or already in the session.
	handoverEnvelope
	// handoverSwitch switches the caller's tmux client onto the prime's session.
	handoverSwitch
	// handoverHint returns the success envelope with a hint to attach from outside tmux.
	handoverHint
)

// attachHint is the envelope hint for a caller inside a tmux server reed does not own.
const attachHint = `run "lyx reed attach" from outside tmux to reach the orchestrator's session`

// decideHandover picks the handover from where the command runs.
// An unreadable current session (empty while reed owns $TMUX) answers with a hint, never a nested attach.
func decideHandover(noAttach bool, tmuxEnv string, reedOwns bool, currentSession, targetSession string) handover {
	switch {
	case noAttach:
		return handoverEnvelope
	case tmuxEnv == "":
		return handoverAttach
	case !reedOwns:
		return handoverHint
	case currentSession == "":
		return handoverHint
	case currentSession == targetSession:
		return handoverEnvelope
	default:
		return handoverSwitch
	}
}

// exitCodeOf maps a finished command's error onto an exit code.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

// currentTmuxSession reads the caller's tmux session when reed owns $TMUX, "" otherwise or on failure.
func (c *orchCLI) currentTmuxSession(reedOwns bool) string {
	if !reedOwns {
		return ""
	}
	sess, err := c.reed.ClientSession(os.Getenv("TMUX_PANE"))
	if err != nil {
		logger.Warn("orch: could not read the client's tmux session, returning a hint instead of switching", "err", err)
		return ""
	}
	return sess
}

// runHandover performs a handoverSwitch or handoverAttach spawn, setting the exit code from the child's.
func (c *orchCLI) runHandover(ctx context.Context, decision handover) {
	if decision == handoverSwitch {
		cmd := exec.Command(c.reed.TmuxPath(), c.reed.SwitchClientArgv()...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		logger.Info("orch: spawning tmux switch-client", "tmux", c.reed.TmuxPath(), "session", c.reed.SessionName())
		code := exitCodeOf(cmd.Run())
		logger.Info("orch: tmux switch-client exited", "tmux", c.reed.TmuxPath(), "exitCode", code)
		if code != 0 {
			clihelp.SetExit(ctx, code)
		}
		return
	}

	// A missing terminal size (piped output) degrades to the bare attach argv rather than failing.
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		logger.Warn("orch: no terminal size available, attaching without a chained layout", "err", err)
		cols, rows = 0, 0
	}
	cmd := exec.Command(c.reed.TmuxPath(), c.reed.AttachArgv(cols, rows)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	logger.Info("orch: spawning tmux attach", "tmux", c.reed.TmuxPath(), "cols", cols, "rows", rows)
	code := exitCodeOf(cmd.Run())
	logger.Info("orch: tmux attach exited", "tmux", c.reed.TmuxPath(), "exitCode", code)
	if code != 0 {
		clihelp.SetExit(ctx, code)
	}
}
