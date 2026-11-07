// overlay.go holds the tmux subprocess layer: TmuxCmd runs tmux on one server and exposes typed helpers.
// Each invocation is traced with logger.Debug, so -vv shows the exact tmux command line.
// The file is domain-free: it knows only tmux session and pane primitives, not any caller's vocabulary.

package reedengine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// TmuxCmd runs tmux for one binary and one server, addressed by -L socket name or by -S socket path.
type TmuxCmd struct {
	tmuxPath string
	socket   string
	// socketPath, when non-empty, addresses the server by `-S <path>` instead of `-L <socket>`.
	socketPath string
	// configFile, when non-empty, leads the server flags with `-f <configFile>`.
	// A server this command starts reads that config, not the operator's; a running server ignores the flag.
	configFile string
	// execHook, when non-nil, replaces the subprocess exec in both run and output.
	// Tests set it to drive a composed engine call site against a scripted tmux response.
	// No live server is needed.
	// It is the only way to reproduce the psmux-only failure shape:
	// tmux exits 0 and prints the id of an existing pane, which native tmux cannot produce.
	// Each split site guards against that shape.
	// capture is true for a call that captures stdout and false for one that discards it.
	// args is the tmux subcommand argv without the leading -L socket.
	// A hook therefore matches on args[0].
	// stdin is the bytes written to the command's standard input, passed beside args rather than inside them.
	// It is nil except for a call that writes stdin.
	// Production never sets the hook.
	// A set hook is a test-only override.
	execHook func(capture bool, stdin []byte, args ...string) (string, error)
	// floorLine is the `tmux -V` line that the floor check reads for a hook-driven command.
	// It is read only when execHook is set; empty admits any version.
	floorLine string
	// pause, when non-nil, replaces time.Sleep between EAGAIN retries, so a test retries without waiting.
	pause func(time.Duration)
}

const (
	// execAgainAttempts is the total number of tries for a tmux exec that fails with EAGAIN.
	execAgainAttempts = 4
	// execAgainFirstDelay is the wait before the first retry of such an exec; each later wait doubles it.
	execAgainFirstDelay = 250 * time.Millisecond
)

// retryExecAgain calls attempt again after a doubling wait while it fails with EAGAIN.
// It makes at most execAgainAttempts calls.
// It returns the first result that is not an EAGAIN failure, or the last attempt's.
// Each EAGAIN failure is noted in the process's spawn-refusal record.
// Each retry is logged at Warn with the tmux argv.
// A tmux that started and exited nonzero is never retried, so a retry never repeats a tmux command that ran.
func retryExecAgain[T any](p TmuxCmd, args []string, attempt func() (T, error)) (T, error) {
	delay := execAgainFirstDelay
	for try := 1; ; try++ {
		value, err := attempt()
		if !proc.NoteSpawnAgain(err) || try == execAgainAttempts {
			return value, err
		}
		logger.Warn("reed: tmux could not be started, retrying", "args", args, "attempt", try, "delay", delay, "err", err)
		if p.pause != nil {
			p.pause(delay)
		} else {
			time.Sleep(delay)
		}
		delay *= 2
	}
}

// NewTmuxCmd builds a TmuxCmd bound to the given binary and -L socket.
func NewTmuxCmd(tmuxPath, socket string) TmuxCmd {
	return TmuxCmd{tmuxPath: tmuxPath, socket: socket}
}

// newTmuxCmdForSocketPath builds a TmuxCmd bound to the given binary and the server listening on socketPath (-S).
func newTmuxCmdForSocketPath(tmuxPath, socketPath string) TmuxCmd {
	return TmuxCmd{tmuxPath: tmuxPath, socketPath: socketPath}
}

// serverArgs returns the leading flags that address this command's server.
func (p TmuxCmd) serverArgs() []string {
	var args []string
	if p.configFile != "" {
		args = append(args, "-f", p.configFile)
	}
	if p.socketPath != "" {
		return append(args, "-S", p.socketPath)
	}
	return append(args, "-L", p.socket)
}

// run runs a command with the server flags prepended and folds stderr into the returned error.
// A tmux that could not be started for EAGAIN is retried by retryExecAgain.
func (p TmuxCmd) run(args ...string) error {
	return p.runStdin(nil, args...)
}

// runStdin runs a command with stdin written to its standard input.
// A nil stdin leaves the standard input empty.
// The bytes are never logged.
func (p TmuxCmd) runStdin(stdin []byte, args ...string) error {
	if err := p.checkFloor(); err != nil {
		return err
	}
	_, err := retryExecAgain(p, args, func() (struct{}, error) {
		if p.execHook != nil {
			_, err := p.execHook(false, stdin, args...)
			return struct{}{}, err
		}
		fullArgs := append(p.serverArgs(), args...)
		logger.Debug("tmux", "args", fullArgs)
		cmd := exec.Command(p.tmuxPath, fullArgs...)
		var stderr bytes.Buffer
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		cmd.Stdout = io.Discard
		cmd.Stderr = &stderr
		err := cmd.Run()
		return struct{}{}, wrapTmuxError(err, stderr.Bytes())
	})
	return err
}

