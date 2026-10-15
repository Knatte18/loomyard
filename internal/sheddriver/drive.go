// drive.go holds the shed driver's settle loop:
// it steps the run through the shared --until-stop loop body, decides each stop by the closed rule table and carries the decision out, leaving a settle record for every move and a stop report for every hand-back.
// The loop works over injected seams, so every branch is testable without a process; the seams a recipe has no use for are optional groups.

package sheddriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// Exit is how a driver run ended.
type Exit string

const (
	// ExitDone is a run that finished.
	ExitDone Exit = "done"
	// ExitHandBack is a run handed back to its parent and parked.
	ExitHandBack Exit = "hand-back"
	// ExitHandOver is a run handed over to a fresh driver of a newer build.
	ExitHandOver Exit = "hand-over"
	// ExitBusy is a step that reported another holder of the run.
	ExitBusy Exit = "busy"
	// ExitRunningElsewhere is a recheck that found the run running or its run lock held.
	ExitRunningElsewhere Exit = "running-elsewhere"
)

// Deps is the shed driver's whole surface to the world.
// The groups below the required seams are optional: a nil group only removes rows or agents from the rule table, and a stop a removed row would have caught meets the later rows unchanged.
type Deps struct {
	// RunID is the run the driver settles.
	RunID string
	// Step runs one --until-stop step, consuming attempt when it is non-empty, and returns the envelope it printed.
	Step func(ctx context.Context, attempt string) ([]byte, error)
	// Goto moves the halted run onto target.
	Goto func(ctx context.Context, target string) error
	// ReadStatus reads the run's status; found is false when it has no status file.
	ReadStatus func() (status shedengine.Status, found bool, err error)
	// RunLockHeld reports whether another process holds the run lock.
	RunLockHeld func() (bool, error)
	// SaveEnvelope keeps a stop envelope under the run's drive-reports directory and returns its path.
	SaveEnvelope func(raw []byte) (string, error)
	// SettlePath is the run's settle-records file.
	SettlePath string
	// FrictionDir is the run's friction directory, which the stop report quotes; empty reads none.
	FrictionDir string
	// Pause reads what the hub's pause record says about the run's slug at now.
	Pause func(now time.Time) (pauserecord.Coverage, error)
	// PauseHold waits until no pause covers the run's slug.
	PauseHold func(ctx context.Context) error
	// NewReportPath returns a fresh stop report path.
	NewReportPath func() string
	// HandBack holds what a hand-back reads; its ReportPath is replaced by NewReportPath for each hand-back.
	HandBack HandBackDeps
	// Tackler spawns the tackler for stop, or returns the verdict of an earlier attempt, writing its report at reportPath.
	Tackler func(stop Stop, reportPath string) (TacklerEnvelope, error)
	// ConsumeTackler applies the resume of the tackler attempt in attemptDir to the run.
	ConsumeTackler func(attemptDir string) error
	// OpenAttempts names the newest tackler and arbiter attempt directories recorded for key, empty when there is none, and whether the tackler attempt was applied.
	OpenAttempts func(key StopKey) (tackler, arbiter string, tacklerApplied bool, err error)
	// Now is the clock.
	Now func() time.Time

	// CommitRecords commits the run's records at a done stop; nil commits nothing.
	CommitRecords func() error
	// WriteFrictionNote writes the friction note of a paused stop that carries no reason; nil writes none.
	WriteFrictionNote func(stop Stop) error
	// Watched reports whether a live batten watches the run; nil is never watched.
	Watched func() bool
	// Arbiter is the stronger agent a tackler escalates to; nil means none is wired.
	Arbiter *ArbiterSeam
	// Verify reads the verify halt record; nil means no stop is a verify halt.
	Verify *VerifySeam
	// HandOver hands the run to a fresh driver of a newer build; nil means the driver reads no build and never hands over.
	HandOver *HandOverSeam
	// Tamper compares the worktree with the snapshot the tackler attempt recorded; nil acts on a tackler verdict without the comparison.
	Tamper *TamperSeam
}

// ArbiterSeam is the stronger agent a tackler escalation, or a stop only it may judge, goes to.
type ArbiterSeam struct {
	// EscalationPending reports whether the arbiter would admit the awaiting stop at producer: its eligibility, not mere pendency of an escalation.
	EscalationPending func(producer string) (bool, error)
	// Judge spawns the arbiter for the stop envelope at envelopePath, or returns the verdict of an earlier attempt.
	Judge func(envelopePath, reportPath, caseBriefPath string) (ArbiterOutcome, error)
	// AttemptEnvelope returns the stop envelope the arbiter attempt in attemptDir recorded, for the restart read of an open attempt.
	AttemptEnvelope func(attemptDir string) (envelopePath string, err error)
}

// ArbiterOutcome is what the driver reads off the arbiter's verdict envelope.
type ArbiterOutcome struct {
	// Resume is true when the arbiter's verdict is to resume the run.
	Resume   bool
	Reason   string
	Question string
	Goto     string
	// Cause is empty for the agent's own verdict, else why the call itself handed the stop on.
	Cause       string
	Attempt     int
	AttemptDir  string
	VerdictPath string
	// Verdict is the verdict word the stop report's attempt line quotes.
	Verdict string
	// Role is the role name the stop report's attempt line quotes.
	Role string
}

