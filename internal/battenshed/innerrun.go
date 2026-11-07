// innerrun.go implements NewInnerRun, the producer that spawns the inner shed run in the task worktree.
// The producer spawns the run only if it has not spawned yet.
// It then checks the child's persisted status once per Call,
// and it ends the Call with a verdict only when something worth a history entry happens:
// the child's state changes, its arm's own event fires, batten is paused, or the child's status file cannot be stat'ed.
// A check that ends nothing yields, and what the next check needs to know rides in the memory record.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// pollFloor is the shortest poll interval NewInnerRun accepts; a told interval below it is floored to it with a Warn.
const pollFloor = time.Second

// waitOrCancel pauses for d, returning as soon as ctx is cancelled if that happens first.
// It is the production sleep, used when a caller supplies none.
// A caller driving a producer under a cancellable context gets it back promptly.
// The lym CLI itself never cancels its context:
// an operator's Ctrl-C ends the process outright;
// "lym batten pause" is read between rows, so it waits out the current interval.
func waitOrCancel(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// spawnConfirmedFileSuffix is the fixed suffix of the marker a producer writes once its spawn has returned success.
// The marker holds the pid of the process that wrote it, so it confirms the spawn for that process only.
const spawnConfirmedFileSuffix = "-spawned"

// SpawnConfirmedFile returns the path of the marker a producer writes once its spawn has returned success.
// It is exported so the file's reader and writer share one declarer of its name.
func SpawnConfirmedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+spawnConfirmedFileSuffix)
}

// haltedWaitReason renders the stuck reason of a halted child's wait.
// Nothing on the prime side resumes the child's own driver,
// so the reason names the operator's resume command and says this run keeps watching.
// startFallback adds `lym loom start`, for a child with no driver strand to wake or whose revive failed.
func haltedWaitReason(status shedengine.Status, startFallback bool) string {
	resume := `run "lym loom resume" in the task worktree to resume it`
	if startFallback {
		resume = `run "lym loom resume" in the task worktree to resume it, or "lym loom start" in the task worktree when no driver can be woken`
	}
	return fmt.Sprintf("inner shed run is %s: error=%q current_producer=%q; %s; this run then keeps watching", status.State, status.Error, status.CurrentProducer, resume)
}

// haltWarnedFileSuffix is the fixed suffix of the marker recording the child's history length at the last halt Warn.
const haltWarnedFileSuffix = "-halt-warned"

// haltWarnedFile returns the path of the marker holding the child's history length, in decimal, at the last halt Warn.
func haltWarnedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+haltWarnedFileSuffix)
}

// revivedFileSuffix is the fixed suffix of the marker recording that a driver revive was tried in this halt episode.
// It holds the attempting process's pid and the attempt's result, `ok` or `failed`, on one line; only its presence is read.
const revivedFileSuffix = "-revived"

// revivedFile returns the path of the revive once-marker.
func revivedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+revivedFileSuffix)
}

// decisionActedFileSuffix is the fixed suffix of the marker recording the decision the child was last resumed on.
const decisionActedFileSuffix = "-decision-acted"

// doneSeenFileSuffix is the fixed suffix of the marker recording when the child was first seen done.
const doneSeenFileSuffix = "-done-seen"

// decisionActedFile returns the path of the marker holding the identity of the last decision resumed on.
// The marker also holds the child's history length at that resume.
func decisionActedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+decisionActedFileSuffix)
}

// doneSeenFile returns the path of the marker holding the RFC 3339 time the child was first seen done.
func doneSeenFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+doneSeenFileSuffix)
}

// decisionIdentity renders a decision as the line the decision-acted marker holds.
// The kind leads, so an approval and a rejection at the same head within the same second never share an identity.
func decisionIdentity(d ChildDecision) string {
	return d.Kind + " " + d.At + " " + d.HeadSHA + "\n"
}

// decisionActedContent renders the decision-acted marker: the identity line, then the child's history length in decimal.
// The length rides beside the identity, never inside it,
// so identity comparison alone decides whether a decision is new.
func decisionActedContent(identity string, historyLen int) string {
	return identity + strconv.Itoa(historyLen) + "\n"
}

// parseDecisionActed splits a marker into its identity line (through the first newline) and the recorded history length.
// hasLen is false for a one-line marker, an unparseable or negative length (a torn write), and input with no newline;
// such input is returned whole as identity, so it never matches a real identity line.
func parseDecisionActed(raw string) (identity string, historyLen int, hasLen bool) {
	i := strings.IndexByte(raw, '\n')
	if i < 0 {
		return raw, 0, false
	}
	identity = raw[:i+1]
	n, err := strconv.Atoi(strings.TrimSpace(raw[i+1:]))
	if err != nil || n < 0 {
		return identity, 0, false
	}
	return identity, n, true
}

// awaitingHandOff is the operator instruction an awaiting child's wait carries.
// The child waits on a review segment's escalation, and only the operator can settle it from inside the task worktree.
// An escalation's circling decision is resumed by the operator's own `lym loom resume`.
const awaitingHandOff = "at a review segment's escalation run \"lym loom circling accept <slug>\" or \"lym loom circling continue <slug>\" and then \"lym loom resume\" in the task worktree"

// innerRunProducer spawns the inner shed run for a task worktree once.
// Each Call then checks the child's persisted status.
// A Call returns Stuck, always budget-exempt, only when the check has something to record;
// shedengine's on_stuck self-route then re-enters it as a new bounce.
// A check with nothing to record returns Yield, which persists nothing.
// Everything a Call remembers for the next one is in the memory record:
// it is loaded at the start of a Call and saved at every return.
type innerRunProducer struct {
	name string
	slug string
	deps InnerRunDeps
	// noticeProbe is deps.NoticeProbe: how rarely a check does anything costing a process or a multiplexer round trip.
	noticeProbe time.Duration
	// cur is the wait state of the Call in progress, loaded from the memory record by loadMemory.
	cur *childWait
	// watched is whether the batten holds the marker, per deps.MarkWatched; watchedAnswer is how it was reached.
	watched       bool
	watchedAnswer watchedAnswer
	// driverBusyNoticed is whether the orch was noticed of the current busy-driver stall; a resume that lands clears it.
	driverBusyNoticed bool
	// episode is the child's current state episode and the delivery of its notices, across Calls.
	episode    noticeEpisode
	scratchDir string
	// driverExitGrace bounds how long a done child's live driver strand is waited for.
	driverExitGrace time.Duration
	// notices is true when the caller wired a Notify, before a nil one resolves to a no-op.
	notices bool
	// failures guards each polled seam's failure by message: Warn at first, Debug while it repeats, Info at recovery.
	failures logger.EpisodeSet
	// restepOn is true when the binary re-step is wired: both its build and dirty-path seams.
	restepOn bool
	// restep is the binary re-step's interval and last reading, across Calls.
	restep restepWatch
	// ownedBy is the machine the owner check last read as owning the run, or empty while it reads as owned here.
	ownedBy string
	// ownedSubject is what `lym adopt` takes to move a run read as owned elsewhere; empty when the verdict brought none.
	ownedSubject string
	// ownerNoticed is whether the orch was noticed of the current owner episode.
	ownerNoticed bool
	// ownerDue is whether the owner check is due at the next check that has a status.
	// ownerCovered is whether the previous check's pause coverage covered the slug.
	ownerDue     bool
	ownerCovered bool
	// respawns is how many times the running child was respawned with no write to its status file between.
	// respawnStatusTime is the status file's modification time taken after the last respawn returned.
	respawns          int
	respawnStatusTime time.Time
}

