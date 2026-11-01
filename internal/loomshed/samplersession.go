// samplersession.go implements the Sampler-Session row's producer: it keeps one interactive agent session in the run's worktree and waits, with no timeout, for the done flag `lym loom sampler-done` writes.

package loomshed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// samplerWaitFloor is the shortest time between two wakes of the wait, and the one a change returns it to.
	samplerWaitFloor = time.Second
	// samplerWaitCeiling is the longest time between two wakes while nothing changes.
	samplerWaitCeiling = time.Minute
)

// samplerUnresumableWayForward follows a way forward of resuming the row, for a recorded session that can no longer be resumed.
const samplerUnresumableWayForward = "when the recorded session can no longer be resumed, commit the work in the task worktree and run lym loom sampler-done first, so the resume ends the row without a session, or end the run with lym batten cancel <slug>"

// SamplerSessions is the seam the Sampler-Session row starts and stops its interactive session through.
type SamplerSessions interface {
	// StartSession starts one unawaited session and returns once its provider is past startup.
	StartSession(spec shuttleengine.Spec) (shuttleengine.OpenResult, error)
	// StopStrand ends the strand guid names and records the stop.
	StopStrand(guid string) error
}

var _ SamplerSessions = (*shuttleengine.Runner)(nil)

// SamplerDeps carries the told seams NewSamplerSession needs.
// Every field but Watch is required.
type SamplerDeps struct {
	// RecordPath and RecordLockPath are the sampler record and its lock.
	RecordPath     string
	RecordLockPath string
	// Spec builds the spec the session starts under; its error halts the run with the way forward that fixes the agent or the stencil.
	Spec func() (shuttleengine.Spec, error)
	// Sessions starts and stops the session.
	Sessions SamplerSessions
	// SessionLive reports whether the strand guid names is running.
	SessionLive func(guid string) (bool, error)
	// RegisterSession registers a started session's pane process with the run, so a teardown can end it.
	RegisterSession func(guid string) error
	// Stamp returns the run's current entry stamp of the row; a done flag counts only at that stamp.
	Stamp func() (int, error)
	// DirtyPaths returns the code checkout's uncommitted paths.
	DirtyPaths func() ([]string, error)
	// CheckRecord validates the decision record once the operator ends the build, returning the findings that halt the run; it commits nothing.
	CheckRecord func() ([]discussionparser.Finding, error)
	// MailAgent posts line to the session whose strand name is strandName.
	MailAgent func(strandName, line string) error
	// PauseCovered reports whether a pause covers the run.
	PauseCovered func() (bool, error)
	// Now returns the current time.
	Now func() time.Time
	// Sleep waits d, or returns the context's error when ctx is done first.
	Sleep func(ctx context.Context, d time.Duration) error
	// Watch delivers when the record's file may have changed, cutting a sleep short; nil leaves the wait on its timer.
	Watch <-chan struct{}
}

// samplerSession is the Sampler-Session row's ShedProducer.
type samplerSession struct {
	name string
	deps SamplerDeps
}

var _ shedengine.ShedProducer = (*samplerSession)(nil)

// NewSamplerSession returns the Sampler-Session producer identified as name.
func NewSamplerSession(name string, deps SamplerDeps) shedengine.ShedProducer {
	return &samplerSession{name: name, deps: deps}
}

// Call keeps the run's one interactive session and waits for the operator's done flag.
// A flag at the run's current entry stamp is judged first, so a flag written while the run was halted ends the row without starting a session; otherwise the recorded session is reused when live, resumed by its recorded id when not, or started.
// The wait has no timeout; a covered pause stops the session and the next uncovered wake resumes it.
// A flag judged on a dirty checkout is cleared and the session is told to commit; on a clean one with an intact decision record the row is Done.
// A transient session start failure is an error the driver re-steps, and any other failure is Stuck with Halt.
// Every verdict, Stuck or error stops a live session, except a cancelled context, which leaves it running so the re-call finds it live.
func (p *samplerSession) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.checkWiring(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	run := &samplerRun{name: p.name, deps: p.deps}
	outcome, pointer, err := run.execute(ctx)
	var halt *samplerHalt
	if errors.As(err, &halt) {
		outcome, pointer, err = run.stuck(ctx, halt.reason)
	}
	run.endSession(ctx, err)
	return outcome, pointer, err
}

