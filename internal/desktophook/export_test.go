// export_test.go exposes AfterWindow under a caller-chosen wait and timeout, so the tests can pin both without real sleeps.

package desktophook

import (
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// AfterWindowWithTiming runs AfterWindow with sleep standing in for the wait.
// The command gets commandTimeout as its timeout and commandWaitDelay as its output wait delay.
func AfterWindowWithTiming(dirs configengine.Dirs, dir string, sleep func(time.Duration), commandTimeout, commandWaitDelay time.Duration) error {
	return afterWindow(dirs, dir, timing{sleep: sleep, commandTimeout: commandTimeout, commandWaitDelay: commandWaitDelay})
}
