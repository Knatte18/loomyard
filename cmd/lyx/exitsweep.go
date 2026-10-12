// exitsweep.go implements the process-exit hook main and run call in place of logger.NotifyExit.
//
// notifyExitAndSweep runs in a fixed order:
// it calls logger.NotifyExit(code), which force-arms the durable sink on a non-zero code;
// it reads logger.CurrentSinkArmState and returns when the sink never armed;
// otherwise it picks the retention bounds and sweeps the sink's directory once.
// The config load waits for the arm check because a degrading configengine load logs at Info,
// and an Info record arms the sink, so loading first would arm a sink on every zero-exit run that logged nothing else.
// A redirected sink sweeps with the compiled-in defaults and never reads logger.yaml,
// since a redirected directory belongs to a caller other than the worktree the config describes.

package main

import (
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// notifyExitAndSweep force-arms the sink on a non-zero code, then sweeps the sink directory when the sink armed.
// It never changes the exit code, and the sweep's error is ignored.
func notifyExitAndSweep(code int) {
	logger.NotifyExit(code)

	state := logger.CurrentSinkArmState()
	if !state.Armed {
		return
	}
	_ = logger.Sweep(state.Dir, exitSweepBounds(state))
}

// exitSweepBounds picks the bounds the exit sweep runs with.
// A redirected sink gets the defaults without a config load;
// otherwise logger.yaml's layer stack under the anchor decides,
// and an unusable one logs one Warn naming the file and falls back to the defaults.
// The sink holds only its anchor, so the hub geometry comes from resolving that anchor's location;
// when resolution fails the load stacks the anchor's own layers alone.
func exitSweepBounds(state logger.SinkArmState) logger.RetentionBounds {
	if state.Redirected {
		return logger.DefaultRetentionBounds()
	}
	dirs := configengine.Dirs{Anchor: state.AnchorPath}
	if location, err := lyxcwd.ResolveWorktree(state.AnchorPath); err == nil {
		dirs = fabricengine.ConfigDirs(location)
	}
	bounds, err := loggerconfig.Load(dirs)
	if err != nil {
		logger.Warn("logger config unusable; sweeping with default retention bounds",
			"path", loggerconfig.ConfigPath(state.AnchorPath), "error", err)
		return logger.DefaultRetentionBounds()
	}
	return bounds
}
