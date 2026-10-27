// loop.go implements dobby's daemon loop: the lock that keeps one dobby per beam worktree, the owner exit, the sleep watch, the coalesced wakeups and the binary check between ticks.

package dobbyengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// wakeFloor is the shortest gap between two wakes of the loop, ticks coalescing events and timers alike to it.
	wakeFloor = time.Second
	// idleCeiling is the longest the timer waits while no run is in flight.
	idleCeiling = time.Minute
	// heartbeatInterval is the gap after which the next tick writes a heartbeat line, so a dead dobby shows in its log.
	heartbeatInterval = 10 * time.Minute
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
	// CheckBinary re-execs onto a changed lym binary and returns when the binary is unchanged.
	// It calls beforeExec right before an exec and failed with the error of an exec that failed.
	CheckBinary func(beforeExec func(), failed func(error))
	// StartTicks returns this process's start time as `proc.StartTime` reads it, empty when unreadable; nil reads it for the process's own pid.
	StartTicks func() string
	// PrimeTidyShuttle stops the board-tidy agent of a beam dobby whose worktree is gone, over the prime's location; nil stops nothing.
	PrimeTidyShuttle TidyShuttle
	// ReexecLockWait is how long a re-exec waits for the dobby lock; zero means ten seconds.
	ReexecLockWait time.Duration
	// Locked runs once the lock is held, before the state record is written; nil does nothing.
	// An error from it ends the dobby.
	Locked func() error
	// ProbeInterval is batten's `notice_probe_s`; a value below one second is floored to it with one Warn.
	ProbeInterval time.Duration
	// Tick is an extra per-tick hook that runs after the scheduling steps: it reports whether anything it drives is in flight for the loop's backoff.
	// A nil Tick does nothing.
	Tick func(ctx context.Context, in TickInput) (inFlight bool, err error)
	// Runs lists the batten runs whose parent worktree is this dobby's own, read from the prime's local batten status files and never from the board.
	// A run's parent is the one its pair's origin record names, the prime for none, so a run whose task Land already pruned from the board is still listed.
	Runs func() ([]RunView, error)
	// SpawnPass starts `<lym> batten run <slug>` with the hidden continue and no-reopen flags as asked, in its own process group with the prime's anchor as its working directory, logged at Info.
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
	// AdoptSubject returns the beam or `root` that `lym adopt` takes to move slug on the local Bolt; nil, or an error, names the slug.
	AdoptSubject func(slug string) (string, error)
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
	// Landings returns the slugs of the runs whose parent worktree is this dobby's own and whose batten history holds a Land Done naming this machine, whatever the run's state.
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
	// CanLand reads whether the beam of this dobby can land, and who owns its parent.
	// A nil CanLand posts no "can land" notice; the prime's dobby posts none whatever it returns.
	CanLand func() (LandReading, error)
	// Log writes one line to the dobby's log file at path; rotate is set for the dobby's own lines while it holds its lock.
	// A nil Log logs nothing.
	Log func(path, line string, rotate bool)
}

// reexecLockWait is how long a re-exec waits for the dobby lock, which the process it replaced releases as it execs.
const reexecLockWait = 10 * time.Second

// stoppedWayForward ends the log line and the notice of a dobby that stopped on an error.
const stoppedWayForward = "; way forward: fix the cause, then run lym beam start"

// ReadsAsReexec reports whether the state record at statePath names this process as the dobby that last recorded itself: it holds pid, and holds startTicks or no start ticks.
// A directory that does not exist, or a record that is absent, reads as false, and nothing is created.
// A record without start ticks was written before they existed, so the first re-exec onto a binary that writes them reads as true.
// A fresh dobby that reused such a record's pid after a reboot reads as a re-exec as well.
func ReadsAsReexec(statePath string, pid int, startTicks string) (bool, error) {
	if _, err := os.Stat(filepath.Dir(statePath)); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	record, found, err := ReadState(statePath)
	if err != nil {
		return false, err
	}
	return found && record.PID == pid && (record.StartTicks == "" || record.StartTicks == startTicks), nil
}

