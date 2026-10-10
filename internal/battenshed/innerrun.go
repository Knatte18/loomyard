// innerrun.go implements NewInnerRun, the producer that spawns the inner shed run inside the task worktree, if it has not spawned yet.
// It then waits inside its Call on the child's persisted status until something worth a history entry happens:
// the child's state changes, its arm's own event fires, batten is paused, or the child's status file cannot be stat'ed.

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
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// pollFloor is the shortest interval the producer's in-call wait checks its child at; a told interval below it is floored to it.
const pollFloor = time.Second

// waitOrCancel pauses for d, returning as soon as ctx is cancelled if that happens first.
// It is the production value a nil InnerRunDeps.Sleep resolves to, so a caller driving the producer
// under a cancellable context gets it back promptly. The lyx CLI itself never cancels its context:
// an operator's Ctrl-C ends the process outright, and "lyx batten pause" is read between rows, so
// it waits out the current interval.
func waitOrCancel(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// spawnConfirmedFileSuffix is the fixed suffix of the marker a producer writes under its scratch
// directory once its spawn has returned success, joined onto the producer's own name.
// The marker holds the pid of the process that wrote it,
// so it confirms the spawn for that process only.
const spawnConfirmedFileSuffix = "-spawned"

// SpawnConfirmedFile returns the path of the marker innerRunProducer writes under scratchDir once
// producer's spawn has returned success.
// It is exported for the same reason StuckReasonFile is: the file's reader and writer share one
// declarer of its name.
func SpawnConfirmedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+spawnConfirmedFileSuffix)
}

// haltedWaitReason renders the stuck reason of a halted child's wait.
// Nothing on the prime side resumes the child's own driver,
// so the reason names the operator's resume command and says this run keeps watching.
// startFallback adds `lyx loom start`, for a child with no driver strand to wake or whose revive failed.
func haltedWaitReason(status shedengine.Status, startFallback bool) string {
	resume := `run "lyx loom resume" in the task worktree to resume it`
	if startFallback {
		resume = `run "lyx loom resume" in the task worktree to resume it, or "lyx loom start" in the task worktree when no driver can be woken`
	}
	return fmt.Sprintf("inner shed run is %s: error=%q current_producer=%q; %s; this run then keeps watching", status.State, status.Error, status.CurrentProducer, resume)
}

// haltWarnedFileSuffix is the fixed suffix of the marker recording the child's history length at the last halt Warn, joined onto the producer's own name.
const haltWarnedFileSuffix = "-halt-warned"

// haltWarnedFile returns the path of the marker holding the child's history length, in decimal, at the last halt Warn.
func haltWarnedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+haltWarnedFileSuffix)
}

// revivedFileSuffix is the fixed suffix of the marker recording that this batten process already tried a driver revive in the current halt episode, joined onto the producer's own name.
// It holds the process's pid and the attempt's result, `ok` or `failed`, on one line.
const revivedFileSuffix = "-revived"

// revivedFile returns the path of the revive once-marker.
func revivedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+revivedFileSuffix)
}

// decisionActedFileSuffix is the fixed suffix of the marker recording the decision identity the producer last resumed the child on, followed by the child's history length at that resume, joined onto the producer's own name.
// A one-line marker reads as the old layout.
const decisionActedFileSuffix = "-decision-acted"

// doneSeenFileSuffix is the fixed suffix of the marker recording when the producer first saw the child done, joined onto the producer's own name.
const doneSeenFileSuffix = "-done-seen"

// decisionActedFile returns the path of the marker holding the decision identity already resumed on and the child's history length at that resume;
// a one-line marker reads as the old layout.
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
// hasLen is false for the one-line old layout, an unparseable or negative length (a torn write), and input with no newline, which is returned whole as identity so it never matches a real identity line.
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

