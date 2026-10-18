// deps.go fills hibernateengine.Deps for a hub: the request file and log paths, the prime's batten status and land lock readers, tender's engine over the hub's pairs, and the daemon's identity.

package hibernatecli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

const (
	// requestFileName is the hibernate request file in the hibernate directory.
	requestFileName = "request.json"
	// watchLogFileName is the daemon's log in the hibernate directory.
	watchLogFileName = "watch.log"
	// hibernateProgram is the program name hibernate signs its notices to the orch with.
	hibernateProgram = "HIBERNATE"
)

// hibernateDir is the hub's board-level directory holding the request, its lock and the daemon's log.
func hibernateDir(location *lyxcwd.Location) string {
	return filepath.Join(fabricengine.HubScratchDir(location.HubPath), lyxdirs.HibernateDirName)
}

// hubDeps fills the engine's Deps for the hub location belongs to.
// Tender's seams build their wiring at each call, so each reading sees the hub's current pairs, and `status` builds none.
func hubDeps(location *lyxcwd.Location) (hibernateengine.Deps, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return hibernateengine.Deps{}, err
	}
	prime, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
	if err != nil {
		return hibernateengine.Deps{}, err
	}
	dir := hibernateDir(location)
	selfPID := os.Getpid()
	selfStart, _ := proc.StartTime(selfPID)

	tenderDeps := func() (tenderengine.Deps, error) {
		deps, _, err := tendercli.HubDeps(location, hibernateengine.PauseCaller)
		return deps, err
	}
	return hibernateengine.Deps{
		RequestPath: filepath.Join(dir, requestFileName),
		Caller:      os.Getenv(agentname.StrandNameEnv),
		GOOS:        runtime.GOOS,
		LookPath:    exec.LookPath,
		Spawn: func() (int, string, error) {
			return spawnDaemon(prime.AnchorPath(), filepath.Join(dir, watchLogFileName))
		},
		SelfPID:   selfPID,
		SelfStart: selfStart,
		Alive:     parentlock.ProcessAlive,
		Kill:      killDaemon,
		RunStatus: func(slug string) (hibernateengine.RunStatus, bool, error) {
			return readRunStatus(prime, slug)
		},
		RunSlugs: func() ([]string, error) { return shedrun.List(prime) },
		LandLockHeld: func() (bool, error) {
			return landLockHeld(shedrun.LandLock(prime))
		},
		Pause: func(req tenderengine.PauseRequest) (tenderengine.PauseReport, error) {
			deps, err := tenderDeps()
			if err != nil {
				return tenderengine.PauseReport{}, err
			}
			return tenderengine.Pause(deps, req)
		},
		TenderStatus: func() (tenderengine.StatusReport, error) {
			deps, err := tenderDeps()
			if err != nil {
				return tenderengine.StatusReport{}, err
			}
			return tenderengine.Status(deps)
		},
		Lift: func(ref tenderengine.EntryRef) (tenderengine.ResumeReport, error) {
			deps, err := tenderDeps()
			if err != nil {
				return tenderengine.ResumeReport{}, err
			}
			return tenderengine.Lift(deps, ref)
		},
		Hibernate: runHibernate,
		Notify: func(line string) error {
			_, err := orchcli.NotifyPrime(location, hibernateProgram, line)
			return err
		},
		Now:   time.Now,
		Sleep: time.Sleep,
		Awake: pauserecord.MonotonicAwake(),
	}, nil
}

// readRunStatus reads the batten status of the run slug in the prime, and reports not found when the run has no status file.
func readRunStatus(prime *lyxcwd.Location, slug string) (hibernateengine.RunStatus, bool, error) {
	statusPath := shedrun.StatusFile(prime, slug)
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		return hibernateengine.RunStatus{}, false, nil
	}
	lockPath := shedrun.StatusLock(prime, slug)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return hibernateengine.RunStatus{}, false, err
	}
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](statusPath, lockPath)
	if err != nil || !found {
		return hibernateengine.RunStatus{}, false, err
	}
	return hibernateengine.RunStatus{State: status.State, Producer: status.CurrentProducer}, true, nil
}

// landLockHeld reports whether a Land row holds the land lock at path.
// A lock whose directory does not exist is not held; otherwise the probe takes the lock once and releases it at once, so a landing that tries it in that instant polls again a second later, as a contended lock already makes it.
func landLockHeld(path string) (bool, error) {
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		return false, nil
	}
	probe, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, probe.Release()
}
