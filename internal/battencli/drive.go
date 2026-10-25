// drive.go runs "lyx batten run" through the shed driver's body: the run verb's pre-run and driver lock, the seams batten fills the driver with, and the envelope the verb prints from how the driver ended.

package battencli

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedarbiter"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
)

// battenTacklerRecipe is the one-line description of a batten run the tackler's prompt names under its run heading.
const battenTacklerRecipe = "batten: a task worktree's whole lifecycle run from the hub's prime (create the pair, seed and run the task's own run, tear the pair down and land it)"

// battenArbiterRecipe describes batten's rows and the arbiter's moves for the arbiter's prompt, which names it under its run heading.
const battenArbiterRecipe = "batten: a task worktree's whole lifecycle run from the hub's prime, in rows. " +
	"Worktree-Create claims the task and makes the pair; " +
	"Seed-Child writes the child's seed and run settings; " +
	"Open-Window, Seed-IDE and Open-IDE run the optional window and IDE commands; " +
	"Run-Shed watches the child's own run; " +
	"Land pulls the prime, deploys and prunes the board; " +
	"Worktree-Teardown ends the session and removes the pair. " +
	"The arbiter's only moves are a plain re-launch, a goto to the halted row, or a hand-on."

// battenDriveRefusal is the refusal of a second batten driver for a run one already drives, before its way-forward clause.
const battenDriveRefusal = "battencli: a batten driver already drives this run"

// battenDriveLock names the driver lock "lyx batten run" holds for its life, the files it removes once it holds it, and what it says when another driver holds the lock.
func battenDriveLock(location *lyxcwd.Location, slug string) sheddriver.DriveLock {
	return sheddriver.DriveLock{
		Path:        shedrun.DriverLock(location, slug),
		Stale:       []string{shedrun.ParkMarker(location, slug), shedrun.ResumeReport(location, slug)},
		Refusal:     battenDriveRefusal + fmt.Sprintf("; way forward: `lyx batten status %s` shows the run, and the live batten driver settles it", slug),
		RetryClause: fmt.Sprintf("re-run lyx batten run %s", slug),
	}
}

// driveRun is the run verb's body: batten's pre-run, then the shed driver over batten's seams under the driver lock, then the run envelope.
func (c *battenCLI) driveRun(cmd *cobra.Command) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if err := c.battenPreRun(ctx); err != nil {
		output.Err(out, err.Error())
		clihelp.SetExit(ctx, passExitCode(err))
		return nil
	}

	exit := sheddriver.DriveVerb(out, battenDriveLock(c.location, c.slug), func(_ func() error) (map[string]any, error) {
		deps, err := c.driverDepsFor()
		if err != nil {
			return nil, err
		}
		exit, err := sheddriver.Run(ctx, deps)
		if err != nil {
			return nil, err
		}
		status, found, err := deps.ReadStatus()
		if err != nil {
			return nil, err
		}
		abandonedSession, _ := readAbandonedSession(c.env.ScratchDir)
		yieldReason, _ := readStuckReason(c.env.ScratchDir, battenrecipe.NameRunShed)
		return battenRunEnvelope(exit, status, found, abandonedSession, yieldReason, c.spec.RunBusyMessage)
	})
	clihelp.SetExit(ctx, exit)
	// A pass that reached origin and exited 0 pays the origin the earlier resume owed; any other pass leaves the marker standing.
	if exit == 0 && c.originReached {
		c.clearOriginOwed()
	}
	// A hand pass leaves the run to its parent's dobby, so the dobby is made live once the pass is over.
	if err := c.ensureDobby(); err != nil {
		logger.Warn("battencli: the pass ended but its parent's dobby is not live", "slug", c.slug, "error", err.Error())
	}
	return nil
}

// passExitCode returns the exit code of a pass that refused with err before the step loop: the owned-elsewhere code for a run another machine owns, the covered code for a run a pause covers, the continue-refused code for a continue pass of a run dobby may not continue, and 1 for any other refusal.
func passExitCode(err error) int {
	var owned *OwnedElsewhereError
	var covered *coveredRefusal
	var continueRefused *continueRefusal
	switch {
	case errors.As(err, &owned):
		return dobbyengine.PassExitOwnedElsewhere
	case errors.As(err, &covered):
		return dobbyengine.PassExitCovered
	case errors.As(err, &continueRefused):
		return dobbyengine.PassExitContinueRefused
	default:
		return 1
	}
}

