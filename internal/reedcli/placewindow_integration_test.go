//go:build tier1 && !windows

// placewindow_integration_test.go runs placeTerminalWindow, which the window verb's tests replace by a seam, against a real hub and a directory outside any worktree; the window command is POSIX shell, so Windows is left out.

package reedcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestPlaceTerminalWindow covers a directory in no worktree running no command and a worktree directory running the hub's configured command there.
func TestPlaceTerminalWindow(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	board := fabricengine.BoltDir(h.Location.HubPath)
	if err := os.MkdirAll(configengine.ScratchDir(board), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ScratchDir(board), err)
	}
	markerName := "marker"
	machine := "window_command: \"pwd > " + markerName + "\"\nwindow_wait_ms: 0\n"
	if err := os.WriteFile(configengine.MachineFile(board, "desktop"), []byte(machine), 0o644); err != nil {
		t.Fatalf("WriteFile the desktop machine override = %v; want nil", err)
	}

	tests := []struct {
		name       string
		dir        string
		wantMarker bool
	}{
		{name: "DirectoryInNoWorktreeRunsNothing", dir: t.TempDir()},
		{name: "WorktreeDirectoryRunsTheConfiguredCommand", dir: h.PrimeWorktree(), wantMarker: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := placeTerminalWindow(tt.dir); err != nil {
				t.Fatalf("placeTerminalWindow(%q) = %v; want nil", tt.dir, err)
			}

			marker, err := os.ReadFile(filepath.Join(tt.dir, markerName))
			switch {
			case !tt.wantMarker && !os.IsNotExist(err):
				t.Errorf("read marker error = %v; want it absent, since the directory is in no worktree", err)
			case tt.wantMarker && err != nil:
				t.Fatalf("read marker: %v; want the command to have written it", err)
			case tt.wantMarker && strings.TrimSpace(string(marker)) != tt.dir:
				t.Errorf("command ran in %q; want %q", strings.TrimSpace(string(marker)), tt.dir)
			}
		})
	}
}