// output runs a command with the server flags prepended and captures its stdout, folding stderr into the error.
// A tmux that could not be started for EAGAIN is retried by retryExecAgain.
func (p TmuxCmd) output(args ...string) (string, error) {
	if err := p.checkFloor(); err != nil {
		return "", err
	}
	return retryExecAgain(p, args, func() (string, error) {
		if p.execHook != nil {
			return p.execHook(true, nil, args...)
		}
		fullArgs := append(p.serverArgs(), args...)
		logger.Debug("tmux", "args", fullArgs)
		cmd := exec.Command(p.tmuxPath, fullArgs...)
		out, err := cmd.Output()
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), wrapTmuxError(err, exitErr.Stderr)
		}
		return string(out), err
	})
}

// wrapTmuxError attaches tmux's stderr to err, so a failure carries tmux's own diagnosis.
// The result wraps err, so errors.As can unwrap it.
func wrapTmuxError(err error, stderr []byte) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
}

// exactSessionTarget returns session as an exact-match target ("=<name>").
// Without the "=" prefix, tmux may prefix-match another session, such as a sibling worktree's.
func exactSessionTarget(session string) string {
	return "=" + session
}

// exactSessionWindowTarget returns session as an exact-match window or pane target ("=<name>:").
// tmux's window and pane parsing rejects a bare "=<name>", which needs the trailing ":".
func exactSessionWindowTarget(session string) string {
	return "=" + session + ":"
}

// ListSessions returns the name of every session live on the -L socket socketKey, using the binary tmuxPath.
// It takes no Engine, because enumerating a hub socket's sessions happens before any Engine exists for them.
// Off Windows it lists only reed's sessions.
// When the server's starter sourced an operator tmux config, a session that config creates is not listed.
//
// The binary is passed explicitly, not loaded from a config.
// Every session on one hub socket shares one tmux binary.
// Naming it is the only coherent answer for an enumeration that spans sessions.
//
// The three outcomes are distinct.
// A non-empty slice means sessions are live.
// An empty slice with a nil error means tmux exited 0 with no sessions, a normal empty hub.
// A non-nil error means the round trip itself failed: no server, an unreachable socket, or another tmux failure.
// The error carries tmux's stderr, as output attaches it, and nothing more.
func ListSessions(tmuxPath, socketKey string) ([]string, error) {
	return listSessionsVia(NewTmuxCmd(tmuxPath, socketKey))
}

// IsNoServer reports whether err is a tmux error for a socket with no server behind it.
// That covers `no server running on`, which a socket file that refuses connections reports.
// It also covers `error connecting to` followed by `(No such file or directory)`, which means the socket is absent.
// An absent socket occurs after a reboot, or on a hub whose server never started.
func IsNoServer(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	if strings.Contains(message, "no server running on") {
		return true
	}
	return strings.Contains(message, "error connecting to") && strings.Contains(message, "(No such file or directory)")
}