// respawnCap is how many times a running child with no driver at work is respawned before the producer leaves it to the operator.
// A write to the child's status file, or a change of its state, starts the count again.
const respawnCap = 3

var _ shedengine.ShedProducer = (*innerRunProducer)(nil)

// seamFailed counts one failure of the seam named by msg, with the producer and slug fields, extra, then err.
func (p *innerRunProducer) seamFailed(msg string, err error, extra ...any) {
	args := append([]any{"producer", p.name, "slug", p.slug}, extra...)
	p.failures.Fail(msg, msg, append(args, "error", err)...)
}

// seamRecovered ends the failure episode of the seam named by msg; it does nothing outside an episode.
func (p *innerRunProducer) seamRecovered(msg string) {
	p.failures.Recover(msg, "battenshed: a repeated failure recovered", "failure", msg, "producer", p.name, "slug", p.slug)
}

// The failure messages of the guarded seams, each the key of its episode.
const (
	failLockRecordUnreadable  = "battenshed: parent lock record unreadable; leaving it alone"
	failAgentStatesUnreadable = "battenshed: the child's agent states are unreadable; keeping its parent lock"
	failLockRelease           = "battenshed: could not release the child's parent lock"
	failDriverStrandRead      = "battenshed: could not read the child's driver strand; skipping the revive"
	failDriverLiveness        = "battenshed: driver liveness read failed; treating the driver as live"
	failChildLockRead         = "battenshed: child lock read failed; treating the driver as live"
	failPairDirectory         = "battenshed: notice skipped; pair directory unresolved"
	failArbiterMarker         = "battenshed: arbiter marker unreadable; holding nothing"
	failTacklerMarker         = "battenshed: tackler marker unreadable; holding nothing"
	failStopReport            = "battenshed: stop report unreadable; reading it as none"
	failNoticeDecision        = "battenshed: decision unreadable for the notice step; reading it as decided"
	failOrchStateUnreadable   = "battenshed: notice held back; the orch state is unreadable"
	failRestepDriverStrand    = "battenshed: binary re-step check skipped; could not read the child's driver strand"
	failRestepLocks           = "battenshed: binary re-step check skipped; could not read the child's locks"
	failRestepBuild           = "battenshed: binary re-step check skipped; could not read the on-disk build"
	failRestepDirtyPaths      = "battenshed: binary re-step check skipped; could not read the task pair's uncommitted paths"
	failRestepMarker          = "battenshed: binary re-step marker unreadable; reading it as absent"
)

// logAttempt logs msg at Info for a first attempt and at Debug for each retry after a refusal.
func logAttempt(first bool, msg string, args ...any) {
	if first {
		logger.Info(msg, args...)
		return
	}
	logger.Debug(msg, args...)
}

