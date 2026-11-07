// deps.go fills hibernateengine.Deps for a hub, and builds the Carrier that the prime's dobby runs.
// The Deps wire the hub's paths, the prime's batten and dobby readers, tender's pause seams and the process identity.

package hibernatecli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
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
	// noticeFileName is the file in the hibernate directory a notice too long for one creel line is written to.
	noticeFileName = "notice.txt"
	// hibernateProgram is the program name hibernate signs its notices to the orch with.
	hibernateProgram = "HIBERNATE"
)

// hibernateDir returns the hub's board-level hibernate directory.
// It holds the request, its lock and the whole text of a notice too long for one creel line.
func hibernateDir(location *lymcwd.Location) string {
	return filepath.Join(fabricengine.HubScratchDir(location.HubPath), lymdirs.HibernateDirName)
}

// RequestPath is the hibernate request file of the hub location belongs to.
func RequestPath(location *lymcwd.Location) string {
	return filepath.Join(hibernateDir(location), requestFileName)
}

// NewDobbyCarrier returns the Carrier the prime's dobby carries the hub's hibernate request with.
// The Carrier is wired over the hub of prime.
// It creates the hibernate directory, so the dobby's file watch can open it before the first request is written.
func NewDobbyCarrier(prime *lymcwd.Location) (*hibernateengine.Carrier, error) {
	deps, err := hubDeps(prime)
	if err != nil {
		return nil, err
	}
	// The dobby's file watch opens only a directory that exists, so a first request on a hub that never hibernated would wake nothing.
	if err := os.MkdirAll(hibernateDir(prime), 0o755); err != nil {
		return nil, fmt.Errorf("create the hibernate directory: %w", err)
	}
	return hibernateengine.NewCarrier(deps), nil
}

// hubDeps fills the engine's Deps for the hub location belongs to.
// Tender's seams build their wiring at each call, so each reading sees the hub's current pairs, and `status` builds none.
func hubDeps(location *lymcwd.Location) (hibernateengine.Deps, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return hibernateengine.Deps{}, err
	}
	prime, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
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
		RequestPath: RequestPath(location),
		Caller:      os.Getenv(agentname.StrandNameEnv),
		GOOS:        runtime.GOOS,
		LookPath:    exec.LookPath,
		DobbyLive: func() (bool, error) {
			geom, err := hubgeom.DobbyGeometry(prime, primeName)
			if err != nil {
				return false, err
			}
			return dobbyengine.Live(geom)
		},
		SelfPID:   selfPID,
		SelfStart: selfStart,
		Alive:     parentlock.ProcessAlive,
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
		Reconcile: func() (hibernateengine.WakeReconcile, error) { return wakeReconcile(prime) },
		EndOwned: func(slug string) error {
			_, err := tendercli.EndOwnedElsewhere(prime, slug)
			return err
		},
		ExemptNetwork: func(ref tenderengine.EntryRef) func() {
			return pauserecord.SetGateExempt(ref.Scope, ref.PausedAt)
		},
		Hibernate: runHibernate,
		Notify: func(line string) error {
			posted, err := orchcli.NotifyPrime(location, hibernateProgram, line)
			if err == nil && !posted {
				logger.Warn("hibernate: no orch strand is recorded, so the notice was not queued", "notice", line)
			}
			return err
		},
		NoticePath: filepath.Join(dir, noticeFileName),
		Now:        time.Now,
		Sleep:      time.Sleep,
		Awake:      pauserecord.MonotonicAwake(),
	}, nil
}

// readRunStatus reads the batten status of the run slug in the prime, and reports not found when the run has no status file.
func readRunStatus(prime *lymcwd.Location, slug string) (hibernateengine.RunStatus, bool, error) {
	statusPath := shedrun.StatusFile(prime, slug)
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		return hibernateengine.RunStatus{}, false, nil
	}
	lockPath := shedrun.StatusLock(prime, slug)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return hibernateengine.RunStatus{}, false, err
	}
	status, found, err := state.ReadJSON[shedengine.Status](statusPath, lockPath)
	if err != nil || !found {
		return hibernateengine.RunStatus{}, false, err
	}
	return hibernateengine.RunStatus{State: status.State, Producer: status.CurrentProducer}, true, nil
}

// landLockHeld reports whether a Land row holds the land lock at path.
// A lock whose directory does not exist is not held.
// Otherwise the probe takes the lock once and releases it at once,
// so a landing that tries it in that instant polls again a second later, as a contended lock already makes it.
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
