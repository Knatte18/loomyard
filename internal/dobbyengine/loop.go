// loop.go implements dobby's daemon loop, which holds the lock for one dobby per beam worktree.
// It also handles the owner exit, the sleep watch, the coalesced wakeups, and the binary check between ticks.

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

// ErrDobbyRunning is returned when another dobby holds this worktree's lock.
var ErrDobbyRunning = errors.New("dobby: another dobby holds this worktree's lock")

// TickInput carries what one tick tells the per-tick work about the loop's state.
type TickInput struct {
	// Now is the tick's wall time.
	Now time.Time
	// ProbeDue reports whether a probe interval has passed since the last probe tick, so this tick is a probe tick.
	ProbeDue bool
	// OwesAll reports whether every run the dobby drives is owed a full pass, and it stays set until a tick returns nil.
	// The dobby owes one after it started, saw a machine sleep, or found its state record without a pid.
	OwesAll bool
}

// Deps holds every seam the loop reads, as the hub wiring supplies them.
type Deps struct {
	// Now is the wall clock.
	Now func() time.Time
	// Awake is a clock that stops while the machine sleeps, `pauserecord.MonotonicAwake()` in production.
	Awake func() time.Duration
	// Sleep waits d, or returns early when ctx is cancelled.
	Sleep func(ctx context.Context, d time.Duration)
	// Watch opens the file watch over the prime's batten runs and status directories, `board.json`, and the pause records.
	// Its channel delivers one value per change; closeWatch releases it.
	// A nil Watch, or one that returns an error, leaves the timer alone to drive the loop.
	Watch func() (events <-chan struct{}, closeWatch func(), err error)
	// Owner returns the machine recorded as this beam's owner, the prime owner for the prime, empty when none is recorded.
	// It also returns whether that owner reads as a machine other than this one, which the hub wiring judges.
	Owner func() (owner string, elsewhere bool, err error)
	// UnderAdopt reports whether a hold this machine wrote in Bolt covers the subject of this dobby's worktree.
	// The hold stands until the adopt's check is green.
	// It is asked only on a probe tick that reads the beam owned by another machine; a nil UnderAdopt covers nothing.
	UnderAdopt func() bool
	// PullBolt pulls the hub's Bolt from origin, so an idle machine learns a new owner; a nil PullBolt pulls nothing.
	PullBolt func() error
	// ArchiveOwner archives every local tip under this dobby's worktree that origin lacks, for the machine that owns it now:
	// the code and records tips of the worktree's own pair and of each run whose parent it is.
	// A nil ArchiveOwner archives nothing.
	ArchiveOwner func() error
	// CatchUp is the seams of the catch-up from origin the dobby runs after every absence it sees.
	// A CatchUp whose PullBolt is nil means no catch-up, as a nil PullBolt pulls nothing.
	// Its Launcher also names the hub's desktop launcher the dobby's start adds.
	CatchUp CatchUpDeps
	// SyncBoltHold brings the bolt-hold entries of the runs in scope in line with the hold, held being the slugs it holds.
	// A nil SyncBoltHold syncs nothing.
	SyncBoltHold func(ctx context.Context, scope, held []string) error
	// EndSubject ends the processes this machine runs for the beam another machine now owns.
	// It runs once the catch-up handed the beam's runs over.
	// A nil EndSubject ends nothing.
	EndSubject func() error
	// CheckBinary re-execs onto a changed lym binary and returns when the binary is unchanged.
	// It calls beforeExec right before an exec and failed with the error of an exec that failed.
	CheckBinary func(beforeExec func(), failed func(error))
	// StartTicks returns the start time `proc.StartTime` reads, empty when unreadable; nil reads this process's pid.
	StartTicks func() string
	// PrimeTidyShuttle stops the board-tidy and describer agents of a beam with no worktree, at the prime's location;
	// nil stops nothing.
	PrimeTidyShuttle AgentShuttle
	// ReexecLockWait is how long a re-exec waits for the dobby lock; zero means ten seconds.
	ReexecLockWait time.Duration
	// Locked runs once the lock is held, before the state record is written; nil does nothing.
	// An error from it ends the dobby.
	Locked func() error
	// ProbeInterval is batten's `notice_probe_s`; a value below one second is floored to it with one Warn.
	ProbeInterval time.Duration
	// Tick is an extra per-tick hook that runs after the scheduling steps.
	// It reports whether anything it drives is in flight for the loop's backoff; a nil Tick does nothing.
	Tick func(ctx context.Context, in TickInput) (inFlight bool, err error)
	// Runs lists the batten runs whose parent is this dobby's worktree, read from the prime's status files, not the board.
	// A run's parent is named by its pair's origin record, the prime for none; a run whose task Land pruned stays listed.
	Runs func() ([]RunView, error)
	// ClearDoneRun removes the records, scratch and task branch of slug's run from the prime.
	// It does so when the run's batten status reads done.
	// It reports false with no error when the run's lock is held or the run no longer reads done.
	// A nil ClearDoneRun clears nothing.
	ClearDoneRun func(slug string) (cleared bool, err error)
	// SpawnPass starts `<lym> batten run <slug>` with the hidden continue and no-reopen flags as asked.
	// It runs in its own process group with the prime's anchor as its working directory, and the spawn is logged at Info.
	// The pass registers itself in `internal/runproc`, so dobby registers nothing; the channel delivers the exit once.
	SpawnPass func(slug string, continuePass, noReopen bool) (<-chan PassExit, error)
	// TearDownLeftovers starts the teardown of the leftover pairs, each by the kind that decides how it goes.
	// The channel delivers once, mapping each slug to nil when its pair was removed and to the reason otherwise.
	// A nil TearDownLeftovers tears nothing down.
	TearDownLeftovers func(pairs []LeftoverPair) <-chan map[string]error
	// Orphans lists the hub's orphan pairs, the pairs neither a board entry nor a batten record names.
	// The hub wiring gives it to the prime's dobby only; a nil Orphans or an empty OrphanRecord notices no orphan.
	Orphans func() ([]OrphanView, error)
	// OrphanRecord is the path of the machine-local first-sight record of the orphan pairs.
	OrphanRecord string
	// Notice posts one line to the nearest live orch.
	Notice func(ctx context.Context, line string) error
	// ReleaseParentLock releases the parent lock record that names the run's child, which a run another machine owns needs.
	ReleaseParentLock func(ctx context.Context, slug string) error
	// SlugOwner returns the effective owner of slug on the local Bolt, with no pull; empty means no other machine owns it.
	SlugOwner func(slug string) (string, error)
	// AdoptSubject returns the beam or `root` that `lym adopt` moves slug to; nil or an error names the slug.
	AdoptSubject func(slug string) (string, error)
	// Tasks reads the local board as TaskViews, each with its parent worktree (`hubgeom.ParentOf`) and effective owner.
	Tasks func() ([]TaskView, error)
	// LoadConfig loads beam.yaml over the dobby's worktree layer stack.
	LoadConfig func() (Config, error)
	// TaskLimit reads the cgroup task limit the dobby sits under, `proc.ReadTaskLimit` in production;
	// found is false where none reads; a nil TaskLimit holds no start.
	TaskLimit func() (limit proc.TaskLimit, found bool)
	// StatusExists reports whether a first pass left batten's status file for slug.
	StatusExists func(slug string) (bool, error)
	// PauseCovers reports whether a live pause entry covers the hub or slug.
	PauseCovers func(slug string) (bool, error)
	// Landings returns the slugs of this dobby's runs that hold a Land Done naming this machine, whatever their state.
	// A nil Landings, or a nil TidyShuttle, leaves the board tidy off.
	Landings func() ([]string, error)
	// TidyShuttle starts and stops the board-tidy agent.
	TidyShuttle AgentShuttle
	// TidyModel is the model the board-tidy agent runs on, resolved by the hub wiring.
	TidyModel shuttleengine.ModelChoice
	// UpkeepCall renders the gate call of the upkeep checks the board-tidy agent runs before its report.
	// The hub wiring sets it on the prime's dobby alone, whose worktree is on the main branch.
	// Nil, and an empty call with no error, run no upkeep.
	UpkeepCall func() (string, error)
	// StencilsDir is the hub's stencils directory, where the board-tidy stencil is read at call time.
	StencilsDir string
	// NearestOrch returns the name of the nearest live orch the tidy agent's parent directive names; empty means none.
	NearestOrch func() (string, error)
	// RunNotices are the seams the run notices of the runs this dobby drives read; a nil RunNotices.ReadChild sends none.
	RunNotices RunNoticeDeps
	// RunWatch are the seams of the run watch; a nil RunWatch.Level or RunWatch.Post watches nothing.
	// The child it reads comes through RunNotices.ReadChild.
	RunWatch RunWatchDeps
	// GateHolds are the seams of the gate-hold notice; a nil GateHolds.Holders or GateHolds.Post checks nothing.
	GateHolds GateHoldDeps
	// WaitReminders are the seams the reminders of a child waiting on the operator read; nil WaitReminders.Post sends none.
	WaitReminders WaitReminderDeps
	// CanLand reads whether the beam of this dobby is flagged ready with nothing working under it, and who owns its parent.
	// A nil CanLand takes no landing step; the prime's dobby takes none whatever it returns.
	CanLand func() (LandReading, error)
	// Land asks the beam's pawl run to land the beam by writing its land request.
	// It returns the refusal that kept it from writing one; it is set whenever CanLand is.
	Land func() error
	// BeamReady reads whether the beam of this dobby is flagged ready to land, from the beam's ready record.
	// While it reports true the dobby starts no task and notices each open member once; an error changes nothing.
	// A nil BeamReady, as on the prime, holds nothing.
	BeamReady func() (bool, error)
	// PullRequestRecorded reads whether the beam's landing record holds a pull request.
	// While it does, a member landing gets the beam's description rewritten.
	// A nil PullRequestRecorded, as on the prime, reads none.
	PullRequestRecorded func() (bool, error)
	// BeamDescribe are the seams of the beam's description rewrite after a member landing; a nil Record rewrites nothing.
	BeamDescribe BeamDescribeDeps
	// LandNotice reports whether land_notice is on for the run slug.
	// It reads through the per-task lookup from the run's worktree up.
	// A nil LandNotice reads every run as off.
	LandNotice func(slug string) (bool, error)
	// SettleFronts settles the front tasks of this dobby's worktree, running the land action of each one that landed.
	// It runs each tick before the start step, so an unrecorded front entry is recorded before it starts; nil does nothing.
	SettleFronts func() error
	// Log writes one line to the dobby's log file at path; rotate is set for the dobby's own lines while it holds its lock.
	// A nil Log logs nothing.
	Log func(path, line string, rotate bool)
}

