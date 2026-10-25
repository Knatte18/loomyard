// loop.go implements dobby's daemon loop: the lock that keeps one dobby per beam worktree, the owner exit, the sleep watch, the coalesced wakeups and the binary check between ticks.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// wakeFloor is the shortest gap between two wakes of the loop, ticks coalescing events and timers alike to it.
	wakeFloor = time.Second
	// idleCeiling is the longest the timer waits while no run is in flight.
	idleCeiling = time.Minute
)

// ErrDobbyRunning is returned by Run when another dobby holds this worktree's lock.
var ErrDobbyRunning = errors.New("dobby: another dobby holds this worktree's lock")

// TickInput is what one tick tells the per-tick work.
type TickInput struct {
	// Now is the tick's wall time.
	Now time.Time
	// ProbeDue reports whether a probe interval has passed since the last probe tick, so this tick is a probe tick.
	ProbeDue bool
	// OwesAll reports whether every run dobby drives is owed a full pass: the dobby started, saw a machine sleep, or found its state record without a pid.
	// It stays set until a tick returns nil.
	OwesAll bool
}

// Deps is every seam Run reads, told by the hub wiring.
type Deps struct {
	// Now is the wall clock.
	Now func() time.Time
	// Awake is a clock that stops while the machine sleeps, `pauserecord.MonotonicAwake()` in production.
	Awake func() time.Duration
	// Sleep waits d, or returns early when ctx is cancelled.
	Sleep func(ctx context.Context, d time.Duration)
	// Watch opens the file watch over the prime's batten runs directory, each run's child status directory, the board's `board.json` and the hub's pause record directory.
	// Its channel delivers one value per change; closeWatch releases it.
	// A nil Watch, or one that returns an error, leaves the timer alone to drive the loop.
	Watch func() (events <-chan struct{}, closeWatch func(), err error)
	// Machine returns this machine's name.
	Machine func() (string, error)
	// Owner returns the machine that owns this dobby's beam, the prime owner for the prime; empty means no other machine owns it.
	Owner func() (string, error)
	// CheckBinary re-execs onto a changed lyx binary and returns when the binary is unchanged.
	CheckBinary func()
	// ProbeInterval is batten's `notice_probe_s`; a value below one second is floored to it with one Warn.
	ProbeInterval time.Duration
	// Tick is an extra per-tick hook that runs after the scheduling steps: it reports whether anything it drives is in flight for the loop's backoff.
	// A nil Tick does nothing.
	Tick func(ctx context.Context, in TickInput) (inFlight bool, err error)
	// Runs lists the batten runs whose parent worktree is this dobby's own, read from the prime's local batten status files and never from the board.
	// A run's parent is the one its pair's origin record names, the prime for none, so a run whose task Land already pruned from the board is still listed.
	Runs func() ([]RunView, error)
	// SpawnPass starts `<lyx> batten run <slug>` with the hidden continue and no-reopen flags as asked, in its own process group with the prime's anchor as its working directory, logged at Info.
	// The pass registers itself in `internal/runproc` as the run's batten, so dobby registers nothing.
	// The channel delivers the pass's exit once.
	SpawnPass func(slug string, continuePass, noReopen bool) (<-chan PassExit, error)
	// Notice posts one line to the nearest live orch.
	Notice func(ctx context.Context, line string) error
	// PauseOwned pauses the run slug under the owner caller, because another machine owns it (`tendercli.PauseOwnedElsewhere` in production).
	PauseOwned func(ctx context.Context, slug string) error
	// ReleaseParentLock releases the parent lock record naming the run's child, the release a run's own owned-elsewhere pause makes.
	ReleaseParentLock func(ctx context.Context, slug string) error
	// SlugOwner returns the effective owner of slug on the local Bolt, with no pull; empty means no other machine owns it.
	SlugOwner func(slug string) (string, error)
	// LiftOwnerPause lifts the pause entry of the slug's own scope that carries the owner caller.
	LiftOwnerPause func(ctx context.Context, slug string) error
	// Tasks reads the local board, each entry as a TaskView, with its parent worktree through `hubgeom.ParentOf` and its effective owner.
	Tasks func() ([]TaskView, error)
	// LoadConfig loads beam.yaml over the dobby's worktree layer stack.
	LoadConfig func() (Config, error)
	// StatusExists reports whether a first pass left batten's status file for slug.
	StatusExists func(slug string) (bool, error)
	// PauseCovers reports whether a live pause entry covers the hub or slug.
	PauseCovers func(slug string) (bool, error)
	// Landings returns the slugs of the runs whose parent worktree is this dobby's own and whose batten history holds a Land Done, whatever the run's state.
	// A nil Landings, or a nil TidyShuttle, leaves the board tidy off.
	Landings func() ([]string, error)
	// TidyShuttle starts and stops the board-tidy agent.
	TidyShuttle TidyShuttle
	// TidyModel is the model the board-tidy agent runs on, resolved by the hub wiring.
	TidyModel shuttleengine.ModelChoice
	// StencilsDir is the hub's stencils directory, where the board-tidy stencil is read at call time.
	StencilsDir string
	// NearestOrch returns the name of the nearest live orch the tidy agent's parent directive names; empty means there is none.
	NearestOrch func() (string, error)
	// Log writes one line to the dobby's log: one for each task start and each pass spawned.
	// A nil Log logs nothing.
	Log func(line string)
}