// awaitingHandOff is the operator instruction an awaiting child's wait carries: the child waits either on a pull-request decision or on a review segment's escalation, and only the operator can settle either from inside the task worktree.
// A pull-request decision is resumed by this run itself; an escalation's circling decision is resumed by the operator's own `lyx loom resume`.
const awaitingHandOff = "at PR-Gate run \"lyx loom approve\" or \"lyx loom reject\" in the task worktree, after which this run resumes the child itself; at a review segment's escalation run \"lyx loom circling accept <slug>\" or \"lyx loom circling continue <slug>\" and then \"lyx loom resume\" in the task worktree"

// innerRunProducer spawns the inner shed run for a task worktree, once, and then waits inside its Call on the child's persisted status.
// A Call returns Stuck, always budget-exempt, only when the wait has something to record, so shedengine's own on_stuck self-route re-enters it as a new bounce.
type innerRunProducer struct {
	name         string
	slug         string
	deps         InnerRunDeps
	pollInterval time.Duration
	// noticeProbe is deps.NoticeProbe: how rarely the wait does anything costing a process or a multiplexer round trip.
	noticeProbe time.Duration
	// watched is the last answer of deps.MarkWatched, whether this batten holds the batten-watched marker, and watchedAnswer how it was reached.
	watched       bool
	watchedAnswer watchedAnswer
	// episode is the child's current state episode and the delivery of its notices, across Calls.
	episode    noticeEpisode
	scratchDir string
	// driverExitGrace bounds how long a done child's live driver strand is waited for.
	driverExitGrace time.Duration
	// notices is true when the caller wired a Notify, before a nil one resolves to a no-op.
	notices bool
	// noticeQuiet is deps.NoticeQuiet, read once at construction.
	noticeQuiet time.Duration
}

var _ shedengine.ShedProducer = (*innerRunProducer)(nil)

