//go:build tier0 && windows

// tmuxfloor_windows_test.go pins that the tmux floor check admits on Windows without probing.

package reedengine

import "testing"

// TestCheckTmuxFloor_RunsNoProbeOnWindows swaps the process-global version probe, so it is not parallel.
func TestCheckTmuxFloor_RunsNoProbeOnWindows(t *testing.T) {
	probes := 0
	setTmuxVersionProbeForTest(t, func(string) (string, error) {
		probes++
		return "tmux 3.3a", nil
	})
	if err := CheckTmuxFloor("tmux"); err != nil {
		t.Errorf("CheckTmuxFloor(tmux) = %v; want nil", err)
	}
	if err := NewTmuxCmd("tmux", "sock").checkFloor(); err != nil {
		t.Errorf("checkFloor() = %v; want nil", err)
	}
	if probes != 0 {
		t.Errorf("probes = %d; want 0", probes)
	}
}
