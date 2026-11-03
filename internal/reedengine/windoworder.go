// windoworder.go fixes the order of a session's windows: the overview windows in ascending order key, agent windows in start order, then the window named shell, then the hidden windows.
// It lists the session's windows, computes where a new window goes and repairs a session booted by an older lym.

package reedengine

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// shellWindowName is the name of the window that holds the operator console pane.
const shellWindowName = "shell"

// overviewWindowName is the name of the window that holds an overview strand whose display carries no title.
const overviewWindowName = "overview"

// overviewWindowTitle returns the name of the window an overview strand with display d opens: its Title, or overviewWindowName when it has none.
func overviewWindowTitle(d render.Display) string {
	if d.Title == "" {
		return overviewWindowName
	}
	return d.Title
}

// shellCheckFailedOption is the session option that records an open episode of failures to repair the shell window.
const shellCheckFailedOption = "@lym_shell_check_failed"

// sessionWindow is one window of the session-wide window listing.
type sessionWindow struct {
	ID    string
	Index int
	// Hidden reports that the window carries @lym_hidden.
	Hidden bool
	// ShellCheckFailed reports that the session's shell-check episode option is set, which every window of the session reads alike.
	ShellCheckFailed bool
	Name             string
}

// sessionWindowFormat is the session-wide window listing format of sessionWindow: window id, index, hidden flag, shell-check episode flag, then the window name.
const sessionWindowFormat = "#{window_id} #{window_index} #{?#{@lym_hidden},1,0} #{?#{@lym_shell_check_failed},1,0} #{window_name}"

// parseSessionWindows parses a sessionWindowFormat listing into windows in listing order, skipping a line it cannot parse.
func parseSessionWindows(out string) []sessionWindow {
	var windows []sessionWindow
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 5)
		if len(fields) != 5 {
			continue
		}
		index, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		windows = append(windows, sessionWindow{
			ID:               fields[0],
			Index:            index,
			Hidden:           fields[2] == "1",
			ShellCheckFailed: fields[3] == "1",
			Name:             fields[4],
		})
	}
	return windows
}

// listSessionWindows returns every window of session in index order.
// The exact-match target keeps sibling sessions out.
func (p TmuxCmd) listSessionWindows(session string) ([]sessionWindow, error) {
	out, err := p.output("list-windows", "-t", exactSessionTarget(session), "-F", sessionWindowFormat)
	if err != nil {
		return nil, err
	}
	return parseSessionWindows(out), nil
}

// listSessionPaneWindows maps each pane id of session to the id of its window.
func (p TmuxCmd) listSessionPaneWindows(session string) (map[string]string, error) {
	out, err := p.output("list-panes", "-s", "-t", exactSessionTarget(session), "-F", sessionPaneWindowFormat)
	if err != nil {
		return nil, err
	}
	return parseSessionPaneWindows(out), nil
}

// findSessionWindow returns the window of windows with the given id.
func findSessionWindow(windows []sessionWindow, id string) (sessionWindow, bool) {
	for _, w := range windows {
		if w.ID == id {
			return w, true
		}
	}
	return sessionWindow{}, false
}

// highestWindow returns the window with the highest index, among the non-hidden ones when visibleOnly is set.
func highestWindow(windows []sessionWindow, visibleOnly bool) (sessionWindow, bool) {
	var highest sessionWindow
	found := false
	for _, w := range windows {
		if visibleOnly && w.Hidden {
			continue
		}
		if !found || w.Index > highest.Index {
			highest, found = w, true
		}
	}
	return highest, found
}

// ownWindowInsertArgs returns the placement arguments for a new own window.
// A relaunch whose previous window is listed goes after it, so it keeps its place;
// otherwise a listed shell window gets the new window just before it;
// otherwise it goes after the highest-index non-hidden window.
// With no window to place against it returns nothing, which opens the plain form.
func ownWindowInsertArgs(windows []sessionWindow, shellWindow, previousWindow string) []string {
	if _, ok := findSessionWindow(windows, previousWindow); ok {
		return []string{"-a", "-t", previousWindow}
	}
	if _, ok := findSessionWindow(windows, shellWindow); ok {
		return []string{"-b", "-t", shellWindow}
	}
	if last, ok := highestWindow(windows, true); ok {
		return []string{"-a", "-t", last.ID}
	}
	return nil
}

// overviewWindowKeys maps the window id of each overview strand's bound pane to that strand's Display.Order, reading windowByPane, a session-wide pane-to-window listing.
// A non-overview strand, a strand with no pane and a pane the listing lacks are left out.
func overviewWindowKeys(strands []Strand, windowByPane map[string]string) map[string]int {
	keys := make(map[string]int)
	for _, s := range strands {
		if !isOverview(s.Display) || s.PaneID == "" {
			continue
		}
		if window, ok := windowByPane[s.PaneID]; ok {
			keys[window] = s.Display.Order
		}
	}
	return keys
}

