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
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// battenTacklerRecipe is the one-line description of a batten run the tackler's prompt names under its run heading.
const battenTacklerRecipe = "batten: a task worktree's whole lifecycle run from the hub's prime (create the pair, seed and run the task's own run, tear the pair down and land it)"

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
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}

	clihelp.SetExit(ctx, sheddriver.DriveVerb(out, battenDriveLock(c.location, c.slug), func(_ func() error) (map[string]any, error) {
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
		return battenRunEnvelope(exit, status, found, abandonedSession, c.spec.RunBusyMessage)
	}))
	return nil
}

// battenRunEnvelope builds the run verb's success envelope from how the driver ended and the status as it then reads.
// A run that finished or stopped at done, blocked, paused or awaiting reports outcome, halted_producer, reason and history_length, and abandonedSession when the teardown row recorded one.
// A busy exit returns busyMessage as the error, and a failed run the status's own error.
func battenRunEnvelope(exit sheddriver.Exit, st shedengine.Status, found bool, abandonedSession, busyMessage string) (map[string]any, error) {
	if exit == sheddriver.ExitBusy || exit == sheddriver.ExitRunningElsewhere {
		return nil, errors.New(busyMessage)
	}
	if !found {
		return nil, errors.New("battencli: the run has no status file after the driver ended")
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

// driverDepsFor fills the shed driver's seams for batten's run in the prime.
// Every path comes from shedrun over the prime's location and the slug, and nothing is read or built here: the configs, the model registry and the shuttle runner load inside the Tackler seam on its first call.
// The seams it leaves nil, the arbiter, verify, watched, hand-over, tamper, record-commit and friction-note ones, remove their rows from the driver's rule table.
func (c *battenCLI) driverDepsFor() (sheddriver.Deps, error) {
	location, slug := c.location, c.slug
	runID := shedrun.ResolveRunID(location, slug)
	pauseGeometry := func() (pauserecord.Geometry, error) { return hubgeom.PauseGeometry(location) }

	return sheddriver.Deps{
		RunID: runID,
		Step: func(ctx context.Context, attempt string) ([]byte, error) {
			if c.untilStop != nil {
				return c.untilStop(ctx, attempt)
			}
			var envelope bytes.Buffer
			shedverbs.RunUntilStop(ctx, c.spec, []string{"batten", "step", slug}, attempt, &envelope)
			if envelope.Len() == 0 {
				return nil, errors.New("the step printed no envelope")
			}
			return envelope.Bytes(), nil
		},
		Goto: func(ctx context.Context, target string) error {
			_, err := shedverbs.RunGoto(ctx, c.spec, target)
			return err
		},
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
			return filepath.Join(shedrun.DriveReportsDir(location, slug), sheddriver.ReportFileName(time.Now(), reportSuffix()))
		},
		HandBack: sheddriver.HandBackDeps{
			// A pause is the operator's own act, so a paused stop sends no notice.
			Notify: func(stop sheddriver.Stop, line string) (bool, error) {
				if stop.Key().State == string(shedengine.StatePaused) {
					return false, nil
				}
				return battenshed.HandBackNotice(context.Background(), rowNotices(location, slug), slug, c.env.ScratchDir, stop.CurrentProducer, stop.HistoryLength, line), nil
			},
			OrchRecorded: func() (bool, error) { return orchStrandRecorded(location) },
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
			return tackler, "", applied, err
		},
		Now: time.Now,
	}, nil
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
				if record.StopKey == key && record.Action == sheddriver.SettleHandBack && record.At.After(spawned) {
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
// The runner is built from the prime's own reed geometry, so the tackler's strand lives in the prime's reed session and the task worktree's session shutdown never ends it.
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

	reedConfig, err := reedengine.LoadConfig(dirs, "reed")
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	shuttleConfig, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	reedGeometry, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	pauseGeometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return sheddriver.TacklerDeps{}, err
	}
	runner := shuttleengine.NewRunner(reedengine.New(reedConfig, reedGeometry), claudeengine.NewFromConfig(shuttleConfig), reedGeometry.AnchorPath, reedGeometry.WorktreeRoot, shuttleConfig)
	runner.SetPause(pauseGeometry)
	runner.SetNotifier(func(line string) error {
		_, err := orchengine.QueueNotice(orchcli.PrimePaths(location), line, time.Now())
		return err
	})
	return c.tacklerDepsFor(settings, sheddriver.RunnerShuttle{Runner: runner}), nil
}