// battenRunEnvelope builds the run verb's success envelope from how the driver ended and the status as it then reads.
// A run that finished or stopped at done, blocked, paused or awaiting reports outcome, halted_producer, reason and history_length, and abandonedSession when the teardown row recorded one.
// A busy exit returns busyMessage as the error, and a failed run the status's own error.
// A yield reports outcome yielded with the run still running, and yieldReason, the hold or wait reason it yielded on, as the reason.
func battenRunEnvelope(exit sheddriver.Exit, st shedengine.Status, found bool, abandonedSession, yieldReason, busyMessage string) (map[string]any, error) {
	if exit == sheddriver.ExitBusy || exit == sheddriver.ExitRunningElsewhere {
		return nil, errors.New(busyMessage)
	}
	if !found {
		return nil, errors.New("battencli: the run has no status file after the driver ended")
	}
	if exit == sheddriver.ExitYield {
		envelope := map[string]any{
			"outcome":         "yielded",
			"halted_producer": st.CurrentProducer,
			"reason":          yieldReason,
			"history_length":  len(st.History),
		}
		if abandonedSession != "" {
			envelope["abandonedSession"] = abandonedSession
		}
		return envelope, nil
	}
	switch st.State {
	case shedengine.StateFailed:
		return nil, errors.New(st.Error)
	case shedengine.StateDone, shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateAwaiting:
	default:
		return nil, fmt.Errorf("battencli: the driver ended with the run's status at %q", st.State)
	}

	reason := ""
	if st.State == shedengine.StateBlocked || st.State == shedengine.StateAwaiting {
		reason = st.Error
	}
	envelope := map[string]any{
		"outcome":         string(st.State),
		"halted_producer": st.CurrentProducer,
		"reason":          reason,
		"history_length":  len(st.History),
	}
	if abandonedSession != "" {
		envelope["abandonedSession"] = abandonedSession
	}
	return envelope, nil
}

// readRunStatus reads batten's own status file.
func (c *battenCLI) readRunStatus() (shedengine.Status, bool, error) {
	return state.ReadJSONNoMkdir[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
}

// reportSuffix returns four lowercase hex characters, the random part of a drive-reports file name.
// Its job is keeping two same-second files apart, so a failed read, which leaves "0000", costs at most a rare name collision.
func reportSuffix() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%04x", b)
}

// driverStep is the Go driver's step seam: the untilStop override when set, else the loop's waiter over the armed spec, labelled shedengine.CallerShed.
// It returns the envelope the waiter printed, or an error when it printed none.
func (c *battenCLI) driverStep(ctx context.Context, attempt string) ([]byte, error) {
	if c.untilStop != nil {
		return c.untilStop(ctx, attempt)
	}
	var envelope bytes.Buffer
	shedverbs.RunUntilStop(ctx, c.spec, []string{"batten", "step", c.slug}, attempt, shedengine.CallerShed, &envelope)
	if envelope.Len() == 0 {
		return nil, errors.New("the step printed no envelope")
	}
	return envelope.Bytes(), nil
}

// driverGoto is the Go driver's goto seam: it moves the run onto target, labelled shedengine.CallerShed.
func (c *battenCLI) driverGoto(ctx context.Context, target string) error {
	_, err := shedverbs.RunGoto(ctx, c.spec, target, shedengine.CallerShed)
	return err
}

// battenHandBackNotice routes the notice of a hand-back at stop and reports whether it was posted.
// A paused stop sends nothing, since the operator asked for the pause, and neither does a Land stop that honoured a pause, whose own reason names the goto that clears it.
// A stop at Land or Worktree-Teardown sends the row-stop notice with the stop's reason and the report at reportPath, and any other stop sends line.
func battenHandBackNotice(deps battenshed.RowNoticeDeps, slug, scratchDir string, stop sheddriver.Stop, line, reportPath string) bool {
	if stop.Key().State == string(shedengine.StatePaused) {
		return false
	}
	switch stop.CurrentProducer {
	case battenrecipe.NameLand, battenrecipe.NameWorktreeTeardown:
		if strings.HasPrefix(stop.Reason, battenshed.PauseHonouredPrefix) {
			return false
		}
		return battenshed.RowStopNotice(context.Background(), deps, slug, scratchDir, stop.CurrentProducer, stop.Reason, reportPath, stop.HistoryLength)
	}
	return battenshed.HandBackNotice(context.Background(), deps, slug, scratchDir, stop.CurrentProducer, stop.HistoryLength, line)
}

