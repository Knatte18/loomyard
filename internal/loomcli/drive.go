// drive.go holds the Go driver's settle loop:
// it steps the run through the shared --until-stop loop body, decides each stop by the closed rule table and carries the decision out, leaving a settle record for every move and a stop report for every hand-back.
// The loop works over injected seams, so every branch is testable without a process; the verb that wires the real ones sits beside it.

package loomcli

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

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// goDriverDeps is the Go driver's whole surface to the world.
type goDriverDeps struct {
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
	// FrictionDir is the run's friction directory, which the stop report quotes.
	FrictionDir string
	// EscalationPending reports whether the arbiter verb would admit the awaiting stop at producer: a Bouncer row whose latest round awaits a decision on a hard-cap or circling escalation, not mere pendency.
	EscalationPending func(producer string) (bool, error)
	// VerifyRecord reads the Publish failure record; found is false when there is none.
	VerifyRecord func() (record battenshed.ChildPublishFailure, found bool, err error)
	// Head returns the task worktree's HEAD commit.
	Head func() (string, error)
	// VerifyRows names the rows whose verify failure the failure record can describe.
	VerifyRows []string
	// Watch reads the batten-watched marker; the bool is true only when that batten is alive.
	Watch func() (watch battenWatch, alive bool)
	// BattenRetrySpent reports whether the watching prime's batten already spent the run's one verify retry.
	BattenRetrySpent func(w battenWatch) (bool, error)
	// Pause reads what the hub's pause record says about the run's slug at now.
	Pause func(now time.Time) (pauserecord.Coverage, error)
	// PauseHold waits until no pause covers the run's slug.
	PauseHold func(ctx context.Context) error
	// NewReportPath returns a fresh stop report path.
	NewReportPath func() string
	// Park holds what a hand-back reads; its ReportPath is replaced by NewReportPath for each hand-back.
	Park driveParkDeps
	// CommitRecords commits the run's records.
	CommitRecords func() error
	// WriteFrictionNote writes the friction note of a paused stop that carries no reason.
	WriteFrictionNote func(stop driveStop) error
	// Tackler spawns the tackler for stop, or returns the verdict of an earlier attempt, writing its report at reportPath.
	Tackler func(stop driveStop, reportPath string) (sheddriver.TacklerEnvelope, error)
	// ConsumeTackler applies the resume of the tackler attempt in attemptDir to the run.
	ConsumeTackler func(attemptDir string) error
	// Arbiter spawns the arbiter for the stop envelope at envelopePath, or returns the verdict of an earlier attempt.
	Arbiter func(envelopePath, reportPath, caseBriefPath string) (loomengine.ArbiterEnvelope, error)
	// Snapshot reads the worktree state a tackler must leave as it found it.
	Snapshot func() (sheddriver.WorktreeSnapshot, error)
	// OpenAttempts names the newest tackler and arbiter attempt directories recorded for key, empty when there is none, and whether the tackler attempt was applied.
	OpenAttempts func(key sheddriver.StopKey) (tackler, arbiter string, tacklerApplied bool, err error)
	// AttemptSnapshot reads the worktree snapshot the tackler attempt in tacklerDir recorded at spawn.
	AttemptSnapshot func(tacklerDir string) (sheddriver.WorktreeSnapshot, error)
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
	// Now is the clock.
	Now func() time.Time
}

// goDriver settles one run's stops over its seams.
// rules and attempts are what the stop report quotes for the stop in hand, and pendingAttempt is the arbiter attempt the next step consumes.
type goDriver struct {
	deps           goDriverDeps
	rules          []string
	attempts       []driveAttemptNote
	pendingAttempt string
}

