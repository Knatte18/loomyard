// window.go implements the reed op that opens a detached, named tmux window running a `lym` command.
// The window lives in this worktree's reed session.
// The op only chooses where a command runs.
// The window is not a strand, holds no reed state, and is never read as the strands' window.

package reedengine

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

// ErrNewWindowFailed marks an error from the multiplexer refusing to open a window.
var ErrNewWindowFailed = errors.New("reed: could not open the window")

// ErrWindowReadBack marks a new window whose remain-on-exit option does not read back as off.
// The option is set through a chained command on a name no other window holds.
// A read-back mismatch therefore means the multiplexer did not apply it; the new window is killed.
var ErrWindowReadBack = errors.New("reed: the opened window did not read remain-on-exit off")

// WindowResult describes the window that a window-open call answers with.
type WindowResult struct {
	// WindowID is the multiplexer's window id ("@<n>"), what every later op addresses the window by.
	WindowID string `json:"window_id"`
	// Name is the window's display name.
	Name string `json:"name"`
	// Existing reports that a window of that name with a live pane was already open and nothing was started.
	Existing bool `json:"existing"`
}

// temporaryWindowNamePrefix starts the name a new window holds between its creation and its rename to the requested name.
const temporaryWindowNamePrefix = "lym-opening-"

// temporaryWindowCounter numbers the temporary window names so that each one is distinct within the process.
var temporaryWindowCounter atomic.Uint64

// windowListFormat is the session-wide listing format that finds a named window:
// window id, pane id, pane dead flag, the hidden-window flag, then the window name.
const windowListFormat = "#{window_id} #{pane_id} #{pane_dead} #{?#{@lym_hidden},1,0} #{window_name}"

// windowReadBackFormat reads back a window's remain-on-exit option: the answering window's id, then the value.
// The id is needed because tmux answers a target window that has already closed from another scope.
// Such an answer exits 0 with an empty id and the global value.
const windowReadBackFormat = "#{window_id} #{remain-on-exit}"

// OpenWindow opens a detached window named name in this worktree's reed session and runs `lym` with lymArgs in it.
// The window is never selected, so it takes no focus.
// It carries @lym_hidden, so the status bar gives it no button, and it closes when its command exits.
// It opens after the highest-index window, so every hidden window sits behind the shell window.
// When the session predates the window order, the shell window is first renamed and moved into place.
//
// A hidden window of that exact name whose pane is live is returned as existing and nothing is started;
// one whose pane is dead is killed and replaced.
// A window of that name without the hidden marker, such as one the operator opened by hand, is left as it is.
// It returns an error wrapping ErrNoSession when the worktree has no reed session, and creates nothing;
// a *CapabilityError when the multiplexer lacks new-window;
// ErrNewWindowFailed wrapping the multiplexer's error when it refuses the window;
// and ErrWindowReadBack when the new window does not read remain-on-exit off, after killing it.
func (e *Engine) OpenWindow(name string, lymArgs []string) (WindowResult, error) {
	return e.OpenWindowIn(name, "", lymArgs)
}

