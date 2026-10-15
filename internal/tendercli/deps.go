// deps.go fills tenderengine.Deps for a hub: its task pairs as shuttle-backed Pair values, the network operation and parent lock readers, and the orch watcher probe.

package tendercli

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// HubDeps fills the engine's Deps for the hub location belongs to, with strand the caller's strand name.
// Every task pair of the hub is a Pair, the prime excluded, and the pair slugs are returned beside the Deps.
func HubDeps(location *lyxcwd.Location, strand string) (tenderengine.Deps, []string, error) {
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return tenderengine.Deps{}, nil, err
	}
	worktrees, err := fabricengine.List(location.WorktreePath())
	if err != nil {
		return tenderengine.Deps{}, nil, err
	}
	var (
		pairs []tenderengine.Pair
		slugs []string
		prime *lyxcwd.Location
	)
	for _, worktree := range worktrees {
		if worktree.Prunable {
			continue
		}
		pairLocation, err := lyxcwd.ResolveWorktree(worktree.Path)
		if err != nil {
			logger.Warn("tendercli: skipping a worktree that does not resolve", "worktree", worktree.Path, "error", err)
			continue
		}
		if worktree.Main {
			prime = pairLocation
			continue
		}
		pair, err := newShuttlePair(pairLocation)
		if err != nil {
			return tenderengine.Deps{}, nil, fmt.Errorf("pause: wire the pair %s: %w", pairLocation.WorktreeName, err)
		}
		pairs = append(pairs, pair)
		slugs = append(slugs, pairLocation.WorktreeName)
	}

	var primePair tenderengine.Pair
	if prime != nil {
		wired, err := newShuttlePair(prime)
		if err != nil {
			logger.Warn("tendercli: the prime does not wire, so its sessions are not listed", "worktree", prime.WorktreePath(), "error", err)
		} else {
			primePair = wired
		}
	}
	// An unreadable executable path is empty, which keeps no stale process.
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}

	locks := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(location.HubPath))}
	deps := tenderengine.Deps{
		RecordPath: geometry.RecordPath,
		Pairs:      pairs,
		Prime:      primePair,
		Processes:  proc.ListProcesses,
		HubPath:    location.HubPath,
		Executable: executable,
		OrchStrand: func() (string, error) {
			if prime == nil {
				return "", nil
			}
			orchState, err := orchengine.LoadState(orchcli.PrimePaths(prime))
			return orchState.Strand, err
		},
		QueueOrchRevive: func(line string) (bool, error) {
			if prime == nil {
				return false, nil
			}
			return orchengine.QueueConditionalNotice(orchcli.PrimePaths(prime), line, orchengine.NoticeCondition{State: shuttleengine.SessionIdleStalled, Cause: shuttleengine.SessionCauseAPIError}, time.Now())
		},
		NetOps: func() ([]pauserecord.NetOp, error) {
			return pauserecord.ListNetOps(geometry.RecordPath, parentlock.ProcessAlive)
		},
		Locks: locks.List,
		OrchWatcherLive: func() bool {
			if prime == nil {
				return false
			}
			live, err := orchengine.WatcherLive(orchcli.PrimePaths(prime))
			if err != nil {
				logger.Warn("tendercli: the orch watcher probe failed, reading it as not live", "error", err)
			}
			return live
		},
		Caller: strand,
		Now:    time.Now,
		Sleep:  time.Sleep,
	}
	return deps, slugs, nil
}

// shuttlePair is one task pair read and driven through its own shuttle runner and reed session.
type shuttlePair struct {
	location *lyxcwd.Location
	cfg      shuttleengine.Config
	engines  shuttleengine.EngineSet
	reed     *reedengine.Engine
	runner   *shuttleengine.Runner
}

// newShuttlePair builds the pair's runner the way the pair's own wiring does: its reed and shuttle config and its reed geometry.
func newShuttlePair(location *lyxcwd.Location) (shuttlePair, error) {
	configDirs := fabricengine.ConfigDirs(location)
	reedCfg, err := reedengine.LoadConfig(configDirs, "reed")
	if err != nil {
		return shuttlePair{}, err
	}
	shuttleCfg, err := shuttleengine.LoadConfig(configDirs, "shuttle")
	if err != nil {
		return shuttlePair{}, err
	}
	reedGeom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return shuttlePair{}, err
	}
	reed := reedengine.New(reedCfg, reedGeom)
	engines, err := shuttleengines.New(shuttleCfg)
	if err != nil {
		return shuttlePair{}, err
	}
	return shuttlePair{
		location: location,
		cfg:      shuttleCfg,
		engines:  engines,
		reed:     reed,
		runner:   shuttleengine.NewRunner(reed, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg),
	}, nil
}

