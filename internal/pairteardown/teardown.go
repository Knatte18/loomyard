package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// RemoveQuietWait is the quiet-wait bound `lyx fabric remove` passes.
// It is a constant rather than config: the wait never kills a working driver on the operator path, and an unbounded wait would hang the hub's land step.
const RemoveQuietWait = 2 * time.Minute

// quietPollInterval is the pause between two quiet probes.
// The probe count is capped from Request.QuietWait and this interval, never read off a clock.
const quietPollInterval = 2 * time.Second

// ErrDriverBusy is the sentinel EndSession's error wraps when the bound is spent with Request.RefuseWhenBusy and the pair's driver is still busy.
var ErrDriverBusy = errors.New("pair driver is busy")

// Request names the pair to end and the caller's policy on a busy driver.
type Request struct {
	// Slug names the pair.
	Slug string
	// Force and Remote are passed to the refusal probe and the removal.
	Force  bool
	Remote bool
	// QuietWait bounds the wait for the driver to go quiet; zero probes once.
	QuietWait time.Duration
	// RefuseWhenBusy makes a spent bound an ErrDriverBusy refusal instead of ending the driver.
	RefuseWhenBusy bool
	// Landed says the caller knows the pair's work landed, so its board entry is marked done after the removal.
	Landed bool
	// Local removes the pair with no origin step and no board write.
	// A busy teardown or records write lock surfaces as fabricengine.ErrPairBusy.
	Local bool
	// Snapshot keeps the pair's task and records worktrees' uncommitted paths under a local snapshot tag before the removal, in local and origin mode alike.
	Snapshot bool
	// LeaveClaim, in origin mode, leaves the pair's board claim to the caller: the removal reads no landed answer and settles no claim.
	LeaveClaim bool
	// AfterSnapshot runs once the snapshot step succeeded and before the removal; its error stops the removal with the snapshot tag kept.
	AfterSnapshot func() error
	// Verdict is the caller's origin reconcile verdict for the slug, empty when it ran none.
	// An ended or owned_elsewhere verdict skips the driver checks.
	Verdict fabricengine.EntryState
}

// SessionResult reports EndSession's outcome.
type SessionResult struct {
	// Ended is true when the session end ran: Engine.Down on a present task worktree, which reports true whether or not a session existed, or a session found and reaped by name on a gone one.
	Ended bool
	// AbandonedSession names a foreign session reed's Down reported and did not kill; empty in every ordinary teardown.
	AbandonedSession string
	// DriverWasLive is true when the bound was spent with the driver still busy and the caller's policy ended it anyway.
	DriverWasLive bool
	// EndedProcesses lists the registered processes of the pair, and the processes below them, that the owner-record end signalled and found gone.
	EndedProcesses []runproc.Ended
	// Survivors lists the processes still alive after that end, reported only when the caller does not refuse a busy driver.
	Survivors []runproc.Ended
}

// Result is Run's outcome: both phases, with the removal passed through whole.
type Result struct {
	Session SessionResult
	Removal fabricengine.RemoveResult
}

// quietState is one quiet probe's answer.
type quietState struct {
	quiet bool
	// driver names the live driver strand blocking quiet; empty when no driver strand blocks.
	driver string
	// loop is true when the pair's loop lock is held, so a detached step loop is live or starting.
	loop bool
	// goDriver is true when the Go driver's lock is held, so a detached Go driver is live between its steps.
	goDriver bool
}