// listSessionsVia lists the sessions of the server that cmd addresses, and returns their names.
// It takes an already-built TmuxCmd, so a test can drive the parsing through the execHook seam without a live server.
func listSessionsVia(cmd TmuxCmd) ([]string, error) {
	out, err := cmd.output(listSessionsArgv(runtime.GOOS, "#{session_name}")...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

// reapSessionPanes lists the panes of every window of session.
// A kill-session ends all of those panes, so a session reap covers the process trees of every window.
// It is split out so a test can drive the pane-listing half through TmuxCmd's execHook seam directly.
func reapSessionPanes(cmd TmuxCmd, session string) ([]LivePane, error) {
	return cmd.listSessionPanes(session)
}

// reapSessionKill kills session by exact-match target.
// The exactSessionTarget wrapper is mandatory: a bare "-t session" prefix-matches a sibling worktree's session.
func reapSessionKill(cmd TmuxCmd, session string) error {
	return cmd.run("kill-session", "-t", exactSessionTarget(session))
}

// ReapSession reaps a session whose worktree no longer exists, along with the processes its panes run.
// It builds no Engine, because a gone worktree has no live Geometry to build one from.
// It acquires no lock and touches no state directory, which is what makes it legal for a gone worktree.
// It reads no geometry beyond socketKey and sessionName, both passed as plain strings.
//
// It runs four steps in this exact order, and the order is load-bearing.
// The pane-derived descendant closure is computed in step 2, before the kill in step 3.
// kill-session reparents the pane children, so a closure computed after the kill collapses to the root pids alone;
// it silently loses the detached agent descendants the reap exists to kill.
//
// timeout bounds how long step 4 waits for the pane children to exit before force-killing them.
// Zero selects reapExitTimeout.
func ReapSession(tmuxPath, shellPath, socketKey, sessionName string, timeout time.Duration) error {
	return reapSessionVia(NewTmuxCmd(tmuxPath, socketKey), shellPath, socketKey, sessionName, timeout)
}

// reapSessionVia reaps a session over an already-built TmuxCmd, so a test can drive it through the exec seam.
func reapSessionVia(cmd TmuxCmd, shellPath, socketKey, sessionName string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = reapExitTimeout
	}
	live, err := reapSessionPanes(cmd, sessionName)
	if err != nil {
		// A session whose panes cannot be listed is the one most worth killing, so a
		// listing failure never skips the kill — it only loses the descendant closure.
		logger.Warn("reed: could not list panes before reaping session", "socket", socketKey, "session", sessionName, "err", err)
		live = nil
	}

	// eng carries only the one field that descendantClosurePIDs' Windows body reads (it spawns e.cfg.Shell);
	// its Linux body reads no Engine field at all, and neither body reads geometry.
	// No Geometry value is built here, not even a zero one: this Engine is a throwaway that exists only to reach one method.
	eng := &Engine{cfg: Config{Shell: shellPath}}
	pids := eng.descendantClosurePIDs(sessionReapRoots(live))

	logger.Info("reed: reaping orphaned session", "socket", socketKey, "session", sessionName, "pids", len(pids))
	killErr := reapSessionKill(cmd, sessionName)
	reapPaneChildren(pids, timeout)
	logger.Info("reed: reaped orphaned session", "socket", socketKey, "session", sessionName, "err", killErr)

	return killErr
}

// EndSessionByName ends the named session of a worktree that is already gone.
// It takes no lock and writes no state, because both lived under the gone worktree and nothing is left to guard.
// Creating a lock would recreate the gone worktree's .lym directory, so none is created.
// It reads no geometry: it is given only plain strings.
// It reaches only the exact-match session it is named, on the socket it is given;
// it kills that socket's server only when no session remains on it.
// After that kill it removes the server's socket file as a best effort.
// It waits a bounded time for the server to stop answering.
// A socket file that still answers after that stays, with no error.
// It must never be called for a session whose worktree is still present.
//
// It reports whether a session existed and was reaped.
// An absent session returns (false, nil) and touches nothing.
func EndSessionByName(tmuxPath, shellPath, socketKey, sessionName string) (bool, error) {
	return endSessionByNameVia(NewTmuxCmd(tmuxPath, socketKey), shellPath, socketKey, sessionName)
}

// endSessionByNameVia ends a session over an already-built TmuxCmd, so a test can drive it through the exec seam.
func endSessionByNameVia(cmd TmuxCmd, shellPath, socketKey, sessionName string) (bool, error) {
	present, err := cmd.hasSession(sessionName)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}

	killErr := reapSessionVia(cmd, shellPath, socketKey, sessionName, 0)

	// Tear the server down once no healthy session remains: an empty or failed list-sessions means none does.
	// kill-server leaves the socket file behind, so it is removed once the server stops answering.
	if out, err := cmd.output(listSessionsArgv(runtime.GOOS, "#{session_name}")...); err != nil || strings.TrimSpace(out) == "" {
		logger.Info("reed: tearing down tmux server after ending session by name", "socket", socketKey, "session", sessionName)
		if err := cmd.run("kill-server"); err != nil {
			logger.Debug("reed: best-effort kill-server failed", "socket", socketKey, "err", err)
		}
		if err := removeSocketFileOnceGone(socketDirFromEnv(), socketKey, socketGoneWait); err != nil {
			logger.Debug("reed: could not remove the torn-down server's socket file", "socket", socketKey, "err", err)
		}
	}
	return true, killErr
}

// hasSession reports whether the named session exists (by exact match, not prefix).
func (p TmuxCmd) hasSession(name string) (bool, error) {
	err := p.run("has-session", "-t", exactSessionTarget(name))
	if err == nil {
		return true, nil
	}

	// The interface, rather than *exec.ExitError, lets a test script tmux's exit-1 answer through the exec seam.
	// The test then spawns no process.
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

// paneListFormat is the list-panes format that the pane parser reads.
// The last field joins the title, the current command and the start command with paneTailSeparator.
const paneListFormat = "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid} #{pane_title}\t#{pane_current_command}\t#{pane_start_command}"

// listPanes returns the panes of the window that windowTarget names.
func (p TmuxCmd) listPanes(windowTarget string) ([]LivePane, error) {
	out, err := p.output("list-panes", "-t", windowTarget, "-F", paneListFormat)
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}

// listSessionPanes returns the panes of every window of session.
func (p TmuxCmd) listSessionPanes(session string) ([]LivePane, error) {
	out, err := p.output("list-panes", "-s", "-t", exactSessionTarget(session), "-F", paneListFormat)
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}