// OpenWindowIn opens a detached window named name in this worktree's reed session and runs `lym` with lymArgs in it.
// A non-empty dir is the directory the window's command starts in; an empty one leaves the multiplexer's default.
// The window is hidden, never selected, and closes when its command exits.
func (e *Engine) OpenWindowIn(name, dir string, lymArgs []string) (WindowResult, error) {
	var result WindowResult
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		if err := e.probeCapabilityLocked(); err != nil {
			return err
		}
		session := e.SessionName()

		existing, err := e.findWindowLocked(name)
		if err != nil {
			return err
		}
		if existing != nil && !existing.paneDead {
			result = WindowResult{WindowID: existing.windowID, Name: name, Existing: true}
			return nil
		}
		if existing != nil {
			if err := e.tmux.run("kill-pane", "-t", existing.paneID); err != nil {
				return fmt.Errorf("kill the dead %q window: %w", name, err)
			}
		}

		st, err := LoadState(e.stateDir())
		if err != nil {
			logger.Debug("reed: could not load the state to check the shell window", "socket", e.Socket(), "session", session, "err", err)
			st = nil
		}
		e.ensureShellWindowLocked(st)
		windows, err := e.tmux.listSessionWindows(session)
		if err != nil {
			logger.Debug("reed: could not list the session's windows to place the new window", "socket", e.Socket(), "session", session, "err", err)
		}

		command := composeWindowCommand(shell.ForGOOS(), e.executablePath, lymArgs)
		// No other window holds the temporary name, so the chained options cannot land on a same-named window.
		temporaryName := fmt.Sprintf("%s%d-%d", temporaryWindowNamePrefix, os.Getpid(), temporaryWindowCounter.Add(1))
		temporaryTarget := exactSessionWindowTarget(session) + "=" + temporaryName
		newWindowArgs := []string{"-P", "-F", "#{window_id}", "-n", temporaryName}
		if dir != "" {
			newWindowArgs = append(newWindowArgs, "-c", dir)
		}
		newWindowArgs = append(newWindowArgs, command,
			";", "set-option", "-w", "-t", temporaryTarget, "remain-on-exit", "off",
			";", "set-option", "-w", "-t", temporaryTarget, "@lym_hidden", "1",
			";", "rename-window", "-t", temporaryTarget, name)
		out, err := e.newWindowLocked(hiddenWindowInsertArgs(windows), newWindowArgs)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrNewWindowFailed, err)
		}
		windowID := strings.TrimSpace(out)
		logger.Info("reed: opened window", "socket", e.Socket(), "session", session, "window", windowID, "name", name, "command", command)

		// A command that exited at once has already closed its window,
		// so a failed read or a reply from another window is not a read-back mismatch.
		reply, err := e.tmux.output("display-message", "-p", "-t", windowID, windowReadBackFormat)
		answeringWindowID, value, _ := strings.Cut(strings.TrimRight(reply, "\r\n"), " ")
		switch {
		case err != nil:
			logger.Debug("reed: opened window is gone before its read-back", "socket", e.Socket(), "session", session, "window", windowID, "err", err)
		case answeringWindowID != windowID:
			logger.Debug("reed: opened window is gone before its read-back", "socket", e.Socket(), "session", session, "window", windowID, "answered_by", answeringWindowID)
		case value != "off":
			if killErr := e.tmux.run("kill-pane", "-t", windowID); killErr != nil {
				logger.Warn("reed: could not kill the window that failed its read-back", "socket", e.Socket(), "session", session, "window", windowID, "err", killErr)
			}
			return fmt.Errorf("%w: window %s read %q", ErrWindowReadBack, windowID, value)
		}
		result = WindowResult{WindowID: windowID, Name: name}
		return nil
	})
	if err != nil {
		return WindowResult{}, err
	}
	return result, nil
}

// namedWindow is a window found by its exact name.
type namedWindow struct {
	windowID string
	paneID   string
	paneDead bool
}

// findWindowLocked returns the hidden window of this session named exactly name, or nil when there is none.
// It reads the session-wide pane listing and compares names itself, since a window-name target that matches nothing falls back to the current window.
// Only a window carrying @lym_hidden, the marker every window this op opens carries, is considered.
// An own window or a window the operator opened by hand is therefore never adopted, killed or replaced.
func (e *Engine) findWindowLocked(name string) (*namedWindow, error) {
	out, err := e.tmux.output("list-panes", "-s", "-t", exactSessionTarget(e.SessionName()), "-F", windowListFormat)
	if err != nil {
		return nil, fmt.Errorf("list windows: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 5)
		if len(fields) != 5 || fields[3] != "1" || fields[4] != name {
			continue
		}
		return &namedWindow{windowID: fields[0], paneID: fields[1], paneDead: fields[2] == "1"}, nil
	}
	return nil, nil
}