// Run drives one dobby for geom until ctx ends or another machine owns the beam.
// It waits lock.ProbeGrace for geom.LockPath and returns ErrDobbyRunning when another dobby still holds it.
// It reads the beam's owner at start and on every probe tick, and returns nil, logging why, when another machine owns it.
// A state record holding this process's pid is a re-exec, which owes no new full pass; any other pid, or no record, is a start.
// A tick that fails is logged once per episode and the loop goes on; a cancelled context ends it after the tick in flight.
func Run(ctx context.Context, geom Geometry, deps Deps) error {
	if err := os.MkdirAll(filepath.Dir(geom.LockPath), 0o755); err != nil {
		return err
	}
	held, acquired, err := lock.AcquireWriteLockWithin(geom.LockPath, lock.ProbeGrace)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrDobbyRunning
	}
	defer func() { _ = held.Release() }()

	d := &daemon{
		geom: geom, deps: deps, probe: deps.ProbeInterval,
		flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{},
	}
	if d.probe < wakeFloor {
		logger.Warn("dobbyengine: the probe interval is below the floor and is floored", "key", "notice_probe_s", "floor", wakeFloor, "configured", d.probe)
		d.probe = wakeFloor
	}
	if err := d.recordStart(); err != nil {
		return err
	}

	var events <-chan struct{}
	if deps.Watch != nil {
		watched, closeWatch, err := deps.Watch()
		if err != nil {
			logger.Warn("dobbyengine: the file watch did not open; the timer alone drives the loop", "error", err)
		} else {
			events = watched
			defer closeWatch()
		}
	}
	return d.loop(ctx, events)
}

// daemon is one running dobby: the told geometry and seams, and what the loop remembers between ticks.
type daemon struct {
	geom Geometry
	deps Deps
	// probe is the floored probe interval.
	probe time.Duration

	sleepWatch pauserecord.SleepWatch
	owesAll    bool
	inFlight   bool
	probeDue   bool
	lastProbe  time.Time
	lastTick   time.Time
	failures   logger.Episode
	ownerRead  logger.Episode
	moveStands logger.Episode

	// tickCtx is the context of the tick in flight, for the seams a pass exit reaches.
	tickCtx context.Context
	// busy reports whether any run is in flight or could be given a pass on the next probe tick.
	busy bool
	// flights holds the passes spawned that have not exited, by slug.
	flights map[string]flight
	// childSeen holds the last child status modification time read, by slug.
	childSeen map[string]time.Time
	// wasCovered holds whether a pause covered the run at the last tick, by slug.
	wasCovered map[string]bool
	// liftPending holds the runs whose covering pause lifted and which have not been given a pass since.
	liftPending map[string]bool
	// ownerPauses tracks, by slug, the episodes of a failing owner pause.
	ownerPauses logger.EpisodeSet
	// runs is the listing of the tick in flight.
	runs []RunView
}

// recordStart reads the state record and decides whether this run of the loop is a start or a re-exec, recording the pid and start time on a start.
func (d *daemon) recordStart() error {
	record, found, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	if found && record.PID == os.Getpid() {
		return nil
	}
	d.owesAll = true
	now := d.deps.Now()
	return UpdateState(d.geom.StatePath, func(s *State) error {
		s.PID, s.StartTime = os.Getpid(), now.UTC().Format(time.RFC3339)
		return nil
	})
}

// loop ticks and waits until ctx ends or another machine owns the beam.
func (d *daemon) loop(ctx context.Context, events <-chan struct{}) error {
	var wait time.Duration
	event := true
	for ctx.Err() == nil {
		now := d.deps.Now()
		d.probeDue = d.lastProbe.IsZero() || now.Sub(d.lastProbe) >= d.probe
		if d.probeDue {
			d.lastProbe = now
			if elsewhere, owner := d.beamOwnedElsewhere(); elsewhere {
				logger.Info("dobbyengine: another machine owns this beam; the dobby exits", "worktree", d.geom.WorktreeName, "owner", owner)
				return nil
			}
		}

		if err := d.tick(ctx, now); err != nil {
			d.failures.Fail("dobbyengine: a tick failed; the loop goes on", "worktree", d.geom.WorktreeName, "error", err)
		} else {
			d.failures.Recover("dobbyengine: ticks succeed again", "worktree", d.geom.WorktreeName)
		}
		d.lastTick = now
		if ctx.Err() != nil {
			return nil
		}

		d.deps.CheckBinary()

		wait = nextWait(wait, d.inFlight, event)
		if wait == 0 {
			wait = d.probe
		}
		wait = max(wait, d.probe, wakeFloor)
		event = d.sleep(ctx, wait, events)
	}
	return nil
}