// checkWiring returns an error naming the first told value the row needs and the deps leave unset.
func (p *samplerSession) checkWiring() error {
	d := p.deps
	seams := []struct {
		name    string
		missing bool
	}{
		{"RecordPath", d.RecordPath == ""},
		{"RecordLockPath", d.RecordLockPath == ""},
		{"Spec", d.Spec == nil},
		{"Sessions", d.Sessions == nil},
		{"SessionLive", d.SessionLive == nil},
		{"RegisterSession", d.RegisterSession == nil},
		{"Stamp", d.Stamp == nil},
		{"DirtyPaths", d.DirtyPaths == nil},
		{"CheckRecord", d.CheckRecord == nil},
		{"MailAgent", d.MailAgent == nil},
		{"PauseCovered", d.PauseCovered == nil},
		{"Now", d.Now == nil},
		{"Sleep", d.Sleep == nil},
	}
	for _, s := range seams {
		if s.missing {
			return fmt.Errorf("loomshed: %s: SamplerDeps.%s is not set", p.name, s.name)
		}
	}
	return nil
}

// samplerHalt is a failure that is not transient, which Call turns into a Stuck verdict with Halt.
type samplerHalt struct {
	reason string
}

func (h *samplerHalt) Error() string { return h.reason }

// samplerRun is the state of one Call: the strand the call knows of, and whether the row stopped it.
type samplerRun struct {
	name string
	deps SamplerDeps
	// guid is the strand the call last read from the record or started.
	guid string
	// rowStopped is true while the row itself holds the session stopped, so a strand that is not live is no death.
	rowStopped bool
}

// execute runs the call: it judges a current done flag, or ensures the session, and then waits.
func (r *samplerRun) execute(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	rec, _, err := samplerengine.ReadRecord(r.deps.RecordPath, r.deps.RecordLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: read the sampler record: %w", r.name, err)
	}
	r.guid = rec.StrandGUID

	current, err := r.flagIsCurrent(rec)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if current {
		outcome, pointer, ended, err := r.judgeDone(ctx, rec)
		if err != nil || ended {
			return outcome, pointer, err
		}
	} else if _, err := r.ensureSession(ctx, rec); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return r.wait(ctx)
}

// flagIsCurrent reports whether rec carries a done flag at the row's current entry stamp; an older stamp is ignored.
func (r *samplerRun) flagIsCurrent(rec samplerengine.Record) (bool, error) {
	if rec.DoneStamp == 0 {
		return false, nil
	}
	stamp, err := r.deps.Stamp()
	if err != nil {
		return false, fmt.Errorf("loomshed: %s: read the row's entry stamp: %w", r.name, err)
	}
	return rec.DoneStamp == stamp, nil
}

// ensureSession makes sure the run has a live session and returns rec with the session's identity.
// A recorded strand that is live is reused; otherwise the spec is started with the recorded session id, empty for a first start, and the strand is recorded.
// A transient start failure is an error, and any other failure is a *samplerHalt.
// A failed registration is logged, since it only lets a teardown end the pane process.
func (r *samplerRun) ensureSession(ctx context.Context, rec samplerengine.Record) (samplerengine.Record, error) {
	if rec.StrandGUID != "" {
		live, err := r.deps.SessionLive(rec.StrandGUID)
		if err != nil {
			return rec, fmt.Errorf("loomshed: %s: read whether the sampler session is live: %w", r.name, err)
		}
		if live {
			r.guid, r.rowStopped = rec.StrandGUID, false
			return rec, nil
		}
	}

	spec, err := r.deps.Spec()
	if err != nil {
		return rec, &samplerHalt{reason: fmt.Sprintf("the sampler session's spec could not be built: %v; way forward: fix the agent with lym config sampler --set agent=<model-spec> from the prime, or restore the stencil, then run lym loom resume", err)}
	}
	spec.ResumeSessionID = rec.SessionID
	result, err := r.deps.Sessions.StartSession(spec)
	if err != nil {
		if terr := r.transientFailure(ctx, "start the sampler session", err); terr != nil {
			return rec, terr
		}
		return rec, &samplerHalt{reason: fmt.Sprintf("the sampler session could not be started: %v; way forward: run lym loom resume; %s", err, samplerUnresumableWayForward)}
	}
	if err := r.deps.RegisterSession(result.StrandGUID); err != nil {
		logger.Warn("loomshed: could not register the sampler session's process, so a teardown will not end it", "row", r.name, "strand", result.StrandGUID, "cause", err)
	}
	r.guid, r.rowStopped = result.StrandGUID, false
	rec.StrandGUID, rec.StrandName, rec.SessionID, rec.Engine = result.StrandGUID, result.StrandName, result.SessionID, result.Engine
	err = samplerengine.UpdateRecord(r.deps.RecordPath, r.deps.RecordLockPath, func(stored *samplerengine.Record) {
		stored.StrandGUID, stored.StrandName, stored.SessionID, stored.Engine = rec.StrandGUID, rec.StrandName, rec.SessionID, rec.Engine
	})
	if err != nil {
		// A session the record does not name could never be found again, and the re-entry would start a second one.
		if stopErr := r.deps.Sessions.StopStrand(result.StrandGUID); stopErr != nil {
			logger.Warn("loomshed: could not stop the sampler session whose record write failed", "row", r.name, "strand", result.StrandGUID, "cause", stopErr)
		}
		return rec, fmt.Errorf("loomshed: %s: record the sampler session: %w", r.name, err)
	}
	return rec, nil
}