// run steps the run and settles each stop until the run is done, another holder has it or a hand-back ends the driver.
// A failure the driver cannot act on is handed back; only a failed hand-back or a cancelled context is returned.
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
func (d *goDriver) settleStop(ctx context.Context, stop driveStop) (attempt string, exit bool, err error) {
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
			d.settle(stop, sheddriver.SettlerGoRule, sheddriver.SettleBusyExit, decision.Rule, "")
			logger.Info("loom: drive: another holder has the run; exiting", "run", d.deps.RunID, "holder", stop.Error)
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
		if next.key() != stop.key() {
			stop = next
			continue
		}

		switch decision.Action {
		case driveActionRestep:
			d.settle(stop, sheddriver.SettlerGoRule, sheddriver.SettleRestep, decision.Rule, "")
			return "", false, nil
		case driveActionTackler, driveActionArbiter:
			var moved driveStop
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
			d.settle(stop, sheddriver.SettlerGoRule, sheddriver.SettleVerifyRetry, decision.Rule, facts.VerifyTarget)
			if err := d.deps.Goto(ctx, facts.VerifyTarget); err != nil {
				logger.Warn("loom: drive: the verify goto was refused; rechecking the run", "run", d.deps.RunID, "target", facts.VerifyTarget, "error", err)
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
		panic(fmt.Sprintf("loom: drive: the rule table returned the unhandled action %q", decision.Action))
	}
}

// handedOver reports whether the settle records hold a hand-over for key under the build labelled build.
func handedOver(records []sheddriver.SettleRecord, key sheddriver.StopKey, build string) bool {
	for _, record := range records {
		if record.StopKey == key && record.Action == sheddriver.SettleHandOver && record.Build == build {
			return true
		}
	}
	return false
}

// handOver hands the run to a fresh driver of the on-disk build at a binary-eligible stop whose last step ran another build, holding any pause first.
// A tree with uncommitted paths outside the run's records hands back with the paths in the stop report and spawns nothing.
// Otherwise it records the hand-over, leaves the stop report, the park marker and the unwatched parent's notice, releases the driver lock and spawns the successor.
// A successor that fails to spawn leaves the parked run and its report for the parent, or a watching batten, to pick up.
func (d *goDriver) handOver(ctx context.Context, stop driveStop) error {
	if err := d.holdPause(ctx); err != nil {
		return err
	}
	dirty, err := d.deps.DirtyPaths()
	if err != nil {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"reading the uncommitted paths: " + err.Error()}}, "the hand-over could not check the tree")
	}
	if len(dirty) > 0 {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"the on-disk build changed but the tree has uncommitted paths outside the run's records, so no successor is spawned: " + strings.Join(dirty, ", ")}}, "the tree is not clean for a hand-over")
	}
	onDisk, err := d.deps.OnDiskBuild()
	if err != nil {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"reading the on-disk build: " + err.Error()}}, "the on-disk build could not be read")
	}

	build := onDisk.Label()
	record := sheddriver.SettleRecord{StopKey: stop.key(), Settler: sheddriver.SettlerGoRule, Action: sheddriver.SettleHandOver, Outcome: "handed over to a fresh driver", At: d.deps.Now(), Build: build}
	if err := sheddriver.AppendSettleRecord(d.deps.SettlePath, record); err != nil {
		return d.handBackAs(stop, driveHandBack{Rules: []string{"recording the hand-over: " + err.Error()}}, "the hand-over could not be recorded")
	}
	if err := d.handBack(stop, driveHandBack{Rules: []string{"handing the run over to a fresh driver of the on-disk build " + build}}); err != nil {
		return err
	}
	if err := d.deps.ReleaseLock(); err != nil {
		return fmt.Errorf("loom: drive: release the driver lock for the hand-over: %w", err)
	}
	if err := d.deps.SpawnSuccessor(d.deps.Executable); err != nil {
		logger.Warn("loom: drive: spawning the successor driver failed; the run stays parked", "run", d.deps.RunID, "build", build, "error", err)
		return nil
	}
	logger.Info("loom: drive: handed the run over to a fresh driver", "run", d.deps.RunID, "build", build)
	return nil
}

// driveRefusal is the refusal of a second Go driver for a run one already drives.
const driveRefusal = "loom: drive: a Go driver already drives this run; way forward: transient, `lyx loom status` shows the run, and the live driver settles it"