// VerifySeam reads the verify halt record a blocked stop may match.
type VerifySeam struct {
	// Halt reports whether the verify failure record matches the blocked stop, and the row it names.
	Halt func() (matches bool, target string, err error)
	// RetrySpentElsewhere reports whether another actor already spent the run's one verify retry; optional, ORed with the settle records.
	RetrySpentElsewhere func() (bool, error)
}

// HandOverSeam hands the run to a fresh driver of a newer build.
type HandOverSeam struct {
	// OnDiskBuild reads the build identity of the lyx executable now on disk.
	OnDiskBuild func() (buildvcs.Identity, error)
	// BuildChanged reports whether the last step ran a known build other than onDisk.
	BuildChanged func(onDisk buildvcs.Identity) bool
	// DirtyPaths lists the task pair's uncommitted or untracked paths outside the run's own records directories.
	DirtyPaths func() ([]string, error)
	// ReleaseLock releases the driver lock.
	ReleaseLock func() error
	// Executable is the on-disk lyx path resolved when the driver was wired, before any deploy replaced it.
	Executable string
	// SpawnSuccessor starts a detached driver of the on-disk build from the lyx at executable.
	SpawnSuccessor func(executable string) error
}

// TamperSeam compares the worktree with what the tackler found.
type TamperSeam struct {
	// Snapshot reads the worktree state a tackler must leave as it found it.
	Snapshot func() (WorktreeSnapshot, error)
	// AttemptSnapshot reads the worktree snapshot the tackler attempt in tacklerDir recorded at spawn.
	AttemptSnapshot func(tacklerDir string) (WorktreeSnapshot, error)
}

// goDriver settles one run's stops over its seams.
// rules and attempts are what the stop report quotes for the stop in hand, pendingAttempt is the arbiter attempt the next step consumes, and exit is how the run ended.
type goDriver struct {
	deps           Deps
	rules          []string
	attempts       []driveAttemptNote
	pendingAttempt string
	exit           Exit
}

// Run steps the run and settles each stop until the run is done, another holder has it or a hand-back ends the driver, and returns how it ended.
// A failure the driver cannot act on is handed back; only a failed hand-back or a cancelled context is returned as an error.
func Run(ctx context.Context, deps Deps) (Exit, error) {
	d := &goDriver{deps: deps}
	err := d.run(ctx)
	return d.exit, err
}

// run steps the run and settles each stop until the driver is finished.
func (d *goDriver) run(ctx context.Context) error {
	stop, resumed, err := d.resumeOpenAttempt(ctx)
	if err != nil {
		return d.handBackFailure(err)
	}
	attempt := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !resumed {
			raw, err := d.deps.Step(ctx, attempt)
			if err != nil {
				return d.handBackFailure(fmt.Errorf("the step failed to run: %w", err))
			}
			stop, err = parseDriveStop(raw)
			if err != nil {
				return d.handBackFailure(err)
			}
			path, err := d.deps.SaveEnvelope(raw)
			if err != nil {
				return d.handBackFailure(fmt.Errorf("saving the stop envelope: %w", err))
			}
			stop.EnvelopePath = path
			if attempt != "" && stop.Kind != shedverbs.KindBusy && arbiterAttemptRefused(raw) {
				d.rules = append(d.rules, "the step refused the arbiter's attempt")
				return d.handBackAs(stop, driveHandBack{}, "the step refused the arbiter attempt")
			}
		}
		if attempt == "" {
			d.rules, d.attempts = nil, nil
		}
		resumed = false

		next, exit, err := d.settleStop(ctx, stop)
		if err != nil || exit {
			return err
		}
		attempt = next
	}
}

// arbiterAttemptRefused reports whether the step envelope refused the arbiter attempt it was handed.
func arbiterAttemptRefused(envelope []byte) bool {
	var decoded struct {
		ArbiterAttempt string `json:"arbiter_attempt"`
	}
	return json.Unmarshal(envelope, &decoded) == nil && decoded.ArbiterAttempt != ""
}

