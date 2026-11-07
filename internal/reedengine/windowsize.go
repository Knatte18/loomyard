// windowsize.go pins the geometry options of a reed session: the three-line status bar, window-size latest,
// and the session shell options.
// It also clears the window-resized hook that an older binary may have pinned on the shell window.
// Every tmux call here is non-fatal.
// A failure is logged with logger.Warn and answered with a safe fallback, never returned as an error.

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

// shellOptionArgvs returns the two session-scoped pins that make tmux start a pane's shell without the login flag.
// They set `default-shell` and `default-command` to the same shellPath.
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

// primeGeometry reports whether this engine's geometry is the hub's prime session.
// A hub geometry has a non-empty DiscoverSignalPath, which a standalone geometry leaves empty.
// The prime session also has an empty NameSlug.
// A pair's session and a standalone session are never the prime.
func (e *Engine) primeGeometry() bool {
	return e.geom.DiscoverSignalPath != "" && e.geom.NameSlug == ""
}

// statusFormatPinArgvs returns the global pins of every status format and the bar style, in order.
// The status formats are one tmux array, so each is pinned with -g.
// A per-session index would drop the global lines for that session.
func statusFormatPinArgvs() [][]string {
	return [][]string{
		{"set-option", "-g", "status-format[0]", statusFormatButtons},
		{"set-option", "-g", "status-format[1]", statusFormatSessions},
		{"set-option", "-g", "status-format[2]", statusFormatActions},
		{"set-option", "-g", "status-style", statusBarStyle},
	}
}

// pinGeometryOptionsLocked pins the three-line status bar and this session's window to "window-size latest".
// It pins the session shell options when shellPath is non-empty.
// It also marks a prime session with "@lym_prime_session", which line 2 of the bar lists first.
// It re-renders the button row.
// A config edit or a pair marked a beam after boot reaches a live session at its next attach.
// It also pins the navigation bindings, so a server booted by an older lym gets them at the next attach.
// On every platform but Windows it pins the unseen-clear hook on the session.
// It also clears the window-resized hook that an older binary may have pinned on the shell window.
//
// Every step runs even when an earlier one fails: each failure is logged via logger.Warn and ignored.
// Windows skips the hook steps, because psmux lacks set-hook.
// The option pins still run there, and psmux may refuse some of them.
// target is the strands' window, as resolved by the window seam;
// the session-wide options ride the window's session.
// It runs at boot and in the attach pre-flight, so a session booted by an older lym loses its hook at the next attach.
// Assumes the op lock is already held.
func (e *Engine) pinGeometryOptionsLocked(target, shellPath string) {
	for _, argv := range statusFormatPinArgvs() {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to pin a status bar option", "socket", e.Socket(), "session", e.SessionName(), "option", argv[2], "err", err)
		}
	}

	pins := []struct {
		option string
		argv   []string
	}{
		{"status", []string{"set-option", "-t", target, "status", "3"}},
		{"status-position", []string{"set-option", "-t", target, "status-position", "bottom"}},
	}
	if e.primeGeometry() {
		pins = append(pins, struct {
			option string
			argv   []string
		}{primeSessionOption, []string{"set-option", "-t", target, primeSessionOption, "1"}})
	}
	for _, pin := range pins {
		if err := e.tmux.run(pin.argv...); err != nil {
			logger.Warn("reed: failed to pin a status bar option", "socket", e.Socket(), "session", e.SessionName(), "option", pin.option, "err", err)
		}
	}

	e.pinButtonRowLocked(target)

	if err := e.tmux.run("set-option", "-w", "-t", target, "window-size", "latest"); err != nil {
		logger.Warn("reed: failed to pin window-size latest", "socket", e.Socket(), "session", e.SessionName(), "option", "window-size", "err", err)
	}

	for _, argv := range shellOptionArgvs(target, shellPath) {
		if err := e.tmux.run(argv...); err != nil {
			logger.Warn("reed: failed to pin a session shell option", "socket", e.Socket(), "session", e.SessionName(), "option", argv[3], "err", err)
		}
	}

	e.pinBindingsLocked()

	// psmux lacks set-hook, so on Windows no hook is pinned and openOwnWindowLocked never sets @lym_unseen.
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
