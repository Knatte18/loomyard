//go:build tier0 && !windows

// lifecycle_unix_test.go holds the lifecycle tests of tmux behavior psmux on Windows does not have.

package reedengine

import (
	"slices"
	"testing"
)

// TestPinBootOptionsLocked_ClearsTheWindowResizedHook pins that a fresh boot unpins the window-resized hook in both scopes and installs no entry, whatever the watchdog value.
func TestPinBootOptionsLocked_ClearsTheWindowResizedHook(t *testing.T) {
	for _, watchdog := range []string{"on", "off"} {
		t.Run("Watchdog_"+watchdog, func(t *testing.T) {
			e := newTestEngine(t)
			e.cfg.Watchdog = watchdog
			fake := installFakeTmux(t, e)
			if err := e.pinBootOptionsLocked("off", ""); err != nil {
				t.Fatalf("pinBootOptionsLocked() = %v, want nil", err)
			}

			if got, want := fake.SetHookCalls(windowResizedHookName), wantResizedUnpins(exactSessionWindowTarget(e.SessionName())); !slices.EqualFunc(got, want, slices.Equal[[]string]) {
				t.Errorf("set-hook calls naming %s = %v, want the two unpins and no installing entry: %v", windowResizedHookName, got, want)
			}
			assertUnseenClearHookPinned(t, e, fake)
		})
	}
}
