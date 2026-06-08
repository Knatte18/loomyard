// spawn_windows.go — windowless process launching on Windows.
//
// Two needs: launch the background pusher detached so a write can return without
// waiting, and run every git subprocess without flashing a console window. Both
// use CREATE_NO_WINDOW — mhgo and git are console apps, and when launched from a
// process without a visible console each would otherwise pop up its own window.
package board

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// spawnSync launches `mhgo board sync` as a detached, windowless process. It has
// its own process group (so the parent's Ctrl-C does not reach it) and survives
// the parent's exit. CREATE_NO_WINDOW keeps it — and the git children it spawns —
// off-screen.
func spawnSync(wikiPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "board", "--wiki-path", wikiPath, "sync")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	return cmd.Start() // intentionally not Wait()ed
}

// hideProcWindow makes a child process (git) run without a console window.
func hideProcWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