// NewInnerRun returns a shedengine.ShedProducer that resolves the task worktree's status path.
// The first Call that finds no status file spawns the inner shed run through deps.Spawn.
// Each later Call checks the child once and yields when the check ends nothing.
// A pollInterval below one second draws one Warn naming poll_interval_s, and no wait reads it.
//
// driverExitGrace bounds the wait for a done child's driver strand to end before the row returns Done anyway.
//
// A nil deps.Now resolves to time.Now, once here rather than on every Call.
// A test's fixed clock is the only substitute for it.
// A nil deps.Notify resolves to a no-op and switches the notice step off.
// A nil deps.OrchStrandRecorded resolves to reporting a strand recorded.
// A nil deps.StopReport resolves to reporting no stop report.
// A nil deps.DriverStrand resolves to reporting no driver strand.
// A nil deps.ChildRunLockHeld or deps.ChildDriverLockHeld resolves to reporting no held lock.
// A nil deps.ReviveStrands resolves to a revive that fails as not wired.
// A nil deps.TacklerInFlight resolves to reporting no tackler.
// A nil deps.ChildBuildChanged or deps.ChildDirtyPaths switches the binary re-step off.
// A nil deps.PauseRequested resolves to never paused.
// A nil deps.Pause resolves to never covered by a pause record.
// A nil deps.MarkWatched resolves to reporting not held.
// A nil deps.ArbiterInFlight resolves to reporting no arbiter.
// A nil deps.ArbiterStop resolves to a reading that is not eligible.
// A nil deps.ParentLock switches the parent lock step off.
// A nil deps.OwnerCheck switches the owner check off.
// A nil deps.PauseOwned resolves to a no-op.
// A nil deps.ParentHoldsTaskWork or deps.Restepped resolves to reporting false.
// A nil deps.HandBack resolves to marking no status.
func NewInnerRun(name, slug string, deps InnerRunDeps, pollInterval time.Duration, scratchDir string, driverExitGrace time.Duration) shedengine.ShedProducer {
	if pollInterval < pollFloor {
		logger.Warn("battenshed: poll_interval_s is below the floor", "key", "poll_interval_s", "value", pollInterval, "floor_s", int(pollFloor.Seconds()))
	}
	if deps.PauseRequested == nil {
		deps.PauseRequested = func() (bool, error) { return false, nil }
	}
	if deps.Pause == nil {
		deps.Pause = func() (pauserecord.Coverage, error) { return pauserecord.Coverage{}, nil }
	}
	if deps.MarkWatched == nil {
		deps.MarkWatched = func(context.Context) (bool, error) { return false, nil }
	}
	if deps.ArbiterInFlight == nil {
		deps.ArbiterInFlight = func(context.Context) (ChildArbiter, bool, error) { return ChildArbiter{}, false, nil }
	}
	if deps.ArbiterStop == nil {
		deps.ArbiterStop = func(context.Context, shedengine.Status) (ChildArbiterStop, error) { return ChildArbiterStop{}, nil }
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Awake == nil {
		origin := time.Now()
		deps.Awake = func() time.Duration { return time.Since(origin) }
	}
	notices := deps.Notify != nil
	if deps.Notify == nil {
		deps.Notify = func(context.Context, string) (bool, error) { return true, nil }
	}
	if deps.OrchStrandRecorded == nil {
		deps.OrchStrandRecorded = func() (bool, error) { return true, nil }
	}
	if deps.PairDir == nil {
		deps.PairDir = func() (string, error) { return "", errors.New("no pair directory wired") }
	}
	if deps.DriverStrand == nil {
		deps.DriverStrand = func(context.Context) (ChildDriverStrand, error) { return ChildDriverNone, nil }
	}
	if deps.ChildRunLockHeld == nil {
		deps.ChildRunLockHeld = func() (bool, error) { return false, nil }
	}
	if deps.ChildDriverLockHeld == nil {
		deps.ChildDriverLockHeld = func() (bool, error) { return false, nil }
	}
	if deps.TacklerInFlight == nil {
		deps.TacklerInFlight = func(context.Context) (ChildArbiter, bool, error) { return ChildArbiter{}, false, nil }
	}
	if deps.ReviveStrands == nil {
		deps.ReviveStrands = func(context.Context) error { return errors.New("no strand revive wired") }
	}
	if deps.PauseOwned == nil {
		deps.PauseOwned = func(context.Context, string) error { return nil }
	}
	if deps.ParentHoldsTaskWork == nil {
		deps.ParentHoldsTaskWork = func() (bool, error) { return false, nil }
	}
	if deps.Restepped == nil {
		deps.Restepped = func(shedengine.Status) (bool, error) { return false, nil }
	}
	if deps.HandBack == nil {
		deps.HandBack = func(shedengine.Status) bool { return false }
	}
	return &innerRunProducer{
		name:        name,
		slug:        slug,
		deps:        deps,
		noticeProbe: deps.NoticeProbe,
		scratchDir:  scratchDir,

		driverExitGrace: driverExitGrace,
		notices:         notices,
		restepOn:        deps.ChildBuildChanged != nil && deps.ChildDirtyPaths != nil,
	}
}

// Call implements shedengine.ShedProducer.
//
// It reads the child's status before doing anything else.
// It spawns only when that status is absent.
// It also spawns only when the status is running with no spawn confirmed by this batten process and no driver at work.
// The child's status file alone cannot say whether a spawn happened:
// the child's bootstrap seeds it as running before it starts the driver,
// so a bootstrap that failed or was killed after seeding leaves a running status with no driver behind it.
// The confirmation is a marker under scratchDir (SpawnConfirmedFile) holding the pid of the process that wrote it.
// The marker is cleared before every spawn attempt and written only once deps.Spawn returns success.
// A marker naming another pid, or one in the old layout, reads as unconfirmed.
// Batten's own run lock means a pid other than this process's belongs to an earlier, dead batten process.
// An unconfirmed running child is adopted, not spawned, when deps.DriverStrand reports a live or retiring driver.
// It is also adopted when deps.ChildRunLockHeld reports a held run lock.
// Adoption records the confirmation for this process and spawns nothing.
// Re-spawning a running child with neither is safe, because the bootstrap it runs is idempotent against a live driver.
// It never stacks a second driver.
// A running child with no driver at work gets at most respawnCap respawns while its status file is unwritten.
// The count lives in the memory record.
// After that nothing is spawned until the file is written or the child's state changes.
// The dobby's driver-dead notice names the cap.
// A halted or done child is never re-spawned, marker or not.
// The outer run watches the child's own run and never restarts it.
//
// The full disposition table is evaluated top to bottom.
// A spawn as above logs two lines around deps.Spawn, then reads the status once more.
// An error from deps.Spawn is a hard error, not Stuck: a failed spawn is a mechanism failure, not an ordinary wait.
// The next Call retries it.
// A successful spawn that leaves no status file is a hard error: the spawn returned success without producing one.
// A spawn that a covering pause or a standing owner holds ends the pass at once with a Yield carrying the hold's reason.
// It spawns on the first pass that finds neither.
// A spawn refused with a ChildOwnedElsewhereError is not a failed spawn.
// For an unconfirmed running child's respawn, the run is paused and Call returns a budget-exempt Stuck naming the owner.
// The next Call spawns nothing while the owner stands.
// Any Call that finds the child in a state other than done first removes a leftover done-seen marker.
// So a marker from an earlier run of the same slug never shortens a later wait.
// Any Call that finds the child out of a halted state likewise removes the halt-warned marker, ending the halt episode.
// Call runs one pass of the child's check, loading the memory record first and saving it on every return.
// The memory's episode, when it holds one, is the state this Call begins with; without one, it is the state read.
// A child whose state differs from that state ends the Call at once with a Stuck naming the change,
// before any arm runs, even when the status file's modification time did not move since.
// Once the child's state is settled, the notice step runs.
// It is informational only: one notice per condition per episode, never changing the outcome.
// Then, by state, each arm does its actions once in this Call:
//   - running reports its wait reason, naming the reviewer the child waits on when there is one;
//   - awaiting first revives the driver strand once per episode; with no decision record it reports the hand-off;
//   - awaiting with a decision not yet acted on spawns the child's driver again;
//     the child's own bootstrap resumes an approved or rejected run;
//     the decision is recorded in the decision-acted marker only once the spawn succeeded,
//     and the arm goes on waiting until the child leaves awaiting;
//     a spawn refused with `ErrChildNotParked` records nothing and is retried at most once per `noticeProbe`;
//     until the driver parks or the state changes;
//     a spawn refused with `ErrChildDriverBusy` is retried the same way and posts one orch notice per stall;
//   - awaiting with an acted-on decision does not spawn while the child's history is unchanged since that resume;
//     the arm reports that the resume was delivered and the child's driver has not re-stepped yet;
//   - awaiting with an acted-on decision does not spawn when the child's history is longer;
//     an old-layout marker is treated the same, and the arm reports the recovery of deciding again;
//   - done records the first-sight time in the done-seen marker;
//     it returns Done once the driver strand is gone or `driverExitGrace` has elapsed since first sight;
//     otherwise it reports the wait for the driver to finish its stop report;
//   - blocked, paused or failed never spawns or resumes the child, except for the binary re-step;
//     the arm logs one Info line per halt episode and revives a dead driver strand once per episode;
//     it reports the child's State, Error and CurrentProducer and the resume command;
//     a resumed child is then read as running again;
//   - any other value is a hard error naming the unrecognised state.
//
// A Call returns a verdict only when:
//   - the child's State differs from the one the Call began with:
//     a Stuck names the change in its Path and Reason, as `child running → blocked at Plan-Review`;
//     each change is its own history entry, so equal entries never fold across changes;
//   - the arm's own event fires: the done arm's Done, or an awaiting child's decision;
//     an awaiting decision is acted on and then waited out;
//   - batten's own status carries pause_requested: a Stuck whose Path is `pause requested at <time>`;
//     the engine then pauses at the row boundary;
//   - the stat of the child's status file fails: a Stuck naming the file;
//     the next Call's status read decides;
//   - the context is cancelled or the status file does not decode: hard errors.
//
// A Call that ends nothing returns Yield with an empty pointer, which persists nothing and ends the pass;
// it is logged at Debug.
// The next pass runs the Call again.
// A file that ends a wait between passes, such as a stop report or a strand's end, is seen at that pass.
//
// Every Stuck this Call returns routes back to this row: the row's OnStuck is a fixed per-producer value naming the row.
// Every such Stuck is exempt from the bounce budget and follows a real event,
// so a loop of them can never spend the budget before a halt.
//
// A running child has no time limit of its own.
// `lym batten pause`, honoured within one check, cancellation and the notices bound its wait.
// A running child whose driver is dead and whose status file does not change yields on every Call;
// its driver-dead notice is the only signal.
//
// Errors from `ResolveStatus` and from `ReadStatus` are hard errors, not verdicts:
// the task worktree is required to exist by the time this row runs,
// and a status file that exists but does not decode, or a status lock that cannot be taken, is a mechanism failure.
func (p *innerRunProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.loadMemory(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	defer p.saveMemory()

	statusPath, statusLockPath, err := p.deps.ResolveStatus()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: resolve status path: %w", p.name, err)
	}

	p.refreshWatched(ctx)

	// The baseline is stat'ed before the status read,
	// so a write that lands between the two is seen by the first check.
	baseline := statusModTime(statusPath)
	status, found, err := p.deps.ReadStatus(statusPath, statusLockPath)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read status: %w", p.name, err)
	}

	confirmedPath := SpawnConfirmedFile(p.scratchDir, p.name)
	mustSpawn := !found
	respawning := false
	if found && status.State != shedengine.StateRunning {
		p.respawns = 0
	}
	if found && status.State == shedengine.StateRunning && !spawnConfirmed(confirmedPath) {
		driverAtWork, err := p.driverAtWork(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if driverAtWork {
			logger.Info("battenshed: adopting the running inner shed run; its driver is already at work", "producer", p.name, "slug", p.slug)
			recordSpawnConfirmed(p.name, p.slug, p.scratchDir, confirmedPath)
		} else {
			// A later write to the status file is progress, a live driver strand or a held lock alone is not.
			if baseline.After(p.respawnStatusTime) {
				p.respawns = 0
			}
			if p.respawns >= respawnCap {
				logger.Debug("battenshed: not respawning the running inner shed run; it was respawned the cap of times with no status write between", "producer", p.name, "slug", p.slug, "respawns", p.respawns)
			} else {
				mustSpawn = true
				respawning = true
			}
		}
	}
	if mustSpawn {
		if end := p.holdSpawnWhileCovered(ctx); end != nil {
			return end.results()
		}
		if err := os.Remove(confirmedPath); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear spawn confirmation: %w", p.name, err)
		}
		logger.Info("battenshed: spawning inner shed run", "producer", p.name, "slug", p.slug, "status_found", found)
		spawnErr := p.deps.Spawn(ctx)
		logger.Info("battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
		if spawnErr != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			if owner, owned := spawnOwnedElsewhere(spawnErr); owned {
				w := p.cur
				w.status = status
				p.ownedElsewhere(ctx, w, owner, "")
				return p.exemptEnd(fmt.Sprintf("the run is owned by machine %s: not spawning its inner shed run", owner)).results()
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: spawn inner shed run (resuming this run retries the spawn): %w", p.name, spawnErr)
		}
		recordSpawnConfirmed(p.name, p.slug, p.scratchDir, confirmedPath)

		baseline = statusModTime(statusPath)
		if respawning {
			p.respawns++
			p.respawnStatusTime = baseline
		}
		status, found, err = p.deps.ReadStatus(statusPath, statusLockPath)
		if err != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read status: %w", p.name, err)
		}
		if !found {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: spawn inner shed run returned success but no status file was found", p.name)
		}
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}

	if status.State != shedengine.StateDone {
		if err := os.Remove(doneSeenFile(p.scratchDir, p.name)); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear stale done-seen marker: %w", p.name, err)
		}
	}
	switch status.State {
	case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
	default:
		// A child seen out of a halted state ends the halt episode, so a re-halt Warns again even at the same history length.
		if err := os.Remove(haltWarnedFile(p.scratchDir, p.name)); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear halt-warned marker: %w", p.name, err)
		}
	}

	switch status.State {
	case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed, shedengine.StateAwaiting:
	default:
		// A child seen out of a halted or awaiting state ends the episode,
		// so the next halt revives afresh.
		if err := os.Remove(revivedFile(p.scratchDir, p.name)); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear revived marker: %w", p.name, err)
		}
	}

	// The state this Call begins with is the episode's state when the memory holds one, otherwise the state just read.
	begun := status.State
	if p.episode.known {
		begun = p.episode.state
	}
	p.observeState(status.State, baseline)

	w := p.cur
	w.statusPath, w.statusLockPath, w.begun, w.status, w.baseline = statusPath, statusLockPath, begun, status, baseline
	if status.State != begun {
		p.coverage(w)
		p.foldOwnerTrigger(w)
		p.ownerStep(ctx, w)
		return p.exemptEnd(stateChangeText(begun, status)).results()
	}
	switch status.State {
	case shedengine.StateDone:
		return p.once(ctx, w, p.doneStep)
	case shedengine.StateRunning:
		return p.once(ctx, w, p.runningStep)
	case shedengine.StateAwaiting:
		return p.once(ctx, w, p.awaitingStep)
	case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
		return p.once(ctx, w, p.haltedStep)
	default:
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: unrecognized status state %q", p.name, status.State)
	}
}