// driveVerb runs the Go driver under its driver lock.
// It takes the lock without blocking and refuses at once when another driver holds it, so a second Go driver never stacks.
// With the lock held it removes each stale file, ignoring an absent one, and calls run, handing it the release that lets a hand-over free the lock early.
// The lock is released exactly once, whether run released it early or not.
func driveVerb(out io.Writer, lockPath string, stale []string, run func(release func() error) error) int {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return output.Err(out, "loom: drive: create the driver lock directory: "+err.Error()+"; way forward: transient, re-run lyx loom drive or lyx loom start")
	}
	held, acquired, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return output.Err(out, "loom: drive: take the driver lock: "+err.Error()+"; way forward: transient, re-run lyx loom drive or lyx loom start")
	}
	if !acquired {
		return output.Err(out, driveRefusal)
	}
	released := false
	release := func() error {
		if released {
			return nil
		}
		released = true
		return held.Release()
	}

	for _, path := range stale {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			_ = release()
			return output.Err(out, "loom: drive: remove the stale file "+path+": "+err.Error()+"; way forward: transient, re-run lyx loom drive or lyx loom start")
		}
	}

	runErr := run(release)
	releaseErr := release()
	switch {
	case runErr != nil:
		return output.Err(out, runErr.Error())
	case releaseErr != nil:
		return output.Err(out, "loom: drive: release the driver lock: "+releaseErr.Error())
	}
	return output.Ok(out, map[string]any{"drove": true})
}

// driveCmd builds the hidden `drive` subcommand.
func (c *loomCLI) driveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "drive",
		Hidden:      true,
		Short:       "drive this worktree's go-seeded loom run, settling its stops by rule (spawned by lyx loom start)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `drive is the Go driver of a go-seeded loom run, spawned by lyx loom start, by
lyx loom resume and by a hand-over from a driver of an older build. It steps the
run through the --until-stop loop, settles each stop by a closed rule table, asks
a cold tackler agent about a stop no rule settles, and hands everything it cannot
settle back to the run's parent in a stop report.

It holds the run's driver lock for its whole life, so a second driver refuses at
once. It removes the park marker and the resume report on start.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			stale := []string{shedrun.ParkMarker(c.location, c.runID), shedrun.ResumeReport(c.location, c.runID)}
			clihelp.SetExit(ctx, driveVerb(cmd.OutOrStdout(), shedrun.DriverLock(c.location, c.runID), stale, func(release func() error) error {
				deps, err := c.goDriverDepsFor(release)
				if err != nil {
					return err
				}
				return (&goDriver{deps: deps}).run(ctx)
			}))
			return nil
		},
	}
}

