// innerrun.go implements NewInnerRun, the producer that spawns the inner shed run inside the task
// worktree, if it has not spawned yet, and checks its persisted status exactly once per Call --
// re-entered via the recipe row's own on_stuck self-route rather than looping internally.

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
// The marker holds the pid of the process that wrote it, so it confirms the spawn for that process only.
const spawnConfirmedFileSuffix = "-spawned"

// SpawnConfirmedFile returns the path of the marker innerRunProducer writes under scratchDir once
// producer's spawn has returned success.
// It is exported for the same reason StuckReasonFile is: the file's reader and writer share one
// declarer of its name.
func SpawnConfirmedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+spawnConfirmedFileSuffix)
}

// haltedWaitReason renders the stuck reason of a halted child's wait.
// Nothing on the prime side resumes the child's own driver, so the reason names the operator's resume command and says this run keeps watching.
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

// ideOpenedFileSuffix is the fixed suffix of the marker recording that the producer already opened the IDE this run, joined onto the producer's own name.
const ideOpenedFileSuffix = "-ide-opened"

// ideOpenedFile returns the path of the once-marker written after the IDE open was attempted.
func ideOpenedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+ideOpenedFileSuffix)
}

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

// awaitingHandOff is the operator instruction an awaiting child's wait carries: the child waits on a pull-request decision that only the operator can give from inside the task worktree.
const awaitingHandOff = "run \"lyx loom approve\" or \"lyx loom reject\" in the task worktree; this run then resumes the child itself"

// innerRunProducer spawns the inner shed run for a task worktree, once, and checks its persisted
// status once per Call, reporting Stuck while the child is still running so shedengine's own
// on_stuck self-route re-enters this producer rather than this type looping internally.
type innerRunProducer struct {
	name         string
	slug         string
	deps         InnerRunDeps
	pollInterval time.Duration
	scratchDir   string
	// driverExitGrace bounds how long a done child's live driver strand is waited for.
	driverExitGrace time.Duration
	// notices is true when the caller wired a Notify, before a nil one resolves to a no-op.
	notices bool
	// noticeQuiet is deps.NoticeQuiet, read once at construction.
	noticeQuiet time.Duration
}

var _ shedengine.ShedProducer = (*innerRunProducer)(nil)

