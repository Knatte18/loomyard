// looparm.go fills what loom tells the generic step verb's --until-stop loop: its files, its trace listing, its watchdog window and activity reading, and the friction note of an interrupted stop.

package loomcli

import (
	"context"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// loopSpecFor is the loop spec of the run runID in loc.
// The loop's files and job name come from shedrun over the resolved run-id.
// The step children run the same binary from the same cwd as the loop, so they share its trace directory and the loop lists a child's trace files there.
// idle is the watchdog window, zero disarming it, and activity is the reading of the run's agent activity.
func loopSpecFor(loc *lyxcwd.Location, runID string, idle time.Duration, activity func() (time.Time, bool, error)) shedverbs.LoopSpec {
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
	}
}

// agentActivityFor returns the reading of the run's agent activity: the newest LastActivity over every live agent run of loc's worktree.
// The runs come from shuttle through a Claude engine, which lists every strand, so a MultiLLM row's chair and each of its advisors count.
// A running verify, or a shuttle wait that is not a held turn end, reads as activity now.
// Both are read from their marker files.
// The boolean is false when nothing is live.
func agentActivityFor(loc *lyxcwd.Location, cfg shuttleengine.Config) func() (time.Time, bool, error) {
	return func() (time.Time, bool, error) {
		anchor := loc.AnchorPath()
		_, verifyLive, err := verifytree.ReadMarker(verifytree.NewPaths(loc.WorktreePath(), verifytree.Dir(anchor)).Marker)
		if err != nil {
			return time.Time{}, false, err
		}
		if verifyLive {
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
		readings, err := shuttleengine.ReadAgentActivity(cfg, anchor, claudeengine.NewFromConfig(cfg))
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
// It writes the halt note a `failed` halt writes, with the cause as the reason, and reflects, so an interrupted stop leaves a note the next reflection covers.
// With Tier 2 off it reports skipped.
func (c *loomCLI) loomAfterInterrupt(_ context.Context, producer, cause, traceFile string) string {
	var history []shedengine.HistoryEntry
	if st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath); err == nil && found {
		history = st.History
	}
	return c.reflectHalt(haltNote{
		Producer:  producer,
		State:     shedengine.StateFailed,
		Reason:    cause,
		History:   history,
		TraceFile: traceFile,
	})
}
