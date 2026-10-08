//go:build tmux && linux

// zoom_integration_test.go proves against a real tmux that a zoomed strand stays zoomed across reed's own steps, a window resize and a re-attach, that the overview's cells are planned correctly at the next unzoom, and that removing the zoomed strand leaves the window in the overview.

package reedengine

import (
	"strings"
	"testing"
	"time"
)

// zoomStateNow reads the strands' window's zoom flag and active pane from outside the pty.
func zoomStateNow(t *testing.T, e *Engine) (paneID string, zoomed bool) {
	t.Helper()
	out := mustTmuxOutput(t, e, "display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), zoomStateFormat)
	return parseZoomState(out)
}

// assertZoomedOn asserts the strands' window is zoomed on paneID.
func assertZoomedOn(t *testing.T, e *Engine, paneID, step string) {
	t.Helper()
	if got, zoomed := zoomStateNow(t, e); !zoomed || got != paneID {
		t.Errorf("(%s) zoomed pane = (%q, %v), want %q zoomed", step, got, zoomed, paneID)
	}
}

// waitZoomedOn waits until the strands' window is zoomed on paneID, for a step tmux runs after reed returns.
func waitZoomedOn(t *testing.T, e *Engine, paneID, step string) {
	t.Helper()
	waitUntil(t, 15*time.Second, step+": the window never came back zoomed on "+paneID, func() bool {
		got, zoomed := zoomStateNow(t, e)
		return zoomed && got == paneID
	})
}

// TestZoomBracket_ZoomedStrandSurvivesEveryReedStep zooms the bottom strand, which is not the top pane, and asserts it is still the zoomed pane after a client resize, a re-attach, an add and a resume,
// and that the window's cells hold their planned heights once it is unzoomed after the resize.
func TestZoomBracket_ZoomedStrandSurvivesEveryReedStep(t *testing.T) {
	e := setupAttachGeometryFixture(t)
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil || len(st.Strands) != 2 {
		t.Fatalf("LoadState = (%+v, %v), want a state of the collapsed parent and its child", st, err)
	}
	childPaneID := st.Strands[1].PaneID

	const cols, rows, resizedRows = 100, 30, 60
	pty := startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(cols, rows)...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)

	mustTmuxOutput(t, e, "select-pane", "-t", childPaneID)
	mustTmuxOutput(t, e, "resize-pane", "-Z", "-t", childPaneID)
	assertZoomedOn(t, e, childPaneID, "after zooming")

	resizeClientAndWait(t, e, pty, cols, resizedRows)
	waitZoomedOn(t, e, childPaneID, "after a window resize")
	mustTmuxOutput(t, e, "resize-pane", "-Z", "-t", childPaneID)
	assertPlannedCells(t, e, st, e.cfg.CollapsedRows, "unzoomed after a resize")
	mustTmuxOutput(t, e, "resize-pane", "-Z", "-t", childPaneID)

	startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(cols, resizedRows)...), cols, resizedRows)
	waitUntil(t, 15*time.Second, "the second client never attached", func() bool {
		out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()))
		return err == nil && len(strings.Split(strings.TrimSpace(out), "\n")) == 2
	})
	waitZoomedOn(t, e, childPaneID, "after a re-attach")

	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: st.Strands[1].Display}); err != nil {
		t.Fatalf("AddStrand: %v", err)
	}
	assertZoomedOn(t, e, childPaneID, "after an add")

	if _, err := e.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	assertZoomedOn(t, e, childPaneID, "after a resume")
}

// TestZoomBracket_RemovingTheZoomedStrandLeavesTheOverview pins that a zoomed pane gone after the step is not zoomed again: the window ends in the overview.
func TestZoomBracket_RemovingTheZoomedStrandLeavesTheOverview(t *testing.T) {
	e := setupAttachGeometryFixture(t)
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil || len(st.Strands) != 2 {
		t.Fatalf("LoadState = (%+v, %v), want a state of the collapsed parent and its child", st, err)
	}
	child := st.Strands[1]
	mustTmuxOutput(t, e, "select-pane", "-t", child.PaneID)
	mustTmuxOutput(t, e, "resize-pane", "-Z", "-t", child.PaneID)
	assertZoomedOn(t, e, child.PaneID, "after zooming")

	if _, err := e.RemoveStrand(child.GUID, false); err != nil {
		t.Fatalf("RemoveStrand: %v", err)
	}

	if got, zoomed := zoomStateNow(t, e); zoomed {
		t.Errorf("window still zoomed on %q after its zoomed strand was removed, want the overview", got)
	}
}
