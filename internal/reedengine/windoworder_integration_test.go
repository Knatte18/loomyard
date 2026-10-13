//go:build tmux

// windoworder_integration_test.go proves the window order against a real tmux.
// The session's windows hold the agents in start order, then the window named shell, then the hidden windows; a relaunch keeps its window's place, and a session booted by an older lyx is brought into that order.
// One cold session serves every step.

package reedengine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestWindowOrder_RealTmux runs its steps in order on one cold session.
// It overrides the executable path so the detached `lyx` window of OpenWindow is a stub that sleeps for its first argument's seconds, which makes it process-global state, so it does not run in parallel.
func TestWindowOrder_RealTmux(t *testing.T) {
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
	windows := func() []sessionWindow {
		t.Helper()
		listed, err := e.tmux.listSessionWindows(e.SessionName())
		if err != nil {
			t.Fatalf("list windows: %v", err)
		}
		return listed
	}
	names := func() []string {
		t.Helper()
		var out []string
		for _, w := range windows() {
			out = append(out, w.Name)
		}
		return out
	}
	wantNames := func(want ...string) {
		t.Helper()
		if got := names(); !slices.Equal(got, want) {
			t.Fatalf("window names in index order = %v, want %v", got, want)
		}
	}
	paneDead := func(pane string) bool {
		out, err := e.tmux.output("display-message", "-p", "-t", pane, "#{pane_dead}")
		return err == nil && out == "1\n"
	}
	ownDisplay := render.Display{Anchor: render.AnchorOwnWindow}
	add := func(name, cmd string) Strand {
		t.Helper()
		s, err := e.AddStrand(AddSpec{NameOverride: name, Cmd: cmd, Display: ownDisplay})
		if err != nil {
			t.Fatalf("AddStrand(%s): %v", name, err)
		}
		return s
	}

	var beta, gamma Strand
	t.Run("UpNamesItsOneWindowShell", func(t *testing.T) {
		wantNames("shell")
	})

	t.Run("OwnWindowsOpenInStartOrderBeforeTheShell", func(t *testing.T) {
		add("alpha", "sleep 300")
		beta = add("beta", "sleep 300")
		gamma = add("gamma", "exit")
		wantNames("alpha", "beta", "gamma", "shell")
		for i, w := range windows() {
			if w.Index != windows()[0].Index+i {
				t.Errorf("window %s has index %d, want ascending indexes with no gap", w.Name, w.Index)
			}
		}
	})

	t.Run("RemovingTheMiddleOneAndAddingAFourthKeepsStartOrder", func(t *testing.T) {
		if _, err := e.RemoveStrand(beta.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(beta): %v", err)
		}
		add("delta", "sleep 300")
		wantNames("alpha", "gamma", "delta", "shell")
	})

	t.Run("RelaunchingADeadAgentKeepsItsPlaceBetweenItsNeighbours", func(t *testing.T) {
		waitUntil(t, 10*time.Second, "gamma's pane never died", func() bool { return paneDead(gamma.PaneID) })
		oldWindow := windowOfPane(t, e, gamma.PaneID)

		relaunched, err := e.AddStrand(AddSpec{NameOverride: "gamma", IfAbsent: true, Cmd: "sleep 300", Display: ownDisplay})
		if err != nil {
			t.Fatalf("AddStrand(gamma, IfAbsent): %v", err)
		}
		if got := windowOfPane(t, e, relaunched.PaneID); got == oldWindow {
			t.Errorf("relaunched pane is in the old window %s, want a new window", got)
		}
		wantNames("alpha", "gamma", "delta", "shell")
	})

	t.Run("OpenedWindowsSitAfterTheShell", func(t *testing.T) {
		for _, name := range []string{"batten:one", "batten:two"} {
			if _, err := e.OpenWindow(name, []string{"300"}); err != nil {
				t.Fatalf("OpenWindow(%s): %v", name, err)
			}
		}
		wantNames("alpha", "gamma", "delta", "shell", "batten:one", "batten:two")
	})

	t.Run("AnOldSessionIsRenamedAndMovedByTheNextOpenWindow", func(t *testing.T) {
		st, err := LoadState(e.stateDir())
		if err != nil || st == nil || st.SelvagePaneID == "" {
			t.Fatalf("LoadState = (%+v, %v), want a persisted SelvagePaneID", st, err)
		}
		shellWindow := windowOfPane(t, e, st.SelvagePaneID)
		if err := e.tmux.run("rename-window", "-t", shellWindow, "tmux"); err != nil {
			t.Fatalf("rename the shell window: %v", err)
		}
		if err := e.tmux.run("new-window", "-d", "-n", "plain", "-t", exactSessionWindowTarget(e.SessionName())); err != nil {
			t.Fatalf("open a plain window: %v", err)
		}

		if _, err := e.OpenWindow("batten:three", []string{"300"}); err != nil {
			t.Fatalf("OpenWindow(batten:three): %v", err)
		}

		listed := windows()
		var shell sessionWindow
		for _, w := range listed {
			if w.ID == shellWindow {
				shell = w
			}
		}
		if shell.Name != shellWindowName {
			t.Errorf("shell window is named %q, want %q", shell.Name, shellWindowName)
		}
		for _, w := range listed {
			if !w.Hidden && w.Index > shell.Index {
				t.Errorf("visible window %s has index %d behind the shell window at %d, want the shell last among visible windows", w.Name, w.Index, shell.Index)
			}
		}
		last, _ := highestWindow(listed, false)
		if last.Name != "batten:three" {
			t.Errorf("highest window is %q, want the newly opened batten:three", last.Name)
		}
	})

	t.Run("ABelowParentStrandRelaunchedByResumeIsBackInTheShellWindow", func(t *testing.T) {
		base, err := e.AddStrand(AddSpec{NameOverride: "base", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}})
		if err != nil {
			t.Fatalf("AddStrand(base): %v", err)
		}
		if err := e.tmux.run("kill-server"); err != nil {
			t.Fatalf("kill-server: %v", err)
		}
		if _, err := e.Resume(); err != nil {
			t.Fatalf("Resume: %v", err)
		}

		status, err := e.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		for _, s := range status.Strands {
			if s.GUID != base.GUID {
				continue
			}
			window := windowOfPane(t, e, s.PaneID)
			for _, w := range windows() {
				if w.ID == window && w.Name != shellWindowName {
					t.Errorf("relaunched below-parent strand is in window %q, want the shell window", w.Name)
				}
			}
			return
		}
		t.Fatalf("Status carries no strand %s after Resume", base.GUID)
	})
}