// driveEnvelopePath composes the path of one saved stop envelope, drive-envelope-<compact-timestamp>-<4-hex>.json beside the run's drive reports.
func driveEnvelopePath(l *lyxcwd.Location, runID string, now func() time.Time, rand func() string) string {
	filename := fmt.Sprintf("drive-envelope-%s-%s.json", now().Format(driverReportTimestampLayout), rand())
	return filepath.Join(shedrun.DriveReportsDir(l, runID), filename)
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
func (d *goDriver) tackle(ctx context.Context, stop driveStop) (next driveStop, exit bool, err error) {
	if err := d.holdPause(ctx); err != nil {
		return driveStop{}, true, err
	}
	reportPath, err := d.startReport(stop)
	if err != nil {
		return driveStop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{err.Error()}}, "the stop report could not be written")
	}
	before, _, _, err := d.deps.OpenAttempts(stop.key())
	if err != nil {
		return driveStop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"listing the stop's attempts: " + err.Error()}}, "the stop's attempts could not be listed")
	}

	envelope, err := d.deps.Tackler(stop, reportPath)
	switch {
	case errors.Is(err, errTacklerResumeReturned):
		return d.arbitrate(ctx, stop, "")
	case err != nil:
		return driveStop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the tackler could not judge the stop: " + err.Error()}}, "the tackler was unavailable or refused the stop")
	}
	if envelope.AttemptDir != before {
		d.settle(stop, sheddriver.SettlerTackler, sheddriver.SettleSpawn, "tackler spawned", fmt.Sprintf("attempt-%d", envelope.Attempt))
	}
	note := driveAttemptNote{Role: sheddriver.TacklerRole, Number: envelope.Attempt, Verdict: envelope.Verdict, Cause: envelope.Cause, VerdictPath: envelope.VerdictPath}
	d.attempts = append(d.attempts, note)

	if moved, movedExit, err := d.recheckAfterVerdict(stop, reportPath, "tackler"); err != nil || movedExit || moved.key() != stop.key() || moved.Restep {
		if err != nil {
			return driveStop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the driver could not act: " + err.Error()}}, "the run could not be rechecked")
		}
		return moved, movedExit, nil
	}

	tampered, err := d.worktreeChangedBy(envelope.AttemptDir)
	if err != nil {
		return driveStop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"comparing the worktree with the tackler's snapshot: " + err.Error()}}, "the tackler's worktree check could not run")
	}
	if tampered {
		d.attempts[len(d.attempts)-1].Cause = "tampered"
		h := driveHandBack{
			Rules:    []string{"the worktree differs from the snapshot taken when the tackler spawned: its verdict is not acted on"},
			Question: "The tackler left the worktree changed, so its verdict was not acted on; decide this stop yourself and report the choice in the stop report.",
		}
		return driveStop{}, true, d.handBackAs(stop, h, "the tackler changed the worktree")
	}

	switch envelope.Verdict {
	case sheddriver.TacklerVerdictResume:
		if err := d.deps.ConsumeTackler(envelope.AttemptDir); err != nil {
			logger.Warn("loom: drive: the tackler's resume was refused", "run", d.deps.RunID, "error", err)
			moved, movedExit, recheckErr := d.recheck(stop)
			switch {
			case recheckErr != nil:
				return driveStop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the driver could not act: " + recheckErr.Error()}}, "the run could not be rechecked")
			case movedExit:
				return driveStop{}, true, nil
			case moved.Restep || moved.key() != stop.key():
				return moved, false, nil
			}
			return driveStop{}, true, d.handBackAs(stop, driveHandBack{Rules: []string{"the tackler's resume was refused: " + err.Error()}}, "the tackler's resume was refused")
		}
		d.settle(stop, sheddriver.SettlerTackler, sheddriver.SettleRestep, "tackler resume", envelope.Goto)
		stop.Restep = true
		return stop, false, nil
	case sheddriver.TacklerVerdictEscalate:
		return d.arbitrate(ctx, stop, envelope.CaseBriefPath)
	}
	h := driveHandBack{Question: envelope.Question, Rules: []string{"the tackler handed the stop back: " + envelope.Reason}}
	return driveStop{}, true, d.handBackAs(stop, h, "the tackler handed the stop back")
}