// stopLive stops the strand guid names when it is live, and reports whether it stopped one.
func (r *samplerRun) stopLive(guid string) (bool, error) {
	if guid == "" {
		return false, nil
	}
	live, err := r.deps.SessionLive(guid)
	if err != nil {
		return false, fmt.Errorf("loomshed: %s: read whether the sampler session is live: %w", r.name, err)
	}
	if !live {
		return false, nil
	}
	if err := r.deps.Sessions.StopStrand(guid); err != nil {
		return false, fmt.Errorf("loomshed: %s: stop the sampler session: %w", r.name, err)
	}
	return true, nil
}

// judgeDone judges a done flag at the current stamp and reports whether it ended the call.
// It stops the recorded session first.
// A dirty checkout clears the flag, brings the session back and mails it the uncommitted paths, and the call goes on waiting.
// A clean checkout whose decision record has findings is Stuck with Halt, and an intact one is Done.
func (r *samplerRun) judgeDone(ctx context.Context, rec samplerengine.Record) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	if _, err := r.stopLive(rec.StrandGUID); err != nil {
		return "", shedengine.OutputPointer{}, true, err
	}
	r.rowStopped = true

	dirty, err := r.deps.DirtyPaths()
	if err != nil {
		return "", shedengine.OutputPointer{}, true, fmt.Errorf("loomshed: %s: read the task worktree's uncommitted paths: %w", r.name, err)
	}
	if len(dirty) > 0 {
		return r.refuseDirtyDone(ctx, rec, dirty)
	}

	findings, err := r.deps.CheckRecord()
	if err != nil {
		return "", shedengine.OutputPointer{}, true, fmt.Errorf("loomshed: %s: check the decision record: %w", r.name, err)
	}
	if len(findings) > 0 {
		outcome, pointer, err := r.stuck(ctx, fmt.Sprintf("the sampler left the decision record invalid: %s; %s", formatDiscussionFindings(findings), decisionRecordRestoreWayForward))
		return outcome, pointer, true, err
	}
	if cerr := cancelErr(ctx, r.name); cerr != nil {
		return "", shedengine.OutputPointer{}, true, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, true, nil
}

// refuseDirtyDone clears the done flag of a build ended on a dirty checkout, brings the session back and tells it which paths are uncommitted.
func (r *samplerRun) refuseDirtyDone(ctx context.Context, rec samplerengine.Record, dirty []string) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	err := samplerengine.UpdateRecord(r.deps.RecordPath, r.deps.RecordLockPath, func(stored *samplerengine.Record) { stored.DoneStamp = 0 })
	if err != nil {
		return "", shedengine.OutputPointer{}, true, fmt.Errorf("loomshed: %s: clear the done flag: %w", r.name, err)
	}
	rec.DoneStamp = 0
	if rec, err = r.ensureSession(ctx, rec); err != nil {
		return "", shedengine.OutputPointer{}, true, err
	}
	line := fmt.Sprintf("lym ended no build: the worktree has uncommitted paths: %s; commit them, then run lym loom sampler-done again, since it ends the build only on a committed worktree", strings.Join(dirty, ", "))
	if err := r.deps.MailAgent(rec.StrandName, line); err != nil {
		logger.Warn("loomshed: could not tell the sampler session its worktree is dirty", "row", r.name, "strand", rec.StrandName, "cause", err)
	}
	return "", shedengine.OutputPointer{}, false, nil
}

