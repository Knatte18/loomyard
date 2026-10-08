//go:build tmux

// window_integration_test.go proves OpenWindow against a real tmux.
// The window opens detached without moving the current window, reads remain-on-exit off while the strands' window still reads on, closes when its command exits (also when it exits at once), and a second call while it is live starts nothing.
// One cold session serves every step.

package reedengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestOpenWindow_RealTmux runs its steps in order on one cold session.
// It overrides the executable path so the window's `lyx` is a stub that sleeps for its first argument's seconds, which makes it process-global state, so it does not run in parallel.
func TestOpenWindow_RealTmux(t *testing.T) {
	stubDir := t.TempDir()
	stub := filepath.Join(stubDir, "lyx")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nsleep \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write lyx stub: %v", err)
	}
	withInjectedExecutablePath(t, func() (string, error) { return stub, nil })

	e := newColdScratchEngine(t)
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil || st.SelvagePaneID == "" {
		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted SelvagePaneID", st, err)
	}
	strandWindow := windowOfPane(t, e, st.SelvagePaneID)
	currentWindow := func() string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_id}")
		if err != nil {
			t.Fatalf("current window: %v", err)
		}
		return strings.TrimSpace(out)
	}
	remainOnExit := func(window string) string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "-t", window, "#{remain-on-exit}")
		if err != nil {
			t.Fatalf("remain-on-exit of %s: %v", window, err)
		}
		return strings.TrimSpace(out)
	}
	windowListed := func(window string) bool {
		t.Helper()
		out, err := e.tmux.output("list-panes", "-s", "-t", exactSessionTarget(e.SessionName()), "-F", "#{window_id}")
		if err != nil {
			t.Fatalf("list windows: %v", err)
		}
		return slices.Contains(strings.Fields(out), window)
	}

	var opened WindowResult
	t.Run("OpensDetachedWithRemainOnExitOffAndLeavesTheStrandWindowOn", func(t *testing.T) {
		opened, err = e.OpenWindow("batten:one", []string{"300"})
		if err != nil {
			t.Fatalf("OpenWindow = %v, want nil", err)
		}
		if opened.Existing || opened.Name != "batten:one" || !strings.HasPrefix(opened.WindowID, "@") {
			t.Fatalf("OpenWindow = %+v, want a started window named batten:one with a window id", opened)
		}
		if got := currentWindow(); got != strandWindow {
			t.Errorf("current window = %q after opening, want it left on the strand window %q", got, strandWindow)
		}
		if got := remainOnExit(opened.WindowID); got != "off" {
			t.Errorf("new window remain-on-exit = %q, want off", got)
		}
		if got := remainOnExit(strandWindow); got != "on" {
			t.Errorf("strand window remain-on-exit = %q, want on", got)
		}
	})

	t.Run("ASecondCallWhileLiveStartsNothing", func(t *testing.T) {
		again, err := e.OpenWindow("batten:one", []string{"300"})
		if err != nil {
			t.Fatalf("OpenWindow (second) = %v, want nil", err)
		}
		if !again.Existing || again.WindowID != opened.WindowID {
			t.Errorf("OpenWindow (second) = %+v, want the existing window %q", again, opened.WindowID)
		}
	})

	for _, tt := range []struct{ name, seconds string }{{"ClosesWhenItsCommandExits", "1"}, {"ClosesWhenItsCommandExitsAtOnce", "0"}} {
		t.Run(tt.name, func(t *testing.T) {
			res, err := e.OpenWindow("batten:"+tt.seconds, []string{tt.seconds})
			if err != nil {
				t.Fatalf("OpenWindow = %v, want nil", err)
			}
			waitUntil(t, 10*time.Second, "window "+res.WindowID+" never closed after its command exited", func() bool {
				return !windowListed(res.WindowID)
			})
		})
	}
}
