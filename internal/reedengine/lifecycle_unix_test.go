//go:build !windows

// lifecycle_unix_test.go holds the lifecycle tests for tmux behavior that psmux on Windows lacks.

package reedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPinBootOptionsLocked_LeavesTheWindowResizedHookAlone pins the fresh boot's hooks.
// It issues no set-hook naming window-resized, whatever the watchdog value.
func TestPinBootOptionsLocked_LeavesTheWindowResizedHookAlone(t *testing.T) {
	tiergate.Require(t, 0)
	for _, watchdog := range []string{"on", "off"} {
		t.Run("Watchdog_"+watchdog, func(t *testing.T) {
			e := newTestEngine(t)
			e.cfg.Watchdog = watchdog
			fake := installFakeTmux(t, e)
			if err := e.pinBootOptionsLocked("off", ""); err != nil {
				t.Fatalf("pinBootOptionsLocked() = %v, want nil", err)
			}

			if got := fake.SetHookCalls(windowResizedHookName); len(got) != 0 {
				t.Errorf("set-hook calls naming %s = %v, want none", windowResizedHookName, got)
			}
			assertUnseenClearHookPinned(t, e, fake)
		})
	}
}