// settleStop decides stop and carries the decision out.
// It returns the arbiter attempt the next step consumes, and exit true when the driver is finished.
func (d *goDriver) settleStop(ctx context.Context, stop Stop) (attempt string, exit bool, err error) {
	for {
		facts, err := d.facts(stop)
		if err != nil {
			return "", true, d.handBackStop(stop, driveDecision{Rule: "the driver could not read the facts of the stop"}, err)
		}
		decision := decideDrive(stop, facts)
		d.rules = append(d.rules, decision.Rule)
		switch decision.Action {
		case driveActionDone:
			return "", true, d.finishDone(stop)
		case driveActionBusyExit:
			d.settle(stop, shedrun.SettlerGoRule, shedrun.SettleBusyExit, decision.Rule, "")
			logger.Info("sheddriver: drive: another holder has the run; exiting", "run", d.deps.RunID, "holder", stop.Error)
			d.exit = ExitBusy
			return "", true, nil
		}
		if decision.Action != driveActionHandBack && decision.Action != driveActionRelayHandBack {
			if err := d.holdPause(ctx); err != nil {
				return "", true, err
			}
		}
		next, exit, err := d.recheck(stop)
		if err != nil {
			return "", true, d.handBackStop(stop, decision, err)
		}
		if exit {
			return "", true, nil
		}
		if next.Restep {
			return d.takePendingAttempt(), false, nil
		}
		if next.Key() != stop.Key() {
			stop = next
			continue
		}

		switch decision.Action {
		case driveActionRestep:
			d.settle(stop, shedrun.SettlerGoRule, shedrun.SettleRestep, decision.Rule, "")
			return "", false, nil
		case driveActionTackler, driveActionArbiter:
			var moved Stop
			if decision.Action == driveActionTackler {
				moved, exit, err = d.tackle(ctx, stop)
			} else {
				moved, exit, err = d.arbitrate(ctx, stop, "")
			}
			switch {
			case err != nil || exit:
				return "", true, err
			case moved.Restep:
				return d.takePendingAttempt(), false, nil
			}
			stop = moved
			continue
		case driveActionVerifyGoto:
			d.settle(stop, shedrun.SettlerGoRule, shedrun.SettleVerifyRetry, decision.Rule, facts.VerifyTarget)
			if err := d.deps.Goto(ctx, facts.VerifyTarget); err != nil {
				logger.Warn("sheddriver: drive: the verify goto was refused; rechecking the run", "run", d.deps.RunID, "target", facts.VerifyTarget, "error", err)
				moved, movedExit, recheckErr := d.recheck(stop)
				if recheckErr != nil {
					return "", true, d.handBackStop(stop, decision, recheckErr)
				}
				switch {
				case movedExit:
					return "", true, nil
				case moved.Restep:
					return "", false, nil
				}
				stop = moved
				continue
			}
			return "", false, nil
		case driveActionHandBack, driveActionRelayHandBack:
			return "", true, d.handBackStop(stop, decision, nil)
		case driveActionHandOver:
			return "", true, d.handOver(ctx, stop)
		}
		panic(fmt.Sprintf("sheddriver: drive: the rule table returned the unhandled action %q", decision.Action))
	}
}

// handedOver reports whether the settle records hold a hand-over for key under the build labelled build.
func handedOver(records []SettleRecord, key StopKey, build string) bool {
	for _, record := range records {
		if record.StopKey == key && record.Action == shedrun.SettleHandOver && record.Build == build {
			return true
		}
	}
	return false
}

// handOver hands the run to a fresh driver of the on-disk build at a binary-eligible stop whose last step ran another build, holding any pause first.
// A tree with uncommitted paths outside the run's records hands back with the paths in the stop report and spawns nothing.
// Otherwise it records the hand-over, leaves the stop report, the park and the unwatched parent's notice, releases the driver lock and spawns the successor.
// A successor that fails to spawn leaves the parked run and its report for the parent, or a watching batten, to pick up.
// The rule table reaches it only over facts the HandOver seam read, so the seam is set.
func (d *goDriver) handOver(ctx context.Context, stop Stop) error {
	seam := d.deps.HandOver
	if err := d.holdPause(ctx); err != nil {
		return err
	}
	dirty, err := seam.DirtyPaths()
	if err != nil {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"reading the uncommitted paths: " + err.Error()}}, "the hand-over could not check the tree")
	}
	if len(dirty) > 0 {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"the on-disk build changed but the tree has uncommitted paths outside the run's records, so no successor is spawned: " + strings.Join(dirty, ", ")}}, "the tree is not clean for a hand-over")
	}
	onDisk, err := seam.OnDiskBuild()
	if err != nil {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"reading the on-disk build: " + err.Error()}}, "the on-disk build could not be read")
	}

	build := onDisk.Label()
	record := SettleRecord{StopKey: stop.Key(), Settler: shedrun.SettlerGoRule, Action: shedrun.SettleHandOver, Outcome: "handed over to a fresh driver", At: d.deps.Now(), Build: build}
	if err := AppendSettleRecord(d.deps.SettlePath, record); err != nil {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"recording the hand-over: " + err.Error()}}, "the hand-over could not be recorded")
	}
	if err := d.handBack(stop, driveHandBack{Rules: []string{"handing the run over to a fresh driver of the on-disk build " + build}}); err != nil {
		return err
	}
	d.exit = ExitHandOver
	if err := seam.ReleaseLock(); err != nil {
		return fmt.Errorf("sheddriver: drive: release the driver lock for the hand-over: %w", err)
	}
	if err := seam.SpawnSuccessor(seam.Executable); err != nil {
		logger.Warn("sheddriver: drive: spawning the successor driver failed; the run stays parked", "run", d.deps.RunID, "build", build, "error", err)
		return nil
	}
	logger.Info("sheddriver: drive: handed the run over to a fresh driver", "run", d.deps.RunID, "build", build)
	return nil
}