// Teardown runs the pair-teardown sequence.
// Each substrate sits behind an unexported function field so the untagged tests drive the sequence with fakes.
type Teardown struct {
	prime    *lyxcwd.Location
	interval time.Duration
	// goos is the operating system the way forward of a held driver lock is worded for.
	goos string

	gone    func(slug string) (bool, error)
	quiet   func(slug string) (quietState, error)
	refusal func(slug string, force bool) error
	// endOwned ends the pair's registered processes, sparing the calling process and its ancestors and every process of a role in spare.
	endOwned func(slug string, spare ...string) ([]runproc.Ended, error)
	// driverLockHeld reports whether the present task worktree's Go-driver lock is held.
	driverLockHeld func(slug string) (bool, error)
	// battenAlive reports whether the slug's run has a live lifecycle owner: a registered batten process, or a dobby driving the run between its passes.
	battenAlive func(slug string) (bool, error)
	// parentOf names the slug of the pair's parent, or "" for the prime.
	parentOf func(slug string) (string, error)
	// cancelMarked reports whether a batten cancel marker of the slug stands.
	cancelMarked func(slug string) bool
	// driverPID returns the pid of the slug's live registered driver; found is false when there is none.
	driverPID func(slug string) (pid int, found bool)
	// paused reports whether a live pause entry covers the slug, and whether a hub entry is among those covering it.
	paused func(slug string) (covered, hubEntry bool, err error)
	// awaitOrPass waits for the pair's run lock before the owned processes are ended, and lets a holder the end reaches pass at the bound.
	// retry names the step the operator runs again, which a refusal names.
	awaitOrPass func(ctx context.Context, slug string, paused, hubEntry bool, retry string) error
	// ownedProcesses lists the slug's live registered processes.
	ownedProcesses func(slug string) ([]runproc.Record, error)
	endSession     func(slug string, gone bool) (ended bool, abandoned string, err error)
	remove         func(req Request) (fabricengine.RemoveResult, error)
	// snapshot keeps the pair's uncommitted paths under a local snapshot tag, recording into rec; the tag is empty when nothing was uncommitted.
	snapshot func(rec *fabricengine.Mutations, slug string) (tag string, paths []string, err error)
	// archive copies the pair's trace and verify logs into the hub's log archive; it never fails the teardown.
	archive func(slug string)
	sleep   func(ctx context.Context, d time.Duration) error

	// landed reports whether the present task worktree's run finished a Finalize.
	landed func(slug string) (bool, error)
	// battenRun reports whether batten seeded a run for the slug on the prime, and that run's state.
	battenRun func(slug string) (seeded bool, state string, err error)
	// readClaim reads the status of the slug's board entry; found is false when the board holds no such entry.
	readClaim func(slug string) (status *string, found bool, err error)
	// writeClaim sets the status of the slug's board entry, or clears it with a nil status.
	writeClaim func(slug string, status *string) error

	// killCapture terminates the pair's drift capture job, which ends the capture child and its command on Windows.
	killCapture func(slug string) (bool, error)
	// killLoop kills the pair's loop and its in-flight step tree.
	killLoop func(slug string) (bool, error)
	// seizeLoop runs kill, takes the pair's loop lock and leaves the teardown's mark in the pid file;
	// its release function drops the lock, and with sessionEnded false first restores the pid file.
	seizeLoop func(ctx context.Context, slug string, kill func() (bool, error)) (release func(sessionEnded bool) error, err error)
	// awaitRunLock waits for the pair's run lock to be released and names its holder when the bound passes.
	awaitRunLock func(ctx context.Context, slug string) error

	// holdWriteLocks takes the prime's and the pair's records write locks, each bounded by writeLockWait; its release drops them.
	holdWriteLocks func(slug string) (release func() error, err error)
	// stopStrands stops the present task worktree's live strands through shuttle and returns their names.
	stopStrands func(slug string) ([]string, error)
	// writeLockWait bounds each records write lock hold of the cancel phase.
	writeLockWait time.Duration
}

