// window.go implements the reed op that opens a detached, named tmux window in this worktree's reed session running a `lyx` command.
// The op only chooses where a command runs: the window is not a strand, holds no reed state, and is never read as the strands' window.

package reedengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

// ErrNewWindowFailed is the sentinel OpenWindow wraps when the multiplexer refuses the window.
var ErrNewWindowFailed = errors.New("reed: could not open the window")

// ErrWindowReadBack is the sentinel OpenWindow wraps when the new window does not read remain-on-exit off.
// That happens when two concurrent calls share a name and the chained option landed on the older window; the new window is killed.
var ErrWindowReadBack = errors.New("reed: the opened window did not read remain-on-exit off")

// WindowResult reports the window OpenWindow answers for.
type WindowResult struct {
	// WindowID is the multiplexer's window id ("@<n>"), what every later op addresses the window by.
	WindowID string `json:"window_id"`
	// Name is the window's display name.
	Name string `json:"name"`
	// Existing reports that a window of that name with a live pane was already open and nothing was started.
	Existing bool `json:"existing"`
}

// windowListFormat is the session-wide listing format OpenWindow finds a named window with: window id, pane id, pane dead flag, then the window name.
const windowListFormat = "#{window_id} #{pane_id} #{pane_dead} #{window_name}"

// OpenWindow opens a detached window named name in this worktree's reed session and runs `lyx` with lyxArgs in it.
// The window is never selected, so it takes no focus, and it closes when its command exits.
//
// A window of that exact name whose pane is live is returned as existing and nothing is started;
// one whose pane is dead is killed and replaced.
// It returns an error wrapping ErrNoSession when the worktree has no reed session, and creates nothing;
// a *CapabilityError when the multiplexer lacks new-window;
// ErrNewWindowFailed wrapping the multiplexer's error when it refuses the window;
// and ErrWindowReadBack when the new window does not read remain-on-exit off, after killing it.
func (e *Engine) OpenWindow(name string, lyxArgs []string) (WindowResult, error) {
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

		command := composeWindowCommand(shell.ForGOOS(), lyxArgs)
		sessionWindows := exactSessionWindowTarget(session)
		namedTarget := sessionWindows + "=" + name
		out, err := e.tmux.output("new-window", "-d", "-P", "-F", "#{window_id}", "-t", sessionWindows, "-n", name, command,
			";", "set-option", "-w", "-t", namedTarget, "remain-on-exit", "off")
		if err != nil {
			return fmt.Errorf("%w: %w", ErrNewWindowFailed, err)
		}
		windowID := strings.TrimSpace(out)
		logger.Info("reed: opened window", "socket", e.Socket(), "session", session, "window", windowID, "name", name, "command", command)

		// A command that exited at once has already closed its window, so a failed read is not a read-back mismatch.
		value, err := e.tmux.output("display-message", "-p", "-t", windowID, "#{remain-on-exit}")
		if err != nil {
			logger.Debug("reed: opened window is gone before its read-back", "socket", e.Socket(), "session", session, "window", windowID, "err", err)
		} else if strings.TrimSpace(value) != "off" {
			if killErr := e.tmux.run("kill-pane", "-t", windowID); killErr != nil {
				logger.Warn("reed: could not kill the window that failed its read-back", "socket", e.Socket(), "session", session, "window", windowID, "err", killErr)
			}
			return fmt.Errorf("%w: window %s read %q", ErrWindowReadBack, windowID, strings.TrimSpace(value))
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

// findWindowLocked returns the window of this session named exactly name, or nil when there is none.
// It reads the session-wide pane listing and compares names itself, since a window-name target that matches nothing falls back to the current window.
func (e *Engine) findWindowLocked(name string) (*namedWindow, error) {
	out, err := e.tmux.output("list-panes", "-s", "-t", exactSessionTarget(e.SessionName()), "-F", windowListFormat)
	if err != nil {
		return nil, fmt.Errorf("list windows: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 4)
		if len(fields) != 4 || fields[3] != name {
			continue
		}
		return &namedWindow{windowID: fields[0], paneID: fields[1], paneDead: fields[2] == "1"}, nil
	}
	return nil, nil
}
