// start.go starts a worktree's pusher as a detached process when the start conditions hold.

package runpusher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// restartEpisodes keeps one failure episode per key, so a start that fails on every tick warns once.
var restartEpisodes logger.EpisodeSet

// spawnRequest is one detached pusher process to start.
type spawnRequest struct {
	// Args is the command line after the lym binary.
	Args []string
	// Dir is the directory the process starts in.
	Dir string
	// RunProcDir is the directory of the owner records the process is registered in.
	RunProcDir string
	// Owner is the owner the process is registered under.
	Owner string
	// Role is the role the process is registered as.
	Role string
}

// starter carries the reads and the spawn a start decides over, so a test drives it with fakes.
type starter struct {
	// skipped reports whether git or pushes are off for this process.
	skipped func() bool
	// lockFree reports whether nobody holds the pusher lock at path.
	lockFree func(path string) (bool, error)
	// refusedPushStands reports whether a refused-push record stands for the target's branches.
	refusedPushStands func(t Target) (bool, error)
	// pauseCovers reports whether a pause covers the target's run.
	pauseCovers func(t Target) (bool, error)
	// handedOver reports whether a hand-over mark names the target's run.
	handedOver func(t Target) (bool, error)
	// ahead reports whether the target holds commits origin lacks.
	ahead func(t Target) (bool, error)
	// spawn starts the pusher process.
	spawn func(req spawnRequest) error
}

// Start starts the pusher of t when its start conditions hold, and reports whether it spawned one.
// The conditions are a free pusher lock and no refused-push record.
// A pair also needs no pause covering its run and no hand-over mark naming it.
// A process with FABRIC_SKIP_GIT or FABRIC_SKIP_PUSH set starts none.
// Two racing starts may both spawn: the loser exits at the lock.
// A failed read returns its error and starts none.
func Start(t Target) (bool, error) {
	return newStarter().start(t, false)
}

// StartIfAhead is Start for a worktree that also holds commits origin lacks.
func StartIfAhead(t Target) (bool, error) {
	return newStarter().start(t, true)
}

// RestartPusher runs start once, as the dobby's tick and a resume do.
// A failed start logs one warning per episode under key, and a later successful start logs the recovery.
// It never fails or drops anything of its caller's.
func RestartPusher(start func() (bool, error), key string) {
	if _, err := start(); err != nil {
		restartEpisodes.Fail(key, "runpusher: the pusher did not start", "worktree", key, "error", err)
		return
	}
	restartEpisodes.Recover(key, "runpusher: the pusher starts again", "worktree", key)
}

// newStarter fills a starter from the hub.
func newStarter() starter {
	return starter{
		skipped: func() bool {
			options := fabricengine.EnvSyncOptions()
			return options.SkipGit || options.SkipPush
		},
		lockFree: func(path string) (bool, error) {
			release, acquired, err := tryLock(path)
			if acquired {
				release()
			}
			return acquired, err
		},
		refusedPushStands: func(t Target) (bool, error) { return fabricengine.RefusedPushStands(t.Location) },
		pauseCovers:       pauseCovers,
		handedOver:        handedOver,
		ahead:             func(t Target) (bool, error) { return fabricengine.RunCommitsAhead(t.Location) },
		spawn:             spawnPusher,
	}
}

// start spawns the pusher of t when the conditions hold, and reports whether it did.
// With ifAhead it also requires commits origin lacks.
func (s starter) start(t Target, ifAhead bool) (bool, error) {
	if s.skipped() {
		return false, nil
	}
	if ifAhead {
		ahead, err := s.ahead(t)
		if err != nil || !ahead {
			return false, err
		}
	}
	free, err := s.lockFree(LockPath(t))
	if err != nil || !free {
		return false, err
	}
	stands, err := s.refusedPushStands(t)
	if err != nil || stands {
		return false, err
	}
	if t.Pair {
		covered, err := s.pauseCovers(t)
		if err != nil || covered {
			return false, err
		}
		marked, err := s.handedOver(t)
		if err != nil || marked {
			return false, err
		}
	}
	if err := s.spawn(pusherRequest(t)); err != nil {
		return false, err
	}
	return true, nil
}

// pusherRequest returns the detached process that serves t.
// It starts in the hub's Bolt directory, outside the pair, so a removal of the pair leaves it standing.
// It is registered under the hub's owner, so no run's teardown, cancel or hand-over reaches it.
func pusherRequest(t Target) spawnRequest {
	args := []string{"loom", "pusher", "--" + WorktreeFlag, t.Location.WorktreePath()}
	if t.Slug != "" {
		args = append(args, "--"+SlugFlag, t.Slug)
	}
	return spawnRequest{
		Args:       args,
		Dir:        fabriclayout.BoltDir(t.Location.HubPath),
		RunProcDir: fabriclayout.RunProcDir(t.Location.HubPath),
		Owner:      runproc.HubOwner,
		Role:       runproc.RolePush,
	}
}

// spawnPusher starts the detached process req describes and registers it.
// It returns an error under go test, where the lym binary is the test binary, and starts nothing.
// A failed registration is logged and never fails the spawn, since the pusher is already running.
func spawnPusher(req spawnRequest) error {
	exe, err := selfexec.LymBinary()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, req.Args...)
	cmd.Dir = req.Dir
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("runpusher: spawn the pusher failed", "exe", exe, "args", req.Args, "dir", req.Dir, "error", err)
		return fmt.Errorf("runpusher: spawn the pusher: %w", err)
	}
	proc.Reap(cmd)
	pid := cmd.Process.Pid
	logger.Info("runpusher: spawned the pusher", "exe", exe, "args", req.Args, "dir", req.Dir, "pid", pid)
	if err := runproc.Register(req.RunProcDir, req.Owner, runproc.ChildRecord(pid, req.Role, req.Args)); err != nil {
		logger.Warn("runpusher: the pusher is not registered", "owner", req.Owner, "pid", pid, "error", err)
	}
	return nil
}

// tryLock takes the pusher lock at path without waiting.
// It reports acquired false when another process holds it.
func tryLock(path string) (release func(), acquired bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, fmt.Errorf("runpusher: create the pusher lock directory: %w", err)
	}
	held, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil || !acquired {
		return nil, false, err
	}
	return func() { _ = held.Release() }, true, nil
}

// pauseCovers reports whether a pause covers the run of the pair t.
func pauseCovers(t Target) (bool, error) {
	geometry, err := hubgeom.PauseGeometry(t.Location)
	if err != nil {
		return false, fmt.Errorf("runpusher: read the pause geometry of %s: %w", t.Location.WorktreePath(), err)
	}
	coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, geometry.Slug, time.Now())
	if err != nil {
		return false, fmt.Errorf("runpusher: read the pause record: %w", err)
	}
	return coverage.Covered, nil
}

// handedOver reports whether this machine's hand-over mark names the run of the pair t.
func handedOver(t Target) (bool, error) {
	_, found, err := fabricengine.ReadHandOverMark(t.Location.HubPath, t.Slug)
	return found, err
}