// New wires the production substrates from the hub's prime location.
func New(prime *lyxcwd.Location) (*Teardown, error) {
	if prime == nil {
		return nil, errors.New("pairteardown: New needs the hub's prime location")
	}
	t := &Teardown{prime: prime, interval: quietPollInterval, goos: runtime.GOOS, sleep: sleepCtx, writeLockWait: runLockWait}
	t.holdWriteLocks = t.holdRecordsWriteLocks
	t.stopStrands = t.stopPairStrands
	t.gone = t.taskWorktreeGone
	t.quiet = t.probeQuiet
	t.endOwned = t.endOwnedProcesses
	t.driverLockHeld = t.driverLockHeldIn
	t.battenAlive = t.registeredBattenAlive
	t.parentOf = func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) }
	t.cancelMarked = func(slug string) bool { return fileExists(shedrun.CancelMarker(prime, slug)) }
	t.driverPID = t.driverRecordPID
	t.paused = t.pausedIn
	t.awaitOrPass = t.awaitRunLockOrPass
	t.ownedProcesses = func(slug string) ([]runproc.Record, error) {
		return runproc.List(fabricengine.RunProcDir(prime.HubPath), slug)
	}
	t.refusal = func(slug string, force bool) error {
		top, err := t.topology()
		if err != nil {
			return err
		}
		return top.RemoveRefusal(prime, slug, force)
	}
	t.endSession = t.endReedSession
	t.archive = t.archivePairLogs
	t.killCapture = t.killPairCapture
	t.killLoop = t.killPairLoop
	t.seizeLoop = t.holdPairLoopLock
	t.awaitRunLock = t.awaitPairRunLock
	t.landed = t.childLanded
	t.battenRun = t.battenRunState
	t.readClaim = t.readBoardClaim
	t.writeClaim = t.writeBoardClaim
	t.remove = func(req Request) (fabricengine.RemoveResult, error) {
		top, err := t.topology()
		if err != nil {
			return fabricengine.RemoveResult{}, err
		}
		if req.Local {
			return top.RemoveLocal(prime, req.Slug, req.Force)
		}
		return top.Remove(prime, req.Slug, req.Force, req.Remote)
	}
	t.snapshot = func(rec *fabricengine.Mutations, slug string) (string, []string, error) {
		top, err := t.topology()
		if err != nil {
			return "", nil, err
		}
		return top.SnapshotUncommitted(rec, prime, slug)
	}
	return t, nil
}

// EndSession waits for the pair's driver to go quiet, probes for removal refusals, ends the pair's registered processes, ends the pair's loop and step tree, waits for the run lock and ends the pair's session.
// A pair with nothing left to do, one paused, ended, owned by another machine, or done locally with batten no longer running, skips the quiet wait;
// with the task worktree present it waits for the run lock first instead, and a holder a pause or a network-gate wait leaves is passed on to the owned-process end.
// A refusal or a busy driver leaves the session and its strands untouched, and a refusal ends no registered process.
// The registered processes are ended for a present or a gone task worktree, and so is the pair's drift capture job, which only Windows holds.
// Processes that survive the end, and a driver lock still held afterwards, refuse the teardown with ErrDriverBusy under Request.RefuseWhenBusy; otherwise survivors land in the result and a held lock sets DriverWasLive.
// The loop lock is held from the kill until the session has ended, and the teardown's mark stays in the loop's pid file after it, so no step starts once the session end begins.
// A failure before the session end puts the pid file back.
func (t *Teardown) EndSession(ctx context.Context, req Request) (SessionResult, error) {
	var res SessionResult

	gone, err := t.gone(req.Slug)
	if err != nil {
		return res, err
	}
	if !gone {
		skipped, err := t.driverChecksSkipped(req)
		if err != nil {
			return res, err
		}
		if skipped {
			covered, hubEntry, err := t.paused(req.Slug)
			if err != nil {
				return res, err
			}
			if err := t.awaitOrPass(ctx, req.Slug, covered, hubEntry, "the removal"); err != nil {
				return res, err
			}
		} else {
			res.DriverWasLive, err = t.waitQuiet(ctx, req)
			if err != nil {
				return res, err
			}
		}
	}

	if err := t.refusal(req.Slug, req.Force); err != nil {
		return res, err
	}

	if err := t.endOwnedBeforeSession(&res, req, gone); err != nil {
		return res, err
	}

	if _, err := t.killCapture(req.Slug); err != nil {
		return res, fmt.Errorf("end the pair's drift capture: %w", err)
	}

	var releaseLoopLock func(sessionEnded bool) error
	if !gone {
		releaseLoopLock, err = t.seizeLoop(ctx, req.Slug, func() (bool, error) { return t.killLoop(req.Slug) })
		if err != nil {
			return res, err
		}
		if err := t.awaitRunLock(ctx, req.Slug); err != nil {
			return res, errors.Join(err, releaseLoopLock(false))
		}
	}

	ended, abandoned, err := t.endSession(req.Slug, gone)
	res.Ended = ended
	res.AbandonedSession = abandoned
	if releaseLoopLock != nil {
		err = errors.Join(err, releaseLoopLock(err == nil))
	}
	if err != nil {
		return res, err
	}
	logger.Info("pairteardown: session ended", "slug", req.Slug, "ended", ended, "task_worktree_gone", gone, "driver_was_live", res.DriverWasLive)
	return res, nil
}

