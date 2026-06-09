// spawn_other.go — process launching on non-Windows.
//
// spawnSync starts `mhgo board sync` in its own session (Setsid) so it survives
// the parent's exit, with no inherited stdio. There are no console-window issues
// on non-Windows platforms. The Windows variants live in spawn_windows.go.

//go:build !windows

package board

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func spawnSync(boardPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "board", "--board-path", abs, "sync")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	return cmd.Start() // intentionally not Wait()ed
}
