// exitsweep.go holds the exit hook that arms the log sink on a non-zero exit code and sweeps its directory.

package main

import (
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// notifyExitAndSweep force-arms the sink on a non-zero code for post-mortem inspection.
// It sweeps the sink directory when the sink armed, and it ignores the sweep's error.
// It never changes the exit code.
// The config load waits for the arm check, because a degrading configengine load logs at Info,
// and an Info record arms the sink;
// loading first would arm a sink on every zero-exit run that logged nothing else.
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
// its directory belongs to a caller other than the worktree the config describes.
// Otherwise logger.yaml's layer stack under the anchor decides,
// and an unusable one logs one Warn naming the file and falls back to the defaults.
// The sink holds only its anchor, so the hub geometry comes from resolving that anchor's location;
// when resolution fails, the load stacks the anchor's own layers alone.
func exitSweepBounds(state logger.SinkArmState) logger.RetentionBounds {
	if state.Redirected {
		return logger.DefaultRetentionBounds()
	}
	dirs := configengine.Dirs{Anchor: state.AnchorPath}
	if location, err := lymcwd.ResolveWorktree(state.AnchorPath); err == nil {
		dirs = fabricengine.ConfigDirs(location)
	}
	return loggerconfig.LoadOrDefault(dirs)
}
