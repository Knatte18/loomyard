// prelaunch.go decides from the opening process's environment whether a window is placed, and opens it.

package kwinplace

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/termwindow"
)

// The environment variables and values a placeable session has.
const (
	kdeVersionEnv      = "KDE_SESSION_VERSION"
	kdeVersion         = "6"
	currentDesktopEnv  = "XDG_CURRENT_DESKTOP"
	kdeDesktopName     = "KDE"
	sessionTypeEnv     = "XDG_SESSION_TYPE"
	waylandSessionType = "wayland"
)

// The programs that reach KWin, its journal and the screen setup.
const (
	qdbusProgram      = "qdbus6"
	journalctlProgram = "journalctl"
	kscreenProgram    = "kscreen-doctor"
)

// sessionProbe carries the facts of the session PreLaunch reads, so the checks are testable without it.
type sessionProbe struct {
	getenv   func(string) string
	lookPath func(string) (string, error)
}

// PreLaunch returns nil when the calling process's session can place a window.
// Otherwise it returns an error naming the first check that failed.
// konsole reports whether the window opens in Konsole.
func PreLaunch(konsole bool) error {
	return preLaunch(sessionProbe{getenv: os.Getenv, lookPath: exec.LookPath}, konsole)
}

func preLaunch(p sessionProbe, konsole bool) error {
	if got := p.getenv(kdeVersionEnv); got != kdeVersion {
		return fmt.Errorf("%s is %q, not %q", kdeVersionEnv, got, kdeVersion)
	}
	if got := p.getenv(currentDesktopEnv); !slices.Contains(strings.Split(got, ":"), kdeDesktopName) {
		return fmt.Errorf("%s is %q and does not name %s", currentDesktopEnv, got, kdeDesktopName)
	}
	if got := p.getenv(sessionTypeEnv); got != waylandSessionType {
		return fmt.Errorf("%s is %q, not %q", sessionTypeEnv, got, waylandSessionType)
	}
	if !konsole {
		return errors.New("the terminal launcher is not Konsole")
	}
	for _, program := range []string{qdbusProgram, journalctlProgram, kscreenProgram} {
		if _, err := p.lookPath(program); err != nil {
			return fmt.Errorf("%s is not on PATH: %w", program, err)
		}
	}
	return nil
}

// OpenWindow opens a terminal window on dir through launcher.
// It returns the window's process id and whether the window is to be placed.
// A window that is not placed opens as it would without placement, with pid 0, and the reason is logged once.
func OpenWindow(launcher termwindow.Launcher, dir, title, lymPath string) (int, bool, error) {
	skipped := PreLaunch(launcher.IsKonsole())
	if skipped != nil {
		logger.Info("kwinplace: window placement skipped", "dir", dir, "reason", skipped)
	}
	placed := skipped == nil
	pid, err := launcher.Open(dir, title, lymPath, placed)
	return pid, placed, err
}
