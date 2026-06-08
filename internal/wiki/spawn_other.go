// spawn_other.go — process launching on non-Windows.
//
// spawnSync starts `mhgo wiki sync` in its own session (Setsid) so it survives
// the parent's exit, with no inherited stdio. There are no console-window issues
// off Windows, so hideProcWindow is a no-op. The Windows variants live in
// spawn_windows.go.

//go:build !windows

package wiki

import (
	"os"
	"os/exec"
	"syscall"
)

func spawnSync(wikiPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "wiki", "--wiki-path", wikiPath, "sync")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	return cmd.Start() // intentionally not Wait()ed
}

// hideProcWindow is a no-op off Windows.
func hideProcWindow(cmd *exec.Cmd) {}