// DriveLock names what DriveVerb holds and says for one recipe.
type DriveLock struct {
	// Path is the driver lock file.
	Path string
	// Stale are the files removed once the lock is held, an absent one ignored.
	Stale []string
	// Refusal is the refusal printed when another driver holds the lock.
	Refusal string
	// RetryClause ends the way forward of every transient failure, naming what the operator re-runs.
	RetryClause string
}

// DriveVerb runs a driver under its driver lock and returns the exit code.
// It takes the lock without blocking and refuses at once when another driver holds it, so a second driver never stacks.
// With the lock held it removes each stale file, ignoring an absent one, and calls run, handing it the release that lets a hand-over free the lock early.
// The lock is released exactly once, whether run released it early or not.
// run returns the fields of the success envelope; a recipe whose verb prints its own envelope reuses the lock handling.
func DriveVerb(out io.Writer, driveLock DriveLock, run func(release func() error) (map[string]any, error)) int {
	transient := "; way forward: transient, " + driveLock.RetryClause
	if err := os.MkdirAll(filepath.Dir(driveLock.Path), 0o755); err != nil {
		return output.Err(out, "sheddriver: drive: create the driver lock directory: "+err.Error()+transient)
	}
	held, acquired, err := lock.TryAcquireWriteLock(driveLock.Path)
	if err != nil {
		return output.Err(out, "sheddriver: drive: take the driver lock: "+err.Error()+transient)
	}
	if !acquired {
		return output.Err(out, driveLock.Refusal)
	}
	released := false
	release := func() error {
		if released {
			return nil
		}
		released = true
		return held.Release()
	}

	for _, path := range driveLock.Stale {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			_ = release()
			return output.Err(out, "sheddriver: drive: remove the stale file "+path+": "+err.Error()+transient)
		}
	}

	fields, runErr := run(release)
	releaseErr := release()
	switch {
	case runErr != nil:
		return output.Err(out, runErr.Error())
	case releaseErr != nil:
		return output.Err(out, "sheddriver: drive: release the driver lock: "+releaseErr.Error())
	}
	return output.Ok(out, fields)
}

// takePendingAttempt returns the arbiter attempt the next step consumes, and clears it.
func (d *goDriver) takePendingAttempt() string {
	attempt := d.pendingAttempt
	d.pendingAttempt = ""
	return attempt
}

// tackle asks a cold tackler to judge stop and acts on its verdict, holding any pause first.
// The stop report so far is written for the tackler to read, and a fresh attempt leaves a spawn record.
// A verdict is acted on only when the run is where the tackler found it: a moved run discards the verdict and the fresh state is acted on as a new stop, and a worktree the tackler changed hands back as tampered.
// It returns the stop to go on with, marked Restep when the run is to be stepped, or exit true when the driver is finished.
func (d *goDriver) tackle(ctx context.Context, stop Stop) (next Stop, exit bool, err error) {
	if err := d.holdPause(ctx); err != nil {
		return Stop{}, true, err
	}
	reportPath, err := d.startReport(stop)
	if err != nil {
		return Stop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{err.Error()}}, "the stop report could not be written")
	}
	before, _, _, err := d.deps.OpenAttempts(stop.Key())
	if err != nil {
		return Stop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"listing the stop's attempts: " + err.Error()}}, "the stop's attempts could not be listed")
	}

	envelope, err := d.deps.Tackler(stop, reportPath)
	switch {
	case errors.Is(err, ErrTacklerResumeReturned):
		return d.arbitrate(ctx, stop, "")
	case err != nil:
		return Stop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the tackler could not judge the stop: " + err.Error()}}, "the tackler was unavailable or refused the stop")
	}
	if envelope.AttemptDir != before {
		d.settle(stop, shedrun.SettlerTackler, shedrun.SettleSpawn, "tackler spawned", fmt.Sprintf("attempt-%d", envelope.Attempt))
	}
	note := driveAttemptNote{Role: TacklerRole, Number: envelope.Attempt, Verdict: envelope.Verdict, Cause: envelope.Cause, VerdictPath: envelope.VerdictPath}
	d.attempts = append(d.attempts, note)

	if moved, movedExit, err := d.recheckAfterVerdict(stop, reportPath, "tackler"); err != nil || movedExit || moved.Key() != stop.Key() || moved.Restep {
		if err != nil {
			return Stop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the driver could not act: " + err.Error()}}, "the run could not be rechecked")
		}
		return moved, movedExit, nil
	}

	if d.deps.Tamper != nil {
		tampered, err := d.worktreeChangedBy(envelope.AttemptDir)
		if err != nil {
			return Stop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"comparing the worktree with the tackler's snapshot: " + err.Error()}}, "the tackler's worktree check could not run")
		}
		if tampered {
			d.attempts[len(d.attempts)-1].Cause = "tampered"
			h := driveHandBack{
				Rules:    []string{"the worktree differs from the snapshot taken when the tackler spawned: its verdict is not acted on"},
				Question: "The tackler left the worktree changed, so its verdict was not acted on; decide this stop yourself and report the choice in the stop report.",
			}
			return Stop{}, true, d.handBackAs(stop, h, "the tackler changed the worktree")
		}
	}

	switch envelope.Verdict {
	case TacklerVerdictResume:
		if err := d.deps.ConsumeTackler(envelope.AttemptDir); err != nil {
			logger.Warn("sheddriver: drive: the tackler's resume was refused", "run", d.deps.RunID, "error", err)
			moved, movedExit, recheckErr := d.recheck(stop)
			switch {
			case recheckErr != nil:
				return Stop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the driver could not act: " + recheckErr.Error()}}, "the run could not be rechecked")
			case movedExit:
				return Stop{}, true, nil
			case moved.Restep || moved.Key() != stop.Key():
				return moved, false, nil
			}
			return Stop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the tackler's resume was refused: " + err.Error()}}, "the tackler's resume was refused")
		}
		d.settle(stop, shedrun.SettlerTackler, shedrun.SettleRestep, "tackler resume", envelope.Goto)
		stop.Restep = true
		return stop, false, nil
	case TacklerVerdictEscalate:
		if d.deps.Arbiter == nil {
			h := driveHandBack{
				Question: fmt.Sprintf("The tackler escalated this stop and no stronger agent is wired; read its case brief at %s, decide the stop yourself and report the choice in the stop report.", envelope.CaseBriefPath),
				Rules:    []string{"the tackler escalated the stop and no arbiter is wired: hand back"},
			}
			return Stop{}, true, d.handBackAs(stop, h, "the tackler escalated and no arbiter is wired")
		}
		return d.arbitrate(ctx, stop, envelope.CaseBriefPath)
	}
	h := driveHandBack{Question: envelope.Question, Rules: []string{"the tackler handed the stop back: " + envelope.Reason}}
	return Stop{}, true, d.handBackAs(stop, h, "the tackler handed the stop back")
}

