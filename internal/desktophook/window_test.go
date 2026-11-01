//go:build !windows

// window_test.go covers desktophook.AfterWindow running a real sh command; the commands are POSIX shell, so Windows is left out.

package desktophook_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAfterWindow covers four behaviors.
// A command that is unset or blank runs nothing and waits for nothing.
// A set command runs in the window's directory only after the configured wait.
// A failing command's error carries its output.
// A hanging command is cut off at the timeout.
func TestAfterWindow(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name    string
		machine string
		// timeout is the command timeout; zero means a minute, far above any row that is not testing the timeout.
		timeout    time.Duration
		wantMarker bool
		wantErr    string
		// wantSleeps is the waits the command was preceded by.
		wantSleeps []time.Duration
	}{
		{name: "UnsetCommandRunsNothing", machine: "window_wait_ms: 7\n"},
		{name: "BlankCommandRunsNothing", machine: "window_command: \"   \"\nwindow_wait_ms: 7\n"},
		{name: "SetCommandRunsInWindowDirAfterTheWait", machine: "window_command: \"pwd > marker\"\nwindow_wait_ms: 40\n", wantMarker: true, wantSleeps: []time.Duration{40 * time.Millisecond}},
		{name: "FailingCommandReportsOutput", machine: "window_command: \"echo boom; exit 3\"\nwindow_wait_ms: 0\n", wantErr: "boom", wantSleeps: []time.Duration{0}},
		{name: "HangingCommandIsCutOffAtTheTimeout", machine: "window_command: \"exec sleep 60\"\nwindow_wait_ms: 0\n", timeout: 50 * time.Millisecond, wantErr: "window command", wantSleeps: []time.Duration{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := newDirsWithMachineConfig(t, tt.machine)
			windowDir := t.TempDir()
			markerPath := filepath.Join(windowDir, "marker")
			timeout := tt.timeout
			if timeout == 0 {
				timeout = time.Minute
			}
			var sleeps []time.Duration
			markerPresentAtSleep := false
			sleep := func(d time.Duration) {
				sleeps = append(sleeps, d)
				if _, err := os.Stat(markerPath); err == nil {
					markerPresentAtSleep = true
				}
			}

			start := time.Now()
			err := desktophook.AfterWindowWithTiming(dirs, windowDir, sleep, timeout, 50*time.Millisecond)
			elapsed := time.Since(start)

			if !slices.Equal(sleeps, tt.wantSleeps) {
				t.Errorf("waits = %v; want %v", sleeps, tt.wantSleeps)
			}
			if markerPresentAtSleep {
				t.Errorf("the command had already run when the wait began; want it to run after the wait")
			}
			if elapsed > 10*time.Second {
				t.Errorf("AfterWindow took %v; want it to return promptly", elapsed)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("AfterWindow() error = %v; want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AfterWindow() error = %v; want nil", err)
			}
			if !tt.wantMarker {
				return
			}

			marker, readErr := os.ReadFile(markerPath)
			switch {
			case readErr != nil:
				t.Fatalf("read marker: %v; want the command to have written it", readErr)
			case strings.TrimSpace(string(marker)) != windowDir:
				t.Errorf("command ran in %q; want %q", strings.TrimSpace(string(marker)), windowDir)
			}
		})
	}
}
