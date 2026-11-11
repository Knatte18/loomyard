// terminalwindowtitle_integration_test.go runs terminalWindowTitle against a real hub.
// It also runs it on a directory outside any worktree.

package reedcli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestTerminalWindowTitle covers a directory in no worktree, which takes its base name.
// It also covers the prime worktree, which takes the console title of the orch session.
func TestTerminalWindowTitle(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	outside := t.TempDir()
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"DirectoryInNoWorktreeTakesItsBaseName", outside, filepath.Base(outside)},
		{"ThePrimeWorktreeTakesTheOrchTitle", h.PrimeWorktree(), "ORCH [" + strings.ToLower(hubforge.TestShortname) + ":P0] root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := terminalWindowTitle(tt.dir); got != tt.want {
				t.Errorf("terminalWindowTitle(%q) = %q; want %q", tt.dir, got, tt.want)
			}
		})
	}
}
