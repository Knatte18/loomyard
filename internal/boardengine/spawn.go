// spawn.go starts the board's background pusher as a detached, windowless process.
// It has its own process group, so the parent's Ctrl-C does not reach it, and it survives the parent's exit.

package boardengine

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// DeferrableSyncFlag is the hidden flag that marks a detached board sync as deferrable:
// under a pause, its network gate exits instead of waiting, and the gate is held under the hub scope.
const DeferrableSyncFlag = "deferrable"

// SpawnSync starts the board's background pusher for the board at boardPath, the one every board write starts.
// A pusher already running takes the change at its next pass, and the started one exits.
// In a test binary it returns nil without spawning, since forking the running binary would re-run the suite.
func SpawnSync(boardPath string) error {
	if testing.Testing() && !spawnUnderTest {
		return nil
	}
	return spawnSync(boardPath)
}

// syncExecutable names the binary spawnSync launches; a test points it at a harmless one.
var syncExecutable = selfexec.LymBinary

// spawnUnderTest lets a test that has pointed syncExecutable elsewhere drive SpawnSync.
var spawnUnderTest bool

// spawnSync launches the board pusher as a detached, windowless process and registers it under the hub owner.
// The process runs in the board directory.
// The board directory as working directory names the hub the process serves.
// A registration failure is logged and never fails the spawn, since the pusher is already running.
func spawnSync(boardPath string) error {
	exe, err := syncExecutable()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, fabriclayout.PusherCommand(fabriclayout.PusherFlags{Worktree: abs, Board: true})...)
	cmd.Dir = abs
	proc.Detach(cmd)
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	logger.Info("boardengine: spawning the board pusher", "exe", exe, "boardPath", abs)
	if err := cmd.Start(); err != nil {
		logger.Warn("boardengine: board pusher spawn failed", "exe", exe, "boardPath", abs, "cause", err)
		return err
	}
	proc.Reap(cmd)
	pid := cmd.Process.Pid
	record := runproc.ChildRecord(pid, runproc.RoleSync, cmd.Args)
	if err := runproc.Register(fabriclayout.RunProcDir(filepath.Dir(abs)), runproc.HubOwner, record); err != nil {
		logger.Warn("boardengine: board pusher not registered", "pid", pid, "cause", err)
	}
	return nil
}