// reexecLockWait is how long a re-exec waits for the dobby lock, which the process it replaced releases as it execs.
const reexecLockWait = 10 * time.Second

// stoppedWayForward ends the log line and the notice of a dobby that stopped on an error.
const stoppedWayForward = "; way forward: fix the cause, then run lym beam start"

// ReadsAsReexec reports whether the state record at statePath names pid as the dobby that last recorded itself.
// The record matches when its start ticks equal startTicks, or it holds none.
// A record without start ticks was written before they existed;
// the first start onto a binary that writes them reads as a re-exec.
// A fresh dobby that reused such a record's pid after a reboot also reads as a re-exec.
// A directory that does not exist, or a record that is absent, reads as false, and nothing is created.
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
// A start that reads as a re-exec waits Deps.ReexecLockWait for geom.LockPath; any other start waits lock.ProbeGrace.
// Run returns ErrDobbyRunning when another dobby still holds the lock.
// It reads the beam's owner at start and on every probe tick, after the probe's Bolt pull.
// While an adopt in flight on this machine covers a beam another machine owns, the dobby stays up and schedules nothing.
// Otherwise it hands such a beam over, and returns nil, logging why, once the hand-over's archive succeeds.
// A start that does not read as a re-exec owes every run a full pass; a re-exec owes none.
// Every start, a re-exec's included, adds the hub's desktop launcher when this machine owns the beam.
// A tick that fails is logged once per episode and the loop goes on; a cancelled context ends it after the tick in flight.
// Every way the dobby ends leaves a line in the log,
// and an error after a re-exec posts a notice.
func Run(ctx context.Context, geom Geometry, deps Deps) error {
	d := &daemon{
		geom: geom, deps: deps, probe: deps.ProbeInterval,
		flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{},
		handedOver: map[string]bool{},
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
	d.keptNoticeOwed = !reexec
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
	d.addLauncher()
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

// daemon is one running dobby: its geometry and seams, and what the loop remembers between ticks.
type daemon struct {
	geom Geometry
	deps Deps
	// probe is the floored probe interval.
	probe time.Duration
	// startTicks is this process's start time, written beside every pid the dobby records.
	startTicks string
	// held is the dobby lock once taken, nil before.
	held *lock.FileLock
	// keptTidy and keptDescribe are the tidy and describer agents in flight the state record last showed.
	// A beam dobby whose worktree is gone stops them.
	keptTidy     *TidyState
	keptDescribe *DescribeState
	stateRead    logger.Episode
	// loggedRuns holds the label last logged for each listed run, by slug, in memory only.
	loggedRuns map[string]string
	// primeOwed reports that a heartbeat failed to read the runs, so the next listing records labels silently.
	primeOwed bool
	// lastHeartbeat is when the last heartbeat line was written.
	lastHeartbeat time.Time
	// tickFailLogged and ownerUnreadable report whether the standing failure episode of the ticks or the owner read has logged its line.
	tickFailLogged  bool
	ownerUnreadable bool
	// taskLimitHeld reports whether the cgroup task limit held the last start the start step tried.
	taskLimitHeld bool
	// adoptHolds reports whether the last probe tick read the beam owned elsewhere under an adopt in flight here.
	adoptHolds bool
	// lastBoltPull is when the probe last pulled Bolt, and boltPull is the failure episode of those pulls.
	lastBoltPull time.Time
	boltPull     logger.Episode
	// archiveFailures is the failure episode of the hand-over's archive; archiveFailing reports whether its line was logged.
	archiveFailures logger.Episode
	archiveFailing  bool
	// catchUpOwed reports whether the catch-up from origin is owed, which holds every scheduling step until it succeeds.
	catchUpOwed bool
	// catchUpNext is the earliest time of the next try of an owed catch-up, and catchUpDelay the delay that try followed.
	catchUpNext  time.Time
	catchUpDelay time.Duration
	// catchUpReason is the failure class whose line the log last carried, empty when the catch-up stands clear.
	catchUpReason string
	// catchUpNoticed holds the failure classes whose notice went out this episode.
	catchUpNoticed map[string]bool
	// handedOver holds the slugs whose hand-over verdict already owed the catch-up, until they stop answering hand over.
	handedOver map[string]bool
	// syncFailures is the failure episode of the bolt-hold sync; syncFailing reports whether its line was logged.
	syncFailures logger.Episode
	syncFailing  bool

	sleepWatch pauserecord.SleepWatch
	owesAll    bool
	inFlight   bool
	probeDue   bool
	lastProbe  time.Time
	lastTick   time.Time
	failures   logger.Episode
	ownerRead  logger.Episode
	moveStands logger.Episode
	// frontFailures is the failure episode of the front settle step; frontNoticed reports whether its notice went out.
	frontFailures logger.Episode
	frontNoticed  bool
	landRead      logger.Episode
	landPost      logger.Episode
	// readyRead and readyPost are the failure episodes of the ready record's read and of the member notices a beam owes.
	readyRead logger.Episode
	readyPost logger.Episode
	// describePullRequest is the failure episode of the description rewrite's read of the beam's landing record.
	describePullRequest logger.Episode
	// describeFailures is the failure episode of the description step, whose error never holds the steps after it.
	describeFailures logger.Episode
	// landNoticeLookup guards each run's land_notice lookup by its slug.
	landNoticeLookup logger.EpisodeSet
	// landNoticePosts guards each owed landing notice's post by its slug.
	landNoticePosts logger.EpisodeSet
	// watchRecordFailed reports that the tick's run watch step could not read or write the state record.
	watchRecordFailed bool
	// watchLevelReads guards each run's beam.run_watch lookup by its slug, and watchPosts each owed watch line's post.
	watchLevelReads logger.EpisodeSet
	watchPosts      logger.EpisodeSet
	// gateHoldReads guards each read of the gate-hold check, and gateHoldPosts each run's notice and its record.
	gateHoldReads logger.EpisodeSet
	gateHoldPosts logger.EpisodeSet
	// listedRuns is the run listing of the tick's pass step, done runs included.
	listedRuns []RunView
	// landed is the slugs whose landing the tick's landing count read, nil while Deps.Landings is nil.
	landed []string
	// doneRunClears guards each failing clear of a done run by its slug.
	doneRunClears logger.EpisodeSet

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
	// runs is the listing of the tick in flight.
	runs []RunView
	// deliveries holds the attempts to post each run notice not yet posted, by slug and episode key, in memory only.
	deliveries map[string]*noticeDelivery
	// noticeFailures guards each run notice read by its slug and seam: Warn at the first failure, Debug while it repeats, Info at recovery.
	noticeFailures logger.EpisodeSet
	// lastWaitCheck is when the waits were last checked.
	lastWaitCheck time.Time
	// lastReminders holds when each wait was last reminded of, by slug, in memory only.
	lastReminders map[string]time.Time
	// waitFailures guards the wait reads by slug or `|`-prefixed name: Warn first, Debug while it repeats, Info on recovery.
	waitFailures logger.EpisodeSet
	// teardown is the teardown of leftover pairs in flight, nil when none is.
	teardown *leftoverTeardown
	// leftoverRetries holds, by slug, when each leftover pair's teardown may start next, in memory only.
	leftoverRetries map[string]leftoverRetry
	// keptNoticeOwed reports whether this fresh start still owes its one notice of the kept pairs.
	keptNoticeOwed bool
	// orphansRead reports whether a probe tick of this process read the orphan pairs, and orphansDue the due ones it found.
	orphansRead bool
	orphansDue  []string
	// orphanFailures is the failure episode of the orphan read and of its first-sight record.
	orphanFailures logger.Episode
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

// postRefusedReexec posts the notice for a re-exec that could not take its lock.
// It posts nothing when the state record no longer holds this pid, which means another dobby runs.
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
// A record with this pid and these start ticks owes nothing.
// One with this pid and no ticks owes a full pass and gains the ticks.
// Any other start owes the catch-up from origin and a full pass, and rewrites the pid, the start time and the ticks.
// A re-exec follows no absence, so it owes no catch-up.
func (d *daemon) recordStart() error {
	record, found, err := ReadState(d.geom.StatePath)
	if err != nil {
		return err
	}
	d.keptTidy, d.keptDescribe = record.Tidy, record.Describe
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
	d.oweCatchUp()
	now := d.deps.Now()
	return UpdateState(d.geom.StatePath, func(s *State) error {
		s.PID, s.StartTime, s.StartTicks = os.Getpid(), now.UTC().Format(time.RFC3339), d.startTicks
		return nil
	})
}

// worktreeGone reports whether this is a beam dobby whose worktree's git link no longer exists.
// A teardown of the beam removes that link with its worktree.
// The worktree's root is no mark, because every state write recreates the record's directory;
// the dobby's own writes never recreate the git link.
// The prime's dobby never reads as gone.
func (d *daemon) worktreeGone() bool {
	if d.geom.Slug == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(d.geom.WorktreeRoot, ".git"))
	return errors.Is(err, fs.ErrNotExist)
}

// readKeptAgents reads the state record's tidy and describer agents in flight without creating a directory.
// The prime's dobby, which never takes the exit, reads nothing.
// A record that does not decode reads as absent with one Warn.
func (d *daemon) readKeptAgents() (*TidyState, *DescribeState, error) {
	if d.geom.Slug == "" {
		return d.keptTidy, d.keptDescribe, nil
	}
	record, _, err := state.ReadJSON[State](d.geom.StatePath, lockPath(d.geom.StatePath))
	if errors.Is(err, state.ErrDecode) {
		logger.Warn("dobbyengine: the state record does not decode and reads as absent", "path", d.geom.StatePath, "error", err)
		return nil, nil, nil
	}
	return record.Tidy, record.Describe, err
}

// rememberKeptAgents keeps the agents a read found, so a dobby whose worktree vanishes can stop those the last tick saw.
// A failed read is logged once per episode and keeps the earlier ones.
func (d *daemon) rememberKeptAgents(tidy *TidyState, describe *DescribeState, readErr error) {
	if readErr != nil {
		d.stateRead.Fail("dobbyengine: the state record is unreadable; the dobby goes on", "worktree", d.geom.WorktreeName, "error", readErr)
		return
	}
	d.stateRead.Recover("dobbyengine: the state record reads again", "worktree", d.geom.WorktreeName)
	d.keptTidy, d.keptDescribe = tidy, describe
}

// exitWorktreeGone writes the reason a beam dobby ends to the prime's dobby log.
// It also stops the tidy and describer agents the dobby last saw in flight.
// It touches nothing at the removed worktree's path.
func (d *daemon) exitWorktreeGone() {
	logPrime := func(text string) {
		if d.deps.Log != nil {
			d.deps.Log(d.geom.PrimeLogPath, d.deps.Now().UTC().Format(time.RFC3339)+" "+text, false)
		}
	}
	logPrime(fmt.Sprintf("stop: the worktree of beam %s is gone", d.geom.Slug))
	if d.deps.PrimeTidyShuttle == nil {
		return
	}
	stop := func(kind, guid string) {
		if err := d.deps.PrimeTidyShuttle.StopStrand(guid); err != nil {
			logPrime(fmt.Sprintf("the %s agent %s of beam %s did not stop: %v; way forward: lym reed remove %s", kind, guid, d.geom.Slug, err, guid))
			return
		}
		logPrime(fmt.Sprintf("stopped the %s agent %s of beam %s", kind, guid, d.geom.Slug))
	}
	if d.keptTidy != nil {
		stop("board-tidy", d.keptTidy.StrandGUID)
	}
	if d.keptDescribe != nil {
		stop("describer", d.keptDescribe.StrandGUID)
	}
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
			d.pullBolt(now)
			if d.probeOwner(ctx, now) {
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

		keptTidy, keptDescribe, readErr := d.readKeptAgents()
		if d.worktreeGone() {
			d.exitWorktreeGone()
			return nil
		}
		d.rememberKeptAgents(keptTidy, keptDescribe, readErr)

		// A re-exec would cut a teardown in flight short, half done, so the check waits for it to end.
		if d.teardown == nil {
			d.deps.CheckBinary(
				func() { d.logOwn(d.deps.Now(), "re-exec onto the changed lym binary") },
				func(err error) {
					d.logOwn(d.deps.Now(), fmt.Sprintf("re-exec failed: %v; the dobby keeps running", err))
					d.post(ctx, fmt.Sprintf("%s: the dobby could not re-exec onto the changed lym binary: %v; it keeps running on the old binary; way forward: fix the binary, then run lym beam stop and lym beam start", d.geom.WorktreeName, err))
				},
			)
		}

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
	owner, elsewhere, err := d.deps.Owner()
	if err == nil {
		d.ownerRead.Recover("dobbyengine: the beam's owner reads again", "worktree", d.geom.WorktreeName)
		if d.ownerUnreadable {
			d.ownerUnreadable = false
			d.logLine(d.deps.Now(), "owner reads again")
		}
		return elsewhere, owner
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

// tick folds one awake reading into the sleep watch and runs the per-tick work, ending with the beam's landing step.
// It records the sleeps the watch saw and fills in the state record's pid when none is set.
// A sleep the watch sees, and a record found without a pid, owe every run a full pass until a tick returns nil.
// An anchor-move record of the geometry holds the scheduling.
// While one exists, a tick spawns no pass, starts no task and runs no tidy or description step.
// It still owes what it owed before.
// That is logged once per episode,
// and the state record's upkeep and the landing count go on.
// The adopt hold holds the scheduling the same way, and the beam's landing step too.
// An error of the description step is logged once per episode and holds neither the tidy step nor the landing step.
// A sleep the watch sees owes the catch-up from origin, which runs first, before any scheduling.
// While it stands owed the tick holds the scheduling and the landing step as under an anchor change.
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
				d.oweCatchUp()
			}
		}
		cutoff := now.Add(-pauserecord.Retention)
		s.Sleeps = slices.DeleteFunc(s.Sleeps, func(sleep pauserecord.Interval) bool { return sleep.End.Before(cutoff) })
		return nil
	})
	if err != nil {
		return err
	}
	// An adopt hold hands nothing over, so the catch-up waits for it to lift.
	catchingUp := !d.adoptHolds && d.catchUp(ctx, now)
	moving := d.standingMove()
	if moving != "" {
		d.moveStands.Fail("dobbyengine: an anchor change stands; the dobby schedules nothing until it ends", "worktree", d.geom.WorktreeName, "record", moving)
	} else {
		d.moveStands.Recover("dobbyengine: the anchor change ended; the dobby schedules again", "worktree", d.geom.WorktreeName)
	}
	scheduling := moving == "" && !d.adoptHolds && !catchingUp
	if scheduling {
		if err := d.drivePasses(ctx, now); err != nil {
			return err
		}
		if err := d.tearDownLeftovers(ctx, now); err != nil {
			return err
		}
		d.noticeKeptPairs(ctx)
		d.runNotices(ctx, now)
		d.waitReminders(ctx, now)
		d.landNotices(ctx, now, d.listedRuns)
		d.runWatch(ctx, now)
		d.gateHolds(ctx, now)
		d.settleFronts(ctx)
		if err := d.startReady(ctx, now); err != nil {
			return err
		}
	}
	if err := d.countLandings(now); err != nil {
		return err
	}
	if scheduling {
		d.clearDoneRuns(now)
		if err := d.describeStep(ctx, now); err != nil {
			d.describeFailures.Fail("dobbyengine: the description step failed; the loop goes on", "worktree", d.geom.WorktreeName, "error", err)
		} else {
			d.describeFailures.Recover("dobbyengine: the description step succeeds again", "worktree", d.geom.WorktreeName)
		}
		if err := d.tidyStep(ctx, now); err != nil {
			return err
		}
	}
	if !d.adoptHolds && !catchingUp {
		if err := d.landBeam(ctx); err != nil {
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
	if scheduling {
		d.owesAll = false
	}
	return nil
}

// sleep waits for the next tick: wait, or a watch event, whichever comes first, and reports whether an event woke it.
// Events are coalesced to at most one tick a second:
// an event arriving sooner than that after the last tick waits out the rest of the second,
// and every event that came meanwhile is dropped.
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
// It doubles the current wait up to idleCeiling while no run is in flight and no event came.
// On an event or while a run is in flight it returns zero, which means the probe interval.
func nextWait(current time.Duration, inFlight, event bool) time.Duration {
	if inFlight || event {
		return 0
	}
	return min(current*2, idleCeiling)
}
