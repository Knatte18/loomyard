// spawn.go — detached board sync process launching.
//
// spawnSync launches `lyx board sync` as a detached, windowless process.
// It has its own process group (so the parent's Ctrl-C does not reach it) and survives the parent's
// exit.

package boardengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// SpawnSync starts the detached `lyx board sync` of the board at boardPath, the same sync every board write spawns.
// In a test binary it returns nil without spawning, since forking the running binary would re-run the suite.
func SpawnSync(boardPath string) error {
	if testing.Testing() {
		return nil
	}
	return spawnSync(boardPath)
}

// syncExecutable names the binary spawnSync launches; a test points it at a harmless one.
var syncExecutable = os.Executable

// spawnSync launches `lyx board sync` as a detached, windowless process and registers it under the hub owner.
// A registration failure is logged and never fails the spawn, since the sync is already running.
func spawnSync(boardPath string) error {
	exe, err := syncExecutable()
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
	logger.Info("boardengine: spawning detached board sync", "exe", exe, "boardPath", abs)
	if err := cmd.Start(); err != nil { // intentionally not Wait()ed
		logger.Warn("boardengine: board sync spawn failed", "exe", exe, "boardPath", abs, "cause", err)
		return err
	}
	pid := cmd.Process.Pid
	record := runproc.ChildRecord(pid, runproc.RoleSync, cmd.Args)
	if err := runproc.Register(fabricengine.RunProcDir(filepath.Dir(abs)), runproc.HubOwner, record); err != nil {
		logger.Warn("boardengine: board sync not registered", "pid", pid, "cause", err)
	}
	return nil
}
