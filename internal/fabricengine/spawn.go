// spawn.go — the detached push spawn, which starts the background pusher of a PushSpawn request's worktree.
// SpawnStatusPush builds that request from a location alone.

package fabricengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// ErrSpawnUnderTest is returned by SpawnDetachedPush in a test binary, where forking the binary would re-run the suite.
var ErrSpawnUnderTest = errors.New("fabricengine: detached push not spawned under test")

// SpawnDetachedPush launches the detached, windowless pusher of the worktree req names.
// The pusher brings the worktree's commits to origin; a pusher already running takes them, and the new one exits.
// Returns nil immediately, forking no child, when FABRIC_SKIP_GIT or FABRIC_SKIP_PUSH is set.
// It also returns nil when both paths are empty, since there is nothing to push.
// Returns ErrSpawnUnderTest, forking no child, in a test binary.
// The child is reaped in the background through proc.Reap rather than waited for,
// and its stdin/stdout/stderr are left nil so no handle is inherited from the parent.
// The started child is registered in the hub's owner record.
func SpawnDetachedPush(req fabriclayout.PushSpawn) error {
	if os.Getenv("FABRIC_SKIP_GIT") == "1" || os.Getenv("FABRIC_SKIP_PUSH") == "1" {
		return nil
	}
	if req.WarpPath == "" && req.WeftPath == "" {
		return nil
	}
	if testing.Testing() {
		return ErrSpawnUnderTest
	}

	exe, err := selfexec.LymBinary()
	if err != nil {
		return err
	}

	return startDetachedPush(exe, req)
}

// startDetachedPush starts the pusher of req from exe and registers it in the owner record.
// A registration failure is logged and never fails the spawn, since the pusher is already running.
func startDetachedPush(exe string, req fabriclayout.PushSpawn) error {
	args, dir, owner := pushSpawnCommand(req)
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	proc.Detach(cmd)
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	if err := cmd.Start(); err != nil {
		logger.Warn("fabricengine: spawn detached push failed", "exe", exe, "args", args, "dir", dir, "error", err)
		return err
	}
	proc.Reap(cmd)
	pid := cmd.Process.Pid
	logger.Info("fabricengine: spawned detached push", "exe", exe, "args", args, "dir", dir, "pid", pid)
	if err := runproc.Register(fabriclayout.RunProcDir(filepath.Dir(dir)), owner, runproc.ChildRecord(pid, runproc.RolePush, args)); err != nil {
		logger.Warn("fabricengine: detached push not registered", "owner", owner, "pid", pid, "error", err)
	}
	return nil
}

// pushSpawnCommand returns the arguments, working directory and owner of the pusher req describes.
// A request with a code path is a pair's, whose slug is that worktree's name.
// One without is the prime's, whose code worktree is the one its records path is paired with, and names no slug.
// The owner is the hub, so no run's teardown, cancel or hand-over ends a pusher.
func pushSpawnCommand(req fabriclayout.PushSpawn) (args []string, dir, owner string) {
	flags := fabriclayout.PusherFlags{Worktree: req.WarpPath}
	if req.WarpPath != "" {
		flags.Slug = filepath.Base(req.WarpPath)
	} else {
		name, _ := fabriclayout.WeftWarpSlug(filepath.Base(req.WeftPath))
		flags.Worktree = filepath.Join(filepath.Dir(req.WeftPath), name)
	}
	flags.Worktree = absoluteOrAsGiven(flags.Worktree)
	return fabriclayout.PusherCommand(flags), req.WorkDir, runproc.HubOwner
}

// absoluteOrAsGiven returns path made absolute, or path itself when the working directory cannot be read.
func absoluteOrAsGiven(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// SpawnStatusPush starts the pusher of the status records at l and reports what it requested.
// It resolves every path from l alone, so no caller names a records path or pushes the prime's code branch.
// In the hub's prime the request names the records side only; in a task pair it names both sides.
// gateSlug is not read: the pusher judges its own pause.
// The child starts in the hub's board worktree, outside both of the pair's worktrees.
// A failed or never-run push leaves the commits local; the pusher's next start carries them.
// That push also creates the upstream of a never-pushed branch.
func SpawnStatusPush(l *lymcwd.Location, gateSlug string) (fabriclayout.SpawnResult, error) {
	req, err := statusPushRequest(l)
	if err != nil {
		return fabriclayout.SpawnResult{}, err
	}
	return fabriclayout.SpawnResult{Request: req, Spawned: true}, spawnDetachedPushFn(req)
}

// statusPushRequest builds the detached push request for l's status records.
func statusPushRequest(l *lymcwd.Location) (fabriclayout.PushSpawn, error) {
	isPrime, err := IsPrimeWorktree(l)
	if err != nil {
		return fabriclayout.PushSpawn{}, err
	}
	req := fabriclayout.PushSpawn{WeftPath: RecordsWorktree(l), WorkDir: fabriclayout.BoltDir(l.HubPath)}
	if !isPrime {
		req.WarpPath = l.WorktreePath()
	}
	return req, nil
}

// SpawnPairPush starts the pusher of the task pair at l: its code worktree and its paired records worktree.
// A failed or never-run push leaves the pair's commits local; the pusher's next start carries them.
// That push also creates the upstream of a never-pushed branch.
func SpawnPairPush(l *lymcwd.Location) error {
	_, err := SpawnStatusPush(l, "")
	return err
}