// NewInnerRun returns a shedengine.ShedProducer that resolves the task worktree's status path,
// spawns the inner shed run via deps.Spawn the first time Call finds no status file, and checks
// deps.ReadStatus exactly once per Call thereafter. The bounded wait lives on the recipe row's own
// max_bounces and on_stuck self-route, one shedengine bounce per Call, not inside this producer.
//
// driverExitGrace bounds the wait for a done child's driver strand to end before the row returns Done anyway.
//
// A nil deps.Sleep resolves to waitOrCancel, and a nil deps.Now to time.Now, once here rather than on every Call, so a test's no-op sleep and fixed clock are the only values ever substituted.
// A nil deps.Notify resolves to a no-op and switches the notice step off.
// A nil deps.DriverStrand resolves to reporting no driver strand, a nil deps.ChildRunLockHeld to reporting no held lock and a nil deps.ReviveStrands to a revive that fails as not wired.
func NewInnerRun(name, slug string, deps InnerRunDeps, pollInterval time.Duration, scratchDir string, driverExitGrace time.Duration) shedengine.ShedProducer {
	if deps.Sleep == nil {
		deps.Sleep = waitOrCancel
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.OpenIDE == nil {
		deps.OpenIDE = func(context.Context) error { return nil }
	}
	notices := deps.Notify != nil
	if deps.Notify == nil {
		deps.Notify = func(context.Context, string) error { return nil }
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
// the child's bootstrap seeds it as running before it starts the driver, so a bootstrap that failed or was killed after seeding leaves a running status with no driver behind it.
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
// After a successful spawn, whether here or in the approved-resume arm, Call opens the operator's IDE through deps.OpenIDE once per run: a once-marker under scratchDir (ideOpenedFile) gates it, a fresh child (no status file) clears the marker first, and an open error is only warned about, never changing the row's outcome.
// A failed spawn returns before the open.
// Any Call that finds the child in a state other than done first removes a leftover done-seen marker, so a marker from an earlier run of the same slug never shortens a later wait.
// Any Call that finds the child out of a halted state likewise removes the halt-warned marker, which ends the halt episode.
// Once the child's state is settled, the notice step runs (noticeStep): informational only, one notice per condition per episode, never changing the outcome below.
// Then by state:
//   - running sleeps p.pollInterval and returns a counted Stuck;
//   - awaiting first revives a dead driver strand once per episode, then with no decision record sleeps and returns a budget-exempt Stuck naming the hand-off;
//   - awaiting with a decision not yet acted on spawns the child's driver again (the child's own bootstrap resumes an approved or rejected run), records the decision in the decision-acted marker only once the spawn succeeded, then sleeps and returns a budget-exempt Stuck;
//     a spawn refused with ErrChildNotParked records nothing and sleeps and returns a budget-exempt Stuck, so the next poll retries the resume;
//   - awaiting with a decision already acted on and the child's history length unchanged since that resume does not spawn, and sleeps and returns a budget-exempt Stuck saying the resume was delivered and the child's driver has not re-stepped yet;
//   - awaiting with a decision already acted on and the child's history longer (or an old-layout marker) does not spawn, and sleeps and returns a budget-exempt Stuck naming the recovery of deciding again;
//   - done records the first-sight time in the done-seen marker and returns Done once the driver strand is gone or driverExitGrace has elapsed since first sight, and otherwise sleeps and returns a budget-exempt Stuck, the wait for the driver to finish its stop report;
//   - blocked, paused or failed never spawns or resumes the child, Warns once per halt episode, revives a dead driver strand once per episode (reviveDeadDriver), and sleeps and returns a budget-exempt Stuck whose reason carries the child's State, Error and CurrentProducer and the resume command;
//     a resumed child is then read as running again;
//   - any other value is a hard error naming the unrecognised state.
//
// The self-route's "sole Stuck arm" reasoning still holds in the sense it exists for:
// ProducerDef.OnStuck is a static per-producer value, so every Stuck this row returns routes back to the same self-route target, which is safe only while every Stuck is a timed wait, never a budget-counted spin that would burn the bounce budget in a tight loop before reaching a halt.
// Every arm above that returns Stuck is such a wait, a halted child's included.
// Only the running arm is counted against the row's bounce budget; the waits on a human, on a halted child and on the driver are exempt, the last bounded by driverExitGrace and the halted wait by nothing but the operator's resume or "lyx batten pause".
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
		if !found {
			if err := os.Remove(ideOpenedFile(p.scratchDir, p.name)); err != nil && !os.IsNotExist(err) {
				return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear ide-opened marker: %w", p.name, err)
			}
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
		p.openIDEOnce(ctx)

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
		// A child seen out of a halted or awaiting state ends the episode, so the next halt revives afresh.
		if err := os.Remove(revivedFile(p.scratchDir, p.name)); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear revived marker: %w", p.name, err)
		}
	}

	p.noticeStep(ctx, statusPath, status)

	switch status.State {
	case shedengine.StateDone:
		return p.callDone(ctx)
	case shedengine.StateRunning:
		p.deps.Sleep(ctx, p.pollInterval)
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("inner shed run still running; sleeping %s before the next bounce", p.pollInterval)
		if note := p.reviewWaitNote(); note != "" {
			reason = fmt.Sprintf("inner shed run waiting: %s; sleeping %s before the next bounce", note, p.pollInterval)
		}
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	case shedengine.StateAwaiting:
		return p.callAwaiting(ctx, status)
	case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
		return p.callHalted(ctx, status)
	default:
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: unrecognized status state %q", p.name, status.State)
	}
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

// exemptWait sleeps one poll interval and returns a budget-exempt Stuck carrying reason.
func (p *innerRunProducer) exemptWait(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.deps.Sleep(ctx, p.pollInterval)
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, BudgetExempt: true}, nil
}

// callHalted handles a blocked, paused or failed child: it never spawns or resumes the child, Warns once per halt episode, revives a dead driver strand once per episode, and waits with a budget-exempt Stuck until the operator resumes the child.
// An episode ends when Call sees the child out of a halted state, which removes the halt-warned marker, or when the child's history length differs from the length the marker recorded at the last Warn:
// a re-halt after an observed resume Warns again even at the same history length, since a hard producer error or a mid-call pause appends no history entry, and the polls within one episode stay quiet.
// A marker read or write failure is a hard error, as the other markers' are.
func (p *innerRunProducer) callHalted(ctx context.Context, status shedengine.Status) (shedengine.Outcome, shedengine.OutputPointer, error) {
	markerPath := haltWarnedFile(p.scratchDir, p.name)
	raw, err := os.ReadFile(markerPath)
	if err != nil && !os.IsNotExist(err) {
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read halt-warned marker: %w", p.name, err)
	}
	historyLen := len(status.History)
	recorded, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || parseErr != nil || recorded != historyLen {
		logger.Warn("battenshed: inner shed run halted; waiting for the operator to resume it", "producer", p.name, "slug", p.slug, "state", status.State, "error", status.Error, "current_producer", status.CurrentProducer)
		if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: create scratch directory for halt-warned marker: %w", p.name, err)
		}
		if err := os.WriteFile(markerPath, []byte(strconv.Itoa(historyLen)+"\n"), 0o644); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: write halt-warned marker: %w", p.name, err)
		}
		// A new halt episode is a new chance to revive, however the episode was told apart from the last one.
		if err := os.Remove(revivedFile(p.scratchDir, p.name)); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear revived marker: %w", p.name, err)
		}
	}
	revival := p.reviveDeadDriver(ctx)
	return p.exemptWait(ctx, haltedWaitReason(status, revival.startFallback))
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