// statusModTime returns the modification time of the status file at path, or the zero time when it cannot be stat'ed.
// A zero baseline differs from any real time, so the first check decodes the file,
// and a stat failure is the check's own to report.
func statusModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// driverAtWork reports whether the child already has a driver at work:
// a live or retiring driver strand, a held child run lock, or a held Go driver lock.
// A retiring strand counts as live, since someone already asked to remove it and `lym loom start` replaces it.
// A held driver lock is a Go driver settling between steps, when the run lock is free.
// A read error from any seam is returned as a hard error, and the next Call retries.
func (p *innerRunProducer) driverAtWork(ctx context.Context) (bool, error) {
	strand, err := p.deps.DriverStrand(ctx)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return false, cerr
		}
		return false, fmt.Errorf("battenshed: %s: read child driver strand: %w", p.name, err)
	}
	held, err := p.deps.ChildRunLockHeld()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return false, cerr
		}
		return false, fmt.Errorf("battenshed: %s: read child run lock: %w", p.name, err)
	}
	driverLockHeld, err := p.deps.ChildDriverLockHeld()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return false, cerr
		}
		return false, fmt.Errorf("battenshed: %s: read child driver lock: %w", p.name, err)
	}
	return strand == ChildDriverLive || strand == ChildDriverRetiring || held || driverLockHeld, nil
}

