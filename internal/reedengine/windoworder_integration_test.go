//go:build tmux

// windoworder_integration_test.go proves the window order against a real tmux.
// The session's windows hold the overview first, the agents in start order, then the window named shell, then the hidden windows; a relaunch keeps its window's place, and a session booted by an older lyx is brought into that order.
// One cold session serves every step.

package reedengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	// A booted session follows the run, which would move the viewer on every add, so the steps keep it off unless a step names the follow state.
	setFollow := func(value string) {
		t.Helper()
		if err := e.tmux.run("set-option", "-t", exactSessionWindowTarget(e.SessionName()), followSessionOption, value); err != nil {
			t.Fatalf("set %s %s: %v", followSessionOption, value, err)
		}
	}
	readFollow := func() string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{"+followSessionOption+"}")
		if err != nil {
			t.Fatalf("read %s: %v", followSessionOption, err)
		}
		return strings.TrimSpace(out)
	}
	assertFollow := func(want string) {
		t.Helper()
		if got := readFollow(); got != want {
			t.Errorf("%s = %q, want %q", followSessionOption, got, want)
		}
	}
	setFollow("off")
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

	overviewDisplay := render.Display{Anchor: render.AnchorOverview}
	currentWindow := func() string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_id}")
		if err != nil {
			t.Fatalf("read the current window: %v", err)
		}
		return strings.TrimSpace(out)
	}
	shellWindowID := func() string {
		t.Helper()
		st, err := LoadState(e.stateDir())
		if err != nil || st == nil || st.SelvagePaneID == "" {
			t.Fatalf("LoadState = (%+v, %v), want a persisted SelvagePaneID", st, err)
		}
		return windowOfPane(t, e, st.SelvagePaneID)
	}
	selectWindow := func(window string) {
		t.Helper()
		if err := e.tmux.run("select-window", "-t", window); err != nil {
			t.Fatalf("select-window %s: %v", window, err)
		}
	}
	agentNames := func() []string {
		t.Helper()
		var out []string
		for _, w := range windows() {
			if w.Name != overviewWindowName && w.Name != shellWindowName && !w.Hidden {
				out = append(out, w.Name)
			}
		}
		return out
	}

	var overview Strand
	t.Run("AnOverviewAddedAfterAgentWindowsHoldsTheLowestIndexAndCarriesNoMarks", func(t *testing.T) {
		agents := agentNames()
		viewed := currentWindow()
		var err error
		overview, err = e.AddStrand(AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: overviewDisplay})
		if err != nil {
			t.Fatalf("AddStrand(overview): %v", err)
		}
		assertFollow("off")
		if got := currentWindow(); got != viewed {
			t.Errorf("current window = %s after the overview add, want the viewer left on %s", got, viewed)
		}
		listed := windows()
		if listed[0].Name != overviewWindowName {
			t.Fatalf("first window is %q, want %q", listed[0].Name, overviewWindowName)
		}
		if got := agentNames(); !slices.Equal(got, agents) {
			t.Errorf("agent windows = %v, want %v unchanged and in start order", got, agents)
		}
		marks, err := e.tmux.output("display-message", "-p", "-t", windowOfPane(t, e, overview.PaneID), "[#{@lyx_own}][#{@lyx_unseen}][#{@strand_color}][#{@lyx_overview}]")
		if err != nil {
			t.Fatalf("read the overview window marks: %v", err)
		}
		if got := strings.TrimSpace(marks); got != "[1][][][1]" {
			t.Errorf("overview window marks = %q, want the own and overview markers alone", got)
		}
		for _, w := range listed {
			if w.Name == overviewWindowName || w.Name == shellWindowName || w.Hidden {
				continue
			}
			agentMark, err := e.tmux.output("display-message", "-p", "-t", w.ID, "#{@lyx_overview}")
			if err != nil {
				t.Fatalf("read the overview marker of window %s: %v", w.Name, err)
			}
			if got := strings.TrimSpace(agentMark); got != "" {
				t.Errorf("agent window %s @lyx_overview = %q, want it unset", w.Name, got)
			}
		}
	})

	t.Run("ReplacingAnUnviewedStrandWithAnOverviewOpensItFirstAndLeavesTheShellWindowSelected", func(t *testing.T) {
		if _, err := e.RemoveStrand(overview.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(overview): %v", err)
		}
		top, err := e.AddStrand(AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}})
		if err != nil {
			t.Fatalf("AddStrand(status): %v", err)
		}
		shellWindow := shellWindowID()
		selectWindow(shellWindow)

		overview, err = e.ReplaceStrand(top.GUID, AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: overviewDisplay})
		if err != nil {
			t.Fatalf("ReplaceStrand(strand -> overview): %v", err)
		}
		if first := windows()[0]; first.Name != overviewWindowName {
			t.Errorf("first window is %q, want %q", first.Name, overviewWindowName)
		}
		panes, err := e.tmux.listWindowPanes(e.SessionName())
		if err != nil {
			t.Fatalf("list panes: %v", err)
		}
		for _, p := range panes {
			if p.PaneID == top.PaneID {
				t.Errorf("the replaced strand's pane %s is still listed", top.PaneID)
			}
		}
		if got := currentWindow(); got != shellWindow {
			t.Errorf("current window = %s, want the shell window %s", got, shellWindow)
		}
	})

	t.Run("ReplacingTheViewedOverviewSelectsTheNewOverviewWindow", func(t *testing.T) {
		selectWindow(windowOfPane(t, e, overview.PaneID))
		var err error
		overview, err = e.ReplaceStrand(overview.GUID, AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: overviewDisplay})
		if err != nil {
			t.Fatalf("ReplaceStrand(overview): %v", err)
		}
		if got, want := currentWindow(), windowOfPane(t, e, overview.PaneID); got != want {
			t.Errorf("current window = %s, want the new overview window %s", got, want)
		}
		assertFollow("off")
	})

	t.Run("ReplacingTheOverviewKeepsOverviewAgentsInStartOrderThenShell", func(t *testing.T) {
		agents := agentNames()
		var err error
		overview, err = e.ReplaceStrand(overview.GUID, AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: overviewDisplay})
		if err != nil {
			t.Fatalf("ReplaceStrand(overview): %v", err)
		}
		want := append(append([]string{overviewWindowName}, agents...), shellWindowName)
		for _, w := range windows() {
			if w.Hidden {
				want = append(want, w.Name)
			}
		}
		wantNames(want...)
	})

	unseen := func(window string) string {
		t.Helper()
		out, err := e.tmux.output("display-message", "-p", "-t", window, "#{@lyx_unseen}")
		if err != nil {
			t.Fatalf("read @lyx_unseen of %s: %v", window, err)
		}
		return strings.TrimSpace(out)
	}
	addFocused := func(name string, focus bool) Strand {
		t.Helper()
		s, err := e.AddStrand(AddSpec{NameOverride: name, Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow, Focus: focus}})
		if err != nil {
			t.Fatalf("AddStrand(%s): %v", name, err)
		}
		return s
	}

	t.Run("WithFollowOnANewAgentWindowIsSelectedFromTheShellAndFromAnOlderLiveAgentWindow", func(t *testing.T) {
		setFollow("on")
		t.Cleanup(func() { setFollow("off") })

		selectWindow(shellWindowID())
		fromShell := addFocused("on-from-shell", false)
		fromShellWindow := windowOfPane(t, e, fromShell.PaneID)
		if got := currentWindow(); got != fromShellWindow {
			t.Errorf("current window = %s, want the new agent window %s selected from the shell", got, fromShellWindow)
		}
		if got := unseen(fromShellWindow); got != "" {
			t.Errorf("selected agent window @lyx_unseen = %q, want it unset", got)
		}

		addFocused("on-newer", false)
		selectWindow(fromShellWindow)
		fromOlder := addFocused("on-from-older", false)
		fromOlderWindow := windowOfPane(t, e, fromOlder.PaneID)
		if got := currentWindow(); got != fromOlderWindow {
			t.Errorf("current window = %s, want the new agent window %s selected from an older live agent window", got, fromOlderWindow)
		}
		if got := unseen(fromOlderWindow); got != "" {
			t.Errorf("selected agent window @lyx_unseen = %q, want it unset", got)
		}
		assertFollow("on")
	})

	t.Run("WithFollowOffANewAgentWindowLeavesTheViewerOnTheShellAnOlderLiveAgentWindowAndTheOverviewAndCarriesUnseen", func(t *testing.T) {
		setFollow("off")
		older := addFocused("off-older", false)
		viewers := map[string]string{
			"the shell":                  shellWindowID(),
			"an older live agent window": windowOfPane(t, e, older.PaneID),
			"the overview":               windowOfPane(t, e, overview.PaneID),
		}
		for name, viewer := range viewers {
			selectWindow(viewer)
			added := addFocused("off-from-"+strings.ReplaceAll(name, " ", "-"), false)
			if got := currentWindow(); got != viewer {
				t.Errorf("current window = %s, want the viewer kept on %s (%s)", got, name, viewer)
			}
			if got := unseen(windowOfPane(t, e, added.PaneID)); got != "1" {
				t.Errorf("unselected agent window @lyx_unseen = %q, want 1", got)
			}
			assertFollow("off")
		}
	})

	t.Run("AFocusStrandIsSelectedFromTheShellWithFollowOff", func(t *testing.T) {
		setFollow("off")
		selectWindow(shellWindowID())
		focused := addFocused("focused", true)
		if got, want := currentWindow(), windowOfPane(t, e, focused.PaneID); got != want {
			t.Errorf("current window = %s, want the focus strand's window %s", got, want)
		}
		assertFollow("off")
	})

	t.Run("AKeepViewerStrandLeavesAViewerOnTheShellWithFollowOn", func(t *testing.T) {
		setFollow("on")
		t.Cleanup(func() { setFollow("off") })

		shellWindow := shellWindowID()
		selectWindow(shellWindow)
		kept, err := e.AddStrand(AddSpec{NameOverride: "kept", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow, KeepViewer: true}})
		if err != nil {
			t.Fatalf("AddStrand(kept): %v", err)
		}
		if got := currentWindow(); got != shellWindow {
			t.Errorf("current window = %s, want the viewer kept on the shell window %s", got, shellWindow)
		}
		if got := unseen(windowOfPane(t, e, kept.PaneID)); got != "1" {
			t.Errorf("unselected agent window @lyx_unseen = %q, want 1", got)
		}
		assertFollow("on")
	})

	t.Run("ReplacingTheViewedAgentLandsTheViewerOnTheReplacement", func(t *testing.T) {
		// With follow off the replacement's new window would not take the viewer from the older agent it fell back to.
		setFollow("on")
		t.Cleanup(func() { setFollow("off") })
		addFocused("replace-older", false)
		viewed := addFocused("replace-viewed", false)
		selectWindow(windowOfPane(t, e, viewed.PaneID))

		replacement, err := e.ReplaceStrand(viewed.GUID, AddSpec{NameOverride: "replace-viewed", Cmd: "sleep 300", Display: ownDisplay})
		if err != nil {
			t.Fatalf("ReplaceStrand: %v", err)
		}

		if got, want := currentWindow(), windowOfPane(t, e, replacement.PaneID); got != want {
			t.Errorf("current window = %s, want the replacement's window %s, reached through the older agent it fell back to", got, want)
		}
	})

	t.Run("ReconcileEndingAViewedOrphanOwnWindowSelectsTheNewestLiveAgent", func(t *testing.T) {
		st, err := LoadState(e.stateDir())
		if err != nil || st == nil {
			t.Fatalf("LoadState = (%+v, %v)", st, err)
		}
		var newest sessionWindow
		live := map[string]bool{}
		for _, s := range st.Strands {
			if s.Display.Anchor == render.AnchorOwnWindow && s.PaneID != "" && !paneDead(s.PaneID) {
				live[windowOfPane(t, e, s.PaneID)] = true
			}
		}
		for _, w := range windows() {
			if live[w.ID] && w.Index > newest.Index {
				newest = w
			}
		}
		orphan := strings.TrimSpace(mustTmuxOutput(t, e, "new-window", "-d", "-P", "-F", "#{window_id}", "-t", exactSessionWindowTarget(e.SessionName())))
		if err := e.tmux.run("set-option", "-w", "-t", orphan, ownWindowOption, "1"); err != nil {
			t.Fatalf("mark the orphan window: %v", err)
		}
		selectWindow(orphan)

		if _, err := e.AddStrand(AddSpec{NameOverride: "reconcile-trigger", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}}); err != nil {
			t.Fatalf("AddStrand(reconcile-trigger): %v", err)
		}

		if got := currentWindow(); got != newest.ID {
			t.Errorf("current window = %s after reconcile ended the viewed orphan, want the newest live agent window %s", got, newest.ID)
		}
	})

	t.Run("OneSeatLeavingAViewedSharedWindowOfTwoSwitchesNothing", func(t *testing.T) {
		group := render.Display{Anchor: render.AnchorOwnWindow, Window: "duo", WindowSize: 2}
		first, err := e.AddStrand(AddSpec{NameOverride: "duo-a", Cmd: "sleep 300", Display: group})
		if err != nil {
			t.Fatalf("AddStrand(duo-a): %v", err)
		}
		second, err := e.AddStrand(AddSpec{NameOverride: "duo-b", Cmd: "sleep 300", Display: group})
		if err != nil {
			t.Fatalf("AddStrand(duo-b): %v", err)
		}
		shared := windowOfPane(t, e, first.PaneID)
		selectWindow(shared)

		if _, err := e.RemoveStrand(second.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(duo-b): %v", err)
		}

		if got := currentWindow(); got != shared {
			t.Errorf("current window = %s, want the viewer left on the shared window %s", got, shared)
		}
	})

	t.Run("RemovingTheViewedAgentWithNoLiveAgentLeftSelectsTheOverview", func(t *testing.T) {
		st, err := LoadState(e.stateDir())
		if err != nil || st == nil {
			t.Fatalf("LoadState = (%+v, %v)", st, err)
		}
		var agents []Strand
		for _, s := range st.Strands {
			if s.Display.Anchor == render.AnchorOwnWindow {
				agents = append(agents, s)
			}
		}
		last := agents[len(agents)-1]
		for _, s := range agents[:len(agents)-1] {
			if _, err := e.RemoveStrand(s.GUID, false); err != nil {
				t.Fatalf("RemoveStrand(%s): %v", s.Name, err)
			}
		}
		selectWindow(windowOfPane(t, e, last.PaneID))

		if _, err := e.RemoveStrand(last.GUID, false); err != nil {
			t.Fatalf("RemoveStrand(%s): %v", last.Name, err)
		}

		if got, want := currentWindow(), windowOfPane(t, e, overview.PaneID); got != want {
			t.Errorf("current window = %s, want the overview window %s", got, want)
		}
		assertFollow("off")
	})
}
