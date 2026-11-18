//go:build windows

// locate_windows.go reads the Windows desktop directory from the shell's known folder.
// That folder follows a redirected desktop.

package desktoplauncher

import "golang.org/x/sys/windows"

// windowsDesktopDir returns the Windows desktop directory of the current user.
func windowsDesktopDir() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
}
