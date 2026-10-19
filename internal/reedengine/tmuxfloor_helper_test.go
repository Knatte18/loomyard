// tmuxfloor_helper_test.go holds the probe-injection helper the tmux floor tests share across operating systems.

package reedengine

import "testing"

// setTmuxVersionProbeForTest replaces tmuxVersionProbe with probe and empties tmuxFloors for the test, restoring both on cleanup.
// It changes process-global state, so the caller must not be parallel.
func setTmuxVersionProbeForTest(t *testing.T, probe func(tmuxPath string) (string, error)) {
	t.Helper()
	resetFloors := func() {
		tmuxFloors.mu.Lock()
		defer tmuxFloors.mu.Unlock()
		tmuxFloors.answers = nil
	}
	previous := tmuxVersionProbe
	tmuxVersionProbe = probe
	resetFloors()
	t.Cleanup(func() {
		tmuxVersionProbe = previous
		resetFloors()
	})
}