// Slug is the pair's slug.
func (p shuttlePair) Slug() string { return p.location.WorktreeName }

// Sessions lists the pair's running runs whose strand reed reports live.
func (p shuttlePair) Sessions() ([]tenderengine.SessionInfo, error) {
	readings, err := shuttleengine.ReadSessionStates(p.cfg, p.location.AnchorPath(), p.engines, time.Now())
	if err != nil || len(readings) == 0 {
		return nil, err
	}
	var sessions []tenderengine.SessionInfo
	for _, reading := range readings {
		live, err := p.Live(reading.StrandGUID)
		if err != nil {
			return nil, err
		}
		if live {
			sessions = append(sessions, tenderengine.SessionInfo{Strand: reading.StrandName, GUID: reading.StrandGUID, Slug: p.Slug()})
		}
	}
	return sessions, nil
}

// State reads the session state of the running run with the strand guid, or the zero state when the strand has none.
func (p shuttlePair) State(guid string) (shuttleengine.SessionState, error) {
	readings, err := shuttleengine.ReadSessionStates(p.cfg, p.location.AnchorPath(), p.engines, time.Now())
	if err != nil {
		return shuttleengine.SessionState{}, err
	}
	for _, reading := range readings {
		if reading.StrandGUID == guid {
			return reading.State, nil
		}
	}
	return shuttleengine.SessionState{}, nil
}

// readTurn derives from state whether the session is in a running turn, and whether its turn ended with background work, with one label per outstanding task.
// A task's label is its description, or its id when it has none.
func readTurn(state shuttleengine.SessionState) (running bool, labels []string, background bool) {
	if state.Name == shuttleengine.SessionBusy && state.Cause == shuttleengine.SessionCauseBackground {
		for _, task := range state.Outstanding {
			labels = append(labels, cmp.Or(task.Label, task.ID))
		}
		return false, labels, true
	}
	return state.RunningTurn(), nil, false
}

// Busy reports whether the run's session state reads a running turn.
func (p shuttlePair) Busy(guid string) (bool, error) {
	state, err := p.State(guid)
	running, _, _ := readTurn(state)
	return running, err
}

// Background reports whether the run's session turn ended with background work still running, with the outstanding tasks' labels.
func (p shuttlePair) Background(guid string) ([]string, bool, error) {
	state, err := p.State(guid)
	_, labels, background := readTurn(state)
	return labels, background, err
}

// PaneRunningTurn reports whether the strand's pane shows a running turn.
func (p shuttlePair) PaneRunningTurn(guid string) (bool, error) {
	return p.runner.PaneRunningTurn(guid)
}

// InterruptSettle is how long after an interrupt key the strand's pane needs before it is read again.
func (p shuttlePair) InterruptSettle(guid string) (time.Duration, error) {
	return p.runner.InterruptSettle(guid)
}

// Live reports whether reed tracks the strand with a live pane.
func (p shuttlePair) Live(guid string) (bool, error) {
	status, err := p.reed.Status()
	if err != nil {
		return false, err
	}
	for _, strand := range status.Strands {
		if strand.GUID == guid {
			return strand.Live, nil
		}
	}
	return false, nil
}

// Interrupt sends the provider's interrupt key through the runner, which checks the pane first.
func (p shuttlePair) Interrupt(guid string) error { return p.runner.Interrupt(guid) }

// Send types text through the runner's guarded paste-submit.
func (p shuttlePair) Send(guid, text string) error { return p.runner.Send(guid, text) }

// SendConditioned types text through the runner's conditioned send, which refuses at once unless the session reads want.
func (p shuttlePair) SendConditioned(guid, text string, want shuttleengine.SendCondition) (bool, error) {
	return p.runner.SendConditioned(guid, text, want)
}

// Running reports the pair's loom run's current producer and whether its status file reads running.
func (p shuttlePair) Running() (string, bool, error) {
	statusPath := shedrun.StatusFile(p.location, shedrun.SelfRunID)
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		return "", false, nil
	}
	lockPath := shedrun.StatusLock(p.location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return "", false, err
	}
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](statusPath, lockPath)
	if err != nil || !found {
		return "", false, err
	}
	return status.CurrentProducer, status.State == shedengine.StateRunning, nil
}
