// child.go starts the detached process that places one window.

package kwinplace

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// ChildVerbName is the reed verb the placement child runs.
const ChildVerbName = "place-window"

// The flags of the placement child's verb.
const (
	ChildHubFlag      = "hub"
	ChildWorktreeFlag = "worktree"
	ChildTitleFlag    = "title"
	ChildPIDFlag      = "pid"
	ChildPrimeFlag    = "prime"
)

// Spawn starts the detached process that places one window.
// The window is that of process pid, titled title and opened for worktree of the hub at hubPath.
// prime reports that worktree is the hub's prime.
// It returns once the process has started, before the window need have appeared.
// A process started for a task from inside a run is registered under the worktree's name,
// so that the end of its run ends it.
// A failed registration only warns.
func Spawn(lymPath, hubPath, worktree, title string, pid int, prime bool) error {
	args := []string{
		"reed", ChildVerbName,
		"--" + ChildHubFlag, hubPath,
		"--" + ChildWorktreeFlag, worktree,
		"--" + ChildTitleFlag, title,
		"--" + ChildPIDFlag, strconv.Itoa(pid),
	}
	if prime {
		args = append(args, "--"+ChildPrimeFlag)
	}
	cmd := exec.Command(lymPath, args...)
	cmd.Dir = hubPath
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("kwinplace: start the placement child: %w", err)
	}
	childPID := cmd.Process.Pid
	logger.Info("kwinplace: spawned the placement child", "worktree", worktree, "window_pid", pid, "pid", childPID)
	// The record reads the child's start time, so it is built before the child can be reaped.
	record := runproc.ChildRecord(childPID, runproc.RolePlacement, cmd.Args)
	proc.Reap(cmd)

	if prime || os.Getenv(pauserecord.RunProcessEnv) == "" {
		return nil
	}
	if err := runproc.Register(fabriclayout.RunProcDir(hubPath), worktree, record); err != nil {
		logger.Warn("kwinplace: placement child not registered with its run", "worktree", worktree, "pid", childPID, "error", err)
	}
	return nil
}