// NewInnerRun returns a shedengine.ShedProducer that resolves the task worktree's status path and spawns the inner shed run via deps.Spawn the first time Call finds no status file.
// It then waits on the child inside the call, checking every pollInterval through deps.Sleep.
// A pollInterval below one second is floored to one second with one Warn naming poll_interval_s.
//
// driverExitGrace bounds the wait for a done child's driver strand to end before the row returns Done anyway.
//
// A nil deps.Sleep resolves to waitOrCancel, and a nil deps.Now to time.Now, once here rather than on every Call, so a test's no-op sleep and fixed clock are the only values ever substituted.
// A nil deps.Notify resolves to a no-op and switches the notice step off.
// A nil deps.OrchStrandRecorded resolves to reporting a strand recorded, and a nil deps.StopReport to reporting no stop report.
// A nil deps.DriverStrand resolves to reporting no driver strand, a nil deps.ChildRunLockHeld to reporting no held lock and a nil deps.ReviveStrands to a revive that fails as not wired.
// A nil deps.PauseRequested resolves to never paused.
// A nil deps.MarkWatched resolves to reporting not held, and a nil deps.Activity to reporting no live run and no live wait.
func NewInnerRun(name, slug string, deps InnerRunDeps, pollInterval time.Duration, scratchDir string, driverExitGrace time.Duration) shedengine.ShedProducer {
	if pollInterval < pollFloor {
		logger.Warn("battenshed: poll_interval_s is below the floor and is raised to it", "key", "poll_interval_s", "value", pollInterval, "floor_s", int(pollFloor.Seconds()))
		pollInterval = pollFloor
	}
	if deps.PauseRequested == nil {
		deps.PauseRequested = func() (bool, error) { return false, nil }
	}
	if deps.MarkWatched == nil {
		deps.MarkWatched = func(context.Context) (bool, error) { return false, nil }
	}
	if deps.Activity == nil {
		deps.Activity = func(context.Context) ([]AgentActivity, bool, error) { return nil, false, nil }
	}
	if deps.Sleep == nil {
		deps.Sleep = waitOrCancel
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
	if deps.AttachDir == nil {
		deps.AttachDir = func() (string, error) { return "", errors.New("no attach directory wired") }
	}
	if deps.DriverStrand == nil {
		deps.DriverStrand = func(context.Context) (ChildDriverStrand, error) { return ChildDriverNone, nil }
	}
	if deps.ChildRunLockHeld == nil {
		deps.ChildRunLockHeld = func() (bool, error) { return false, nil }
	}
	if deps.ReviveStrands == nil {
		deps.ReviveStrands = func(context.Context) error { return errors.New("no strand revive wired") }
	}
	return &innerRunProducer{
		name:         name,
		slug:         slug,
		deps:         deps,
		pollInterval: pollInterval,
		noticeProbe:  deps.NoticeProbe,
		scratchDir:   scratchDir,

		driverExitGrace: driverExitGrace,
		notices:         notices,
		noticeQuiet:     deps.NoticeQuiet,
	}
}

// Call implements shedengine.ShedProducer.
//
// It reads the child's status before doing anything else.
// It spawns only when that status is absent, or is still running with no spawn confirmed by this batten process and no driver at work.
// The child's status file alone cannot say whether a spawn happened:
// the child's bootstrap seeds it as running before it starts the driver,
// so a bootstrap that failed or was killed after seeding leaves a running status with no driver behind it.
// The confirmation is a marker under scratchDir (SpawnConfirmedFile) holding the pid of the process that wrote it,
// cleared before every spawn attempt and written only once deps.Spawn returns success;
// a marker naming another pid, or in the old layout, reads as unconfirmed,
// since batten's own run lock means a pid other than this process's is an earlier, dead batten process.
// A running child with an unconfirmed spawn is adopted rather than spawned when deps.DriverStrand reports a live or retiring driver strand or deps.ChildRunLockHeld reports a held run lock:
// the confirmation is recorded for this process and nothing is spawned.
// Re-spawning a running child with neither is safe because the bootstrap it runs is idempotent against a driver that is already alive,
// and it never stacks a second driver.
// A halted or done child is never re-spawned, marker or not: the outer run watches the child's own run and never restarts it.
//
// The full disposition table, evaluated top to bottom: a spawn as above (logging both Live-Substrate
// Spawn Observability lines around deps.Spawn), then one more read;
// deps.Spawn returning an error is a hard error, not Stuck, since a failed spawn is mechanism failure, not an ordinary wait, and the next Call retries it; still no status file after a successful spawn is a hard error naming the spawn that returned success without producing one.
// Any Call that finds the child in a state other than done first removes a leftover done-seen marker, so a marker from an earlier run of the same slug never shortens a later wait.
// Any Call that finds the child out of a halted state likewise removes the halt-warned marker, which ends the halt episode.
// Once the child's state is settled, the wait runs the notice step (noticeStep) on entry and at every check: informational only, one notice per condition per episode, never changing the outcome below.
// Then by state, each arm does its actions and enters the one wait loop (wait):
//   - running reports its wait reason, naming the reviewer the child waits on when there is one;
//   - awaiting first revives a dead driver strand once per episode; with no decision record it reports the hand-off;
//   - awaiting with a decision not yet acted on spawns the child's driver again (the child's own bootstrap resumes an approved or rejected run), records the decision in the decision-acted marker only once the spawn succeeded, and goes on waiting until the child leaves awaiting;
//     a spawn refused with ErrChildNotParked records nothing and is retried at most once per noticeProbe until the driver parks or the state changes;
//   - awaiting with a decision already acted on and the child's history length unchanged since that resume does not spawn, and reports that the resume was delivered and the child's driver has not re-stepped yet;
//   - awaiting with a decision already acted on and the child's history longer (or an old-layout marker) does not spawn, and reports the recovery of deciding again;
//   - done records the first-sight time in the done-seen marker and returns Done once the driver strand is gone or driverExitGrace has elapsed since first sight, and otherwise reports the wait for the driver to finish its stop report;
//   - blocked, paused or failed never spawns or resumes the child, Warns once per halt episode, revives a dead driver strand once per episode (reviveDeadDriver), and reports the child's State, Error and CurrentProducer and the resume command;
//     a resumed child is then read as running again;
//   - any other value is a hard error naming the unrecognised state.
//
// The wait loop sleeps pollInterval, then checks, and returns only when:
//   - the child's State differs from the one the Call began with: a Stuck whose Path and Reason name the change (`child running → blocked at Plan-Review`), so each change is its own history entry and equal entries never fold across changes;
//   - the arm's own event fires: the done arm's Done, or an awaiting child's decision, which is acted on and then waited out;
//   - batten's own status carries pause_requested: a Stuck whose Path is `pause requested at <time>`, after which the engine pauses at the row boundary;
//   - the stat of the child's status file fails: a Stuck naming the file, leaving the next Call's status read to decide;
//   - the context is cancelled or the status file does not decode: hard errors.
//
// The self-route's "sole Stuck arm" reasoning still holds in the sense it exists for:
// ProducerDef.OnStuck is a static per-producer value, so every Stuck this row returns routes back to the same self-route target.
// Every Stuck here is budget-exempt and follows a real event, so the self-route can never burn the bounce budget in a tight loop before reaching a halt.
//
// Bound: a running child is waited on without a time limit.
// "lyx batten pause", honoured within one check, cancellation and the notices bound it;
// a running child whose driver is dead and whose status file does not change returns nothing from the wait, the driver-dead notice being the only signal.
//
// A ResolveStatus error and a ReadStatus error are both returned hard errors, not verdicts: the
// task worktree is required to exist by the time this row runs, and a status file that exists but
// does not decode, or a status lock that cannot be taken, is mechanism failure.
func (p *innerRunProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	statusPath, statusLockPath, err := p.deps.ResolveStatus()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: resolve status path: %w", p.name, err)
	}

	p.refreshWatched(ctx)

	// The baseline is stat'ed before the read that defines the state this Call begins with, so a write landing between the two is seen by the first check.
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
	if found && status.State == shedengine.StateRunning && !spawnConfirmed(confirmedPath) {
		driverAtWork, err := p.driverAtWork(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if driverAtWork {
			logger.Info("battenshed: adopting the running inner shed run; its driver is already at work", "producer", p.name, "slug", p.slug)
			recordSpawnConfirmed(p.name, p.slug, p.scratchDir, confirmedPath)
		} else {
			mustSpawn = true
		}
	}
	if mustSpawn {
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
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: spawn inner shed run (resuming this run retries the spawn): %w", p.name, spawnErr)
		}
		recordSpawnConfirmed(p.name, p.slug, p.scratchDir, confirmedPath)

		baseline = statusModTime(statusPath)
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

	p.observeState(status.State, baseline)

	w := &childWait{statusPath: statusPath, statusLockPath: statusLockPath, begun: status.State, status: status, baseline: baseline}
	switch status.State {
	case shedengine.StateDone:
		return p.wait(ctx, w, p.doneStep)
	case shedengine.StateRunning:
		return p.wait(ctx, w, p.runningStep)
	case shedengine.StateAwaiting:
		return p.wait(ctx, w, p.awaitingStep)
	case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
		return p.wait(ctx, w, p.haltedStep)
	default:
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: unrecognized status state %q", p.name, status.State)
	}
}

// statusModTime returns the modification time of the status file at path, or the zero time when it cannot be stat'ed.
// A zero baseline differs from any real time, so the first check decodes the file, and a stat failure is the check's own to report.
func statusModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// driverAtWork reports whether the child already has a driver at work: a live or retiring driver strand, or a held child run lock.
// A retiring strand counts as live, since someone already asked to remove it and `lyx loom start` replaces it.
// A read error from either seam is returned as a hard error,
// and the next Call retries.
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
	return strand == ChildDriverLive || strand == ChildDriverRetiring || held, nil
}