// childWait is the state of one Call's check of the child.
// It keeps the part of that state the memory record carries from pass to pass.
type childWait struct {
	statusPath     string
	statusLockPath string
	// begun is the state the Call began with; a decoded state that differs ends the check.
	begun shedengine.State
	// status is the newest decoded status; baseline is the status file's modification time when it was decoded.
	status   shedengine.Status
	baseline time.Time
	// lastProbe is when a check last did the work that costs a process or a multiplexer round trip,
	// and probeDue is true on the check that does it.
	lastProbe time.Time
	probeDue  bool
	// reason is the wait reason last written to the reason file; reasonKeys holds the keys reasons were logged under.
	reason     string
	reasonKeys map[string]bool
	// pauseWarned is the pause-flag read error last warned about, so a persistent one warns once.
	pauseWarned string
	// pause is the newest reading of the slug's pause coverage; coverageWarned is the read error last warned about.
	pause          pauserecord.Coverage
	coverageWarned string

	// revival is the halted arm's newest reading of the child's driver strand.
	revival driverRevival
	// alive is the newest strand reading; aliveKnown is whether it succeeded; aliveStale is true on a probe until read.
	alive      bool
	aliveKnown bool
	aliveStale bool
	// resumeRetryAt is when the awaiting arm may try a resume again after the child's driver refused one as not parked yet.
	// notParkedLogged is whether that refusal was logged already.
	resumeRetryAt   time.Time
	notParkedLogged bool
	// busyLogged is whether a busy-pane refusal was logged already; busyReason is that refusal's report, empty otherwise.
	// busyKey is busyReason without its spawn-error tail, which carries the driver pane's draft text.
	busyLogged bool
	busyReason string
	busyKey    string

	// lock is the parent lock step's state across checks.
	lock lockWatch
}

// waitEnd is how a check ended: the verdict, pointer and error Call returns.
type waitEnd struct {
	outcome shedengine.Outcome
	pointer shedengine.OutputPointer
	err     error
}

// armStep is one arm's part of a check: its actions and its wait reason.
// It returns nil when the check ends nothing.
type armStep func(ctx context.Context, w *childWait) *waitEnd

// hardEnd ends the check with a returned error.
func hardEnd(err error) *waitEnd {
	return &waitEnd{err: err}
}

// exemptEnd ends the check with a budget-exempt Stuck whose Path and Reason both carry text,
// so each cause is its own history entry and equal entries never fold across causes.
// It logs at Info, since its callers end the check on a state change or a requested pause.
func (p *innerRunProducer) exemptEnd(text string) *waitEnd {
	reportWait(p.name, text, p.scratchDir, "slug", p.slug)
	p.cur.reason = text
	return stuckWaitEnd(text)
}

// failedEnd is the verdict of a check that ends because a read of the child failed.
// It ends the check with a budget-exempt Stuck whose Path and Reason both carry text.
// The verdict logs at Warn.
func (p *innerRunProducer) failedEnd(text string) *waitEnd {
	reportStuck(p.name, text, p.scratchDir, "slug", p.slug)
	p.cur.reason = text
	return stuckWaitEnd(text)
}

// stuckWaitEnd is the budget-exempt Stuck verdict whose Path and Reason both carry text.
func stuckWaitEnd(text string) *waitEnd {
	return &waitEnd{outcome: shedengine.Stuck, pointer: shedengine.OutputPointer{Path: text, Reason: text, BudgetExempt: true}}
}

// report writes reason to the reason file when it differs from the last one written,
// so `lym batten status` shows what the child is waited on for.
// It logs the reason at Info only the first time key is seen,
// so a reason whose text only counts up logs once per state of the child.
// An arm can alternate between reasons, so a set of keys, not the last key, keeps each from logging on every check.
// key is the reason with its counters, countdowns and error tails left out.
func (p *innerRunProducer) report(w *childWait, reason, key string) {
	if reason == w.reason {
		return
	}
	w.reason = reason
	if w.reasonKeys[key] {
		writeStuckReason(p.name, reason, p.scratchDir)
		return
	}
	if w.reasonKeys == nil {
		w.reasonKeys = map[string]bool{}
	}
	w.reasonKeys[key] = true
	reportWait(p.name, reason, p.scratchDir, "slug", p.slug)
}

// reportState reports a reason that carries no counter, countdown or error tail, so the reason is its own key.
func (p *innerRunProducer) reportState(w *childWait, reason string) {
	p.report(w, reason, reason)
}

// results unpacks a wait end into the values Call returns.
func (e *waitEnd) results() (shedengine.Outcome, shedengine.OutputPointer, error) {
	return e.outcome, e.pointer, e.err
}

// once runs the one check of a Call: the arm's step and everything around it.
// It ends the Call when the check ends something, and it yields when the check ends nothing.
// File stats and small file reads run on every check;
// anything costing a process or a multiplexer round trip runs only on a probe check.
// A probe check is due when the memory holds no probe or the last one is noticeProbe old.
// A yield persists nothing in batten's status, so the next pass checks again.
func (p *innerRunProducer) once(ctx context.Context, w *childWait, step armStep) (shedengine.Outcome, shedengine.OutputPointer, error) {
	now := p.deps.Now()
	w.probeDue = w.lastProbe.IsZero() || now.Sub(w.lastProbe) >= p.noticeProbe
	if w.probeDue {
		w.lastProbe = now
		w.aliveStale = true
	}
	if end := p.check(ctx, w, step, now); end != nil {
		return end.results()
	}
	logger.Debug("battenshed: inner shed run yields; the check ended nothing", "producer", p.name, "slug", p.slug, "state", w.status.State)
	return shedengine.Yield, shedengine.OutputPointer{}, nil
}

// check is one check of the child.
// It reads the child's status file and runs the owner check when it is due.
// On a probe check it also runs the watched-marker refresh and the parent lock step.
// Then it runs the notice step and the arm's step, and last it checks batten's own pause flag.
// It returns nil when the check ends nothing.
// While the pause record covers the slug, a state change still ends the check and a halted child's notice still goes,
// but the arm's step does not run.
func (p *innerRunProducer) check(ctx context.Context, w *childWait, step armStep, now time.Time) *waitEnd {
	p.coverage(w)
	p.foldOwnerTrigger(w)
	if end := p.checkChild(ctx, w); end != nil {
		// Only an end that read a new status runs the owner check;
		// a failed stat or a vanished file leaves it due for the next Call.
		if w.status.State != w.begun {
			p.ownerStep(ctx, w)
		}
		return end
	}
	p.ownerStep(ctx, w)
	if w.probeDue {
		p.lockStep(ctx, w)
	}
	p.noticeStep(ctx, w)
	if end := p.runArm(ctx, w, step); end != nil {
		return end
	}
	if p.pauseRequested(w) {
		return p.exemptEnd("pause requested at " + now.Format(time.RFC3339))
	}
	return nil
}

