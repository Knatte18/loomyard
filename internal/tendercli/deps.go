// deps.go fills tenderengine.Deps for a hub: its task pairs as shuttle-backed Pair values, the network operation and parent lock readers, and the orch watcher probe.

package tendercli

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// HubDeps fills the engine's Deps for the hub location belongs to, with strand the caller's strand name.
// Every task pair of the hub is a Pair, the prime excluded, and the pair slugs are returned beside the Deps.
func HubDeps(location *lymcwd.Location, strand string) (tenderengine.Deps, []string, error) {
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
		prime *lymcwd.Location
		// pairLocations holds each task pair's location by slug, for the push a lift spawns.
		pairLocations = map[string]*lymcwd.Location{}
		// pairByWorktree holds the shuttle-backed Pair of each wired worktree, the prime's included, by worktree path, for the orch that runs there.
		pairByWorktree = map[string]tenderengine.Pair{}
	)
	worktreePaths := make([]string, 0, len(worktrees))
	for _, worktree := range worktrees {
		worktreePaths = append(worktreePaths, worktree.Path)
	}
	for _, worktree := range worktrees {
		if worktree.Prunable {
			continue
		}
		pairLocation, err := lymcwd.ResolveWorktree(worktree.Path)
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
		pairByWorktree[pairLocation.WorktreePath()] = pair
		slugs = append(slugs, pairLocation.WorktreeName)
		pairLocations[pairLocation.WorktreeName] = pairLocation
	}

	var primePair tenderengine.Pair
	if prime != nil {
		wired, err := newShuttlePair(prime)
		if err != nil {
			logger.Warn("tendercli: the prime does not wire, so its sessions are not listed", "worktree", prime.WorktreePath(), "error", err)
		} else {
			primePair = wired
			pairByWorktree[prime.WorktreePath()] = wired
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
		RunProcs: func() (map[string][]runproc.Record, error) {
			return runproc.Owners(fabricengine.RunProcDir(location.HubPath))
		},
		Worktrees: func() ([]string, error) { return worktreePaths, nil },
		EndOwner: func(owner string) ([]runproc.Ended, error) {
			return runproc.End(fabricengine.RunProcDir(location.HubPath), owner, os.Getpid())
		},
		EndProcess: func(pid int, startTime string) ([]runproc.Ended, error) {
			return runproc.EndProcess(pid, startTime, "")
		},
		HubPath:    location.HubPath,
		Executable: executable,
		Orchs: func() ([]tenderengine.Orch, error) {
			return hubOrchs(location, pairByWorktree)
		},
		NetOps: func() ([]pauserecord.NetOp, error) {
			return pauserecord.ListNetOps(geometry.RecordPath, parentlock.ProcessAlive)
		},
		Locks: locks.List,
		Owners: func() ([]tenderengine.PairOwner, error) {
			here, err := machinename.Name()
			if err != nil {
				return nil, err
			}
			owners, err := boardengine.ReadOwners(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return nil, err
			}
			return pairOwners(owners, slugs, here, func(slug string) (string, error) {
				return hubgeom.ParentOf(location, slug)
			})
		},
		PushPair: func(slug string) error {
			return fabricengine.SpawnPairPush(pairLocations[slug])
		},
		PushPrimeRecords: func() error {
			if prime == nil {
				return nil
			}
			_, err := fabricengine.SpawnStatusPush(prime, "")
			return err
		},
		SyncBoardIfOwed: func() error {
			boardDir := fabricengine.BoardDir(location.HubPath)
			bolt := fabricengine.NewBolt(boardDir)
			return syncBoardIfOwed(bolt.HasUnpushed, bolt.HasUncommitted, func() error { return boardengine.SpawnSync(boardDir) })
		},
		Caller: strand,
		Now:    time.Now,
		Sleep:  time.Sleep,
	}
	return deps, slugs, nil
}