// reviewWaitNote returns the child's reviewer-wait note, or empty when there is none or ReviewWait is nil.
// A ReviewWait error logs one Warn and reads as no note, since the note never fails the row.
func (p *innerRunProducer) reviewWaitNote() string {
	if p.deps.ReviewWait == nil {
		return ""
	}
	note, err := p.deps.ReviewWait()
	if err != nil {
		logger.Warn("battenshed: reviewer wait note unavailable", "producer", p.name, "slug", p.slug, "error", err)
		return ""
	}
	return note
}

// childWait is the state of one Call's wait on the child.
type childWait struct {
	statusPath     string
	statusLockPath string
	// begun is the state the Call began with; a decoded state that differs ends the wait.
	begun shedengine.State
	// status is the newest decoded status, and baseline the status file's modification time when it was decoded.
	status   shedengine.Status
	baseline time.Time
	// lastProbe is when the wait last did the work that costs a process or a multiplexer round trip,
	// and probeDue is true on the checks that do it.
	lastProbe time.Time
	probeDue  bool
	// reason is the wait reason last written through reportStuck.
	reason string
	// pauseWarned is the pause-flag read error last warned about, so a persistent one warns once.
	pauseWarned string

	// reviewNote is the running arm's newest reviewer-wait note.
	reviewNote string
	// revival is the halted arm's newest reading of the child's driver strand.
	revival driverRevival
	// alive is the newest reading of the driver strand and aliveKnown whether it succeeded; aliveStale is true on a probe check until the strand is read.
	alive      bool
	aliveKnown bool
	aliveStale bool
	// activity is the newest reading of the child's agent runs, activityOK whether it succeeded, and activityStale is true on a probe check until it is read.
	activity      agentReading
	activityOK    bool
	activityStale bool
	// prevWallNanos and prevAwake are the wall and awake readings of the previous idle check, and clocksSeen whether there was one.
	prevWallNanos int64
	prevAwake     time.Duration
	clocksSeen    bool
	// asleep is the suspended time observed since the newest agent activity last moved, and asleepStamp the activity stamp it belongs to.
	asleep      time.Duration
	asleepStamp activityStamp
	// resumeRetryAt is when the awaiting arm may try a resume again after the child's driver refused one as not parked yet,
	// and notParkedLogged whether that refusal was logged already.
	resumeRetryAt   time.Time
	notParkedLogged bool
}