// Run drives one dobby for geom until ctx ends or another machine owns the beam.
// A start that reads as a re-exec of this process waits Deps.ReexecLockWait for geom.LockPath, any other start lock.ProbeGrace, and Run returns ErrDobbyRunning when another dobby still holds it.
// It reads the beam's owner at start and on every probe tick, and returns nil, logging why, when another machine owns it.
// A start that does not read as a re-exec owes every run a full pass; a re-exec owes none.
// A tick that fails is logged once per episode and the loop goes on; a cancelled context ends it after the tick in flight.
// Every way the dobby ends leaves a line in the log, and an error after a re-exec posts a notice.
func Run(ctx context.Context, geom Geometry, deps Deps) error {
	d := &daemon{
		geom: geom, deps: deps, probe: deps.ProbeInterval,
		flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{},
	}
	defer d.releaseLock()
	if d.probe < wakeFloor {
		logger.Warn("dobbyengine: the probe interval is below the floor and is floored", "key", "notice_probe_s", "floor", wakeFloor, "configured", d.probe)
		d.probe = wakeFloor
	}
	d.startTicks = deps.ownStartTicks()
	reexec, err := ReadsAsReexec(geom.StatePath, os.Getpid(), d.startTicks)
	if err != nil {
		d.logOwn(deps.Now(), "dobby: stopped on error: "+err.Error()+stoppedWayForward)
		return err
	}
	err = d.run(ctx, reexec)
	if err != nil && !errors.Is(err, ErrDobbyRunning) {
		d.logOwn(deps.Now(), "dobby: stopped on error: "+err.Error()+stoppedWayForward)
		if reexec {
			d.post(ctx, fmt.Sprintf("%s: the dobby stopped on an error after a re-exec: %v%s", geom.WorktreeName, err, stoppedWayForward))
		}
	}
	return err
}

// ownStartTicks returns this process's start ticks through StartTicks, or from the process table when it is nil.
func (deps Deps) ownStartTicks() string {
	if deps.StartTicks != nil {
		return deps.StartTicks()
	}
	ticks, _ := proc.StartTime(os.Getpid())
	return ticks
}