// beamOwnedElsewhere reads the beam's owner and reports the other machine that owns it.
// A failed read is logged once per episode and reads as not owned elsewhere.
func (d *daemon) beamOwnedElsewhere() (bool, string) {
	machine, err := d.deps.Machine()
	if err == nil {
		var owner string
		if owner, err = d.deps.Owner(); err == nil {
			d.ownerRead.Recover("dobbyengine: the beam's owner reads again", "worktree", d.geom.WorktreeName)
			return owner != "" && owner != machine, owner
		}
	}
	d.ownerRead.Fail("dobbyengine: the beam's owner is unreadable; the dobby goes on", "worktree", d.geom.WorktreeName, "error", err)
	return false, ""
}

// standingMove returns the path of the first anchor-move record of the geometry that exists, empty when none does.
func (d *daemon) standingMove() string {
	for _, path := range d.geom.MoveRecords {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// tick folds one awake reading into the sleep watch, keeps the state record's pid and sleeps, and runs the per-tick work.
// A sleep the watch sees, and a record found without a pid, owe every run a full pass until a tick returns nil.
// While an anchor-move record of the geometry exists, a tick spawns no pass, starts no task and runs no tidy step, and owes what it owed before.
// That is logged once per episode, and the state record's upkeep and the landing count go on.
func (d *daemon) tick(ctx context.Context, now time.Time) error {
	d.sleepWatch.Observe(now, d.deps.Awake())
	seen := d.sleepWatch.Sleeps()
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		if s.PID == 0 {
			s.PID, s.StartTime = os.Getpid(), now.UTC().Format(time.RFC3339)
			d.owesAll = true
		}
		for _, sleep := range seen {
			// Compared by instant: a record read back from JSON carries no time zone pointer of the one it was written with.
			if !slices.ContainsFunc(s.Sleeps, func(known pauserecord.Interval) bool {
				return known.Start.Equal(sleep.Start) && known.End.Equal(sleep.End)
			}) {
				s.Sleeps = append(s.Sleeps, sleep)
				d.owesAll = true
			}
		}
		cutoff := now.Add(-pauserecord.Retention)
		s.Sleeps = slices.DeleteFunc(s.Sleeps, func(sleep pauserecord.Interval) bool { return sleep.End.Before(cutoff) })
		return nil
	})
	if err != nil {
		return err
	}
	moving := d.standingMove()
	if moving != "" {
		d.moveStands.Fail("dobbyengine: an anchor change stands; the dobby schedules nothing until it ends", "worktree", d.geom.WorktreeName, "record", moving)
	} else {
		d.moveStands.Recover("dobbyengine: the anchor change ended; the dobby schedules again", "worktree", d.geom.WorktreeName)
	}
	if moving == "" {
		if err := d.drivePasses(ctx, now); err != nil {
			return err
		}
		if err := d.startReady(ctx, now); err != nil {
			return err
		}
	}
	if err := d.countLandings(now); err != nil {
		return err
	}
	if moving == "" {
		if err := d.tidyStep(ctx, now); err != nil {
			return err
		}
	}
	d.inFlight = d.busy
	if d.deps.Tick != nil {
		hookInFlight, err := d.deps.Tick(ctx, TickInput{Now: now, ProbeDue: d.probeDue, OwesAll: d.owesAll})
		d.inFlight = d.inFlight || hookInFlight
		if err != nil {
			return err
		}
	}
	if moving == "" {
		d.owesAll = false
	}
	return nil
}

// sleep waits for the next tick: wait, or a watch event, whichever comes first, and reports whether an event woke it.
// Events are coalesced to at most one tick a second: an event arriving sooner than that after the last tick waits out the rest of the second, and every event that came meanwhile is dropped.
func (d *daemon) sleep(ctx context.Context, wait time.Duration, events <-chan struct{}) bool {
	woke := false
	select {
	case <-events:
		woke = true
	default:
		timerCtx, stopTimer := context.WithCancel(ctx)
		timerDone := make(chan struct{})
		go func() {
			defer close(timerDone)
			d.deps.Sleep(timerCtx, wait)
		}()
		select {
		case <-events:
			woke = true
		case <-timerDone:
		}
		stopTimer()
		<-timerDone
	}
	if woke {
		if rest := wakeFloor - d.deps.Now().Sub(d.lastTick); rest > 0 {
			d.deps.Sleep(ctx, rest)
		}
		for drained := false; !drained; {
			select {
			case <-events:
			default:
				drained = true
			}
		}
	}
	return woke
}

// nextWait returns the timer's next wait given the current one.
// It doubles the current wait up to a minute while no run is in flight and no event came, and returns the zero duration, meaning the probe interval, on any event or while a run is in flight.
func nextWait(current time.Duration, inFlight, event bool) time.Duration {
	if inFlight || event {
		return 0
	}
	return min(current*2, idleCeiling)
}