// endOwnedBeforeSession ends the pair's registered processes and, with the task worktree present, re-checks the driver lock they should have released.
// The lock still held means a process the owner record could not reach, such as one an older binary started.
func (t *Teardown) endOwnedBeforeSession(res *SessionResult, req Request, gone bool) error {
	ended, err := t.endOwned(req.Slug)
	res.EndedProcesses = ended
	var survivors *runproc.SurvivorsError
	switch {
	case errors.As(err, &survivors):
		res.Survivors = survivors.Survivors
		if req.RefuseWhenBusy {
			return survivorsBusyError(req.Slug, survivors.Survivors)
		}
		logger.Warn("pairteardown: processes of the pair survived the end", "slug", req.Slug, "survivors", survivorNames(survivors.Survivors))
	case err != nil:
		return fmt.Errorf("end the pair's registered processes: %w", err)
	}
	if gone {
		return nil
	}

	held, err := t.driverLockHeld(req.Slug)
	if err != nil {
		return err
	}
	if !held {
		return nil
	}
	if req.RefuseWhenBusy {
		return t.driverLockBusyError(req.Slug, "the removal")
	}
	logger.Warn("pairteardown: ending a pair whose driver lock is still held", "slug", req.Slug)
	res.DriverWasLive = true
	return nil
}

// survivorNames renders each survivor as "pid (role)".
func survivorNames(survivors []runproc.Ended) string {
	names := make([]string, len(survivors))
	for i, survivor := range survivors {
		names[i] = fmt.Sprintf("%d (%s)", survivor.PID, survivor.Role)
	}
	return strings.Join(names, ", ")
}

// survivorsBusyError builds the ErrDriverBusy refusal for registered processes that survived the end.
func survivorsBusyError(slug string, survivors []runproc.Ended) error {
	return fmt.Errorf("%w: processes of pair %q survived the end: %s; retry the removal once they are gone", ErrDriverBusy, slug, survivorNames(survivors))
}

// driverLockBusyError builds the ErrDriverBusy refusal for a driver lock held after the owned processes were ended.
// retry names the step the operator retries once the holder is gone.
func (t *Teardown) driverLockBusyError(slug, retry string) error {
	pair, err := lyxcwd.NamedWorktree(t.prime.HubPath, slug)
	if err != nil {
		return fmt.Errorf("locate the driver lock of pair %q: %w", slug, err)
	}
	lockPath := shedrun.DriverLock(pair, shedrun.SelfRunID)
	var wayForward string
	switch {
	case t.goos != "windows":
		wayForward = "find the holder in the replaced-binary part of `lyx tender status` or with `ps`, end it, then retry " + retry
	default:
		if pid, found := t.driverPID(slug); found {
			wayForward = fmt.Sprintf("end the driver with `taskkill /PID %d /T /F`, then retry %s", pid, retry)
		} else {
			wayForward = fmt.Sprintf("end the `lyx loom drive` process of the pair in Task Manager, the one holding %s, then retry %s", lockPath, retry)
		}
	}
	return fmt.Errorf("%w: the driver lock %s of pair %q is held by a process the owner record does not reach; %s", ErrDriverBusy, lockPath, slug, wayForward)
}