// checkChild stats the child's status file.
// It decodes the file again when its modification time moved since the Call decoded it.
// It ends the check when the stat fails or the child's state is no longer the one the Call began with.
// A decode error is a hard error, and a status file that vanished between the stat and the read is a stat failure.
func (p *innerRunProducer) checkChild(ctx context.Context, w *childWait) *waitEnd {
	info, err := os.Stat(w.statusPath)
	if err != nil {
		return p.failedEnd(fmt.Sprintf("the child's status file %s cannot be read: %v", w.statusPath, err))
	}
	if info.ModTime().Equal(w.baseline) {
		return nil
	}
	status, found, err := p.deps.ReadStatus(w.statusPath, w.statusLockPath)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return hardEnd(cerr)
		}
		return hardEnd(fmt.Errorf("battenshed: %s: read status: %w", p.name, err))
	}
	if !found {
		return p.failedEnd(fmt.Sprintf("the child's status file %s is gone", w.statusPath))
	}
	w.baseline = info.ModTime()
	w.status = status
	if status.State != w.begun {
		p.observeState(status.State, info.ModTime())
		return p.exemptEnd(stateChangeText(w.begun, status))
	}
	return nil
}

// stateChangeText renders a child's state change as wait-reason text, naming the producer its new status is at.
// The text is parsed back elsewhere, so the format and the parser change together.
func stateChangeText(from shedengine.State, to shedengine.Status) string {
	text := fmt.Sprintf("child %s → %s", from, to.State)
	if to.CurrentProducer != "" {
		text += " at " + to.CurrentProducer
	}
	return text
}

// pauseRequested reports whether batten's own status carries pause_requested.
// The flag is only the operator's early exit, so a read error reads as not paused.
// Each distinct read error is warned about once.
func (p *innerRunProducer) pauseRequested(w *childWait) bool {
	paused, err := p.deps.PauseRequested()
	if err != nil {
		if msg := err.Error(); msg != w.pauseWarned {
			w.pauseWarned = msg
			logger.Warn("battenshed: could not read batten's own pause flag", "producer", p.name, "slug", p.slug, "error", err)
		}
		return false
	}
	w.pauseWarned = ""
	return paused
}

// runningStep is the running arm: it reports the plain wait reason.
func (p *innerRunProducer) runningStep(ctx context.Context, w *childWait) *waitEnd {
	p.reportState(w, "inner shed run still running; dobby re-checks it on each probe or child status write")
	return nil
}

// haltedStep is the halted arm for a blocked, paused or failed child.
// Each halt episode gets one Info log line and one revival attempt for a dead driver strand.
// It reports the resume command the operator runs.
// It never spawns or resumes the child, except for one re-step of a driverless go child on a new lym build.
// The child is waited on until its state changes.
func (p *innerRunProducer) haltedStep(ctx context.Context, w *childWait) *waitEnd {
	if err := p.warnHaltEpisode(w.status); err != nil {
		return hardEnd(err)
	}
	if w.probeDue {
		w.revival = p.reviveDeadDriver(ctx)
	}
	if end := p.binaryRestep(ctx, w); end != nil {
		return end
	}
	p.reportState(w, haltedWaitReason(w.status, w.revival.startFallback))
	return nil
}

// warnHaltEpisode logs one Info line per halt episode.
// An episode ends when the child is seen out of a halted state, which removes the halt-warned marker.
// It also ends when the child's history length differs from the length the marker recorded at the last logged line:
// a re-halt after an observed resume logs again even at the same history length;
// since a hard producer error or a mid-call pause appends no history entry.
// The checks within one episode stay quiet.
// A marker read or write failure is a hard error, as the other markers' are.
func (p *innerRunProducer) warnHaltEpisode(status shedengine.Status) error {
	markerPath := haltWarnedFile(p.scratchDir, p.name)
	raw, err := os.ReadFile(markerPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("battenshed: %s: read halt-warned marker: %w", p.name, err)
	}
	historyLen := len(status.History)
	recorded, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err == nil && parseErr == nil && recorded == historyLen {
		return nil
	}
	logger.Info("battenshed: inner shed run halted; waiting for the operator to resume it", "producer", p.name, "slug", p.slug, "state", status.State, "error", status.Error, "current_producer", status.CurrentProducer)
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		return fmt.Errorf("battenshed: %s: create scratch directory for halt-warned marker: %w", p.name, err)
	}
	if err := os.WriteFile(markerPath, []byte(strconv.Itoa(historyLen)+"\n"), 0o644); err != nil {
		return fmt.Errorf("battenshed: %s: write halt-warned marker: %w", p.name, err)
	}
	// A new halt episode is a new chance to revive, however the episode was told apart from the last one.
	if err := os.Remove(revivedFile(p.scratchDir, p.name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("battenshed: %s: clear revived marker: %w", p.name, err)
	}
	return nil
}

// driverRevival reports what a revive of the child's driver strand found.
type driverRevival struct {
	// startFallback is true when the halted reason should also name `lym loom start`:
	// the child has no driver strand in reed state, or a revive failed in this episode.
	startFallback bool
}

// reviveDeadDriver brings back the dead driver strand of a halted or awaiting child, at most once per episode.
// It does so by reviving the child's strands.
// A retiring strand is left to `lym loom start`.
// A live strand needs nothing.
// A child with no driver strand has nothing to revive, which it never tells from a Go-driven child by the seed.
// The attempt is recorded in the revived marker, which stands until the episode ends;
// its presence alone says the episode was tried, since each pass is a new process.
// It changes no run state and removes no park marker.
// Every failure is warned about and never fails the row; a repeating strand is read once per episode.
func (p *innerRunProducer) reviveDeadDriver(ctx context.Context) driverRevival {
	strand, err := p.deps.DriverStrand(ctx)
	if err != nil {
		p.seamFailed(failDriverStrandRead, err)
		return driverRevival{}
	}
	p.seamRecovered(failDriverStrandRead)
	switch strand {
	case ChildDriverNone:
		return driverRevival{startFallback: true}
	case ChildDriverDead:
	default:
		return driverRevival{}
	}

	markerPath := revivedFile(p.scratchDir, p.name)
	if _, err := os.Stat(markerPath); err == nil {
		// Already tried in this episode,
		// and the strand is dead still.
		return driverRevival{startFallback: true}
	}

	logger.Info("battenshed: reviving the halted child's dead driver strand", "producer", p.name, "slug", p.slug)
	reviveErr := p.deps.ReviveStrands(ctx)
	after, readErr := p.deps.DriverStrand(ctx)
	revived := reviveErr == nil && readErr == nil && after == ChildDriverLive
	result := "failed"
	if revived {
		result = "ok"
	} else {
		logger.Warn("battenshed: the revive left the child's driver strand not live", "producer", p.name, "slug", p.slug, "revive_error", reviveErr, "read_error", readErr, "strand", after)
	}
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for revived marker failed", "producer", p.name, "slug", p.slug, "scratchDir", p.scratchDir, "error", err)
	} else if err := os.WriteFile(markerPath, []byte(strconv.Itoa(os.Getpid())+" "+result+"\n"), 0o644); err != nil {
		logger.Warn("battenshed: write revived marker failed", "producer", p.name, "slug", p.slug, "path", markerPath, "error", err)
	}
	return driverRevival{startFallback: !revived}
}