// waitEnd is how a wait ended: the verdict, pointer and error Call returns.
type waitEnd struct {
	outcome shedengine.Outcome
	pointer shedengine.OutputPointer
	err     error
}

// armStep is one arm's part of a check: its actions and its wait reason.
// It returns nil to keep waiting.
type armStep func(ctx context.Context, w *childWait) *waitEnd

// hardEnd ends the wait with a returned error.
func hardEnd(err error) *waitEnd {
	return &waitEnd{err: err}
}

// exemptEnd ends the wait with a budget-exempt Stuck whose Path and Reason both carry text, so each cause is its own history entry and equal entries never fold across causes.
func (p *innerRunProducer) exemptEnd(text string) *waitEnd {
	reportStuck(p.name, text, p.scratchDir, "slug", p.slug)
	return &waitEnd{outcome: shedengine.Stuck, pointer: shedengine.OutputPointer{Path: text, Reason: text, BudgetExempt: true}}
}

// report writes reason through reportStuck when it differs from the last one written, so `lyx batten status` shows what the child is waited on for.
func (p *innerRunProducer) report(w *childWait, reason string) {
	if reason == w.reason {
		return
	}
	w.reason = reason
	reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
}

// results unpacks a wait end into the values Call returns.
func (e *waitEnd) results() (shedengine.Outcome, shedengine.OutputPointer, error) {
	return e.outcome, e.pointer, e.err
}