// callAwaiting handles a child halted at a human hand-off: it waits for a decision, resumes the child once per decision, and otherwise waits, always with a budget-exempt Stuck.
// A decision already acted on has two outcomes, told apart by the child's history length against the length the marker recorded at the resume:
// an equal length means the resume was delivered and the child's driver has not re-stepped yet,
// and any other length (or an old-layout marker) means the child is awaiting again and gets the decide-again hint.
// The length is a clock-free discriminator because a re-await appends at least the child's own Awaiting entry, which the history fold never folds onto.
func (p *innerRunProducer) callAwaiting(ctx context.Context, status shedengine.Status) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.reviveDeadDriver(ctx)
	decision, found, err := p.deps.ReadDecision()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read decision: %w", p.name, err)
	}
	if !found {
		return p.exemptWait(ctx, fmt.Sprintf("inner shed run is awaiting a human hand-off (%s); %s", status.Error, awaitingHandOff))
	}

	markerPath := decisionActedFile(p.scratchDir, p.name)
	identity := decisionIdentity(decision)
	acted, err := os.ReadFile(markerPath)
	if err != nil && !os.IsNotExist(err) {
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read decision-acted marker: %w", p.name, err)
	}
	if err == nil {
		actedIdentity, actedLen, hasLen := parseDecisionActed(string(acted))
		if actedIdentity == identity {
			if hasLen && len(status.History) == actedLen {
				return p.exemptWait(ctx, fmt.Sprintf("the %s at %s was acted on and the resume was delivered, but the child's driver has not re-stepped yet; if this persists, inspect a live driver in its pane, or run \"lyx loom start\" in the task worktree if the driver has ended", decision.Kind, decision.At))
			}
			return p.exemptWait(ctx, fmt.Sprintf("the %s at %s was already acted on and the child is awaiting again; re-run \"lyx loom approve\" or \"lyx loom reject\" in the task worktree, which records a new decision and resumes the child once more", decision.Kind, decision.At))
		}
	}

	logger.Info("battenshed: resuming decided inner shed run", "producer", p.name, "slug", p.slug, "kind", decision.Kind, "decided_at", decision.At)
	spawnErr := p.deps.Spawn(ctx)
	logger.Info("battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
	if spawnErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		if errors.Is(spawnErr, ErrChildNotParked) {
			// The child's driver is between its stop and its park, so the decision stays unacted and the next poll resumes it.
			logger.Info("battenshed: child driver has not parked yet; retrying the resume on the next poll", "producer", p.name, "slug", p.slug, "error", spawnErr)
			return p.exemptWait(ctx, fmt.Sprintf("the %s at %s is not acted on yet: the child's driver has not parked; retrying the resume on the next poll", decision.Kind, decision.At))
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: resume decided inner shed run (resuming this run retries the spawn): %w", p.name, spawnErr)
	}
	recordSpawnConfirmed(p.name, p.slug, p.scratchDir, SpawnConfirmedFile(p.scratchDir, p.name))
	p.openIDEOnce(ctx)
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: create scratch directory for decision-acted marker: %w", p.name, err)
	}
	if err := os.WriteFile(markerPath, []byte(decisionActedContent(identity, len(status.History))), 0o644); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: write decision-acted marker: %w", p.name, err)
	}
	return p.exemptWait(ctx, fmt.Sprintf("the %s at %s was acted on: the child was resumed; watching it", decision.Kind, decision.At))
}