// arbitrate asks the arbiter to judge stop and acts on its verdict, holding any pause first.
// A resume leaves the attempt the next step consumes; a refusal or a hand-on hands back, and at an awaiting stop relays its parent notice.
// With no arbiter wired it hands back at once, relaying the same notice.
// The moved-run and spawn-record rules are the tackler's.
func (d *goDriver) arbitrate(ctx context.Context, stop Stop, caseBriefPath string) (next Stop, exit bool, err error) {
	relay := ""
	if stop.Key().State == string(shedengine.StateAwaiting) {
		relay = stop.ParentNotice
	}
	if d.deps.Arbiter == nil {
		return Stop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{"no arbiter is wired: hand back"}}, "no arbiter is wired")
	}
	if err := d.holdPause(ctx); err != nil {
		return Stop{}, true, err
	}
	reportPath, err := d.startReport(stop)
	if err != nil {
		return Stop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{err.Error()}}, "the stop report could not be written")
	}
	_, before, _, err := d.deps.OpenAttempts(stop.Key())
	if err != nil {
		return Stop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{"listing the stop's attempts: " + err.Error()}}, "the stop's attempts could not be listed")
	}

	outcome, err := d.deps.Arbiter.Judge(stop.EnvelopePath, reportPath, caseBriefPath)
	if err != nil {
		return Stop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{"the arbiter refused the stop: " + err.Error()}}, "the arbiter refused the stop")
	}
	if outcome.AttemptDir != before {
		d.settle(stop, shedrun.SettlerArbiter, shedrun.SettleSpawn, "arbiter spawned", fmt.Sprintf("attempt-%d", outcome.Attempt))
	}
	d.attempts = append(d.attempts, driveAttemptNote{Role: outcome.Role, Number: outcome.Attempt, Verdict: outcome.Verdict, Cause: outcome.Cause, VerdictPath: outcome.VerdictPath})

	if moved, movedExit, err := d.recheckAfterVerdict(stop, reportPath, "arbiter"); err != nil || movedExit || moved.Key() != stop.Key() || moved.Restep {
		if err != nil {
			return Stop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{"the driver could not act: " + err.Error()}}, "the run could not be rechecked")
		}
		return moved, movedExit, nil
	}

	if outcome.Resume {
		d.settle(stop, shedrun.SettlerArbiter, shedrun.SettleRestep, "arbiter resume", outcome.Goto)
		d.pendingAttempt = strconv.Itoa(outcome.Attempt)
		stop.Restep = true
		return stop, false, nil
	}
	h := driveHandBack{Question: outcome.Question, Relay: relay, Rules: []string{"the arbiter handed the stop on: " + outcome.Reason}}
	return Stop{}, true, d.handBackAs(stop, h, "the arbiter handed the stop on")
}

// recheckAfterVerdict re-reads the run after an agent's verdict, and when the run moved while the agent worked, notes the discarded verdict in the stop report at reportPath.
// It returns the stop itself for an unmoved run.
func (d *goDriver) recheckAfterVerdict(stop Stop, reportPath, agent string) (next Stop, exit bool, err error) {
	next, exit, err = d.recheck(stop)
	if err != nil || exit {
		return Stop{}, exit, err
	}
	if next.Restep || next.Key() != stop.Key() {
		line := fmt.Sprintf("\n- The run moved while the %s worked, from stop key %+v to %+v; its verdict was discarded and the new state is acted on.\n", agent, stop.Key(), next.Key())
		if err := appendToFile(reportPath, line); err != nil {
			logger.Warn("sheddriver: drive: noting the moved run in the stop report failed", "run", d.deps.RunID, "error", err)
		}
	}
	return next, false, nil
}