// wait runs the arm's step once on entry and then, every pollInterval, checks the child, returning when a check ends the wait.
// File stats and small file reads run on every check; anything costing a process or a multiplexer round trip runs only on the checks where probeDue is set, at most once per noticeProbe.
func (p *innerRunProducer) wait(ctx context.Context, w *childWait, step armStep) (shedengine.Outcome, shedengine.OutputPointer, error) {
	w.lastProbe = p.deps.Now()
	w.probeDue = true
	w.aliveStale = true
	w.activityStale = true
	p.noticeStep(ctx, w, false)
	if end := step(ctx, w); end != nil {
		return end.results()
	}
	for {
		p.deps.Sleep(ctx, p.pollInterval)
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		now := p.deps.Now()
		w.probeDue = now.Sub(w.lastProbe) >= p.noticeProbe
		if w.probeDue {
			w.lastProbe = now
			w.aliveStale = true
			w.activityStale = true
		}
		if end := p.check(ctx, w, step, now); end != nil {
			return end.results()
		}
	}
}

// check is one check of the wait: the child's status file, the watched-marker refresh and the notice step on the probe checks, the arm's step, and last batten's own pause flag.
// It returns nil to keep waiting.
func (p *innerRunProducer) check(ctx context.Context, w *childWait, step armStep, now time.Time) *waitEnd {
	if end := p.checkChild(ctx, w); end != nil {
		return end
	}
	if w.probeDue {
		p.refreshWatched(ctx)
	}
	p.noticeStep(ctx, w, false)
	if end := step(ctx, w); end != nil {
		return end
	}
	if p.pauseRequested(w) {
		return p.exemptEnd("pause requested at " + now.Format(time.RFC3339))
	}
	return nil
}

// checkChild stats the child's status file and decodes it when needed, ending the wait when the stat fails or the child's state is no longer the one the Call began with.
// A decode error is a hard error, and a status file that vanished between the stat and the read is a stat failure.
func (p *innerRunProducer) checkChild(ctx context.Context, w *childWait) *waitEnd {
	info, err := os.Stat(w.statusPath)
	if err != nil {
		return p.exemptEnd(fmt.Sprintf("the child's status file %s cannot be read: %v", w.statusPath, err))
	}
	if info.ModTime().Equal(w.baseline) && !w.probeDue {
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
		return p.exemptEnd(fmt.Sprintf("the child's status file %s is gone", w.statusPath))
	}
	w.baseline = info.ModTime()
	w.status = status
	if status.State != w.begun {
		p.observeState(status.State, info.ModTime())
		return p.exemptEnd(stateChangeText(w.begun, status))
	}
	return nil
}

// stateChangeText names a change of the child's state, with the producer the child's new status is at.
func stateChangeText(from shedengine.State, to shedengine.Status) string {
	text := fmt.Sprintf("child %s → %s", from, to.State)
	if to.CurrentProducer != "" {
		text += " at " + to.CurrentProducer
	}
	return text
}

// pauseRequested reports whether batten's own status carries pause_requested.
// A read error reads as not paused and is warned about once per distinct message, since the flag is only the operator's early exit.
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

// runningStep is the running arm: it reports the wait reason, naming the reviewer the child waits on, refreshing the note on the probe checks.
func (p *innerRunProducer) runningStep(ctx context.Context, w *childWait) *waitEnd {
	if w.probeDue {
		w.reviewNote = p.reviewWaitNote()
	}
	reason := fmt.Sprintf("inner shed run still running; checking it every %s", p.pollInterval)
	if w.reviewNote != "" {
		reason = fmt.Sprintf("inner shed run waiting: %s; checking it every %s", w.reviewNote, p.pollInterval)
	}
	p.report(w, reason)
	return nil
}

