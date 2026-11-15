// proctree_windows_test.go pins the Windows process-table Name derivation that proctree_windows.go filters on.

package reedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestTmuxProcessName checks that the Windows process-table Name derivation follows the configured tmux binary.
// A hardcoded filter matches nothing where reed resolves tmux to tmux.exe, so Down would leak every server.
func TestTmuxProcessName(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		binary string
		want   string
	}{
		{"bare name", "tmux", "tmux.exe"},
		{"bare psmux", "psmux", "psmux.exe"},
		{"full windows path with exe", `C:\Code\tools\bin\tmux.exe`, "tmux.exe"},
		{"uppercase extension kept", `C:\tools\PSMUX.EXE`, "PSMUX.EXE"},
		{"unix-style path", "/usr/bin/tmux", "tmux.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tmuxProcessName(tt.binary); got != tt.want {
				t.Errorf("tmuxProcessName(%q) = %q; want %q", tt.binary, got, tt.want)
			}
		})
	}
}