// awaitingStep is the awaiting arm for a child halted at a human hand-off.
// It reports the hand-off while no decision exists, and it resumes the child once per decision.
// A decision already acted on has two outcomes.
// They are told apart by the child's history length against the length the marker recorded at the resume:
// an equal length means the resume was delivered and the child's driver has not re-stepped yet,
// and any other length (or an old-layout marker) means the child is awaiting again and gets the decide-again hint.
// The length is a clock-free discriminator.
// A re-await appends at least the child's own Awaiting entry, which the history fold never folds onto.
// The wait goes on after a resume until the child leaves awaiting.
func (p *innerRunProducer) awaitingStep(ctx context.Context, w *childWait) *waitEnd {
	if w.probeDue {
		p.reviveDeadDriver(ctx)
	}
	decision, found, err := p.deps.ReadDecision()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return hardEnd(cerr)
		}
		return hardEnd(fmt.Errorf("battenshed: %s: read decision: %w", p.name, err))
	}
	if !found {
		p.reportState(w, fmt.Sprintf("inner shed run is awaiting a human hand-off (%s); %s", w.status.Error, awaitingHandOff))
		return nil
	}

	markerPath := decisionActedFile(p.scratchDir, p.name)
	identity := decisionIdentity(decision)
	acted, err := os.ReadFile(markerPath)
	if err != nil && !os.IsNotExist(err) {
		return hardEnd(fmt.Errorf("battenshed: %s: read decision-acted marker: %w", p.name, err))
	}
	if err == nil {
		actedIdentity, actedLen, hasLen := parseDecisionActed(string(acted))
		if actedIdentity == identity {
			if hasLen && len(w.status.History) == actedLen {
				p.reportState(w, fmt.Sprintf("the %s at %s was acted on and the resume was delivered, but the child's driver has not re-stepped yet; if this persists, inspect a live driver in its pane, or run \"lym loom start\" in the task worktree if the driver has ended", decision.Kind, decision.At))
				return nil
			}
			p.reportState(w, fmt.Sprintf("the %s at %s was already acted on and the child is awaiting again; decide again: %s", decision.Kind, decision.At, awaitingHandOff))
			return nil
		}
	}

	resumed, end := p.resumeAwaitingChild(ctx, w, fmt.Sprintf("the %s at %s", decision.Kind, decision.At))
	if end != nil || !resumed {
		return end
	}
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		return hardEnd(fmt.Errorf("battenshed: %s: create scratch directory for decision-acted marker: %w", p.name, err))
	}
	if err := os.WriteFile(markerPath, []byte(decisionActedContent(identity, len(w.status.History))), 0o644); err != nil {
		return hardEnd(fmt.Errorf("battenshed: %s: write decision-acted marker: %w", p.name, err))
	}
	p.reportState(w, fmt.Sprintf("the %s at %s was acted on: the child was resumed; watching it", decision.Kind, decision.At))
	return nil
}

// resumeAwaitingChild resumes an awaiting child through the spawn dependency, and reports whether the resume landed.
// subject names what the resume answers, for the wait reasons ("the approve at <time>").
// A spawn refused because the child is not parked or its driver is busy leaves subject unacted:
// it is retried at most once per notice probe, reporting the refusal as the wait reason.
// Any other spawn error is a hard error.
// A resume that landed has recorded the spawn and opened the terminal window once,
// and it leaves the wait reason to the caller.
func (p *innerRunProducer) resumeAwaitingChild(ctx context.Context, w *childWait, subject string) (resumed bool, end *waitEnd) {
	if p.ownedBy != "" {
		p.reportState(w, p.ownerHoldReason())
		return false, nil
	}
	notParkedReason := fmt.Sprintf("%s is not acted on yet: the child's driver has not parked; retrying the resume", subject)
	if now := p.deps.Now(); now.Before(w.resumeRetryAt) {
		if w.busyReason != "" {
			p.report(w, w.busyReason, w.busyKey)
			return false, nil
		}
		p.reportState(w, notParkedReason)
		return false, nil
	}

	first := !w.notParkedLogged && !w.busyLogged
	logAttempt(first, "battenshed: resuming awaiting inner shed run", "producer", p.name, "slug", p.slug, "subject", subject)
	spawnErr := p.deps.Spawn(ctx)
	logAttempt(first, "battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
	if spawnErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return false, hardEnd(cerr)
		}
		if owner, owned := spawnOwnedElsewhere(spawnErr); owned {
			p.ownedElsewhere(ctx, w, owner, "")
			return false, nil
		}
		if errors.Is(spawnErr, ErrChildNotParked) {
			// The child's driver is between its stop and its park,
			// so the resume stays unacted and is retried at most once per noticeProbe.
			w.resumeRetryAt = p.deps.Now().Add(p.noticeProbe)
			w.busyReason, w.busyKey = "", ""
			if !w.notParkedLogged {
				w.notParkedLogged = true
				logger.Info("battenshed: child driver has not parked yet; retrying the resume until it does", "producer", p.name, "slug", p.slug, "error", spawnErr)
			}
			p.reportState(w, notParkedReason)
			return false, nil
		}
		if errors.Is(spawnErr, ErrChildDriverBusy) {
			// A draft or an active operator in the parked driver's pane only delays the resume:
			// it stays unacted and is retried at most once per noticeProbe.
			w.resumeRetryAt = p.deps.Now().Add(p.noticeProbe)
			w.busyKey = fmt.Sprintf("%s is not acted on yet: the child's driver pane holds a draft or the operator is typing in it; clear the draft in the driver's pane and batten resumes it", subject)
			w.busyReason = fmt.Sprintf("%s (%s)", w.busyKey, oneLine(spawnErr.Error(), noticeErrorMax))
			if !w.busyLogged {
				w.busyLogged = true
				logger.Info("battenshed: child driver pane is busy; retrying the resume until it is clear", "producer", p.name, "slug", p.slug, "error", spawnErr)
			}
			p.noticeDriverBusy(ctx, w, spawnErr)
			p.report(w, w.busyReason, w.busyKey)
			return false, nil
		}
		return false, hardEnd(fmt.Errorf("battenshed: %s: resume awaiting inner shed run (resuming this run retries the spawn): %w", p.name, spawnErr))
	}
	p.driverBusyNoticed = false
	w.busyReason, w.busyKey = "", ""
	recordSpawnConfirmed(p.name, p.slug, p.scratchDir, SpawnConfirmedFile(p.scratchDir, p.name))
	return true, nil
}