// haltedStep is the halted arm for a blocked, paused or failed child: it never spawns or resumes the child, Warns once per halt episode, revives a dead driver strand once per episode, and reports the resume command the operator runs.
// The child is waited on until its state changes.
func (p *innerRunProducer) haltedStep(ctx context.Context, w *childWait) *waitEnd {
	if err := p.warnHaltEpisode(w.status); err != nil {
		return hardEnd(err)
	}
	if w.probeDue {
		w.revival = p.reviveDeadDriver(ctx)
	}
	p.report(w, haltedWaitReason(w.status, w.revival.startFallback))
	return nil
}

// warnHaltEpisode Warns once per halt episode.
// An episode ends when the child is seen out of a halted state, which removes the halt-warned marker, or when the child's history length differs from the length the marker recorded at the last Warn:
// a re-halt after an observed resume Warns again even at the same history length, since a hard producer error or a mid-call pause appends no history entry, and the checks within one episode stay quiet.
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
	logger.Warn("battenshed: inner shed run halted; waiting for the operator to resume it", "producer", p.name, "slug", p.slug, "state", status.State, "error", status.Error, "current_producer", status.CurrentProducer)
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

// driverRevival is what reviveDeadDriver learned about the child's driver strand.
type driverRevival struct {
	// startFallback is true when the halted reason should also name `lyx loom start`: the child has no driver strand in reed state,
	// or a revive failed in this episode.
	startFallback bool
}

// reviveDeadDriver brings back the dead driver strand of a halted or awaiting child through deps.ReviveStrands, at most once per episode per batten process.
// A retiring strand is left to `lyx loom start`,
// a live one needs nothing,
// and a child with no driver strand has nothing to revive, which it never tells from a Go-driven child by the seed.
// The attempt is recorded in the revived marker with this process's pid;
// a marker naming another pid belongs to an earlier process, whose episode this one retries once.
// It changes no run state and removes no park marker,
// and every failure, a strand read included, is warned about and never fails the row.
func (p *innerRunProducer) reviveDeadDriver(ctx context.Context) driverRevival {
	strand, err := p.deps.DriverStrand(ctx)
	if err != nil {
		logger.Warn("battenshed: could not read the child's driver strand; skipping the revive", "producer", p.name, "slug", p.slug, "error", err)
		return driverRevival{}
	}
	switch strand {
	case ChildDriverNone:
		return driverRevival{startFallback: true}
	case ChildDriverDead:
	default:
		return driverRevival{}
	}

	markerPath := revivedFile(p.scratchDir, p.name)
	if raw, err := os.ReadFile(markerPath); err == nil {
		if pid, _, _ := strings.Cut(strings.TrimSpace(string(raw)), " "); pid == strconv.Itoa(os.Getpid()) {
			// Already tried by this process in this episode,
			// and the strand is dead still.
			return driverRevival{startFallback: true}
		}
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

// awaitingStep is the awaiting arm for a child halted at a human hand-off: it reports the hand-off while no decision exists and resumes the child once per decision.
// A decision already acted on has two outcomes, told apart by the child's history length against the length the marker recorded at the resume:
// an equal length means the resume was delivered and the child's driver has not re-stepped yet,
// and any other length (or an old-layout marker) means the child is awaiting again and gets the decide-again hint.
// The length is a clock-free discriminator because a re-await appends at least the child's own Awaiting entry, which the history fold never folds onto.
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
		p.report(w, fmt.Sprintf("inner shed run is awaiting a human hand-off (%s); %s", w.status.Error, awaitingHandOff))
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
				p.report(w, fmt.Sprintf("the %s at %s was acted on and the resume was delivered, but the child's driver has not re-stepped yet; if this persists, inspect a live driver in its pane, or run \"lyx loom start\" in the task worktree if the driver has ended", decision.Kind, decision.At))
				return nil
			}
			p.report(w, fmt.Sprintf("the %s at %s was already acted on and the child is awaiting again; decide again: %s", decision.Kind, decision.At, awaitingHandOff))
			return nil
		}
	}

	notParkedReason := fmt.Sprintf("the %s at %s is not acted on yet: the child's driver has not parked; retrying the resume", decision.Kind, decision.At)
	if now := p.deps.Now(); now.Before(w.resumeRetryAt) {
		p.report(w, notParkedReason)
		return nil
	}

	logger.Info("battenshed: resuming decided inner shed run", "producer", p.name, "slug", p.slug, "kind", decision.Kind, "decided_at", decision.At)
	spawnErr := p.deps.Spawn(ctx)
	logger.Info("battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
	if spawnErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return hardEnd(cerr)
		}
		if errors.Is(spawnErr, ErrChildNotParked) {
			// The child's driver is between its stop and its park, so the decision stays unacted and the resume is retried at most once per noticeProbe.
			w.resumeRetryAt = p.deps.Now().Add(p.noticeProbe)
			if !w.notParkedLogged {
				w.notParkedLogged = true
				logger.Info("battenshed: child driver has not parked yet; retrying the resume until it does", "producer", p.name, "slug", p.slug, "error", spawnErr)
			}
			p.report(w, notParkedReason)
			return nil
		}
		return hardEnd(fmt.Errorf("battenshed: %s: resume decided inner shed run (resuming this run retries the spawn): %w", p.name, spawnErr))
	}
	recordSpawnConfirmed(p.name, p.slug, p.scratchDir, SpawnConfirmedFile(p.scratchDir, p.name))
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		return hardEnd(fmt.Errorf("battenshed: %s: create scratch directory for decision-acted marker: %w", p.name, err))
	}
	if err := os.WriteFile(markerPath, []byte(decisionActedContent(identity, len(w.status.History))), 0o644); err != nil {
		return hardEnd(fmt.Errorf("battenshed: %s: write decision-acted marker: %w", p.name, err))
	}
	p.report(w, fmt.Sprintf("the %s at %s was acted on: the child was resumed; watching it", decision.Kind, decision.At))
	return nil
}

