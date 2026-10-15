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
	// A booted session follows the run, which would move the viewer on every add, so the steps keep it off unless a step names the follow state.
	setFollow := func(value string) {
		t.Helper()
		if err := e.tmux.run("set-option", "-t", exactSessionWindowTarget(e.SessionName()), followSessionOption, value); err != nil {
			t.Fatalf("set %s %s: %v", followSessionOption, value, err)
		}
	}
	readFollow := func() string {
		t.Helper()
		return display(exactSessionWindowTarget(e.SessionName()), "#{"+followSessionOption+"}")
	}
	setFollow("off")
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
		if got := readFollow(); got != "on" {
			t.Errorf("follow state = %q after the server restart, want the booted session on", got)
		}
		// With follow on, every relaunched own window takes the viewer in turn, so it ends on the last one opened, the highest-index window of an own-window strand, dead or alive.
		res, err := e.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		windows, err := e.tmux.listSessionWindows(e.SessionName())
		if err != nil {
			t.Fatalf("list windows: %v", err)
		}
		ownWindows := make(map[string]bool)
		for _, s := range res.Strands {
			if s.OwnWindow && s.PaneID != "" {
				ownWindows[windowOfPane(t, e, s.PaneID)] = true
			}
		}
		newest, newestIndex := "", -1
		for _, w := range windows {
			if ownWindows[w.ID] && w.Index > newestIndex {
				newest, newestIndex = w.ID, w.Index
			}
		}
		if got := currentWindow(); got != newest {
			t.Errorf("current window = %q after Resume, want the newest relaunched own window %q", got, newest)
		}
		setFollow("off")
		if _, ok := statusOf(finished.GUID); ok {
			t.Errorf("finished strand %s survived Resume, want it dropped instead of relaunched", finished.GUID)
		}
		explorer.PaneID = status.PaneID
	})

	addGrouped := func(role, group string, size int, cmd, resumeCmd string) Strand {
		t.Helper()
		s, err := e.AddStrand(AddSpec{
			Role:      role,
			Cmd:       cmd,
			ResumeCmd: resumeCmd,
			Display:   render.Display{Anchor: render.AnchorOwnWindow, Window: group, WindowSize: size},
		})
		if err != nil {
			t.Fatalf("AddStrand(%s in %s): %v", role, group, err)
		}
		return s
	}
	paneOrigin := func(pane string) (left, top string) {
		t.Helper()
		fields := strings.Fields(display(pane, "#{pane_left} #{pane_top}"))
		if len(fields) != 2 {
			t.Fatalf("pane origin of %s = %v, want left and top", pane, fields)
		}
		return fields[0], fields[1]
	}
	// restoreConfig puts the config fields a subtest edits back when the subtest ends.
	restoreConfig := func(t *testing.T) {
		shareMax, split := e.cfg.WindowShareMax, e.cfg.WindowSplit
		t.Cleanup(func() { e.cfg.WindowShareMax, e.cfg.WindowSplit = shareMax, split })
	}

	t.Run("GroupOfTwoSharesOneWindowSideBySide", func(t *testing.T) {
		first := addGrouped("pa", "pair", 2, "sleep 300", "")
		window := windowOfPane(t, e, first.PaneID)
		if got := display(window, "#{window_name}"); got != "pair" {
			t.Errorf("shared window name = %q, want the group name pair", got)
		}
		if got := display(window, "#{@lyx_group}"); got != "pair" {
			t.Errorf("shared window @lyx_group = %q, want pair", got)
		}

		if err := e.tmux.run("select-window", "-t", window); err != nil {
			t.Fatalf("select-window: %v", err)
		}
		second := addGrouped("pb", "pair", 2, "sleep 300", "")

		if got := windowOfPane(t, e, second.PaneID); got != window {
			t.Fatalf("second strand is in window %q, want the shared window %q", got, window)
		}
		if got := display(window, "#{pane_id}"); got != first.PaneID {
			t.Errorf("shared window active pane = %q after the join, want the first strand's pane %q", got, first.PaneID)
		}
		if got := currentWindow(); got != window {
			t.Errorf("current window = %q after the join, want it left on %q", got, window)
		}
		firstLeft, firstTop := paneOrigin(first.PaneID)
		secondLeft, secondTop := paneOrigin(second.PaneID)
		if firstTop != secondTop || firstLeft == secondLeft {
			t.Errorf("panes at (%s,%s) and (%s,%s), want them side by side: equal top, different left", firstLeft, firstTop, secondLeft, secondTop)
		}
	})

	t.Run("ADeadMemberStillCountsTowardTheGroup", func(t *testing.T) {
		first := addGrouped("da", "dying-group", 2, "exit", "")
		waitUntil(t, 10*time.Second, "the first group member never died", func() bool { return paneDead(first.PaneID) })

		second := addGrouped("db", "dying-group", 2, "sleep 300", "")
		third := addGrouped("dc", "dying-group", 2, "sleep 300", "")

		window := windowOfPane(t, e, first.PaneID)
		if got := windowOfPane(t, e, second.PaneID); got != window {
			t.Errorf("second member is in window %q, want the shared window %q", got, window)
		}
		if got := windowOfPane(t, e, third.PaneID); got == window {
			t.Errorf("third member joined the full shared window %q, want a window of its own", window)
		}
	})

	t.Run("HeightSplitStacks", func(t *testing.T) {
		restoreConfig(t)
		e.cfg.WindowSplit = "height"

		first := addGrouped("ha", "stack", 2, "sleep 300", "")
		second := addGrouped("hb", "stack", 2, "sleep 300", "")

		firstLeft, firstTop := paneOrigin(first.PaneID)
		secondLeft, secondTop := paneOrigin(second.PaneID)
		if firstLeft != secondLeft || firstTop == secondTop {
			t.Errorf("panes at (%s,%s) and (%s,%s), want them stacked: equal left, different top", firstLeft, firstTop, secondLeft, secondTop)
		}
	})

	t.Run("GroupOfFiveGivesEachItsOwnWindow", func(t *testing.T) {
		seen := map[string]bool{}
		for _, role := range []string{"fa", "fb", "fc"} {
			s := addGrouped(role, "five", 5, "sleep 300", "")
			window := windowOfPane(t, e, s.PaneID)
			if seen[window] {
				t.Errorf("strand %s shares window %s with another member of a size-5 group, want a window each", role, window)
			}
			seen[window] = true
			if got := display(window, "#{window_name}"); got != role {
				t.Errorf("window name = %q, want the role %q", got, role)
			}
		}
	})

	t.Run("OverflowGetsAWindowOfItsOwn", func(t *testing.T) {
		overflow := addGrouped("pc", "pair", 2, "sleep 300", "")

		window := windowOfPane(t, e, overflow.PaneID)
		if got := display(window, "#{window_name}"); got != "pc" {
			t.Errorf("overflow window name = %q, want its role pc", got)
		}
		pairWindow := ""
		out, err := e.tmux.output("list-windows", "-t", exactSessionTarget(e.SessionName()), "-F", "#{window_id} #{window_name}")
		if err != nil {
			t.Fatalf("list-windows: %v", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if fields := strings.Fields(line); len(fields) == 2 && fields[1] == "pair" {
				pairWindow = fields[0]
			}
		}
		if pairWindow == "" || pairWindow == window {
			t.Fatalf("pair window = %q, overflow window = %q; want the pair window kept apart from the overflow", pairWindow, window)
		}
		if got := windowPanes(pairWindow); len(got) != 2 {
			t.Errorf("pair window panes = %v, want its two panes kept", got)
		}
	})

	t.Run("ShareMaxEditBetweenJoinsDoesNotMoveTheSecondJoiner", func(t *testing.T) {
		restoreConfig(t)
		e.cfg.WindowShareMax = 2
		first := addGrouped("la", "late", 2, "sleep 300", "")

		e.cfg.WindowShareMax = 0
		second := addGrouped("lb", "late", 2, "sleep 300", "")

		if got, want := windowOfPane(t, e, second.PaneID), windowOfPane(t, e, first.PaneID); got != want {
			t.Errorf("second strand is in window %q, want it in the first strand's window %q despite window_share_max 0", got, want)
		}
	})

	t.Run("SizeMismatchIsRefused", func(t *testing.T) {
		panesBefore := func() int {
			out, err := e.tmux.output("list-panes", "-s", "-t", exactSessionTarget(e.SessionName()), "-F", "#{pane_id}")
			if err != nil {
				t.Fatalf("list panes: %v", err)
			}
			return len(strings.Fields(out))
		}()

		_, err := e.AddStrand(AddSpec{Role: "mismatch", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow, Window: "pair", WindowSize: 3}})

		if err == nil || !strings.Contains(err.Error(), "size 2") || !strings.Contains(err.Error(), "size 3") {
			t.Fatalf("AddStrand = %v, want a refusal naming size 2 and size 3", err)
		}
		out, listErr := e.tmux.output("list-panes", "-s", "-t", exactSessionTarget(e.SessionName()), "-F", "#{pane_id}")
		if listErr != nil {
			t.Fatalf("list panes: %v", listErr)
		}
		if got := len(strings.Fields(out)); got != panesBefore {
			t.Errorf("session holds %d panes after the refusal, want the %d from before", got, panesBefore)
		}
	})

	t.Run("RelaunchOfADeadGroupMemberRejoinsItsWindow", func(t *testing.T) {
		dead := addGrouped("ra", "relaunch", 2, "exit", "sleep 300")
		partner := addGrouped("rb", "relaunch", 2, "sleep 300", "")
		waitUntil(t, 10*time.Second, "the first group member never died", func() bool { return paneDead(dead.PaneID) })
		window := windowOfPane(t, e, dead.PaneID)

		if _, err := e.Resume(); err != nil {
			t.Fatalf("Resume: %v", err)
		}

		status, ok := statusOf(dead.GUID)
		if !ok || !status.Live || status.PaneID == dead.PaneID {
			t.Fatalf("Status of the relaunched member = %+v (found %v), want it live in a new pane", status, ok)
		}
		if got := windowOfPane(t, e, status.PaneID); got != window {
			t.Errorf("relaunched member is in window %q, want the group window %q", got, window)
		}
		panes := windowPanes(window)
		if slices.Contains(panes, dead.PaneID) || !slices.Contains(panes, partner.PaneID) || !slices.Contains(panes, status.PaneID) || len(panes) != 2 {
			t.Errorf("group window panes = %v, want the old dead pane %s gone and only %s and the relaunched pane %s", panes, dead.PaneID, partner.PaneID, status.PaneID)
		}
	})

	t.Run("RelaunchRejoinsAWindowKeptByItsDeadPaneAndTheLastRemovalEndsIt", func(t *testing.T) {
		dead := addGrouped("sa", "relaunch-alone", 2, "exit", "sleep 300")
		partner := addGrouped("sb", "relaunch-alone", 2, "sleep 300", "")
		waitUntil(t, 10*time.Second, "the first group member never died", func() bool { return paneDead(dead.PaneID) })
		window := windowOfPane(t, e, dead.PaneID)

		if _, err := e.RemoveStrand(partner.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(partner): %v", err)
		}
		if !windowListed(window) {
			t.Fatalf("group window %s ended with its partner, want it kept by the dead pane that is still bound", window)
		}
		if _, err := e.Resume(); err != nil {
			t.Fatalf("Resume: %v", err)
		}
		status, ok := statusOf(dead.GUID)
		if !ok || !status.Live || windowOfPane(t, e, status.PaneID) != window {
			t.Fatalf("Status of the relaunched member = %+v (found %v), want it live in the kept window %s", status, ok, window)
		}

		if _, err := e.RemoveStrand(dead.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(relaunched): %v", err)
		}
		if windowListed(window) {
			t.Errorf("group window %s is still listed after its last strand was removed, want it ended", window)
		}
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

	addOwn := func(role string) Strand {
		t.Helper()
		s, err := e.AddStrand(AddSpec{Role: role, Cmd: "sleep 300", Display: ownDisplay})
		if err != nil {
			t.Fatalf("AddStrand(%s): %v", role, err)
		}
		return s
	}

	// The move off an ending window neither reads nor changes the follow state, so it runs with follow on and with follow off.
	t.Run("RemoveMovesAViewerOnTheEndingWindowToTheNewestLiveAgent", func(t *testing.T) {
		t.Cleanup(func() { setFollow("off") })
		for _, follow := range []string{"on", "off"} {
			setFollow(follow)
			older := addOwn("older")
			viewed := addOwn("viewed")
			if err := e.tmux.run("select-window", "-t", windowOfPane(t, e, viewed.PaneID)); err != nil {
				t.Fatalf("select-window: %v", err)
			}

			if _, err := e.RemoveStrand(viewed.GUID, false); err != nil {
				t.Fatalf("RemoveStrand: %v", err)
			}

			want := windowOfPane(t, e, older.PaneID)
			if got := currentWindow(); got != want {
				t.Errorf("follow %s: current window = %q after removing the viewed strand, want the older live agent's window %q and never the strands' window %q", follow, got, want, strandWindow())
			}
			if got := readFollow(); got != follow {
				t.Errorf("follow state = %q after the removal, want %q unchanged", got, follow)
			}
			if _, err := e.RemoveStrand(older.GUID, false); err != nil {
				t.Fatalf("RemoveStrand(older): %v", err)
			}
		}
	})

	t.Run("RemoveFromAnotherWindowLeavesTheViewer", func(t *testing.T) {
		kept := addOwn("kept")
		gone := addOwn("gone")
		keptWindow := windowOfPane(t, e, kept.PaneID)
		if err := e.tmux.run("select-window", "-t", keptWindow); err != nil {
			t.Fatalf("select-window: %v", err)
		}

		if _, err := e.RemoveStrand(gone.GUID, false); err != nil {
			t.Fatalf("RemoveStrand: %v", err)
		}

		if got := currentWindow(); got != keptWindow {
			t.Errorf("current window = %q after removing another window's strand, want the viewer left on %q", got, keptWindow)
		}
		if _, err := e.RemoveStrand(kept.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(kept): %v", err)
		}
	})

	t.Run("LeavingASharedWindowRelaysItOut", func(t *testing.T) {
		first := addGrouped("lva", "leaving", 2, "sleep 300", "")
		second := addGrouped("lvb", "leaving", 2, "sleep 300", "")
		window := windowOfPane(t, e, first.PaneID)

		if _, err := e.RemoveStrand(second.GUID, false); err != nil {
			t.Fatalf("RemoveStrand: %v", err)
		}

		if panes := windowPanes(window); len(panes) != 1 {
			t.Fatalf("shared window panes = %v after one strand left, want one", panes)
		}
		if width, windowWidth := display(first.PaneID, "#{pane_width}"), display(window, "#{window_width}"); width != windowWidth {
			t.Errorf("remaining pane width = %s, want the window's full width %s", width, windowWidth)
		}
	})

	t.Run("InvalidConfigStillRemovesAndReportsTheKey", func(t *testing.T) {
		restoreConfig(t)
		first := addGrouped("iva", "invalid-split", 2, "sleep 300", "")
		second := addGrouped("ivb", "invalid-split", 2, "sleep 300", "")
		e.cfg.WindowSplit = "diagonal"

		removed, err := e.RemoveStrand(second.GUID, false)
		if err != nil {
			t.Fatalf("RemoveStrand under an invalid window_split = %v, want nil", err)
		}
		if !strings.Contains(removed.ConfigWarning, "window_split") {
			t.Errorf("ConfigWarning = %q, want it to name window_split", removed.ConfigWarning)
		}
		panes, err := e.tmux.output("list-panes", "-s", "-t", exactSessionTarget(e.SessionName()), "-F", "#{pane_id}")
		if err != nil {
			t.Fatalf("list panes: %v", err)
		}
		if slices.Contains(strings.Fields(panes), second.PaneID) {
			t.Errorf("pane %s survived its strand's removal", second.PaneID)
		}

		e.cfg.WindowSplit = "width"
		e.cfg.WindowShareMax = -1
		window := windowOfPane(t, e, first.PaneID)
		removed, err = e.RemoveStrand(first.GUID, false)
		if err != nil {
			t.Fatalf("RemoveStrand under an invalid window_share_max = %v, want nil", err)
		}
		if !strings.Contains(removed.ConfigWarning, "window_share_max") {
			t.Errorf("ConfigWarning = %q, want it to name window_share_max", removed.ConfigWarning)
		}
		if windowListed(window) {
			t.Errorf("window %s is still listed after its last strand was removed, want it ended", window)
		}
	})

	// This subtest tears the session down, so it runs last.
	t.Run("SessionTeardownEndsTheShellBeforeTheChainRuns", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "chain-ran")
		if _, err := e.AddStrand(AddSpec{Role: "chain", Cmd: "sleep 300; touch '" + marker + "'", Display: ownDisplay}); err != nil {
			t.Fatalf("AddStrand(chain): %v", err)
		}

		if _, err := e.Down(); err != nil {
			t.Fatalf("Down: %v", err)
		}

		if _, err := os.Stat(marker); err == nil {
			t.Errorf("marker %s exists after Down, want the pane's shell ended before its command chain ran on", marker)
		}
	})
}
