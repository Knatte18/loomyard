// looparm.go fills what loom tells the generic step verb's --until-stop loop: its files, its trace listing, its watchdog window and activity reading, and the friction note of an interrupted stop.

package loomcli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// loopSpecFor is the loop spec of the run runID in loc.
// The loop's files and job name come from shedrun over the resolved run-id.
// The step children run the same binary from the same cwd as the loop, so they share its trace directory and the loop lists a child's trace files there.
// idle is the watchdog window, zero disarming it, and activity is the reading of the run's agent activity.
// The watchdog reads the pause record of loc's hub for loc's slug, and reads the run uncovered when the geometry or the record cannot be read.
func loopSpecFor(loc *lymcwd.Location, runID string, idle time.Duration, activity func() (time.Time, bool, error)) shedverbs.LoopSpec {
	executable, err := os.Executable()
	if err != nil {
		logger.Warn("loom: could not resolve the running binary, so the loop has no executable", "error", err.Error())
	}
	return shedverbs.LoopSpec{
		LockPath:     shedrun.LoopLock(loc, runID),
		PIDPath:      shedrun.LoopPIDFile(loc, runID),
		LogPath:      shedrun.LoopLog(loc, runID),
		EnvelopePath: shedrun.LoopEnvelope(loc, runID),
		JobName:      shedrun.LoopJobName(loc, runID),
		Delivered:    func(loopID string) string { return shedrun.LoopDelivered(loc, runID, loopID) },
		StopFiles: func(traceID string) (string, string) {
			return shedrun.StepTraceCopy(loc, runID, traceID), shedrun.StepStderr(loc, runID, traceID)
		},
		TraceFiles:  func(traceID string) ([]string, error) { return logger.TraceFilesFor(logger.TraceDir(), traceID) },
		Executable:  executable,
		IdleTimeout: idle,
		Activity:    activity,
		OnStart:     func() { registerRunProcess(loc, loc.WorktreeName, runproc.RoleLoop) },
		Pause: func(now time.Time) (pauserecord.Coverage, error) {
			geometry, err := hubgeom.PauseGeometry(loc)
			if err != nil {
				return pauserecord.Coverage{}, err
			}
			return pauserecord.ReadCoverage(geometry.RecordPath, geometry.Slug, now)
		},
	}
}

// registerRunProcess registers the calling process in the hub's owner record under owner with the given role.
// A failure is logged at Warn and never fails the role, since the record only lets a teardown find the process.
func registerRunProcess(loc *lymcwd.Location, owner, role string) {
	if err := runproc.Register(fabricengine.RunProcDir(loc.HubPath), owner, runproc.SelfRecord(role)); err != nil {
		logger.Warn("loom: could not register the run process", "owner", owner, "role", role, "error", err.Error())
	}
}

// watchdogKillNote returns the hook the loop calls after the watchdog killed a step.
// It stops the shuttle runs of the killed step through stop, told the steps directory and the kill's trace id, and renders the note that ends the interrupted cause:
// the runs it stopped, the live runs whose record names no step, which it leaves alone, and a stop that failed.
func watchdogKillNote(stop func(shuttleengine.StepTie) (shuttleengine.StepStopReport, error), stepsDir string) func(shedverbs.WatchdogKill) string {
	return func(kill shedverbs.WatchdogKill) string {
		report, err := stop(shuttleengine.StepTie{StepsDir: stepsDir, TraceID: kill.TraceID})
		var parts []string
		if len(report.Stopped) > 0 {
			parts = append(parts, "stopped shuttle runs: "+strings.Join(report.Stopped, ", "))
		}
		if len(report.Untied) > 0 {
			parts = append(parts, "left live runs naming no step: "+strings.Join(report.Untied, ", "))
		}
		for _, failure := range report.Failed {
			parts = append(parts, fmt.Sprintf("could not stop the run %s: %s", failure.Strand, failure.Error))
		}
		if err != nil {
			parts = append(parts, "could not stop the step's runs: "+err.Error())
		}
		return strings.Join(parts, "; ")
	}
}

