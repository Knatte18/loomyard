// spawn.go — detached board sync process launching.
//
// spawnSync launches `lyx board sync` as a detached, windowless process.
// It has its own process group (so the parent's Ctrl-C does not reach it)
// and survives the parent's exit.

package boardengine

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/proc"
)

// spawnSync launches `lyx board sync` as a detached, windowless process.
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
	proc.Detach(cmd)
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	return cmd.Start() // intentionally not Wait()ed
}