// driverDepsFor fills the shed driver's seams for batten's run in the prime.
// Every path comes from shedrun over the prime's location and the slug, and nothing is read or built here: the configs, the model registry and the shuttle runner load inside the Tackler and Arbiter seams on their first call.
// The seams it leaves nil, the verify, watched, hand-over, tamper, record-commit and friction-note ones, remove their rows from the driver's rule table.
func (c *battenCLI) driverDepsFor() (sheddriver.Deps, error) {
	location, slug := c.location, c.slug
	runID := shedrun.ResolveRunID(location, slug)
	pauseGeometry := func() (pauserecord.Geometry, error) { return hubgeom.PauseGeometry(location) }
	var reportPath string

	return sheddriver.Deps{
		RunID:       runID,
		Step:        c.driverStep,
		Goto:        c.driverGoto,
		ReadStatus:  c.readRunStatus,
		RunLockHeld: func() (bool, error) { return runLockHeld(c.shedPaths.LockPath) },
		SaveEnvelope: func(raw []byte) (string, error) {
			path := filepath.Join(shedrun.DriveReportsDir(location, slug), sheddriver.EnvelopeFileName(time.Now(), reportSuffix()))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return "", err
			}
			return path, os.WriteFile(path, raw, 0o644)
		},
		SettlePath: shedrun.SettleRecords(location, slug),
		Pause: func(now time.Time) (pauserecord.Coverage, error) {
			geometry, err := pauseGeometry()
			if err != nil {
				return pauserecord.Coverage{}, err
			}
			return pauserecord.ReadCoverage(geometry.RecordPath, slug, now)
		},
		PauseHold: func(ctx context.Context) error {
			geometry, err := pauseGeometry()
			if err != nil {
				return err
			}
			return pauserecord.WaitUncovered(ctx, geometry.RecordPath, slug, pauserecord.Waiter{})
		},
		NewReportPath: func() string {
			reportPath = filepath.Join(shedrun.DriveReportsDir(location, slug), sheddriver.ReportFileName(time.Now(), reportSuffix()))
			return reportPath
		},
		HandBack: sheddriver.HandBackDeps{
			// The driver asks for the hand-back's report path just before it sends the hand-back's notices, so reportPath is the report the notice names.
			Notify: func(stop sheddriver.Stop, line string) (bool, error) {
				return battenHandBackNotice(rowNotices(location, slug), slug, c.env.ScratchDir, stop, line, reportPath), nil
			},
			OrchRecorded: func() (bool, error) { return runParentRecorded(location, slug) },
			Park: func(reportPath string) error {
				marker := shedrun.ParkMarker(location, slug)
				if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
					return err
				}
				return os.WriteFile(marker, []byte(reportPath), 0o644)
			},
			Now: time.Now,
		},
		Tackler: func(stop sheddriver.Stop, reportPath string) (sheddriver.TacklerEnvelope, error) {
			deps, err := c.loadedTacklerDeps()
			if err != nil {
				return sheddriver.TacklerEnvelope{}, err
			}
			call, err := sheddriver.NewTacklerCall(deps, sheddriver.TacklerInput{RunID: runID, EnvelopePath: stop.EnvelopePath, ReportPath: reportPath, TraceCopy: stop.TraceCopy, StderrPath: stop.StderrPath})
			if err != nil {
				return sheddriver.TacklerEnvelope{}, err
			}
			return call.Settle()
		},
		ConsumeTackler: func(attemptDir string) error {
			return sheddriver.ConsumeTacklerAttempt(attemptDir, shedengine.RelaunchRequest{
				StatusPath:              c.shedPaths.StatusPath,
				LockPath:                c.shedPaths.LockPath,
				StatusLockPath:          c.shedPaths.StatusLockPath,
				Producers:               c.routing.Producers,
				RunID:                   runID,
				MissingStatusWayForward: c.shedPaths.MissingStatusWayForward,
			})
		},
		OpenAttempts: func(key sheddriver.StopKey) (string, string, bool, error) {
			tackler, applied, err := sheddriver.NewestTacklerAttempt(shedrun.TacklerDir(location, slug), key)
			if err != nil {
				return "", "", false, err
			}
			arbiters, err := loomengine.ListArbiterAttempts(shedrun.ArbiterDir(location, slug))
			if err != nil {
				return "", "", false, err
			}
			arbiter := ""
			for i := len(arbiters) - 1; i >= 0; i-- {
				if arbiters[i].StopKey == key {
					arbiter = arbiters[i].Dir
					break
				}
			}
			return tackler, arbiter, applied, nil
		},
		Arbiter: c.arbiterSeam(runID),
		Now:     time.Now,
	}, nil
}