// arbitrate asks the arbiter to judge stop, through the same call `lyx loom arbiter` runs, and acts on its verdict, holding any pause first.
// A resume leaves the attempt the next step consumes; a refusal or a hand-on hands back, and at an awaiting stop relays its parent notice.
// The moved-run and spawn-record rules are the tackler's.
func (d *goDriver) arbitrate(ctx context.Context, stop driveStop, caseBriefPath string) (next driveStop, exit bool, err error) {
	if err := d.holdPause(ctx); err != nil {
		return driveStop{}, true, err
	}
	relay := ""
	if stop.key().State == string(shedengine.StateAwaiting) {
		relay = stop.ParentNotice
	}
	reportPath, err := d.startReport(stop)
	if err != nil {
		return driveStop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{err.Error()}}, "the stop report could not be written")
	}
	_, before, _, err := d.deps.OpenAttempts(stop.key())
	if err != nil {
		return driveStop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{"listing the stop's attempts: " + err.Error()}}, "the stop's attempts could not be listed")
	}

	envelope, err := d.deps.Arbiter(stop.EnvelopePath, reportPath, caseBriefPath)
	if err != nil {
		return driveStop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{"the arbiter refused the stop: " + err.Error()}}, "the arbiter refused the stop")
	}
	if envelope.AttemptDir != before {
		d.settle(stop, sheddriver.SettlerArbiter, sheddriver.SettleSpawn, "arbiter spawned", fmt.Sprintf("attempt-%d", envelope.Attempt))
	}
	d.attempts = append(d.attempts, driveAttemptNote{Role: loomengine.ArbiterRole, Number: envelope.Attempt, Verdict: envelope.Verdict, Cause: envelope.Cause, VerdictPath: envelope.VerdictPath})

	if moved, movedExit, err := d.recheckAfterVerdict(stop, reportPath, "arbiter"); err != nil || movedExit || moved.key() != stop.key() || moved.Restep {
		if err != nil {
			return driveStop{}, true, d.handBackAs(stop, driveHandBack{Relay: relay, Rules: []string{"the driver could not act: " + err.Error()}}, "the run could not be rechecked")
		}
		return moved, movedExit, nil
	}

	if envelope.Verdict == loomengine.ArbiterVerdictResume {
		d.settle(stop, sheddriver.SettlerArbiter, sheddriver.SettleRestep, "arbiter resume", envelope.Goto)
		d.pendingAttempt = strconv.Itoa(envelope.Attempt)
		stop.Restep = true
		return stop, false, nil
	}
	h := driveHandBack{Question: envelope.Question, Relay: relay, Rules: []string{"the arbiter handed the stop on: " + envelope.Reason}}
	return driveStop{}, true, d.handBackAs(stop, h, "the arbiter handed the stop on")
}

// recheckAfterVerdict re-reads the run after an agent's verdict, and when the run moved while the agent worked, notes the discarded verdict in the stop report at reportPath.
// It returns the stop itself for an unmoved run.
func (d *goDriver) recheckAfterVerdict(stop driveStop, reportPath, agent string) (next driveStop, exit bool, err error) {
	next, exit, err = d.recheck(stop)
	if err != nil || exit {
		return driveStop{}, exit, err
	}
	if next.Restep || next.key() != stop.key() {
		line := fmt.Sprintf("\n- The run moved while the %s worked, from stop key %+v to %+v; its verdict was discarded and the new state is acted on.\n", agent, stop.key(), next.key())
		if err := appendToFile(reportPath, line); err != nil {
			logger.Warn("loom: drive: noting the moved run in the stop report failed", "run", d.deps.RunID, "error", err)
		}
	}
	return next, false, nil
}

// worktreeChangedBy reports whether the worktree differs from the snapshot the tackler attempt in attemptDir recorded at spawn.
func (d *goDriver) worktreeChangedBy(attemptDir string) (bool, error) {
	spawned, err := d.deps.AttemptSnapshot(attemptDir)
	if err != nil {
		return false, err
	}
	current, err := d.deps.Snapshot()
	if err != nil {
		return false, err
	}
	return !spawned.Equal(current), nil
}

