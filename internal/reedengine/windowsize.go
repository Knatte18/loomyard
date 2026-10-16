// windowsize.go owns the geometry option pins (the two-line status bar and window-size latest), the session shell options,
// and the unpin of the `window-resized` hook an older binary pinned on the shell window.
// Every tmux interaction here is non-fatal, per the Shared Decision geometry-tmux-failures-are-non-fatal-everywhere:
// a failure is logged via logger.Warn and answered with a safe fallback, never returned as an error.

package reedengine

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/Knatte18/loomyard/internal/logger"
)

// resolveShellPath is the absolute path of the configured shell, found through the executable search path.
// An unresolvable shell is an error naming the `shell` key and its value, so the operator can set a shell that exists.
func resolveShellPath(shell string) (string, error) {
	path, err := exec.LookPath(shell)
	if err != nil {
		return "", fmt.Errorf("reed: the configured shell %q (key shell) is not an executable on PATH: %w; set shell in reed.yaml to a shell that exists", shell, err)
	}
	return path, nil
}

// shellOptionArgvs is the two session-scoped pins that make tmux start a pane's shell without the login flag: `default-shell` and an equal non-empty `default-command`.
// They are never global, because one server serves every worktree of the hub.
// An empty shellPath yields no pins.
func shellOptionArgvs(target, shellPath string) [][]string {
	if shellPath == "" {
		return nil
	}
	return [][]string{
		{"set-option", "-t", target, "default-shell", shellPath},
		{"set-option", "-t", target, "default-command", shellPath},
	}
}

// pinGeometryOptionsLocked pins the two-line status bar (bar.go) and this session's window to "window-size latest".
// It also clears the `window-resized` hook an older binary pinned on the shell window.
//
// It issues, in order: "status-format[0]" and "status-format[1]" with -g, because tmux's status formats are one array and a per-session index would drop the global lines for that session;
// "status-style" with -g, like them, so the gaps between the buttons are dark grey instead of tmux's default green;
// and "status" "2" and "status-position" "bottom" on the session.
// After the option pins it issues the navigation bindings (bindings.go), so a server booted by an older lyx gets them at the next attach.
// Then, everywhere but Windows, it pins the session hook `session-window-changed` with `set-option -w -u @lyx_unseen`, which unsets the unseen mark on the window the client moved to and nothing else.
// Last, everywhere but Windows, it unpins `window-resized` twice: `set-hook -u -w -t <target> window-resized`, the window-scoped clear, and `set-hook -u -t <target> window-resized`, the session-scoped form, so an entry an older binary pinned in either scope goes.
// The unpin clears only that one hook on the shell window, which only reed ever pinned.
//
// Each call's error is logged via logger.Warn and then ignored.
// Every later step, including the hook block, is attempted even when an earlier one failed, per the Shared Decision geometry-tmux-failures-are-non-fatal-everywhere.
// No runtime.GOOS == "windows" branch guards any of the set-option calls: per the Shared
// Decision windows-status-line-is-an-unbranched-accepted-degrade they are attempted on every platform,
// and psmux may refuse some or all of them.
//
// It runs both at boot (lifecycle.go) and in the attach pre-flight (attach.go), so a session booted by an older lyx loses the hook at the next attach.
// target is the strands' window, as resolved by the window seam;
// the session-wide options ride the window's session.
// After the option pins it issues shellOptionArgvs for shellPath, each non-fatal like them; the empty string skips both.
// Assumes the op lock is already held.
func (e *Engine) pinGeometryOptionsLocked(target, shellPath string) {
	pins := []struct {
		option string
		argv   []string
	}{
		{"status-format[0]", []string{"set-option", "-g", "status-format[0]", statusFormatButtons}},
		{"status-format[1]", []string{"set-option", "-g", "status-format[1]", statusFormatSessions}},
		{"status-style", []string{"set-option", "-g", "status-style", statusBarStyle}},
		{"status", []string{"set-option", "-t", target, "status", "2"}},
		{"status-position", []string{"set-option", "-t", target, "status-position", "bottom"}},
	}
	for _, pin := range pins {
		if err := e.tmux.run(pin.argv...); err != nil {
			logger.Warn("reed: failed to pin a status bar option", "socket", e.Socket(), "session", e.SessionName(), "option", pin.option, "err", err)
		}
	}

	if err := e.tmux.run("set-option", "-w", "-t", target, "window-size", "latest"); err != nil {
		logger.Warn("reed: failed to pin window-size latest", "socket", e.Socket(), "session", e.SessionName(), "option", "window-size", "err", err)
	}

	for _, argv := range shellOptionArgvs(target, shellPath) {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to pin a session shell option", "socket", e.Socket(), "session", e.SessionName(), "option", argv[3], "err", err)
		}
	}

	e.pinBindingsLocked()

	// psmux lacks set-hook, so on Windows no hook is pinned and openOwnWindowLocked never sets @lyx_unseen.
	if runtime.GOOS == "windows" {
		return
	}
	if err := e.tmux.run("set-hook", "-t", exactSessionWindowTarget(e.SessionName()), unseenClearHookName, unseenClearHookCommand); err != nil {
		logger.Warn("reed: failed to pin the unseen-clear hook", "socket", e.Socket(), "session", e.SessionName(), "err", err)
	}

	// set-hook -u is idempotent and exits 0 whether or not a hook was set.
	for _, argv := range [][]string{
		{"set-hook", "-u", "-w", "-t", target, windowResizedHookName},
		{"set-hook", "-u", "-t", target, windowResizedHookName},
	} {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to unset window-resized hook", "socket", e.Socket(), "session", e.SessionName(), "argv", argv, "err", err)
		}
	}
}