// RemovePair archives the pair's logs, removes the pair, then settles the pair's board claim.
// It runs only after EndSession succeeded for the same request: Run and batten's one-row producer are its only sequencers.
// The archive copies and never fails the teardown, so the removal follows it whatever it did.
//
// Whether the pair's work landed is req.Landed, or else read before the removal from the task worktree's status file, since the file goes with the worktree;
// a task worktree already gone is not landed, and its claim falls to the batten-state rule.
// A failed read of that answer leaves the board alone.
// A failed removal returns its result and error with the board untouched, and the settle only warns on a failure.
//
// With req.Local the removal has no origin step and the board is never read or written.
// With req.Snapshot, local or not, the pair's uncommitted paths are first kept under a local snapshot tag, then req.AfterSnapshot runs, then the pair is removed.
// A refused snapshot or a failed AfterSnapshot returns its error with nothing removed, and a refused snapshot never runs AfterSnapshot.
// The returned result carries the snapshot's tag, paths and mutation record on every return after the snapshot step.
//
// With req.LeaveClaim and no req.Local the landed answer is not read and the board claim is not settled, which the caller does itself.
func (t *Teardown) RemovePair(req Request) (fabricengine.RemoveResult, error) {
	var snap snapshotOutcome
	if req.Snapshot {
		var err error
		snap, err = t.takeSnapshot(req)
		if err != nil {
			return snap.applyTo(fabricengine.RemoveResult{}), err
		}
	}

	settles := !req.Local && !req.LeaveClaim
	landed, landedKnown := req.Landed, settles
	if !landed && settles {
		gone, err := t.gone(req.Slug)
		switch {
		case err != nil:
			logger.Warn("pairteardown: could not tell whether the task worktree is gone, so the board claim is left alone", "slug", req.Slug, "cause", err)
			landedKnown = false
		case !gone:
			landed, err = t.landed(req.Slug)
			if err != nil {
				logger.Warn("pairteardown: could not read whether the run landed, so the board claim is left alone", "slug", req.Slug, "cause", err)
				landedKnown = false
			}
		}
	}

	t.archive(req.Slug)
	res, err := t.remove(req)
	res = snap.applyTo(res)
	if err != nil {
		return res, err
	}
	if landedKnown {
		t.settleBoardClaim(req.Slug, landed)
	}
	return res, nil
}

// snapshotOutcome is what the snapshot step of a local removal produced.
type snapshotOutcome struct {
	tag   string
	paths []string
	rec   fabricengine.Mutations
}

// applyTo puts the snapshot's tag, paths and mutation record on res, the record ahead of the removal's own.
func (s snapshotOutcome) applyTo(res fabricengine.RemoveResult) fabricengine.RemoveResult {
	if s.tag == "" && s.rec.Len() == 0 {
		return res
	}
	res.SnapshotTag = s.tag
	res.SnapshotPaths = s.paths
	merged := s.rec.Snapshot()
	merged.Extend(res.Mutations)
	res.Mutations = merged
	return res
}