// startReport writes the stop report so far at a fresh path, for an agent to read, and returns the path.
func (d *goDriver) startReport(stop driveStop) (string, error) {
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
// It returns the stop rebuilt from the envelope the attempt recorded; the rule table then sends it to the tackler or the arbiter, whose own call reuses the attempt.
func (d *goDriver) resumeOpenAttempt(ctx context.Context) (stop driveStop, found bool, err error) {
	status, statusFound, err := d.deps.ReadStatus()
	if err != nil {
		return driveStop{}, false, fmt.Errorf("reading the status before the first step: %w", err)
	}
	if !statusFound || status.State == shedengine.StateRunning || status.State == shedengine.StateDone {
		return driveStop{}, false, nil
	}
	key := sheddriver.StopKeyOf(status)
	tacklerDir, arbiterDir, _, err := d.deps.OpenAttempts(key)
	if err != nil {
		return driveStop{}, false, fmt.Errorf("listing the stop's attempts: %w", err)
	}
	if tacklerDir == "" && arbiterDir == "" {
		return driveStop{}, false, nil
	}
	records, err := sheddriver.ReadSettleRecords(d.deps.SettlePath)
	if err != nil {
		return driveStop{}, false, err
	}
	if attemptClosed(records, key) {
		return driveStop{}, false, nil
	}

	envelopePath := ""
	if tacklerDir != "" {
		if attempt, err := sheddriver.ReadTacklerAttempt(tacklerDir); err == nil {
			envelopePath = attempt.EnvelopePath
		}
	}
	if envelopePath == "" && arbiterDir != "" {
		if attempt, err := loomengine.ReadArbiterAttempt(arbiterDir); err == nil {
			envelopePath = attempt.EnvelopePath
		}
	}
	raw, err := os.ReadFile(envelopePath)
	if err != nil {
		logger.Warn("loom: drive: the open attempt's stop envelope is unreadable; stepping instead", "run", d.deps.RunID, "path", envelopePath, "error", err)
		return driveStop{}, false, nil
	}
	stop, err = parseDriveStop(raw)
	if err != nil || stop.key() != key {
		logger.Warn("loom: drive: the open attempt's stop envelope is not the run's stop; stepping instead", "run", d.deps.RunID, "path", envelopePath, "error", err)
		return driveStop{}, false, nil
	}
	stop.EnvelopePath = envelopePath
	return stop, true, nil
}

// attemptClosed reports whether the settle records hold a hand-back for key later than the key's latest spawn.
// A key with no spawn record counts any hand-back as later.
func attemptClosed(records []sheddriver.SettleRecord, key sheddriver.StopKey) bool {
	var spawned time.Time
	for _, record := range records {
		if record.StopKey == key && record.Action == sheddriver.SettleSpawn && record.At.After(spawned) {
			spawned = record.At
		}
	}
	for _, record := range records {
		if record.StopKey == key && record.Action == sheddriver.SettleHandBack && record.At.After(spawned) {
			return true
		}
	}
	return false
}

// facts reads the disk facts decideDrive needs for stop.
func (d *goDriver) facts(stop driveStop) (driveFacts, error) {
	var facts driveFacts
	key := stop.key()
	if key.State == string(shedengine.StateAwaiting) {
		pending, err := d.deps.EscalationPending(key.CurrentProducer)
		if err != nil {
			return driveFacts{}, fmt.Errorf("reading the pending escalation: %w", err)
		}
		facts.EscalationPending = pending
	}

	records, err := sheddriver.ReadSettleRecords(d.deps.SettlePath)
	if err != nil {
		return driveFacts{}, err
	}
	facts.RepairCount = repairCount(records, key)
	_, _, applied, err := d.deps.OpenAttempts(key)
	if err != nil {
		return driveFacts{}, fmt.Errorf("listing the stop's attempts: %w", err)
	}
	facts.ResumeReturned = applied
	watch, alive := d.deps.Watch()
	facts.Watched = alive

	if binaryEligible(stop) {
		onDisk, err := d.deps.OnDiskBuild()
		if err != nil {
			logger.Warn("loom: drive: the on-disk build is unreadable; reading it as unchanged", "run", d.deps.RunID, "error", err)
		} else {
			facts.BuildChanged = d.deps.BuildChanged(onDisk)
			facts.HandedOver = handedOver(records, key, onDisk.Label())
		}
	}

	if key.State == string(shedengine.StateBlocked) {
		record, found, err := d.deps.VerifyRecord()
		if err != nil {
			return driveFacts{}, fmt.Errorf("reading the Publish failure record: %w", err)
		}
		if found {
			head, err := d.deps.Head()
			if err != nil {
				return driveFacts{}, fmt.Errorf("reading the worktree HEAD: %w", err)
			}
			status, statusFound, err := d.deps.ReadStatus()
			if err != nil || !statusFound {
				return driveFacts{}, fmt.Errorf("reading the status to match the failure record: found %t: %v", statusFound, err)
			}
			facts.VerifyMatches = battenshed.RecordMatchesHalt(record, head, status, d.deps.VerifyRows)
			facts.VerifyTarget = record.Target
		}
	}
	if facts.VerifyMatches {
		facts.VerifyRetrySpent = sheddriver.GoVerifyRetrySpent(records)
		if watch.ScratchDir != "" {
			spent, err := d.deps.BattenRetrySpent(watch)
			if err != nil {
				return driveFacts{}, fmt.Errorf("reading the batten publish-retry marker: %w", err)
			}
			facts.VerifyRetrySpent = facts.VerifyRetrySpent || spent
		}
	}
	return facts, nil
}

// recheck re-reads the status before the driver acts on stop, so nothing the run's other actors did since the step goes unseen.
// A running status or a run lock another process holds ends the driver with a running-exit record.
// A status at another stop key is a moved run: it returns the stop to act on, marked Restep when an orch goto moved the run, and otherwise decided afresh from the status.
// A moved run is never parked and gets no friction note.
// An unmoved run returns stop itself.
func (d *goDriver) recheck(stop driveStop) (next driveStop, exit bool, err error) {
	status, found, err := d.deps.ReadStatus()
	if err != nil {
		return driveStop{}, false, fmt.Errorf("re-reading the status: %w", err)
	}
	if !found {
		return driveStop{}, false, fmt.Errorf("re-reading the status: the run has no status file")
	}
	held, err := d.deps.RunLockHeld()
	if err != nil {
		return driveStop{}, false, fmt.Errorf("probing the run lock: %w", err)
	}
	if held || (status.State == shedengine.StateRunning && stop.key().State != string(shedengine.StateRunning)) {
		d.settle(stop, sheddriver.SettlerGoRule, sheddriver.SettleRunningExit, "another process is running the run", "")
		logger.Info("loom: drive: the run is running elsewhere; exiting", "run", d.deps.RunID, "lock_held", held, "state", status.State)
		return stop, true, nil
	}
	if sheddriver.StopKeyOf(status) == stop.key() {
		return stop, false, nil
	}

	if n := len(status.History); n > 0 && status.History[n-1].Outcome == shedengine.OutcomeGoto {
		d.settle(stop, sheddriver.SettlerGoRule, sheddriver.SettleMoved, "an orch goto moved the run: re-step", status.CurrentProducer)
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
		return driveStop{}, false, fmt.Errorf("encoding the moved run's envelope: %w", err)
	}
	path, err := d.deps.SaveEnvelope(raw)
	if err != nil {
		return driveStop{}, false, fmt.Errorf("saving the moved run's envelope: %w", err)
	}
	moved.Raw = raw
	moved.EnvelopePath = path
	d.settle(stop, sheddriver.SettlerGoRule, sheddriver.SettleMoved, "the run moved to another stop: decide it afresh", status.CurrentProducer)
	return moved, false, nil
}

// holdPause returns once no pause covers the run's slug, waiting in PauseHold while one does.
// An unreadable pause record holds nothing, with one Warn.
func (d *goDriver) holdPause(ctx context.Context) error {
	for {
		coverage, err := d.deps.Pause(d.deps.Now())
		if err != nil {
			logger.Warn("loom: drive: the pause record is unreadable; not holding", "run", d.deps.RunID, "error", err)
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
func (d *goDriver) settle(stop driveStop, settler, action, outcome, target string) {
	record := sheddriver.SettleRecord{StopKey: stop.key(), Settler: settler, Action: action, Outcome: outcome, At: d.deps.Now(), Target: target}
	if err := sheddriver.AppendSettleRecord(d.deps.SettlePath, record); err != nil {
		logger.Warn("loom: drive: appending a settle record failed", "run", d.deps.RunID, "action", action, "error", err)
	}
}

// finishDone commits the run's records, and hands a run that finished with friction: failed to the parent without parking it.
func (d *goDriver) finishDone(stop driveStop) error {
	if err := d.deps.CommitRecords(); err != nil {
		return fmt.Errorf("loom: drive: commit the run's records: %w", err)
	}
	if stop.Friction != "failed" {
		return nil
	}
	return d.handBack(stop, driveHandBack{DoneFrictionFailed: true})
}

// handBackStop hands the run back at stop on the decision's rule, after the friction note a paused stop without a reason asks for.
// cause, when set, is a failure that kept the driver from acting and goes into the rules the report quotes.
func (d *goDriver) handBackStop(stop driveStop, decision driveDecision, cause error) error {
	if decision.FrictionNote {
		if err := d.deps.WriteFrictionNote(stop); err != nil {
			logger.Warn("loom: drive: writing the friction note failed", "run", d.deps.RunID, "error", err)
		}
	}
	h := driveHandBack{WatchedVerifyLine: decision.WatchedVerifyLine}
	if cause != nil {
		logger.Warn("loom: drive: the driver could not act at the stop; handing back", "run", d.deps.RunID, "error", cause)
		h.Rules = []string{"the driver could not act: " + cause.Error()}
	}
	if decision.Action == driveActionRelayHandBack {
		h.Relay = stop.ParentNotice
	}
	return d.handBackAs(stop, h, decision.Rule)
}

// handBackAs records the hand-back in the settle records, with outcome as its reason, and leaves the stop report, the park marker and the parent's notices.
func (d *goDriver) handBackAs(stop driveStop, h driveHandBack, outcome string) error {
	d.settle(stop, sheddriver.SettlerParent, sheddriver.SettleHandBack, outcome, "")
	return d.handBack(stop, h)
}

// handBackFailure hands the run back at the stop its status shows, or at an empty stop when the status cannot be read, for a failure that left no stop to decide.
func (d *goDriver) handBackFailure(cause error) error {
	var stop driveStop
	if status, found, err := d.deps.ReadStatus(); err == nil && found {
		stop = stopFromStatus(status)
	}
	rule := "a failure the driver cannot act on: hand back"
	d.rules = append(d.rules, rule)
	return d.handBackStop(stop, driveDecision{Action: driveActionHandBack, Rule: rule}, cause)
}

// handBack builds the stop report for stop and h, and leaves the report, the park marker and the parent's notices through handBackRun.
// The report quotes the rules applied and the agents' attempts at the stop so far, then h's own.
func (d *goDriver) handBack(stop driveStop, h driveHandBack) error {
	reportPath := d.deps.NewReportPath()
	park := d.deps.Park
	park.ReportPath = reportPath

	if stop.Error != "" {
		logger.Warn("loom: drive: handing the run back", "run", d.deps.RunID, "state", stop.key().State, "producer", stop.CurrentProducer, "error", stop.Error, "report", reportPath)
	} else {
		logger.Info("loom: drive: handing the run back", "run", d.deps.RunID, "state", stop.key().State, "producer", stop.CurrentProducer, "report", reportPath)
	}

	h.RunID = d.deps.RunID
	h.ReportPath = reportPath
	h.Stop = stop
	h.Rules = append(slices.Clone(d.rules), h.Rules...)
	h.Attempts = append(slices.Clone(d.attempts), h.Attempts...)
	_, watched := d.deps.Watch()
	return handBackRun(park, h, d.reportFor(stop, h, reportPath), watched)
}

// reportFor builds the stop report of a hand-back at stop that is to be written at reportPath.
func (d *goDriver) reportFor(stop driveStop, h driveHandBack, reportPath string) driveReport {
	reason := stop.Reason
	if reason == "" {
		reason = stop.Error
	}
	if reason == "" {
		reason = stop.key().State
	}
	return driveReport{
		RunID:         d.deps.RunID,
		HistoryLength: stop.HistoryLength,
		Progress:      fmt.Sprintf("%s at %s", stop.key().State, stop.CurrentProducer),
		Reason:        reason,
		Rules:         h.Rules,
		Attempts:      h.Attempts,
		Question:      h.Question,
		FrictionDir:   d.deps.FrictionDir,
		RepairDir:     filepath.Dir(reportPath),
	}
}