// arbiterSeam builds batten's arbiter seam, whose deps load on its first Judge call.
// A batten stop never awaits a review escalation, so EscalationPending answers false.
func (c *battenCLI) arbiterSeam(runID string) *sheddriver.ArbiterSeam {
	var loaded *sheddriver.ArbiterSeam
	seam := shedarbiter.Seam(shedarbiter.Deps{}, runID, func(string) bool { return false })
	seam.Judge = func(envelopePath, reportPath, caseBriefPath string) (sheddriver.ArbiterOutcome, error) {
		if loaded == nil {
			deps, err := c.loadedArbiterDeps()
			if err != nil {
				return sheddriver.ArbiterOutcome{}, err
			}
			loaded = shedarbiter.Seam(deps, runID, func(string) bool { return false })
		}
		return loaded.Judge(envelopePath, reportPath, caseBriefPath)
	}
	return seam
}

// tacklerDepsFor fills the tackler call's seams and fixed values over settings and shuttle.
// A batten tackler has no worktree snapshot, since the prime has other writers, and a resume names the halted row only.
func (c *battenCLI) tacklerDepsFor(settings sheddriver.TacklerSettings, shuttle sheddriver.AgentShuttle) sheddriver.TacklerDeps {
	location, slug := c.location, c.slug
	settlePath := shedrun.SettleRecords(location, slug)
	return sheddriver.TacklerDeps{
		Closed: func(key sheddriver.StopKey, spawned time.Time) (bool, error) {
			records, err := sheddriver.ReadSettleRecords(settlePath)
			if err != nil {
				return false, err
			}
			for _, record := range records {
				if record.StopKey == key && record.Action == shedrun.SettleHandBack && record.At.After(spawned) {
					return true, nil
				}
			}
			return false, nil
		},
		ReadStatus: c.readRunStatus,
		Settings:   settings,
		TacklerDir: shedrun.TacklerDir(location, slug),
		AttemptDir: func(number int) string { return shedrun.TacklerAttemptDir(location, slug, number) },
		MarkerPath: shedrun.TacklerMarker(location, slug),
		Producers:  c.routing.Producers,
		PausedWithin: func(from, to time.Time) (time.Duration, error) {
			geometry, err := hubgeom.PauseGeometry(location)
			if err != nil {
				return 0, err
			}
			coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, slug, to)
			if err != nil {
				return 0, err
			}
			return pauserecord.Excluded(coverage.Intervals, from, to), nil
		},
		Recipe:         battenTacklerRecipe,
		GotoRoutedFrom: false,
		Prompt: sheddriver.TacklerPromptInput{
			Slug:      slug,
			Worktree:  location.WorktreePath(),
			TraceDir:  logger.TraceDir(),
			DriverLog: logger.TraceFile(),
		},
		StencilsDir: fabricengine.StencilsDir(location.HubPath),
		Shuttle:     shuttle,
		Now:         time.Now,
	}
}

// loadedTacklerDeps loads the configs and the model registry and builds the prime's shuttle runner, then fills the tackler call's seams over them.
func (c *battenCLI) loadedTacklerDeps() (sheddriver.TacklerDeps, error) {
	location := c.location
	dirs := fabricengine.ConfigDirs(location)
	shedConfig, err := sheddriver.LoadConfig(dirs)
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	battenConfig, err := battenshed.LoadConfig(dirs)
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	settings, err := sheddriver.ResolveTackler(shedConfig, battenConfig.ShedOverride(), registry)
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}

	shuttle, err := c.primeShuttle(dirs)
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	return c.tacklerDepsFor(settings, shuttle), nil
}