// takeSnapshot keeps the pair's uncommitted paths under a local snapshot tag, then runs req.AfterSnapshot.
// Whatever the snapshot recorded is returned with an error too, so the caller reports it.
func (t *Teardown) takeSnapshot(req Request) (snapshotOutcome, error) {
	rec := fabricengine.NewMutations(t.prime.HubPath)
	tag, paths, err := t.snapshot(rec, req.Slug)
	out := snapshotOutcome{tag: tag, paths: paths, rec: rec.Snapshot()}
	if err != nil {
		return out, err
	}
	if tag != "" {
		logger.Info("pairteardown: uncommitted paths kept before the local removal", "slug", req.Slug, "tag", tag, "paths", paths)
	}
	if req.AfterSnapshot != nil {
		if err := req.AfterSnapshot(); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Run is EndSession then RemovePair; a failed EndSession never reaches RemovePair.
func (t *Teardown) Run(ctx context.Context, req Request) (Result, error) {
	session, err := t.EndSession(ctx, req)
	if err != nil {
		return Result{Session: session}, err
	}
	removal, err := t.RemovePair(req)
	return Result{Session: session, Removal: removal}, err
}

// waitQuiet probes until the driver is quiet or the bound is spent, and reports whether a busy driver was left to the caller's policy.
func (t *Teardown) waitQuiet(ctx context.Context, req Request) (driverWasLive bool, err error) {
	attempts := int(req.QuietWait / t.interval)
	for i := 0; ; i++ {
		st, err := t.quiet(req.Slug)
		if err != nil {
			return false, err
		}
		if st.quiet {
			return false, nil
		}
		if i >= attempts {
			if req.RefuseWhenBusy {
				return false, t.busyError(req.Slug, st)
			}
			logger.Warn("pairteardown: ending a driver that is not quiet", "slug", req.Slug, "driver", st.driver)
			return true, nil
		}
		if err := t.sleep(ctx, t.interval); err != nil {
			return false, err
		}
	}
}

// busyError builds the ErrDriverBusy refusal naming the blocker and the way forward.
func (t *Teardown) busyError(slug string, st quietState) error {
	blocker := "the run lock is held"
	switch {
	case st.driver != "":
		blocker = fmt.Sprintf("driver strand %q is live", st.driver)
	case st.loop:
		blocker = "the loop is live"
	case st.goDriver:
		blocker = "the Go driver is live"
	}
	anchor, err := t.taskAnchor(slug)
	if err != nil {
		return fmt.Errorf("locate the anchor of pair %q: %w", slug, err)
	}
	attach := fmt.Sprintf("lyx reed attach, run from %s", anchor)
	wayForward := fmt.Sprintf("retry once the driver has written its report, or pause the run with `lyx tender pause %s` and run the removal again, which the pause lets through", slug)
	if st.loop {
		wayForward = fmt.Sprintf("pause the run with `lyx shed pause %s --after <current_producer>`, or retry once the loop stops; %s", slug, wayForward)
	}
	return fmt.Errorf("%w: %s in pair %q%s; attach with %s; %s", ErrDriverBusy, blocker, slug, t.ownedProcessesText(slug), attach, wayForward)
}

// ownedProcessesText renders the slug's live owned processes as a clause for a refusal, or the read error in their place; it is empty when there are none.
func (t *Teardown) ownedProcessesText(slug string) string {
	records, err := t.ownedProcesses(slug)
	if err != nil {
		return fmt.Sprintf(" (owned processes unreadable: %v)", err)
	}
	if len(records) == 0 {
		return ""
	}
	names := make([]string, len(records))
	for i, record := range records {
		names[i] = fmt.Sprintf("%d (%s)", record.PID, record.Role)
	}
	return fmt.Sprintf(" (live owned processes: %s)", strings.Join(names, ", "))
}

// taskAnchor returns the task worktree's anchor path, at the task's own anchor.
func (t *Teardown) taskAnchor(slug string) (string, error) {
	pair, err := lyxcwd.NamedWorktree(t.prime.HubPath, slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(fabricengine.WorktreePath(t.prime, slug), pair.AnchorRel), nil
}

// topology loads the hub's fabric config fresh and builds a topology over it.
func (t *Teardown) topology() (*fabricengine.Topology, error) {
	cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(t.prime))
	if err != nil {
		return nil, err
	}
	top := fabricengine.NewTopology(cfg)
	top.SetInFlightProbe(t.otherPairSessions)
	return top, nil
}

// otherPairSessions returns the live reed sessions of the hub's pairs other than slug's.
// A hub socket with no tmux server behind it has no sessions.
// The prime's own sessions, the orch among them, are never in the pair set and never count.
func (t *Teardown) otherPairSessions(slug string) ([]string, error) {
	cfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(t.prime), "reed")
	if err != nil {
		return nil, err
	}
	live, err := reedengine.ListSessions(cfg.Tmux, reedengine.ServerName(t.prime.HubPath))
	if err != nil {
		if reedengine.IsNoServer(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list the hub's live sessions: %w", err)
	}

	entries, err := fabricengine.List(t.prime.WorktreePath())
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	removed := filepath.Clean(fabricengine.WorktreePath(t.prime, slug))
	var pairSessions []string
	for _, entry := range entries {
		path := filepath.Clean(filepath.FromSlash(entry.Path))
		if entry.Main || path == removed {
			continue
		}
		pairSessions = append(pairSessions, reedengine.SessionName(path))
	}
	return sessionsOfOtherPairs(live, pairSessions), nil
}

// sessionsOfOtherPairs returns the sessions of live that are named in pairSessions, in live's order.
func sessionsOfOtherPairs(live, pairSessions []string) []string {
	var others []string
	for _, session := range live {
		if slices.Contains(pairSessions, session) {
			others = append(others, session)
		}
	}
	return others
}

// endOwnedProcesses ends the pair's registered processes through the hub's owner record.
// The calling process and its ancestors are spared, so batten's own teardown, run inside a registered batten, never ends itself, and so is every process of a role in spare.
func (t *Teardown) endOwnedProcesses(slug string, spare ...string) ([]runproc.Ended, error) {
	return runproc.EndSparing(fabricengine.RunProcDir(t.prime.HubPath), slug, os.Getpid(), spare...)
}

// driverLockHeldIn reports whether the present task worktree's Go-driver lock is held.
func (t *Teardown) driverLockHeldIn(slug string) (bool, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return false, err
	}
	free, err := runLockFree(shedrun.DriverLock(task, shedrun.SelfRunID))
	return !free, err
}

// registeredBattenAlive reports whether the slug's run has a live lifecycle owner:
// a live record of the slug carries the batten role, or batten's status reads running with no cancel marker and a live dobby record stands under the run's parent, the gap between two passes dobby drives.
func (t *Teardown) registeredBattenAlive(slug string) (bool, error) {
	records, err := t.ownedProcesses(slug)
	if err != nil {
		return false, err
	}
	if hasRole(records, runproc.RoleBatten) {
		return true, nil
	}
	_, state, err := t.battenRun(slug)
	if err != nil || state != string(shedengine.StateRunning) || t.cancelMarked(slug) {
		return false, err
	}
	parent, err := t.parentOf(slug)
	if err != nil {
		return false, err
	}
	if parent == "" {
		parent = runproc.HubOwner
	}
	dobbies, err := t.ownedProcesses(parent)
	if err != nil {
		return false, err
	}
	return hasRole(dobbies, runproc.RoleDobby), nil
}

// hasRole reports whether any record carries the role.
func hasRole(records []runproc.Record, role string) bool {
	for _, record := range records {
		if record.Role == role {
			return true
		}
	}
	return false
}

// driverRecordPID returns the pid of a live record of the slug with the driver role.
func (t *Teardown) driverRecordPID(slug string) (int, bool) {
	records, err := runproc.List(fabricengine.RunProcDir(t.prime.HubPath), slug)
	if err != nil {
		return 0, false
	}
	for _, record := range records {
		if record.Role == runproc.RoleDriver {
			return record.PID, true
		}
	}
	return 0, false
}

// taskWorktreeGone reports whether the task worktree path is not a registered linked worktree of the prime's repo.
func (t *Teardown) taskWorktreeGone(slug string) (bool, error) {
	entries, err := fabricengine.List(t.prime.WorktreePath())
	if err != nil {
		return false, fmt.Errorf("list worktrees: %w", err)
	}
	target := filepath.Clean(fabricengine.WorktreePath(t.prime, slug))
	for _, entry := range entries {
		if !entry.Main && filepath.Clean(filepath.FromSlash(entry.Path)) == target {
			return false, nil
		}
	}
	return true, nil
}

// TaskWorktreeGone reports whether the slug's task worktree path is not a registered linked worktree of the prime's repo, so a caller can tell a pair already removed by hand from a present one.
func (t *Teardown) TaskWorktreeGone(slug string) (bool, error) {
	return t.gone(slug)
}

// taskLocation resolves the present task worktree's own location.
func (t *Teardown) taskLocation(slug string) (*lyxcwd.Location, error) {
	return lyxcwd.ResolveWorktree(fabricengine.WorktreePath(t.prime, slug))
}

// reedEngine builds the reed engine for the present task worktree.
func (t *Teardown) reedEngine(task *lyxcwd.Location) (*reedengine.Engine, error) {
	cfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(task), "reed")
	if err != nil {
		return nil, err
	}
	geom, err := hubgeom.ReedGeometry(task)
	if err != nil {
		return nil, err
	}
	return reedengine.New(cfg, geom, shuttleengines.EnvScrub()), nil
}