// worktreeChangedBy reports whether the worktree differs from the snapshot the tackler attempt in attemptDir recorded at spawn.
// The Tamper seam is set when it is called.
func (d *goDriver) worktreeChangedBy(attemptDir string) (bool, error) {
	spawned, err := d.deps.Tamper.AttemptSnapshot(attemptDir)
	if err != nil {
		return false, err
	}
	current, err := d.deps.Tamper.Snapshot()
	if err != nil {
		return false, err
	}
	return !spawned.Equal(current), nil
}

// startReport writes the stop report so far at a fresh path, for an agent to read, and returns the path.
func (d *goDriver) startReport(stop Stop) (string, error) {
	path := d.deps.NewReportPath()
	report := d.reportFor(stop, driveHandBack{Rules: slices.Clone(d.rules), Attempts: slices.Clone(d.attempts)}, path)
	report.At = d.deps.Now()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create the stop report directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(report.render()), 0o644); err != nil {
		return "", fmt.Errorf("write the stop report %s: %w", path, err)
	}
	return path, nil
}

// appendToFile appends text to the file at path.
func appendToFile(path, text string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// resumeOpenAttempt finds, once before the first step, the stop a halted run left with an agent attempt still open, so a restarted driver reuses the attempt instead of spawning a second agent.
// An attempt is open when the settle records hold no hand-back for its stop key later than its spawn: a stop the parent took and then resumed finds its attempt closed, and the driver steps.
// An arbiter attempt is ignored when no arbiter is wired.
// It returns the stop rebuilt from the envelope the attempt recorded; the rule table then sends it to the tackler or the arbiter, whose own call reuses the attempt.
func (d *goDriver) resumeOpenAttempt(ctx context.Context) (stop Stop, found bool, err error) {
	status, statusFound, err := d.deps.ReadStatus()
	if err != nil {
		return Stop{}, false, fmt.Errorf("reading the status before the first step: %w", err)
	}
	if !statusFound || status.State == shedengine.StateRunning || status.State == shedengine.StateDone {
		return Stop{}, false, nil
	}
	key := StopKeyOf(status)
	tacklerDir, arbiterDir, _, err := d.deps.OpenAttempts(key)
	if err != nil {
		return Stop{}, false, fmt.Errorf("listing the stop's attempts: %w", err)
	}
	if d.deps.Arbiter == nil {
		arbiterDir = ""
	}
	if tacklerDir == "" && arbiterDir == "" {
		return Stop{}, false, nil
	}
	records, err := ReadSettleRecords(d.deps.SettlePath)
	if err != nil {
		return Stop{}, false, err
	}
	if attemptClosed(records, key) {
		return Stop{}, false, nil
	}

	envelopePath := ""
	if tacklerDir != "" {
		if attempt, err := ReadTacklerAttempt(tacklerDir); err == nil {
			envelopePath = attempt.EnvelopePath
		}
	}
	if envelopePath == "" && arbiterDir != "" {
		if path, err := d.deps.Arbiter.AttemptEnvelope(arbiterDir); err == nil {
			envelopePath = path
		}
	}
	raw, err := os.ReadFile(envelopePath)
	if err != nil {
		logger.Warn("sheddriver: drive: the open attempt's stop envelope is unreadable; stepping instead", "run", d.deps.RunID, "path", envelopePath, "error", err)
		return Stop{}, false, nil
	}
	stop, err = parseDriveStop(raw)
	if err != nil || stop.Key() != key {
		logger.Warn("sheddriver: drive: the open attempt's stop envelope is not the run's stop; stepping instead", "run", d.deps.RunID, "path", envelopePath, "error", err)
		return Stop{}, false, nil
	}
	stop.EnvelopePath = envelopePath
	return stop, true, nil
}

// attemptClosed reports whether the settle records hold a hand-back for key later than the key's latest spawn.
// A key with no spawn record counts any hand-back as later.
func attemptClosed(records []SettleRecord, key StopKey) bool {
	var spawned time.Time
	for _, record := range records {
		if record.StopKey == key && record.Action == shedrun.SettleSpawn && record.At.After(spawned) {
			spawned = record.At
		}
	}
	for _, record := range records {
		if record.StopKey == key && record.Action == shedrun.SettleHandBack && record.At.After(spawned) {
			return true
		}
	}
	return false
}

// facts reads the disk facts decideDrive needs for stop; a group the recipe leaves unwired reads as its zero fact.
func (d *goDriver) facts(stop Stop) (driveFacts, error) {
	var facts driveFacts
	key := stop.Key()
	if key.State == string(shedengine.StateAwaiting) && d.deps.Arbiter != nil {
		pending, err := d.deps.Arbiter.EscalationPending(key.CurrentProducer)
		if err != nil {
			return driveFacts{}, fmt.Errorf("reading the pending escalation: %w", err)
		}
		facts.EscalationPending = pending
	}

	records, err := ReadSettleRecords(d.deps.SettlePath)
	if err != nil {
		return driveFacts{}, err
	}
	facts.RepairCount = repairCount(records, key)
	_, _, applied, err := d.deps.OpenAttempts(key)
	if err != nil {
		return driveFacts{}, fmt.Errorf("listing the stop's attempts: %w", err)
	}
	facts.ResumeReturned = applied
	facts.Watched = d.watched()

	if binaryEligible(stop) && d.deps.HandOver != nil {
		onDisk, err := d.deps.HandOver.OnDiskBuild()
		if err != nil {
			logger.Warn("sheddriver: drive: the on-disk build is unreadable; reading it as unchanged", "run", d.deps.RunID, "error", err)
		} else {
			facts.BuildChanged = d.deps.HandOver.BuildChanged(onDisk)
			facts.HandedOver = handedOver(records, key, onDisk.Label())
		}
	}

	if key.State == string(shedengine.StateBlocked) && d.deps.Verify != nil {
		matches, target, err := d.deps.Verify.Halt()
		if err != nil {
			return driveFacts{}, fmt.Errorf("reading the verify halt: %w", err)
		}
		facts.VerifyMatches, facts.VerifyTarget = matches, target
	}
	if facts.VerifyMatches {
		facts.VerifyRetrySpent = GoVerifyRetrySpent(records)
		if d.deps.Verify.RetrySpentElsewhere != nil {
			spent, err := d.deps.Verify.RetrySpentElsewhere()
			if err != nil {
				return driveFacts{}, fmt.Errorf("reading whether the verify retry was spent elsewhere: %w", err)
			}
			facts.VerifyRetrySpent = facts.VerifyRetrySpent || spent
		}
	}
	return facts, nil
}

// watched reports whether a live batten watches the run; an unwired Watched is never watched.
func (d *goDriver) watched() bool {
	return d.deps.Watched != nil && d.deps.Watched()
}

// recheck re-reads the status before the driver acts on stop, so nothing the run's other actors did since the step goes unseen.
// A running status or a run lock another process holds ends the driver with a running-exit record.
// A status at another stop key is a moved run: it returns the stop to act on, marked Restep when an orch goto moved the run, and otherwise decided afresh from the status.
// A moved run is never parked and gets no friction note.
// An unmoved run returns stop itself.
func (d *goDriver) recheck(stop Stop) (next Stop, exit bool, err error) {
	status, found, err := d.deps.ReadStatus()
	if err != nil {
		return Stop{}, false, fmt.Errorf("re-reading the status: %w", err)
	}
	if !found {
		return Stop{}, false, fmt.Errorf("re-reading the status: the run has no status file")
	}
	held, err := d.deps.RunLockHeld()
	if err != nil {
		return Stop{}, false, fmt.Errorf("probing the run lock: %w", err)
	}
	if held || (status.State == shedengine.StateRunning && stop.Key().State != string(shedengine.StateRunning)) {
		d.settle(stop, shedrun.SettlerGoRule, shedrun.SettleRunningExit, "another process is running the run", "")
		logger.Info("sheddriver: drive: the run is running elsewhere; exiting", "run", d.deps.RunID, "lock_held", held, "state", status.State)
		d.exit = ExitRunningElsewhere
		return stop, true, nil
	}
	if StopKeyOf(status) == stop.Key() {
		return stop, false, nil
	}

	if n := len(status.History); n > 0 && status.History[n-1].Outcome == shedengine.OutcomeGoto {
		d.settle(stop, shedrun.SettlerGoRule, shedrun.SettleMoved, "an orch goto moved the run: re-step", status.CurrentProducer)
		moved := stopFromStatus(status)
		moved.Restep = true
		return moved, false, nil
	}

	moved := stopFromStatus(status)
	raw, err := json.Marshal(map[string]any{
		"state":  moved.State,
		"reason": "",
		"loop":   map[string]any{"current_producer": status.CurrentProducer, "history_length": len(status.History)},
	})
	if err != nil {
		return Stop{}, false, fmt.Errorf("encoding the moved run's envelope: %w", err)
	}
	path, err := d.deps.SaveEnvelope(raw)
	if err != nil {
		return Stop{}, false, fmt.Errorf("saving the moved run's envelope: %w", err)
	}
	moved.Raw = raw
	moved.EnvelopePath = path
	d.settle(stop, shedrun.SettlerGoRule, shedrun.SettleMoved, "the run moved to another stop: decide it afresh", status.CurrentProducer)
	return moved, false, nil
}

// holdPause returns once no pause covers the run's slug, waiting in PauseHold while one does.
// An unreadable pause record holds nothing, with one Warn.
func (d *goDriver) holdPause(ctx context.Context) error {
	for {
		coverage, err := d.deps.Pause(d.deps.Now())
		if err != nil {
			logger.Warn("sheddriver: drive: the pause record is unreadable; not holding", "run", d.deps.RunID, "error", err)
			return nil
		}
		if !coverage.Covered {
			return nil
		}
		if err := d.deps.PauseHold(ctx); err != nil {
			return err
		}
	}
}

// settle appends one settle record for the action taken at stop; a failed append is logged and never stops the driver.
func (d *goDriver) settle(stop Stop, settler, action, outcome, target string) {
	record := SettleRecord{StopKey: stop.Key(), Settler: settler, Action: action, Outcome: outcome, At: d.deps.Now(), Target: target}
	if err := AppendSettleRecord(d.deps.SettlePath, record); err != nil {
		logger.Warn("sheddriver: drive: appending a settle record failed", "run", d.deps.RunID, "action", action, "error", err)
	}
}

// finishDone commits the run's records when CommitRecords is wired, and hands a run that finished with friction: failed to the parent without parking it.
func (d *goDriver) finishDone(stop Stop) error {
	d.exit = ExitDone
	if d.deps.CommitRecords != nil {
		if err := d.deps.CommitRecords(); err != nil {
			return fmt.Errorf("sheddriver: drive: commit the run's records: %w", err)
		}
	}
	if stop.Friction != "failed" {
		return nil
	}
	err := d.handBack(stop, driveHandBack{DoneFrictionFailed: true})
	d.exit = ExitDone
	return err
}

// handBackStop hands the run back at stop on the decision's rule, after the friction note a paused stop without a reason asks for.
// cause, when set, is a failure that kept the driver from acting and goes into the rules the report quotes.
func (d *goDriver) handBackStop(stop Stop, decision driveDecision, cause error) error {
	if decision.FrictionNote && d.deps.WriteFrictionNote != nil {
		if err := d.deps.WriteFrictionNote(stop); err != nil {
			logger.Warn("sheddriver: drive: writing the friction note failed", "run", d.deps.RunID, "error", err)
		}
	}
	h := driveHandBack{WatchedVerifyLine: decision.WatchedVerifyLine}
	if cause != nil {
		logger.Warn("sheddriver: drive: the driver could not act at the stop; handing back", "run", d.deps.RunID, "error", cause)
		h.Rules = []string{"the driver could not act: " + cause.Error()}
	}
	if decision.Action == driveActionRelayHandBack {
		h.Relay = stop.ParentNotice
	}
	return d.handBackAs(stop, h, decision.Rule)
}

// handBackAs records the hand-back in the settle records, with outcome as its reason, and leaves the stop report, the park and the parent's notices.
func (d *goDriver) handBackAs(stop Stop, h driveHandBack, outcome string) error {
	d.settle(stop, shedrun.SettlerParent, shedrun.SettleHandBack, outcome, "")
	return d.handBack(stop, h)
}

// handBackFailure hands the run back at the stop its status shows, or at an empty stop when the status cannot be read, for a failure that left no stop to decide.
func (d *goDriver) handBackFailure(cause error) error {
	var stop Stop
	if status, found, err := d.deps.ReadStatus(); err == nil && found {
		stop = stopFromStatus(status)
	}
	rule := "a failure the driver cannot act on: hand back"
	d.rules = append(d.rules, rule)
	return d.handBackStop(stop, driveDecision{Action: driveActionHandBack, Rule: rule}, cause)
}

// handBack builds the stop report for stop and h, and leaves the report, the park and the parent's notices through handBackRun.
// The report quotes the rules applied and the agents' attempts at the stop so far, then h's own.
func (d *goDriver) handBack(stop Stop, h driveHandBack) error {
	d.exit = ExitHandBack
	reportPath := d.deps.NewReportPath()
	park := d.deps.HandBack
	park.ReportPath = reportPath

	if stop.Error != "" {
		logger.Warn("sheddriver: drive: handing the run back", "run", d.deps.RunID, "state", stop.Key().State, "producer", stop.CurrentProducer, "error", stop.Error, "report", reportPath)
	} else {
		logger.Info("sheddriver: drive: handing the run back", "run", d.deps.RunID, "state", stop.Key().State, "producer", stop.CurrentProducer, "report", reportPath)
	}

	h.RunID = d.deps.RunID
	h.ReportPath = reportPath
	h.Stop = stop
	h.Rules = append(slices.Clone(d.rules), h.Rules...)
	h.Attempts = append(slices.Clone(d.attempts), h.Attempts...)
	return handBackRun(park, h, d.reportFor(stop, h, reportPath), d.watched())
}

// reportFor builds the stop report of a hand-back at stop that is to be written at reportPath.
func (d *goDriver) reportFor(stop Stop, h driveHandBack, reportPath string) driveReport {
	reason := stop.Reason
	if reason == "" {
		reason = stop.Error
	}
	if reason == "" {
		reason = stop.Key().State
	}
	return driveReport{
		RunID:         d.deps.RunID,
		HistoryLength: stop.HistoryLength,
		Progress:      fmt.Sprintf("%s at %s", stop.Key().State, stop.CurrentProducer),
		Reason:        reason,
		Rules:         h.Rules,
		Attempts:      h.Attempts,
		Question:      h.Question,
		FrictionDir:   d.deps.FrictionDir,
		RepairDir:     filepath.Dir(reportPath),
	}
}
