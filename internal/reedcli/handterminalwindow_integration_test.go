// handterminalwindow_integration_test.go runs handTerminalWindow on directories no hub holds.
// Making a repository runs git.

package reedcli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHandTerminalWindow_DirectoryInNoHubHandsNothing(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// repository makes the directory a git repository of its own.
		repository bool
		// wantErr is nil where nothing is left for the caller to run.
		wantErr error
	}{
		{name: "NoWorktree"},
		{name: "PlainRepository", repository: true, wantErr: errNoHub},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parent := t.TempDir()
			dir := filepath.Join(parent, "checkout")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.repository {
				gitkit.MustRun(t, dir, "git", "init")
			}

			// A started child would fail on this path.
			if err := handTerminalWindow("/nonexistent/lym", dir, "title", 1); !errors.Is(err, tt.wantErr) {
				t.Errorf("handTerminalWindow() = %v; want %v, with nothing handed", err, tt.wantErr)
			}
			if bolt := fabriclayout.BoltDir(parent); fileExists(bolt) {
				t.Errorf("%s was created; want nothing written beside the directory", bolt)
			}
		})
	}
}