// pairOwners lists each local pair of slugs with the owner it reads as, then each run-held board entry owned by a machine other than here that has no local pair.
// parentOf names the slug of a pair's parent pair, or "" for the prime; its error is returned.
func pairOwners(owners boardengine.Owners, slugs []string, here string, parentOf func(slug string) (string, error)) ([]tenderengine.PairOwner, error) {
	listed := []tenderengine.PairOwner{}
	local := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		local[slug] = true
		owner, err := owners.Effective(slug, parentOf)
		if err != nil {
			return nil, fmt.Errorf("owner of %s: %w", slug, err)
		}
		listed = append(listed, tenderengine.PairOwner{Slug: slug, Owner: owner, Local: true, OwnedHere: !boardengine.OwnedElsewhere(owner, here)})
	}
	var remote []string
	for slug, entry := range owners.Entries {
		if entry.RunHeld && !local[slug] {
			remote = append(remote, slug)
		}
	}
	slices.Sort(remote)
	for _, slug := range remote {
		owner, err := owners.Effective(slug, parentOf)
		if err != nil {
			return nil, fmt.Errorf("owner of %s: %w", slug, err)
		}
		if boardengine.OwnedElsewhere(owner, here) {
			listed = append(listed, tenderengine.PairOwner{Slug: slug, Owner: owner})
		}
	}
	return listed, nil
}

// syncBoardIfOwed spawns the board sync when the board is ahead of its upstream or holds uncommitted changes, which is what a sync deferred at its start leaves.
// A probe error is returned and nothing is spawned.
func syncBoardIfOwed(ahead, uncommitted func() (bool, error), spawn func() error) error {
	for _, owed := range []func() (bool, error){ahead, uncommitted} {
		isOwed, err := owed()
		if err != nil {
			return err
		}
		if isOwed {
			return spawn()
		}
	}
	return nil
}

// ownerPauseGrace is how long the running turns of a run owned by another machine get to finish before the Esc.
const ownerPauseGrace = 10 * time.Second

// PauseOwnedElsewhere pauses the pair of slug in the hub location belongs to, because another machine owns its run.
// The entry carries pauserecord.OwnerCaller, and the pair's running turns are interrupted after the grace.
// A slug whose entry already stands adds none.
// Once the pause stands it ends the run's owned processes on this machine, unless the run's Finalize is past its parent-side merge;
// a failed end or a survivor is logged at Warn and never fails the pause.
func PauseOwnedElsewhere(location *lymcwd.Location, slug string) error {
	deps, _, err := HubDeps(location, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return err
	}
	if _, err = tenderengine.Pause(deps, tenderengine.PauseRequest{Slug: slug, Grace: ownerPauseGrace, Caller: pauserecord.OwnerCaller}); err != nil {
		return err
	}
	ended, err := endOwnedElsewhere(location, slug)
	if err != nil {
		logger.Warn("tendercli: ending the processes of a run owned elsewhere failed", "slug", slug, "error", err)
		return nil
	}
	if len(ended) > 0 {
		logger.Info("tendercli: ended the processes of a run owned elsewhere", "slug", slug, "ended", len(ended))
	}
	return nil
}

// ReviveStalled gives the revive line to the sessions of the pair slug that are stalled on an API error, through the conditioned send.
// It returns an error when the revive sent nothing and a session refused or did not land the line, so a caller that revives once per episode can warn.
func ReviveStalled(location *lymcwd.Location, slug string) error {
	deps, _, err := HubDeps(location, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return err
	}
	report, err := tenderengine.Revive(deps, tenderengine.ReviveRequest{Slug: slug})
	if err != nil {
		return err
	}
	return reviveError(report)
}

// reviveError maps a revive report onto an error: nil when a session was given the line or none refused it, otherwise an error naming the first refusal.
func reviveError(report tenderengine.ReviveReport) error {
	if len(report.Sent) > 0 || len(report.SentNoTurnStart) > 0 {
		return nil
	}
	refusals := append(slices.Clone(report.Refused), report.NotLanded...)
	if len(refusals) == 0 {
		return nil
	}
	first := refusals[0]
	return fmt.Errorf("revive sent no line: %s (%s): %s", first.Strand, first.Slug, first.Reason)
}

// endOwnedElsewhere is the end PauseOwnedElsewhere runs after its pause.
var endOwnedElsewhere = EndOwnedElsewhere

