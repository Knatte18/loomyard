// locate.go finds this machine's desktop directory and the launcher form it takes.

package desktoplauncher

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNoDesktop reports that this machine has no desktop directory to hold launchers.
var ErrNoDesktop = errors.New("desktoplauncher: no desktop directory on this machine")

const (
	// desktopEnv names the desktop directory on every platform when it is set.
	desktopEnv = "XDG_DESKTOP_DIR"
	// userDirsFileName is the freedesktop file in the user config directory that names the desktop directory.
	userDirsFileName = "user-dirs.dirs"
	// wslDistroEnv names the WSL distribution this process runs in; WSL sets it.
	wslDistroEnv = "WSL_DISTRO_NAME"
	// wslInteropPath exists on every WSL machine whose Windows interop is enabled.
	wslInteropPath = "/proc/sys/fs/binfmt_misc/WSLInterop"
)

// probe carries the machine facts locate reads, so the choice is testable without the machine.
type probe struct {
	goos       string
	getenv     func(string) string
	fileExists func(string) bool
	isDir      func(string) bool
	// freedesktopDir returns the Linux desktop directory.
	freedesktopDir func() (string, error)
	// windowsDir returns the Windows desktop directory, as a path this process opens.
	windowsDir func() (string, error)
}

// Locate returns this machine's desktop, or an error wrapping ErrNoDesktop that names why there is none.
func Locate() (Desktop, error) {
	return locate(probe{
		goos:       runtime.GOOS,
		getenv:     os.Getenv,
		fileExists: func(path string) bool { _, err := os.Stat(path); return err == nil },
		isDir: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
		freedesktopDir: func() (string, error) { return freedesktopDesktopDir(os.UserConfigDir, os.UserHomeDir) },
		windowsDir:     windowsDesktopDir,
	})
}

func locate(p probe) (Desktop, error) {
	desktop := Desktop{Form: FormFreedesktop}
	resolveDir := p.freedesktopDir
	switch {
	case p.goos == "windows":
		desktop.Form, resolveDir = FormWindows, p.windowsDir
	case p.goos != "linux":
		return Desktop{}, fmt.Errorf("%w: no launcher form for %s", ErrNoDesktop, p.goos)
	case p.getenv(wslDistroEnv) != "" || p.fileExists(wslInteropPath):
		desktop.Form, desktop.Distro, resolveDir = FormWindowsWSL, p.getenv(wslDistroEnv), p.windowsDir
	}
	dir := p.getenv(desktopEnv)
	if dir == "" {
		resolved, err := resolveDir()
		if err != nil {
			return Desktop{}, fmt.Errorf("%w: %v", ErrNoDesktop, err)
		}
		dir = resolved
	}
	if !p.isDir(dir) {
		return Desktop{}, fmt.Errorf("%w: %s is not a directory", ErrNoDesktop, dir)
	}
	desktop.Dir = dir
	return desktop, nil
}

// freedesktopDesktopDir returns the desktop directory that user-dirs.dirs in the directory userConfigDir returns names.
// Without one it returns `$HOME/Desktop`, with the home directory userHomeDir returns.
func freedesktopDesktopDir(userConfigDir, userHomeDir func() (string, error)) (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	if config, err := userConfigDir(); err == nil {
		data, err := os.ReadFile(filepath.Join(config, userDirsFileName))
		switch {
		case err == nil:
			if dir, found := parseUserDirs(data, home); found {
				return dir, nil
			}
		case !errors.Is(err, fs.ErrNotExist):
			return "", err
		}
	}
	return filepath.Join(home, "Desktop"), nil
}

// parseUserDirs reads the desktop directory from the text of a user-dirs.dirs file, with "$HOME" read as home.
// It reports false when the file names none or names one that is neither absolute nor under "$HOME".
func parseUserDirs(data []byte, home string) (string, bool) {
	for line := range strings.Lines(string(data)) {
		value, found := strings.CutPrefix(strings.TrimSpace(line), desktopEnv+"=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"`)
		if rest, underHome := strings.CutPrefix(value, "$HOME"); underHome {
			value = home + rest
		}
		if !filepath.IsAbs(value) {
			return "", false
		}
		return filepath.Clean(value), true
	}
	return "", false
}
