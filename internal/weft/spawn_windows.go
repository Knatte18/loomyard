// spawn_windows.go — windowless detached process launching for weft push on Windows.
//
// spawnPush launches `lyx weft --weft-path <abs> push` as a detached, windowless process.
// It has its own process group (so parent Ctrl-C does not reach it) and survives
// parent exit. CREATE_NO_WINDOW keeps it — and its git children — off-screen.

package weft

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// spawnPush launches `lyx weft --weft-path <abs> push` as a detached, windowless process.
// Returns nil immediately if WEFT_SKIP_GIT or WEFT_SKIP_PUSH is set (no child process forked).
// Otherwise builds the command, sets it to run detached and windowless, and starts it
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
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	return cmd.Start() // intentionally not Wait()ed
}