// EndOwnedElsewhere ends the processes registered under slug in the hub location belongs to, because another machine owns its run.
// A run whose Finalize is past its parent-side merge keeps them, since it finishes its landing on either machine, and so does a run whose reading fails.
// The calling process and its ancestors are never ended.
func EndOwnedElsewhere(location *lymcwd.Location, slug string) ([]runproc.Ended, error) {
	taskPath := fabricengine.WorktreePath(location, slug)
	var taskLocation *lymcwd.Location
	locateTask := func() (*lymcwd.Location, error) {
		if taskLocation != nil {
			return taskLocation, nil
		}
		resolved, err := lymcwd.ResolveWorktree(taskPath)
		taskLocation = resolved
		return resolved, err
	}
	return ownedElsewhereEnder{
		slug: slug,
		readStatus: func() (shedengine.Status, bool, error) {
			if _, err := os.Stat(taskPath); os.IsNotExist(err) {
				return shedengine.Status{}, false, nil
			} else if err != nil {
				return shedengine.Status{}, false, err
			}
			task, err := locateTask()
			if err != nil {
				return shedengine.Status{}, false, err
			}
			statusPath := shedrun.StatusFile(task, shedrun.SelfRunID)
			if _, err := os.Stat(statusPath); os.IsNotExist(err) {
				return shedengine.Status{}, false, nil
			}
			lockPath := shedrun.StatusLock(task, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				return shedengine.Status{}, false, err
			}
			return state.ReadJSON[shedengine.Status](statusPath, lockPath)
		},
		restepped: func(status shedengine.Status) (bool, error) {
			task, err := locateTask()
			if err != nil {
				return false, err
			}
			records, err := sheddriver.ReadSettleRecords(shedrun.SettleRecords(task, shedrun.SelfRunID))
			if err != nil {
				return false, err
			}
			return sheddriver.RestepRecorded(records, status), nil
		},
		parentHolds: func() (bool, error) {
			task, err := locateTask()
			if err != nil {
				return false, err
			}
			return fabricengine.ParentHoldsTaskWork(location, task, slug)
		},
		end: func() ([]runproc.Ended, error) {
			return runproc.End(fabricengine.RunProcDir(location.HubPath), slug, os.Getpid())
		},
	}.run()
}

// ownedElsewhereEnder is one run's end of its owned processes, over the seams that read the run.
type ownedElsewhereEnder struct {
	slug string
	// readStatus reads the task worktree's shed status; found is false when the worktree or its status file is absent.
	readStatus func() (status shedengine.Status, found bool, err error)
	// restepped reads whether the settle records re-stepped the stop the status names.
	restepped func(status shedengine.Status) (bool, error)
	// parentHolds reads whether the parent branch already holds the task's work.
	parentHolds func() (bool, error)
	// end ends the processes registered under the slug.
	end func() ([]runproc.Ended, error)
}

// run ends the slug's owned processes and returns what it ended.
// A run whose Finalize is past its parent-side merge ends nothing, and neither does a run whose status or Finalize reading fails, which returns that error.
// A task worktree with no status file, or none at all, ends by slug, since no Finalize can be running there.
// An end error and a *runproc.SurvivorsError come back wrapped with the commands that finish the end.
func (e ownedElsewhereEnder) run() ([]runproc.Ended, error) {
	status, found, err := e.readStatus()
	if err != nil {
		return nil, fmt.Errorf("read the status of %s before ending its processes: %w", e.slug, err)
	}
	if found {
		past, err := battenshed.FinalizePastMerge(status, e.restepped, e.parentHolds)
		if err != nil {
			return nil, fmt.Errorf("read whether the Finalize of %s is past its parent-side merge: %w", e.slug, err)
		}
		if past {
			return nil, nil
		}
	}
	ended, err := e.end()
	if err != nil {
		return ended, fmt.Errorf("end the processes of %s: %w; run `lym fabric remove %s`, or `lym reed down` in the pair, while the pair exists, and `lym tender reap` once it is gone", e.slug, err, e.slug)
	}
	return ended, nil
}

// tenderProgram is the program name tender signs its creel calls with.
const tenderProgram = "TENDER"

// orchReviveTTL is how long a revive inject waits for the orch's next idle before it expires.
const orchReviveTTL = time.Minute

// postedEvent is the log event of a message or inject being left in creel.
const postedEvent = "posted"

