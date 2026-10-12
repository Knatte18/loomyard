//go:build tmux

// ownwindow_integration_test.go proves own-window strands against a real tmux.
// An own-window strand opens in a detached window of its own named by its role without moving the viewer, keeps a dead pane bound across later operations, is found again by Resume after a server restart, is refused a move out of its anchor, and takes its window with it on removal.
// One cold session serves every step.

package reedengine

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestOwnWindow_RealTmux runs its steps in order on one cold session.
// It overrides the executable path so the detached `lyx` window of OpenWindow is a stub that sleeps for its first argument's seconds, which makes it process-global state, so it does not run in parallel.
func TestOwnWindow_RealTmux(t *testing.T) {
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
	strandWindow := func() string {
		t.Helper()
		st, err := LoadState(e.stateDir())
		if err != nil || st == nil || st.SelvagePaneID == "" {
			t.Fatalf("LoadState = (%+v, %v), want a persisted SelvagePaneID", st, err)
		}
		return windowOfPane(t, e, st.SelvagePaneID)
	}
	display := func(target, format string) string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "-t", target, format)
		if err != nil {
			t.Fatalf("display-message %s %s: %v", target, format, err)
		}
		return strings.TrimSpace(out)
	}
	currentWindow := func() string {
		t.Helper()
		return display(exactSessionWindowTarget(e.SessionName()), "#{window_id}")
	}
	activePane := func() string {
		t.Helper()
		return display(exactSessionWindowTarget(e.SessionName()), "#{pane_id}")
	}
	windowPanes := func(window string) []string {
		t.Helper()
		out, err := e.tmux.output("list-panes", "-t", window, "-F", "#{pane_id}")
		if err != nil {
			t.Fatalf("list-panes %s: %v", window, err)
		}
		return strings.Fields(out)
	}
	windowListed := func(window string) bool {
		t.Helper()
		out, err := e.tmux.output("list-panes", "-s", "-t", exactSessionTarget(e.SessionName()), "-F", "#{window_id}")
		if err != nil {
			t.Fatalf("list windows: %v", err)
		}
		return slices.Contains(strings.Fields(out), window)
	}
	paneDead := func(pane string) bool {
		out, err := e.tmux.output("display-message", "-p", "-t", pane, "#{pane_dead}")
		return err == nil && strings.TrimSpace(out) == "1"
	}
	statusOf := func(guid string) (StrandStatus, bool) {
		t.Helper()
		res, err := e.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		for _, s := range res.Strands {
			if s.GUID == guid {
				return s, true
			}
		}
		return StrandStatus{}, false
	}
	ownDisplay := render.Display{Anchor: render.AnchorOwnWindow}
	windowZero := render.Display{Anchor: render.AnchorBelowParent}

	if _, err := e.AddStrand(AddSpec{Role: "base", Cmd: "sleep 300", Display: windowZero}); err != nil {
		t.Fatalf("AddStrand(base): %v", err)
	}

	var explorer Strand
	t.Run("OpensDetachedNamedByRoleWithoutMovingTheViewer", func(t *testing.T) {
		wantWindow, wantPane := currentWindow(), activePane()
		wantWindowZeroPanes := windowPanes(strandWindow())

		var err error
		explorer, err = e.AddStrand(AddSpec{Role: "explore", Segment: "review", Cmd: "sleep 300", Display: ownDisplay})
		if err != nil {
			t.Fatalf("AddStrand(own-window) = %v, want nil", err)
		}

		window := windowOfPane(t, e, explorer.PaneID)
		if window == strandWindow() {
			t.Fatalf("own-window pane %s is in the strands' window %s, want a window of its own", explorer.PaneID, window)
		}
		if got := currentWindow(); got != wantWindow {
			t.Errorf("current window = %q after the add, want it left on %q", got, wantWindow)
		}
		if got := activePane(); got != wantPane {
			t.Errorf("active pane = %q after the add, want it left on %q", got, wantPane)
		}
		if got := display(window, "#{window_name}"); got != "explore" {
			t.Errorf("own window name = %q, want its role explore", got)
		}
		if got := display(window, "#{@lyx_own}"); got != "1" {
			t.Errorf("own window @lyx_own = %q, want 1", got)
		}
		if got := display(window, "#{@strand_color}"); got != "colour208" {
			t.Errorf("own window @strand_color = %q, want colour208", got)
		}
		if runtime.GOOS != "windows" {
			if got := display(window, "#{@lyx_unseen}"); got != "1" {
				t.Errorf("own window @lyx_unseen = %q, want 1", got)
			}
		}
		status, ok := statusOf(explorer.GUID)
		if !ok || !status.Live || !status.Present || !status.OwnWindow {
			t.Errorf("Status of the own-window strand = %+v (found %v), want live, present and own-window", status, ok)
		}
		if got := windowPanes(strandWindow()); !slices.Equal(got, wantWindowZeroPanes) {
			t.Errorf("strands' window panes = %v after the add, want them unchanged %v", got, wantWindowZeroPanes)
		}
	})

	t.Run("DeadPaneStaysBoundAcrossLaterOps", func(t *testing.T) {
		dyingOwn, err := e.AddStrand(AddSpec{Role: "dying", Cmd: "exit", Display: ownDisplay})
		if err != nil {
			t.Fatalf("AddStrand(dying own-window): %v", err)
		}
		dyingZero, err := e.AddStrand(AddSpec{Role: "dying-zero", Cmd: "exit", Display: windowZero})
		if err != nil {
			t.Fatalf("AddStrand(dying window 0): %v", err)
		}
		waitUntil(t, 10*time.Second, "dying own-window pane never died", func() bool { return paneDead(dyingOwn.PaneID) })
		waitUntil(t, 10*time.Second, "dying window 0 pane never died", func() bool { return paneDead(dyingZero.PaneID) })

		if _, err := e.AddStrand(AddSpec{Role: "later", Cmd: "sleep 300", Display: windowZero}); err != nil {
			t.Fatalf("AddStrand(later): %v", err)
		}

		own, ok := statusOf(dyingOwn.GUID)
		if !ok || own.PaneID != dyingOwn.PaneID || own.Live || !own.Present {
			t.Errorf("Status of the dead own-window strand = %+v (found %v), want it bound to %s, not live and present", own, ok, dyingOwn.PaneID)
		}
		zero, ok := statusOf(dyingZero.GUID)
		if !ok || zero.PaneID != "" || zero.Present {
			t.Errorf("Status of the dead window 0 strand = %+v (found %v), want its pane killed and its binding cleared", zero, ok)
		}
	})

	t.Run("OrphanOwnWindowIsEndedAndAPlainWindowIsKept", func(t *testing.T) {
		newWindow := func() string {
			t.Helper()
			out, err := e.tmux.output("new-window", "-d", "-P", "-F", "#{window_id}", "-t", exactSessionWindowTarget(e.SessionName()))
			if err != nil {
				t.Fatalf("new-window: %v", err)
			}
			return strings.TrimSpace(out)
		}
		orphan, plain := newWindow(), newWindow()
		if err := e.tmux.run("set-option", "-w", "-t", orphan, ownWindowOption, "1"); err != nil {
			t.Fatalf("mark the orphan own: %v", err)
		}

		if _, err := e.AddStrand(AddSpec{Role: "after-orphan", Cmd: "sleep 300", Display: windowZero}); err != nil {
			t.Fatalf("AddStrand(after-orphan): %v", err)
		}

		if windowListed(orphan) {
			t.Errorf("orphan own window %s is still listed, want it ended", orphan)
		}
		if !windowListed(plain) {
			t.Errorf("plain window %s is gone, want a window without the own marker kept", plain)
		}
	})

	t.Run("OpenWindowOfTheSameNameLeavesTheOwnWindowAlone", func(t *testing.T) {
		ownWindow := windowOfPane(t, e, explorer.PaneID)

		res, err := e.OpenWindow("explore", []string{"300"})
		if err != nil {
			t.Fatalf("OpenWindow = %v, want nil", err)
		}
		if res.Existing || res.WindowID == ownWindow {
			t.Errorf("OpenWindow = %+v, want a new window other than the own window %s", res, ownWindow)
		}
		if got := windowOfPane(t, e, explorer.PaneID); got != ownWindow {
			t.Errorf("own-window pane is in %q, want it still in %q", got, ownWindow)
		}
	})

	t.Run("UpdateStrandRefusesAMoveOutOfOwnWindow", func(t *testing.T) {
		ownWindow := windowOfPane(t, e, explorer.PaneID)

		_, err := e.UpdateStrand(explorer.GUID, windowZero)
		if err == nil || !strings.Contains(err.Error(), "way forward: lyx reed remove") {
			t.Fatalf("UpdateStrand = %v, want a refusal naming lyx reed remove", err)
		}
		if got := windowOfPane(t, e, explorer.PaneID); got != ownWindow {
			t.Errorf("own-window pane is in %q after the refusal, want it still in %q", got, ownWindow)
		}
	})

	t.Run("ResumeAfterAServerRestartReopensDetached", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "done")
		finished, err := e.AddStrand(AddSpec{
			Role:     "finished",
			Cmd:      "touch '" + marker + "'; exit",
			DoneWhen: []string{marker},
			Display:  ownDisplay,
		})
		if err != nil {
			t.Fatalf("AddStrand(finished): %v", err)
		}
		waitUntil(t, 10*time.Second, "finished own-window pane never died", func() bool { return paneDead(finished.PaneID) })
		if status, ok := statusOf(finished.GUID); !ok || status.PaneID != finished.PaneID || status.Live {
			t.Fatalf("Status of the finished strand = %+v (found %v), want it bound to its dead pane %s", status, ok, finished.PaneID)
		}

		if err := e.tmux.run("kill-server"); err != nil {
			t.Fatalf("kill-server: %v", err)
		}
		if _, err := e.Resume(); err != nil {
			t.Fatalf("Resume: %v", err)
		}

		status, ok := statusOf(explorer.GUID)
		if !ok || !status.Live || !status.OwnWindow {
			t.Fatalf("Status of the live own-window strand after Resume = %+v (found %v), want it relaunched live", status, ok)
		}
		reopened := windowOfPane(t, e, status.PaneID)
		if reopened == strandWindow() {
			t.Errorf("relaunched own-window pane is in the strands' window %s, want a window of its own", reopened)
		}
		if got := currentWindow(); got == reopened {
			t.Errorf("current window = %q after Resume, want it not the reopened own window", got)
		}
		if _, ok := statusOf(finished.GUID); ok {
			t.Errorf("finished strand %s survived Resume, want it dropped instead of relaunched", finished.GUID)
		}
		explorer.PaneID = status.PaneID
	})

	t.Run("RemoveEndsTheOwnWindow", func(t *testing.T) {
		ownWindow := windowOfPane(t, e, explorer.PaneID)

		if _, err := e.RemoveStrand(explorer.GUID, false); err != nil {
			t.Fatalf("RemoveStrand: %v", err)
		}

		if windowListed(ownWindow) {
			t.Errorf("own window %s is still listed after its strand was removed, want it gone", ownWindow)
		}
	})
}