// overviewWindowInsertArgs returns the placement arguments for an overview window whose order key is order.
// keys maps the overview windows to their order keys.
// The window goes after the highest-index listed overview window whose key is not higher than order, so equal keys sit in the order they last opened;
// with none it goes before the lowest-index window, hidden ones included, so it holds the lowest index.
// With no window listed it returns nothing, which opens the plain form.
func overviewWindowInsertArgs(windows []sessionWindow, keys map[string]int, order int) []string {
	var lowest, after sessionWindow
	foundLowest, foundAfter := false, false
	for _, w := range windows {
		if !foundLowest || w.Index < lowest.Index {
			lowest, foundLowest = w, true
		}
		if key, ok := keys[w.ID]; ok && key <= order && (!foundAfter || w.Index > after.Index) {
			after, foundAfter = w, true
		}
	}
	switch {
	case foundAfter:
		return []string{"-a", "-t", after.ID}
	case foundLowest:
		return []string{"-b", "-t", lowest.ID}
	default:
		return nil
	}
}

// hiddenWindowInsertArgs returns the placement arguments for a new hidden window: after the highest-index window, hidden ones included.
// With no window listed it returns nothing, which opens the plain form.
func hiddenWindowInsertArgs(windows []sessionWindow) []string {
	if last, ok := highestWindow(windows, false); ok {
		return []string{"-a", "-t", last.ID}
	}
	return nil
}

// planShellWindowFix decides what repairing the window shellWindow takes.
// rename is set when its name is not shell; after is the id of the highest-index non-hidden window when one sits behind it, else empty.
// A shellWindow that windows does not list plans nothing.
func planShellWindowFix(windows []sessionWindow, shellWindow string) (rename bool, after string) {
	shell, ok := findSessionWindow(windows, shellWindow)
	if !ok {
		return false, ""
	}
	if last, ok := highestWindow(windows, true); ok && last.Index > shell.Index {
		after = last.ID
	}
	return shell.Name != shellWindowName, after
}

// ensureShellWindowLocked brings the window holding the recorded console pane into its place: named shell, behind every non-hidden window.
// It repairs a session that an older lym booted, and runs wherever a window opens and on attach.
// A check that cannot read what it needs does nothing and logs at Debug, and a failed repair never fails the caller; the next open or attach retries.
// A failed repair logs at Warn once per failure episode, which the session option shellCheckFailedOption keeps open, and at Debug while it stays open or on Windows.
// A repair that succeeds with the episode open logs at Info and closes it.
// Only the window of the recorded pane is renamed and moved; strands already placed stay where they are.
// Assumes the op lock is already held.
func (e *Engine) ensureShellWindowLocked(st *ReedState) {
	session := e.SessionName()
	windowByPane, err := e.tmux.listSessionPaneWindows(session)
	if err != nil {
		logger.Debug("reed: could not list the session's panes to find the shell window", "socket", e.Socket(), "session", session, "err", err)
		return
	}
	shellWindow, ok := shellWindowOf(st, windowByPane)
	if !ok {
		logger.Debug("reed: no shell window is recorded for this session, leaving the window order as it is", "socket", e.Socket(), "session", session)
		return
	}
	windows, err := e.tmux.listSessionWindows(session)
	if err != nil {
		logger.Debug("reed: could not list the session's windows to check the shell window", "socket", e.Socket(), "session", session, "err", err)
		return
	}
	if _, listed := findSessionWindow(windows, shellWindow); !listed {
		logger.Debug("reed: the shell window is not in the window listing, leaving the window order as it is", "socket", e.Socket(), "session", session, "window", shellWindow)
		return
	}
	episodeOpen := windows[0].ShellCheckFailed

	rename, after := planShellWindowFix(windows, shellWindow)
	var failure error
	if rename {
		if err := e.tmux.run("rename-window", "-t", shellWindow, shellWindowName); err != nil {
			failure = fmt.Errorf("rename the shell window: %w", err)
		} else if err := e.tmux.run("set-option", "-w", "-t", shellWindow, "automatic-rename", "off"); err != nil {
			failure = fmt.Errorf("pin the shell window's name: %w", err)
		}
	}
	if after != "" {
		if err := e.tmux.run("move-window", "-a", "-s", shellWindow, "-t", after); err != nil && failure == nil {
			failure = fmt.Errorf("move the shell window behind the last window: %w", err)
		}
	}

	switch {
	case failure != nil && (episodeOpen || runtime.GOOS == "windows"):
		logger.Debug("reed: could not repair the shell window", "socket", e.Socket(), "session", session, "window", shellWindow, "err", failure)
	case failure != nil:
		logger.Warn("reed: could not repair the shell window, retrying at the next window open or attach", "socket", e.Socket(), "session", session, "window", shellWindow, "err", failure)
		if err := e.tmux.run("set-option", "-t", exactSessionTarget(session), shellCheckFailedOption, "1"); err != nil {
			logger.Debug("reed: could not record the shell window failure", "socket", e.Socket(), "session", session, "err", err)
		}
	case episodeOpen:
		logger.Info("reed: the shell window is in place again", "socket", e.Socket(), "session", session, "window", shellWindow)
		if err := e.tmux.run("set-option", "-u", "-t", exactSessionTarget(session), shellCheckFailedOption); err != nil {
			logger.Debug("reed: could not clear the shell window failure", "socket", e.Socket(), "session", session, "err", err)
		}
	}
}
