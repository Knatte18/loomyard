// spawn_other.go — detached process launching for weft push on non-Windows.
//
// spawnPush launches `lyx weft --weft-path <abs> push` in its own session (Setsid)
// so it survives parent exit. There are no console-window issues on non-Windows platforms.
// The Windows variants live in spawn_windows.go.

//go:build !windows

package weft

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// spawnPush launches `lyx weft --weft-path <abs> push` as a detached process.
// Returns nil immediately if WEFT_SKIP_GIT or WEFT_SKIP_PUSH is set (no child process forked).
// Otherwise builds the command, sets it to run in its own session, and starts it
// without waiting.
func spawnPush(weftPath string) error {
	if os.Getenv("WEFT_SKIP_GIT") == "1" || os.Getenv("WEFT_SKIP_PUSH") == "1" {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(weftPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "weft", "--weft-path", abs, "push")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	return cmd.Start() // intentionally not Wait()ed
}