// probeQuiet answers whether the present task worktree's driver is quiet: the run lock, the loop lock and the Go-driver lock are free, and the driver strand is absent, dead, retiring or parked.
func (t *Teardown) probeQuiet(slug string) (quietState, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return quietState{}, err
	}

	lockFree, err := runLockFree(shedrun.RunLock(task, shedrun.SelfRunID))
	if err != nil {
		return quietState{}, err
	}
	loopFree, err := runLockFree(shedrun.LoopLock(task, shedrun.SelfRunID))
	if err != nil {
		return quietState{}, err
	}

	driverLockFree, err := runLockFree(shedrun.DriverLock(task, shedrun.SelfRunID))
	if err != nil {
		return quietState{}, err
	}

	var driver string
	if !fileExists(shedrun.ParkMarker(task, shedrun.SelfRunID)) {
		eng, err := t.reedEngine(task)
		if err != nil {
			return quietState{}, err
		}
		status, err := eng.Status()
		if err != nil && !errors.Is(err, reedengine.ErrNoSession) {
			return quietState{}, err
		}
		for _, s := range status.Strands {
			if loomengine.IsDriverStrand(s.Name) && s.Live && !s.Retiring {
				driver = s.Name
				break
			}
		}
	}
	return quietState{quiet: lockFree && loopFree && driverLockFree && driver == "", driver: driver, loop: !loopFree, goDriver: !driverLockFree}, nil
}

