//go:build !windows

// window_test.go covers desktophook.AfterWindow and AfterPlacement running a real sh command.
// The commands are POSIX shell, so Windows is left out.

package desktophook_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAfterPlacement covers these behaviors of the call.
// An unset command runs nothing.
// A config that fails to load is an error.
// A set command runs with no wait.
// A cancel ends the command with its whole process group.
func TestAfterPlacement(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("UnsetCommandRunsNothing", func(t *testing.T) {
		t.Parallel()
		dirs := newDirsWithMachineConfig(t, "window_wait_ms: 600000\n")
		if err := desktophook.AfterPlacement(context.Background(), dirs, t.TempDir()); err != nil {
			t.Errorf("AfterPlacement() = %v; want nil", err)
		}
	})

	t.Run("ConfigThatFailsToLoadIsAnError", func(t *testing.T) {
		t.Parallel()
		dirs := newDirsWithMachineConfig(t, "window_wait_ms: -1\n")
		err := desktophook.AfterPlacement(context.Background(), dirs, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "desktop.window_wait_ms") {
			t.Errorf("AfterPlacement() = %v; want an error naming desktop.window_wait_ms", err)
		}
	})

	t.Run("SetCommandRunsAtOnce", func(t *testing.T) {
		t.Parallel()
		dirs := newDirsWithMachineConfig(t, "window_command: \"pwd > marker\"\nwindow_wait_ms: 600000\n")
		windowDir := t.TempDir()

		done := make(chan error, 1)
		go func() { done <- desktophook.AfterPlacement(context.Background(), dirs, windowDir) }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("AfterPlacement() = %v; want nil", err)
			}
		case <-time.After(time.Minute):
			t.Fatalf("AfterPlacement() has not returned after a minute; want it to run the command with no wait")
		}

		marker, err := os.ReadFile(filepath.Join(windowDir, "marker"))
		if err != nil {
			t.Fatalf("read marker: %v; want the command to have written it", err)
		}
		if got := strings.TrimSpace(string(marker)); got != windowDir {
			t.Errorf("command ran in %q; want %q", got, windowDir)
		}
	})

	t.Run("CancelEndsTheCommandAndItsGroup", func(t *testing.T) {
		t.Parallel()
		dirs := newDirsWithMachineConfig(t, "window_command: \"sleep 300 & echo $! > child; wait\"\n")
		windowDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		done := make(chan error, 1)
		go func() {
			done <- desktophook.AfterPlacementWithTiming(ctx, dirs, windowDir, time.Hour, time.Second, time.Second)
		}()

		deadline := time.Now().Add(time.Minute)
		var childPid int
		for childPid == 0 {
			// The newline marks the pid as written whole.
			content, err := os.ReadFile(filepath.Join(windowDir, "child"))
			if err == nil && strings.HasSuffix(string(content), "\n") {
				childPid, err = strconv.Atoi(strings.TrimSpace(string(content)))
				if err != nil {
					t.Fatalf("parse child pid %q: %v", content, err)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the command wrote no child pid within a minute")
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()

		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("AfterPlacement() = %v; want an error wrapping context.Canceled", err)
			}
		case <-time.After(time.Minute):
			t.Fatalf("AfterPlacement() has not returned a minute after the cancel")
		}
		for proc.IsAlive(childPid) {
			if time.Now().After(deadline) {
				t.Fatalf("the command's child %d is still alive after the cancel", childPid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

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
		// wantSleeps lists the waits that precede the command.
		wantSleeps []time.Duration
	}{
		{name: "UnsetCommandRunsNothing", machine: "window_wait_ms: 7\n"},
		{name: "BlankCommandRunsNothing", machine: "window_command: \"   \"\nwindow_wait_ms: 7\n"},
		{name: "SetCommandRunsInWindowDirAfterTheWait", machine: "window_command: \"pwd > marker\"\nwindow_wait_ms: 40\n", wantMarker: true, wantSleeps: []time.Duration{40 * time.Millisecond}},
		{name: "FailingCommandReportsOutput", machine: "window_command: \"echo boom; exit 3\"\nwindow_wait_ms: 0\n", wantErr: "boom", wantSleeps: []time.Duration{0}},
		// The hanging command outlasts the minute every row allows, so a call that waits on it fails however loaded the machine.
		{name: "HangingCommandIsCutOffAtTheTimeout", machine: "window_command: \"exec sleep 300\"\nwindow_wait_ms: 0\n", timeout: 50 * time.Millisecond, wantErr: "window command", wantSleeps: []time.Duration{0}},
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
			if elapsed > time.Minute {
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