// stopStepRuns stops the shuttle runs recorded under tie.
// The receiver's runner is used when wire built one; a receiver wired lightly gets the pair's runner built the way wire builds it.
func (c *loomCLI) stopStepRuns(tie shuttleengine.StepTie) (shuttleengine.StepStopReport, error) {
	runner := c.runner
	if runner == nil {
		configDirs := fabricengine.ConfigDirs(c.location)
		reedCfg, err := reedengine.LoadConfig(configDirs, "reed")
		if err != nil {
			return shuttleengine.StepStopReport{}, err
		}
		shuttleCfg, err := shuttleengine.LoadConfig(configDirs, "shuttle")
		if err != nil {
			return shuttleengine.StepStopReport{}, err
		}
		reedGeom, err := hubgeom.ReedGeometry(c.location)
		if err != nil {
			return shuttleengine.StepStopReport{}, err
		}
		engines, err := shuttleengines.New(shuttleCfg, configDirs)
		if err != nil {
			return shuttleengine.StepStopReport{}, err
		}
		runner = shuttleengine.NewRunner(reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub()), engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	}
	return runner.StopStepRuns(tie)
}

// agentActivityFor returns the reading of the run's agent activity: the newest LastActivity over every live agent run of loc's worktree.
// The runs come from shuttle through its registered engines, which list every strand, so a MultiLLM row's chair and each of its advisors count.
// A running verify, a live parent lock wait, a held drift capture lock or compare marker, or a shuttle wait that is not a held turn end, reads as activity now.
// A verify held by a pause does no work and reads as none, and a drift lock a dead process left reads free.
// All of them are read from their marker, lock or record files.
// The boolean is false when nothing is live.
func agentActivityFor(loc *lymcwd.Location, cfg shuttleengine.Config) func() (time.Time, bool, error) {
	return func() (time.Time, bool, error) {
		anchor := loc.AnchorPath()
		verifyMarker, verifyLive, err := verifytree.ReadMarker(verifytree.NewPaths(loc.WorktreePath(), verifytree.Dir(anchor)).Marker)
		if err != nil {
			return time.Time{}, false, err
		}
		if verifyLive && verifyMarker.State != verifytree.MarkerStatePaused {
			return time.Now(), true, nil
		}
		_, lockWaiting, err := parentlock.ReadWait(parentlock.WaitRecordPath(loomengine.LoomScratchDir(loc)), parentlock.ProcessAlive)
		if err != nil {
			return time.Time{}, false, err
		}
		if lockWaiting {
			return time.Now(), true, nil
		}
		driftActive, err := driftengine.Active(driftengine.NewPaths(anchor))
		if err != nil {
			return time.Time{}, false, err
		}
		if driftActive {
			return time.Now(), true, nil
		}
		markers, err := shuttleengine.ReadWaitMarkers(cfg, anchor)
		if err != nil {
			return time.Time{}, false, err
		}
		for _, marker := range markers {
			if !marker.Held() {
				return time.Now(), true, nil
			}
		}
		engines, err := shuttleengines.New(cfg, fabricengine.ConfigDirs(loc))
		if err != nil {
			return time.Time{}, false, err
		}
		readings, err := shuttleengine.ReadAgentActivity(cfg, anchor, engines)
		if err != nil {
			return time.Time{}, false, err
		}
		var newest time.Time
		for _, reading := range readings {
			if reading.LastActivity.After(newest) {
				newest = reading.LastActivity
			}
		}
		return newest, len(readings) > 0, nil
	}
}

// loomAfterInterrupt is loom's AfterInterrupt hook and returns the envelope's "friction" key.
// It writes a halt note classified as an interrupted stop, with the cause as the reason, and spawns no reflection, so an interrupted stop leaves a note the run's Friction-Reflect row covers.
// The note records the state the status file holds after the stop, or `failed` when the file is unreadable.
// When the file holds an error the stop did not write, such as the child's own producer failure, the reason carries it on a `status error:` line, so the two halts of one step are linked.
// It always reports skipped.
func (c *loomCLI) loomAfterInterrupt(_ context.Context, producer, cause, traceFile string) string {
	note := haltNote{
		Producer:    producer,
		State:       shedengine.StateFailed,
		Reason:      cause,
		TraceFile:   traceFile,
		Interrupted: true,
	}
	if st, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath); err == nil && found {
		note.State = st.State
		note.History = st.History
		if st.Error != "" && !strings.Contains(st.Error, cause) {
			note.Reason = cause + "\nstatus error: " + st.Error
		}
	}
	return c.reflectHalt(note)
}