// callDone handles a done child: it returns Done once the driver strand has ended or the grace window counted from first sight has elapsed, and otherwise waits with a budget-exempt Stuck.
func (p *innerRunProducer) callDone(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	markerPath := doneSeenFile(p.scratchDir, p.name)
	now := p.deps.Now()

	raw, err := os.ReadFile(markerPath)
	if err != nil && !os.IsNotExist(err) {
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read done-seen marker: %w", p.name, err)
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
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: create scratch directory for done-seen marker: %w", p.name, err)
		}
		if err := os.WriteFile(markerPath, []byte(now.Format(time.RFC3339)+"\n"), 0o644); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: write done-seen marker: %w", p.name, err)
		}
	}

	finish := func() (shedengine.Outcome, shedengine.OutputPointer, error) {
		if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: remove done-seen marker: %w", p.name, err)
		}
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}

	alive, err := p.deps.DriverAlive(ctx)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		logger.Warn("battenshed: driver liveness read failed; treating the driver as live", "producer", p.name, "slug", p.slug, "error", err)
		alive = true
	}
	if !alive {
		return finish()
	}
	elapsed := now.Sub(firstSeen)
	if elapsed >= p.driverExitGrace {
		logger.Warn("battenshed: driver still live after the exit grace; finishing anyway", "producer", p.name, "slug", p.slug, "grace", p.driverExitGrace, "elapsed", elapsed)
		return finish()
	}
	return p.exemptWait(ctx, fmt.Sprintf("inner shed run is done; waiting for its driver to finish its stop report (%s of %s grace elapsed)", elapsed.Round(time.Second), p.driverExitGrace))
}

// openIDEOnce opens the operator's IDE through deps.OpenIDE unless the once-marker already exists.
// An open error is logged rather than escalated, and the marker is written whatever the outcome, so a failed open is not retried;
// a marker write failure is logged too, since a lost marker costs only one extra open.
func (p *innerRunProducer) openIDEOnce(ctx context.Context) {
	markerPath := ideOpenedFile(p.scratchDir, p.name)
	if _, err := os.Stat(markerPath); err == nil {
		return
	}
	if err := p.deps.OpenIDE(ctx); err != nil {
		logger.Warn("battenshed: open IDE failed", "producer", p.name, "slug", p.slug, "error", err)
	}
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for ide-opened marker failed", "producer", p.name, "slug", p.slug, "scratchDir", p.scratchDir, "error", err)
		return
	}
	if err := os.WriteFile(markerPath, []byte("opened\n"), 0o644); err != nil {
		logger.Warn("battenshed: write ide-opened marker failed", "producer", p.name, "slug", p.slug, "path", markerPath, "error", err)
	}
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