// killPairCapture terminates the pair's drift capture job and reports whether one existed.
// The job name is computed from the pair's worktree path, so it needs no present worktree; on Linux it reports false and ends nothing.
func (t *Teardown) killPairCapture(slug string) (bool, error) {
	name := driftengine.CaptureJobName(fabricengine.WorktreePath(t.prime, slug))
	ended, err := proc.TerminateNamedJob(name)
	if err != nil {
		return false, err
	}
	if ended {
		logger.Info("pairteardown: drift capture job terminated", "slug", slug, "job", name)
	}
	return ended, nil
}

// killPairLoop kills the present task worktree's loop and its in-flight step tree.
func (t *Teardown) killPairLoop(slug string) (bool, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return false, err
	}
	return killLoop(shedrun.LoopPIDFile(task, shedrun.SelfRunID), shedrun.LoopLock(task, shedrun.SelfRunID), shedrun.LoopJobName(task, shedrun.SelfRunID))
}

// holdPairLoopLock takes the present task worktree's loop lock after kill and leaves the teardown's mark in its loop pid file.
// The steps directory is created first, since the lock file lives in it.
func (t *Teardown) holdPairLoopLock(ctx context.Context, slug string, kill func() (bool, error)) (func(sessionEnded bool) error, error) {
	task, err := t.taskLocation(slug)
	if err != nil {
		return nil, err
	}
	lockPath := shedrun.LoopLock(task, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	return holdLoopLock(ctx, kill, shedrun.LoopPIDFile(task, shedrun.SelfRunID), lockPath, int(runLockWait/quietPollInterval), t.sleep)
}

// awaitPairRunLock waits for the present task worktree's run lock to be released.
func (t *Teardown) awaitPairRunLock(ctx context.Context, slug string) error {
	task, err := t.taskLocation(slug)
	if err != nil {
		return err
	}
	return awaitRunLockNamingHolder(ctx, shedrun.RunLock(task, shedrun.SelfRunID), shedrun.StepsDir(task, shedrun.SelfRunID), slug, int(runLockWait/quietPollInterval), t.sleep)
}

// endReedSession ends the pair's session: Engine.Down while the task worktree is present, EndSessionByName once it is gone.
func (t *Teardown) endReedSession(slug string, gone bool) (bool, string, error) {
	if !gone {
		task, err := t.taskLocation(slug)
		if err != nil {
			return false, "", err
		}
		eng, err := t.reedEngine(task)
		if err != nil {
			return false, "", err
		}
		res, err := eng.Down()
		if err != nil {
			return false, "", err
		}
		return true, res.AbandonedSession, nil
	}

	cfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(t.prime), "reed")
	if err != nil {
		return false, "", err
	}
	ended, err := reedengine.EndSessionByName(
		cfg.Tmux, cfg.Shell,
		reedengine.ServerName(t.prime.HubPath),
		reedengine.SessionName(fabricengine.WorktreePath(t.prime, slug)),
	)
	return ended, "", err
}

// runLockFree probes the run lock without blocking and releases it at once.
// An absent scratch directory means no run ever took the lock.
func runLockFree(lockPath string) (bool, error) {
	if _, err := os.Stat(filepath.Dir(lockPath)); errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	held, ok, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return true, held.Release()
}

// fileExists reports whether path names an existing file or directory.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sleepCtx waits d or until ctx is done.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