// orchReviveTarget returns a creel client signing as tender and the name of the orch strand of worktree the revive injects into.
func orchReviveTarget(worktree *lymcwd.Location) (*creelcli.GoClient, string, error) {
	orch, recorded, err := orchcli.OrchStrandOf(worktree)
	if err != nil {
		return nil, "", err
	}
	if !recorded {
		return nil, "", fmt.Errorf("the orch of %s records no orch strand", worktree.WorktreeName)
	}
	client, err := creelcli.NewProgramClient(worktree, tenderProgram)
	if err != nil {
		return nil, "", err
	}
	return client, orch, nil
}

// hubOrchs lists the orchs of the hub location belongs to that record a strand, the prime's and each beam's.
// An orch is listed with the Pair in pairByWorktree for its worktree, nil when that worktree did not wire, which leaves its sessions unread.
func hubOrchs(location *lymcwd.Location, pairByWorktree map[string]tenderengine.Pair) ([]tenderengine.Orch, error) {
	worktrees, err := orchcli.HubOrchs(location)
	if err != nil {
		return nil, err
	}
	var orchs []tenderengine.Orch
	for _, worktree := range worktrees {
		orchState, err := orchengine.LoadState(orchcli.PrimePaths(worktree))
		if err != nil {
			return nil, fmt.Errorf("read the orch state of %s: %w", worktree.WorktreeName, err)
		}
		if orchState.Strand == "" {
			continue
		}
		name, _, err := orchcli.OrchStrandOf(worktree)
		if err != nil {
			return nil, err
		}
		orchs = append(orchs, tenderengine.Orch{
			Strand: orchState.Strand,
			Name:   name,
			Pair:   pairByWorktree[worktree.WorktreePath()],
			WatcherLive: func() bool {
				live, err := orchengine.WatcherLive(orchcli.PrimePaths(worktree))
				if err != nil {
					logger.Warn("tendercli: the orch watcher probe failed, reading it as not live", "worktree", worktree.WorktreeName, "error", err)
				}
				return live
			},
			RevivePending: func() (bool, error) {
				client, orch, err := orchReviveTarget(worktree)
				if err != nil {
					return false, err
				}
				from, err := agentname.FormatProgram(client.Shortname(), tenderProgram)
				if err != nil {
					return false, err
				}
				entries, err := client.Log(creelengine.LogFilter{From: from, To: orch})
				return revivePending(entries), err
			},
			InjectRevive: func(line string) (int64, error) {
				client, orch, err := orchReviveTarget(worktree)
				if err != nil {
					return 0, err
				}
				return client.Inject(orch, line, orchReviveTTL)
			},
		})
	}
	return orchs, nil
}

// revivePending reports whether entries, tender's log to the orch strand, hold an inject whose posted event has no injected, unconfirmed or expired event for its id after it.
func revivePending(entries []creelengine.LogEntry) bool {
	pending := map[int64]bool{}
	for _, entry := range entries {
		switch entry.Event {
		case postedEvent:
			pending[entry.ID] = true
		case string(creelengine.OutcomeInjected), string(creelengine.OutcomeUnconfirmed), string(creelengine.OutcomeExpired):
			delete(pending, entry.ID)
		}
	}
	return len(pending) > 0
}

// shuttlePair is one task pair read and driven through its own shuttle runner and reed session.
type shuttlePair struct {
	location *lymcwd.Location
	cfg      shuttleengine.Config
	engines  shuttleengine.EngineSet
	reed     *reedengine.Engine
	runner   *shuttleengine.Runner
}

// newShuttlePair builds the pair's runner the way the pair's own wiring does: its reed and shuttle config and its reed geometry.
func newShuttlePair(location *lymcwd.Location) (shuttlePair, error) {
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
	pauseGeom, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return shuttlePair{}, err
	}
	reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	engines, err := shuttleengines.New(shuttleCfg, configDirs)
	if err != nil {
		return shuttlePair{}, err
	}
	runner := shuttleengine.NewRunner(reed, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	// Told the hub's record with no slug, so each send judges its target by the target's own strand name.
	runner.SetPause(pauserecord.Geometry{RecordPath: pauseGeom.RecordPath})
	return shuttlePair{
		location: location,
		cfg:      shuttleCfg,
		engines:  engines,
		reed:     reed,
		runner:   runner,
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
	status, found, err := state.ReadJSON[shedengine.Status](statusPath, lockPath)
	if err != nil || !found {
		return "", false, err
	}
	return status.CurrentProducer, status.State == shedengine.StateRunning, nil
}