// primeShuttle builds the prime's shuttle runner, shared by the tackler and the arbiter.
// The runner is built from the prime's own reed geometry, so an agent's strand lives in the prime's reed session and the task worktree's session shutdown never ends it.
func (c *battenCLI) primeShuttle(dirs configengine.Dirs) (sheddriver.AgentShuttle, error) {
	location := c.location
	reedConfig, err := reedengine.LoadConfig(dirs, "reed")
	if err != nil {
		return nil, err
	}
	shuttleConfig, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		return nil, err
	}
	reedGeometry, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return nil, err
	}
	pauseGeometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return nil, err
	}
	engines, err := shuttleengines.New(shuttleConfig, dirs)
	if err != nil {
		return nil, err
	}
	runner := shuttleengine.NewRunner(reedengine.New(reedConfig, reedGeometry, shuttleengines.EnvScrub()), engines, reedGeometry.AnchorPath, reedGeometry.WorktreeRoot, shuttleConfig)
	runner.SetPause(pauseGeometry)
	runner.SetNotifier(func(line string) error {
		_, err := notifyRunParent(location, c.slug, line)
		return err
	})
	return sheddriver.RunnerShuttle{Runner: runner}, nil
}

// loadedArbiterDeps loads the configs and the model registry and builds the prime's shuttle runner, then fills the arbiter call's seams over them.
// The arbiter's settings are shed.yaml's under batten.yaml's override.
func (c *battenCLI) loadedArbiterDeps() (shedarbiter.Deps, error) {
	location, slug := c.location, c.slug
	dirs := fabricengine.ConfigDirs(location)
	shedConfig, err := sheddriver.LoadConfig(dirs)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	battenConfig, err := battenshed.LoadConfig(dirs)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	settings, err := sheddriver.ResolveArbiter(shedConfig, battenConfig.ShedOverride(), registry)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	shuttle, err := c.primeShuttle(dirs)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	return shedarbiter.Deps{
		ReadStatus: c.readRunStatus,
		Settings:   settings,
		ArbiterDir: shedrun.ArbiterDir(location, slug),
		AttemptDir: func(number int) string { return shedrun.ArbiterAttemptDir(location, slug, number) },
		MarkerPath: shedrun.ArbiterMarker(location, slug),
		Producers:  c.routing.Producers,
		GotoGuard:  c.haltedRowGuard(),
		Stop:       arbiterstop.NewBattenReaders(),
		Prompt: shedarbiter.PromptInput{
			Recipe:   battenArbiterRecipe,
			Slug:     slug,
			Worktree: location.WorktreePath(),
		},
		StencilsDir:        fabricengine.StencilsDir(location.HubPath),
		AmendmentTarget:    func() string { return "" },
		Shuttle:            shuttle,
		Now:                time.Now,
		RelaunchRunID:      shedrun.ResolveRunID(location, slug),
		HandBackWayForward: arbiterstop.BattenHandBackWayForward,
	}, nil
}

// haltedRowGuard returns the arbiter's goto guard: it admits only the row the run is halted at, read from the status at the call.
// A goto to an earlier row would re-run Seed-Child over a live pair's local config layer and seed, or Worktree-Create.
func (c *battenCLI) haltedRowGuard() shedengine.GotoGuard {
	return func(target string) (string, error) {
		status, found, err := c.readRunStatus()
		if err != nil {
			return "", err
		}
		if !found {
			return "", errors.New("the run has no status file")
		}
		if target == status.CurrentProducer {
			return "", nil
		}
		return fmt.Sprintf("goto target %s is not the halted row %s; batten's arbiter resumes only the halted row; way forward: the run's parent runs lyx batten goto %s --to %s", target, status.CurrentProducer, c.slug, target), nil
	}
}

// battenConsumeAttempt is batten's ConsumeAttempt hook: it parses attempt as a positive attempt number of the run and consumes that attempt's directory over batten's relaunch request.
func (c *battenCLI) battenConsumeAttempt(_ context.Context, attempt string) error {
	number, err := strconv.Atoi(attempt)
	if err != nil || number < 1 {
		return fmt.Errorf("battencli: arbiter attempt %q is not a positive integer; %s", attempt, arbiterstop.BattenHandBackWayForward)
	}
	return shedarbiter.ConsumeAttempt(shedrun.ArbiterAttemptDir(c.location, c.slug, number), shedengine.RelaunchRequest{
		StatusPath:              c.shedPaths.StatusPath,
		LockPath:                c.shedPaths.LockPath,
		StatusLockPath:          c.shedPaths.StatusLockPath,
		Producers:               c.routing.Producers,
		RunID:                   shedrun.ResolveRunID(c.location, c.slug),
		MissingStatusWayForward: c.shedPaths.MissingStatusWayForward,
	})
}