// wait holds the call until the operator's done flag ends it.
// A cancelled context is an error.
func (r *samplerRun) wait(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	var interval time.Duration
	pausedStop := false
	for {
		wokeAt := r.deps.Now()
		outcome, pointer, ended, moved, err := r.wake(ctx, &pausedStop)
		if err != nil || ended {
			return outcome, pointer, err
		}
		interval = nextSamplerWait(interval, moved)
		watched, err := r.pause(ctx, interval, wokeAt)
		if err != nil {
			if cerr := cancelErr(ctx, r.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: sleep between wakes: %w", r.name, err)
		}
		if watched {
			interval = samplerWaitFloor
		}
	}
}

// wake runs one wake of the wait.
// It reports whether the call ended with the returned verdict, and whether the wake changed anything, which returns the wait to its shortest interval.
// pausedStop is true while a covered pause holds the session stopped.
func (r *samplerRun) wake(ctx context.Context, pausedStop *bool) (outcome shedengine.Outcome, pointer shedengine.OutputPointer, ended, moved bool, err error) {
	fail := func(err error) (shedengine.Outcome, shedengine.OutputPointer, bool, bool, error) {
		return "", shedengine.OutputPointer{}, true, false, err
	}
	if cerr := cancelErr(ctx, r.name); cerr != nil {
		return fail(cerr)
	}

	covered, err := r.deps.PauseCovered()
	if err != nil {
		logger.Warn("loomshed: the pause record is unreadable, reading the run as uncovered", "row", r.name, "cause", err)
		covered = false
	}
	if covered {
		if !*pausedStop {
			stopped, err := r.stopLive(r.guid)
			if err != nil {
				return fail(err)
			}
			*pausedStop = stopped
			r.rowStopped = r.rowStopped || stopped
		}
		return "", shedengine.OutputPointer{}, false, false, nil
	}

	rec, _, err := samplerengine.ReadRecord(r.deps.RecordPath, r.deps.RecordLockPath)
	if err != nil {
		return fail(fmt.Errorf("loomshed: %s: read the sampler record: %w", r.name, err))
	}
	if *pausedStop {
		if rec, err = r.ensureSession(ctx, rec); err != nil {
			return fail(err)
		}
		*pausedStop = false
		moved = true
	}

	current, err := r.flagIsCurrent(rec)
	if err != nil {
		return fail(err)
	}
	if current {
		outcome, pointer, ended, err := r.judgeDone(ctx, rec)
		return outcome, pointer, ended, true, err
	}

	if rec.StrandGUID != "" && !r.rowStopped {
		live, err := r.deps.SessionLive(rec.StrandGUID)
		if err != nil {
			return fail(fmt.Errorf("loomshed: %s: read whether the sampler session is live: %w", r.name, err))
		}
		if !live {
			outcome, pointer, err := r.stuck(ctx, fmt.Sprintf("the sampler session ended before the build did; way forward: run lym loom resume, which re-enters the row and resumes the recorded session; %s", samplerUnresumableWayForward))
			return outcome, pointer, true, false, err
		}
	}
	return "", shedengine.OutputPointer{}, false, moved, nil
}

// nextSamplerWait is the wait after current: a second once something changed, otherwise double current up to a minute and never under a second.
func nextSamplerWait(current time.Duration, moved bool) time.Duration {
	if moved {
		return samplerWaitFloor
	}
	return min(max(current*2, samplerWaitFloor), samplerWaitCeiling)
}

// pause waits wait, or until Watch delivers, and reports whether it did.
// An event still waits out the rest of samplerWaitFloor since lastWake, so no wake follows another by less than a second.
func (r *samplerRun) pause(ctx context.Context, wait time.Duration, lastWake time.Time) (bool, error) {
	if r.deps.Watch == nil {
		return false, r.deps.Sleep(ctx, wait)
	}
	sleepCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	slept := make(chan error, 1)
	go func() { slept <- r.deps.Sleep(sleepCtx, wait) }()
	select {
	case err := <-slept:
		return false, err
	case <-r.deps.Watch:
		cancel()
		<-slept
		if rest := samplerWaitFloor - r.deps.Now().Sub(lastWake); rest > 0 {
			return true, r.deps.Sleep(ctx, rest)
		}
		return true, nil
	}
}

// stuck is the Stuck verdict with Halt of reason, or the cancellation error when the context is cancelled.
func (r *samplerRun) stuck(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, r.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Halt: true}, nil
}

// transientFailure returns the error a transient failure of action ends the call with, so the driver re-steps it, and nil for any other failure.
// A cancelled context is that error first.
func (r *samplerRun) transientFailure(ctx context.Context, action string, err error) error {
	if cerr := cancelErr(ctx, r.name); cerr != nil {
		return cerr
	}
	if shedtransient.Class(err) == "" {
		return nil
	}
	return fmt.Errorf("loomshed: %s: %s: %w", r.name, action, err)
}

// endSession stops the call's live session when the call ends with a verdict or a failure of its own, and leaves it running when the context was cancelled, so the re-call after an interrupt finds it live.
// A failed stop is logged, since the verdict stands whatever it says.
func (r *samplerRun) endSession(ctx context.Context, err error) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	if _, stopErr := r.stopLive(r.guid); stopErr != nil {
		logger.Warn("loomshed: could not stop the sampler session at the end of the row", "row", r.name, "cause", stopErr)
	}
}