// run takes the lock, records the start and drives the loop; reexec tells whether the start reads as a re-exec.
func (d *daemon) run(ctx context.Context, reexec bool) error {
	wait := lock.ProbeGrace
	if reexec {
		wait = d.deps.ReexecLockWait
		if wait == 0 {
			wait = reexecLockWait
		}
	}
	if err := os.MkdirAll(filepath.Dir(d.geom.LockPath), 0o755); err != nil {
		return err
	}
	held, acquired, err := lock.AcquireWriteLockWithin(d.geom.LockPath, wait)
	if err != nil {
		return err
	}
	if !acquired {
		d.logOwn(d.deps.Now(), "dobby: start refused: another dobby holds the lock")
		if reexec {
			d.postRefusedReexec(ctx)
		}
		return ErrDobbyRunning
	}
	d.held = held

	if d.deps.Locked != nil {
		if err := d.deps.Locked(); err != nil {
			return err
		}
	}
	if err := d.recordStart(); err != nil {
		return err
	}
	if reexec {
		d.logOwn(d.deps.Now(), fmt.Sprintf("start re-exec pid %d", os.Getpid()))
	} else {
		d.logOwn(d.deps.Now(), fmt.Sprintf("start fresh pid %d", os.Getpid()))
	}
	d.heartbeat(d.deps.Now(), nil)

	var events <-chan struct{}
	if d.deps.Watch != nil {
		watched, closeWatch, err := d.deps.Watch()
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
	// startTicks is this process's start time, written beside every pid the dobby records.
	startTicks string
	// held is the dobby lock once taken, nil before.
	held *lock.FileLock
	// keptTidy is the tidy agent in flight the state record last showed, which a beam dobby whose worktree is gone stops.
	keptTidy  *TidyState
	stateRead logger.Episode
	// loggedRuns holds the label last logged for each listed run, by slug, in memory only.
	loggedRuns map[string]string
	// primeOwed reports that a heartbeat could not read the runs before the labels were recorded, so the next run listing records them without logging a change.
	primeOwed bool
	// lastHeartbeat is when the last heartbeat line was written.
	lastHeartbeat time.Time
	// tickFailLogged and ownerUnreadable report whether the standing failure episode of the ticks or the owner read has logged its line.
	tickFailLogged  bool
	ownerUnreadable bool

	sleepWatch pauserecord.SleepWatch
	owesAll    bool
	inFlight   bool
	probeDue   bool
	lastProbe  time.Time
	lastTick   time.Time
	failures   logger.Episode
	ownerRead  logger.Episode
	moveStands logger.Episode
	landRead   logger.Episode
	landPost   logger.Episode

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

// releaseLock releases the dobby lock when it is held.
func (d *daemon) releaseLock() {
	if d.held != nil {
		_ = d.held.Release()
		d.held = nil
	}
}

// logOwn writes one line to the dobby's log, stamped with at, rotating the file only while this dobby holds its lock.
func (d *daemon) logOwn(at time.Time, text string) {
	if d.deps.Log != nil {
		d.deps.Log(d.geom.LogPath, at.UTC().Format(time.RFC3339)+" "+text, d.held != nil)
	}
}

// post sends one line through the Notice seam; a failed post is logged at Warn.
func (d *daemon) post(ctx context.Context, line string) {
	if d.deps.Notice == nil {
		return
	}
	if err := d.deps.Notice(ctx, line); err != nil {
		logger.Warn("dobbyengine: a dobby notice was not posted", "worktree", d.geom.WorktreeName, "error", err)
	}
}

// postRefusedReexec posts the notice for a re-exec that could not take its lock, unless the state record no longer holds this pid, which means another dobby runs.
func (d *daemon) postRefusedReexec(ctx context.Context) {
	record, found, err := ReadState(d.geom.StatePath)
	if err != nil {
		logger.Warn("dobbyengine: the state record is unreadable after a refused re-exec; no notice is posted", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	if found && record.PID == os.Getpid() {
		d.post(ctx, d.geom.WorktreeName+": the dobby could not take its lock after a re-exec; way forward: run lym beam status, and once it shows the dobby not alive, run lym beam start")
	}
}

// recordStart reads the state record under the held lock and records this process as the dobby.
// A record with this pid and these start ticks owes nothing; one with this pid and no ticks owes a full pass and gains the ticks; any other start owes a full pass and rewrites the pid, the start time and the ticks.
func (d *daemon) recordStart() error {
	record, found, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	d.keptTidy = record.Tidy
	own := found && record.PID == os.Getpid()
	if own && record.StartTicks == d.startTicks {
		return nil
	}
	d.owesAll = true
	if own && record.StartTicks == "" {
		return UpdateState(d.geom.StatePath, func(s *State) error {
			s.StartTicks = d.startTicks
			return nil
		})
	}
	now := d.deps.Now()
	return UpdateState(d.geom.StatePath, func(s *State) error {
		s.PID, s.StartTime, s.StartTicks = os.Getpid(), now.UTC().Format(time.RFC3339), d.startTicks
		return nil
	})
}

// worktreeGone reports whether this is a beam dobby whose worktree's git link no longer exists, which a teardown of the beam removes with its worktree.
// The worktree's root is no mark, because every state write recreates the record's directory; the dobby's own writes never recreate the git link.
// The prime's dobby never reads as gone.
func (d *daemon) worktreeGone() bool {
	if d.geom.Slug == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(d.geom.WorktreeRoot, ".git"))
	return errors.Is(err, fs.ErrNotExist)
}

// readKeptTidy reads the state record's tidy in flight without creating a directory; the prime's dobby, which never takes the exit, reads nothing.
// A record that does not decode reads as absent with one Warn.
func (d *daemon) readKeptTidy() (*TidyState, error) {
	if d.geom.Slug == "" {
		return d.keptTidy, nil
	}
	record, _, err := state.ReadJSON[State](d.geom.StatePath, lockPath(d.geom.StatePath))
	if errors.Is(err, state.ErrDecode) {
		logger.Warn("dobbyengine: the state record does not decode and reads as absent", "path", d.geom.StatePath, "error", err)
		return nil, nil
	}
	return record.Tidy, err
}

// rememberKeptTidy keeps the tidy a read found, so a dobby whose worktree vanishes can stop the agent the last tick saw.
// A failed read is logged once per episode and keeps the earlier one.
func (d *daemon) rememberKeptTidy(kept *TidyState, readErr error) {
	if readErr != nil {
		d.stateRead.Fail("dobbyengine: the state record is unreadable; the dobby goes on", "worktree", d.geom.WorktreeName, "error", readErr)
		return
	}
	d.stateRead.Recover("dobbyengine: the state record reads again", "worktree", d.geom.WorktreeName)
	d.keptTidy = kept
}

// exitWorktreeGone leaves the prime's dobby log the reason a beam dobby ends and stops the tidy agent the dobby last saw in flight.
// It touches nothing at the removed worktree's path.
func (d *daemon) exitWorktreeGone() {
	logPrime := func(text string) {
		if d.deps.Log != nil {
			d.deps.Log(d.geom.PrimeLogPath, d.deps.Now().UTC().Format(time.RFC3339)+" "+text, false)
		}
	}
	logPrime(fmt.Sprintf("stop: the worktree of beam %s is gone", d.geom.Slug))
	if d.keptTidy == nil || d.deps.PrimeTidyShuttle == nil {
		return
	}
	guid := d.keptTidy.StrandGUID
	if err := d.deps.PrimeTidyShuttle.StopStrand(guid); err != nil {
		logPrime(fmt.Sprintf("the board-tidy agent %s of beam %s did not stop: %v; way forward: lym reed remove %s", guid, d.geom.Slug, err, guid))
		return
	}
	logPrime(fmt.Sprintf("stopped the board-tidy agent %s of beam %s", guid, d.geom.Slug))
}

// loop ticks and waits until ctx ends or another machine owns the beam.
func (d *daemon) loop(ctx context.Context, events <-chan struct{}) error {
	var wait time.Duration
	event := true
	for ctx.Err() == nil {
		if d.worktreeGone() {
			d.exitWorktreeGone()
			return nil
		}
		now := d.deps.Now()
		d.probeDue = d.lastProbe.IsZero() || now.Sub(d.lastProbe) >= d.probe
		if d.probeDue {
			d.lastProbe = now
			if elsewhere, owner := d.beamOwnedElsewhere(); elsewhere {
				logger.Info("dobbyengine: another machine owns this beam; the dobby exits", "worktree", d.geom.WorktreeName, "owner", owner)
				subject := d.geom.Slug
				if subject == "" {
					subject = "root"
				}
				d.logOwn(now, fmt.Sprintf("stop: another machine owns this beam (%s); lym adopt %s takes it over here", owner, subject))
				return nil
			}
		}

		if err := d.tick(ctx, now); err != nil {
			d.failures.Fail("dobbyengine: a tick failed; the loop goes on", "worktree", d.geom.WorktreeName, "error", err)
			if !d.tickFailLogged {
				d.tickFailLogged = true
				d.logLine(now, "tick failing: "+oneLine(err.Error()))
			}
		} else {
			d.failures.Recover("dobbyengine: ticks succeed again", "worktree", d.geom.WorktreeName)
			if d.tickFailLogged {
				d.tickFailLogged = false
				d.logLine(now, "ticks succeed again")
			}
		}
		d.lastTick = now
		if now.Sub(d.lastHeartbeat) >= heartbeatInterval {
			d.heartbeat(now, nil)
		}
		if ctx.Err() != nil {
			break
		}

		kept, readErr := d.readKeptTidy()
		if d.worktreeGone() {
			d.exitWorktreeGone()
			return nil
		}
		d.rememberKeptTidy(kept, readErr)

		d.deps.CheckBinary(
			func() { d.logOwn(d.deps.Now(), "re-exec onto the changed lym binary") },
			func(err error) {
				d.logOwn(d.deps.Now(), fmt.Sprintf("re-exec failed: %v; the dobby keeps running", err))
				d.post(ctx, fmt.Sprintf("%s: the dobby could not re-exec onto the changed lym binary: %v; it keeps running on the old binary; way forward: fix the binary, then run lym beam stop and lym beam start", d.geom.WorktreeName, err))
			},
		)

		wait = nextWait(wait, d.inFlight, event)
		if wait == 0 {
			wait = d.probe
		}
		wait = max(wait, d.probe, wakeFloor)
		event = d.sleep(ctx, wait, events)
	}
	d.logOwn(d.deps.Now(), "stop: the context ended")
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
			if d.ownerUnreadable {
				d.ownerUnreadable = false
				d.logLine(d.deps.Now(), "owner reads again")
			}
			return owner != "" && owner != machine, owner
		}
	}
	d.ownerRead.Fail("dobbyengine: the beam's owner is unreadable; the dobby goes on", "worktree", d.geom.WorktreeName, "error", err)
	if !d.ownerUnreadable {
		d.ownerUnreadable = true
		d.logLine(d.deps.Now(), "owner unreadable: "+oneLine(err.Error()))
	}
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

// tick folds one awake reading into the sleep watch, keeps the state record's pid and sleeps, and runs the per-tick work, ending with the beam's "can land" notice.
// A sleep the watch sees, and a record found without a pid, owe every run a full pass until a tick returns nil.
// While an anchor-move record of the geometry exists, a tick spawns no pass, starts no task and runs no tidy step, and owes what it owed before.
// That is logged once per episode, and the state record's upkeep and the landing count go on.
func (d *daemon) tick(ctx context.Context, now time.Time) error {
	d.sleepWatch.Observe(now, d.deps.Awake())
	seen := d.sleepWatch.Sleeps()
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		if s.PID == 0 {
			s.PID, s.StartTime, s.StartTicks = os.Getpid(), now.UTC().Format(time.RFC3339), d.startTicks
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
	if err := d.landNotice(ctx); err != nil {
		return err
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
