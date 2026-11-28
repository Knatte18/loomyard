// export_test.go exposes AfterWindow and AfterPlacement under caller-chosen waits and bounds,
// so the tests can pin them without real sleeps.

package desktophook

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// AfterWindowWithTiming runs AfterWindow with sleep standing in for the wait.
// The command gets commandTimeout as its timeout and commandWaitDelay as its output wait delay.
func AfterWindowWithTiming(dirs configengine.Dirs, dir string, sleep func(time.Duration), commandTimeout, commandWaitDelay time.Duration) error {
	return afterWindow(dirs, dir, timing{sleep: sleep, commandTimeout: commandTimeout, commandWaitDelay: commandWaitDelay})
}

// AfterPlacementWithTiming runs AfterPlacement under caller-chosen bounds.
// The command gets commandTimeout as its timeout and commandWaitDelay as its output wait delay.
// groupKillAfter is the time between the SIGTERM and the SIGKILL of its process group.
func AfterPlacementWithTiming(ctx context.Context, dirs configengine.Dirs, dir string, commandTimeout, commandWaitDelay, groupKillAfter time.Duration) error {
	return afterPlacement(ctx, dirs, dir, timing{commandTimeout: commandTimeout, commandWaitDelay: commandWaitDelay, groupKillAfter: groupKillAfter})
}
