// spawn.go — detached weft push process launching.
//
// spawnPush launches `lyx weft --weft-path <abs> push` as a detached, windowless process.
// It has its own process group (so the parent's Ctrl-C does not reach it)
// and survives the parent's exit.

package weft

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/proc"
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
	proc.Detach(cmd)
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	return cmd.Start() // intentionally not Wait()ed
}