// doneStep is the done arm: it ends the wait with Done when the driver strand has ended.
// It also ends it once the grace window, counted from first sight, has elapsed.
// Otherwise it reports the wait for the driver's stop report.
// The driver strand is read on the probe checks only, and the grace is judged on every check.
func (p *innerRunProducer) doneStep(ctx context.Context, w *childWait) *waitEnd {
	markerPath := doneSeenFile(p.scratchDir, p.name)
	now := p.deps.Now()

	raw, err := os.ReadFile(markerPath)
	if err != nil && !os.IsNotExist(err) {
		return hardEnd(fmt.Errorf("battenshed: %s: read done-seen marker: %w", p.name, err))
	}
	var firstSeen time.Time
	fresh := err != nil
	if !fresh {
		var parseErr error
		firstSeen, parseErr = time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
		if parseErr != nil {
			// A marker that reads but does not parse (a write torn by a killed process) restarts the grace.
			// Failing instead would fail every Call: no resume could clear the marker, and it only bounds a wait.
			logger.Warn("battenshed: unparseable done-seen marker; restarting the driver-exit grace", "producer", p.name, "slug", p.slug, "path", markerPath, "error", parseErr)
			fresh = true
		}
	}
	if fresh {
		firstSeen = now
		if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
			return hardEnd(fmt.Errorf("battenshed: %s: create scratch directory for done-seen marker: %w", p.name, err))
		}
		if err := os.WriteFile(markerPath, []byte(now.Format(time.RFC3339)+"\n"), 0o644); err != nil {
			return hardEnd(fmt.Errorf("battenshed: %s: write done-seen marker: %w", p.name, err))
		}
	}

	finish := func() *waitEnd {
		if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
			return hardEnd(fmt.Errorf("battenshed: %s: remove done-seen marker: %w", p.name, err))
		}
		return &waitEnd{outcome: shedengine.Done}
	}

	alive, known := p.driverAlive(ctx, w)
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return hardEnd(cerr)
	}
	if known && !alive {
		return finish()
	}
	elapsed := now.Sub(firstSeen)
	if elapsed >= p.driverExitGrace {
		logger.Warn("battenshed: driver still live after the exit grace; finishing anyway", "producer", p.name, "slug", p.slug, "grace", p.driverExitGrace, "elapsed", elapsed)
		return finish()
	}
	if w.probeDue {
		p.report(w, fmt.Sprintf("inner shed run is done; waiting for its driver to finish its stop report (%s of %s grace elapsed)", elapsed.Round(time.Second), p.driverExitGrace),
			"inner shed run is done; waiting for its driver to finish its stop report")
	}
	return nil
}

// driverAlive reports whether the child has a live driver: a live driver strand, or a held driver lock or run lock.
// It reads them at most once per probe and reuses the answer on the checks between.
// known is false when the strand read failed;
// the done arm then treats the driver as live, and the notice step skips what needs the answer.
// A lock read error reads as alive.
// A failed strand or lock read warns once per episode.
func (p *innerRunProducer) driverAlive(ctx context.Context, w *childWait) (alive, known bool) {
	if w.aliveStale {
		w.aliveStale = false
		a, err := p.deps.DriverAlive(ctx)
		w.alive, w.aliveKnown = a, err == nil
		if err == nil {
			p.seamRecovered(failDriverLiveness)
		} else if ctx.Err() == nil {
			p.seamFailed(failDriverLiveness, err)
		}
		if !w.alive && w.aliveKnown {
			w.alive = p.driverLocksHeld(ctx)
		}
	}
	return w.alive, w.aliveKnown
}

// driverLocksHeld reports whether the child's driver lock or run lock is held.
// A read error warns once per episode and reads as held.
// A lock that cannot be read must not make a working driver look dead.
func (p *innerRunProducer) driverLocksHeld(ctx context.Context) bool {
	for _, lock := range []struct {
		name string
		held func() (bool, error)
	}{
		{"driver", p.deps.ChildDriverLockHeld},
		{"run", p.deps.ChildRunLockHeld},
	} {
		held, err := lock.held()
		if err != nil {
			if ctx.Err() == nil {
				p.seamFailed(failChildLockRead, err, "lock", lock.name)
			}
			return true
		}
		if held {
			return true
		}
	}
	p.seamRecovered(failChildLockRead)
	return false
}

// spawnConfirmed reports whether the spawn-confirmation marker at path names this process's own pid.
// A marker from an earlier process, one that does not hold a pid, and any read failure all report false.
// The cost of a wrong false is one idempotent re-spawn or adoption check;
// the cost of a wrong true is a driverless child watched as running for the whole bounce budget.
func spawnConfirmed(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	return err == nil && pid == os.Getpid()
}

// recordSpawnConfirmed writes the spawn-confirmation marker at path, holding this process's pid.
// A write failure is logged rather than escalated, since the spawn itself succeeded.
// A missing marker costs only one idempotent re-spawn or adoption check on the next Call.
func recordSpawnConfirmed(producer, slug, scratchDir, path string) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for spawn confirmation failed", "producer", producer, "slug", slug, "scratchDir", scratchDir, "error", err)
		return
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		logger.Warn("battenshed: write spawn confirmation failed", "producer", producer, "slug", slug, "path", path, "error", err)
	}
}

// watchedAnswer is how the producer last got the answer to whether it holds the batten-watched marker.
type watchedAnswer int

const (
	// watchedUnknown is the state before the seam was first asked.
	watchedUnknown watchedAnswer = iota
	// watchedHeld means the seam reported the marker held.
	watchedHeld
	// watchedNotHeld means the seam reported the marker not held.
	watchedNotHeld
	// watchedFailed means the seam returned an error, which reads as not held.
	watchedFailed
)

// refreshWatched asks deps.MarkWatched whether this batten holds the batten-watched marker and records the answer.
// An error reads as not held and is warned about once per change of answer.
// The marker only decides who messages the parent and never the row's outcome.
func (p *innerRunProducer) refreshWatched(ctx context.Context) {
	held, err := p.deps.MarkWatched(ctx)
	answer := watchedNotHeld
	switch {
	case err != nil:
		answer = watchedFailed
		held = false
		if p.watchedAnswer != watchedFailed {
			logger.Warn("battenshed: could not write or remove the batten-watched marker; reading it as not held", "producer", p.name, "slug", p.slug, "error", err)
		}
	case held:
		answer = watchedHeld
	}
	p.watched = held
	p.watchedAnswer = answer
}