// doneStep is the done arm: it ends the wait with Done once the driver strand has ended or the grace window counted from first sight has elapsed, and otherwise reports the wait for the driver's stop report.
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
			// A marker that reads but does not parse (a write torn by a killed process) restarts the grace rather than failing every Call: no resume could clear it, and it only bounds a wait.
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
		p.noticeStep(ctx, w, true)
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
		p.report(w, fmt.Sprintf("inner shed run is done; waiting for its driver to finish its stop report (%s of %s grace elapsed)", elapsed.Round(time.Second), p.driverExitGrace))
	}
	return nil
}

// driverAlive reports whether the child's driver strand is live, reading deps.DriverAlive at most once per probe and reusing the answer on the checks between.
// known is false when the read failed, which is warned about once per read; the done arm then treats the driver as live and the notice step skips what needs the answer.
func (p *innerRunProducer) driverAlive(ctx context.Context, w *childWait) (alive, known bool) {
	if w.aliveStale {
		w.aliveStale = false
		a, err := p.deps.DriverAlive(ctx)
		w.alive, w.aliveKnown = a, err == nil
		if err != nil && ctx.Err() == nil {
			logger.Warn("battenshed: driver liveness read failed; treating the driver as live", "producer", p.name, "slug", p.slug, "error", err)
		}
	}
	return w.alive, w.aliveKnown
}

// spawnConfirmed reports whether the spawn-confirmation marker at path names this process's own pid.
// A marker from an earlier process, one in the old `spawned` layout and any read failure all report false: the cost of a wrong false is one idempotent re-spawn or adoption check,
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
// A write failure is logged rather than escalated:
// the spawn itself succeeded,
// and a missing marker costs only one idempotent re-spawn or adoption check on the next Call.
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
// An error reads as not held and is warned about once per change of answer, since the marker only decides who messages the parent and never the row's outcome.
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
