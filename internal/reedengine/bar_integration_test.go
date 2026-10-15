//go:build tmux && linux

// bar_integration_test.go expands the two-line status bar and the pane-border title against a real tmux server and an attached client,
// and asserts the acceptance rows: which button is lit, which strands and windows get a button, the wait mark, and the session line.
// The formats are expanded with display-message -c <client>, so the client's own session and current window are the context.
// The client is attached through the pty harness attachgeometry_integration_test.go defines.

package reedengine

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// barFixture is a session with Selvage, two strand panes (alpha in the review segment, beta in plan), a second window, a window carrying @lyx_hidden and a second session, with a client attached.
type barFixture struct {
	e                         *Engine
	client                    string
	alpha, beta               Strand
	selvagePane               string
	strandWindow, otherWindow string
}

func newBarFixture(t *testing.T) *barFixture {
	t.Helper()
	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	below := render.Display{Anchor: render.AnchorBelowParent}
	alpha, err := e.AddStrand(AddSpec{NameOverride: "alpha", Segment: "review", Cmd: "sleep 300", Display: below})
	if err != nil {
		t.Fatalf("AddStrand(alpha): %v", err)
	}
	beta, err := e.AddStrand(AddSpec{NameOverride: "beta", Segment: "plan", Cmd: "sleep 300", Display: below})
	if err != nil {
		t.Fatalf("AddStrand(beta): %v", err)
	}
	if err := e.tmux.run("new-window", "-d", "-t", exactSessionWindowTarget(e.SessionName()), "-n", "extra"); err != nil {
		t.Fatalf("new-window: %v", err)
	}
	if err := e.tmux.run("new-window", "-d", "-t", exactSessionWindowTarget(e.SessionName()), "-n", "hidden"); err != nil {
		t.Fatalf("new-window hidden: %v", err)
	}
	if err := e.tmux.run("set-option", "-w", "-t", exactSessionWindowTarget(e.SessionName())+"=hidden", "@lyx_hidden", "1"); err != nil {
		t.Fatalf("set-option @lyx_hidden: %v", err)
	}
	if err := e.tmux.run("new-session", "-d", "-s", "a-very-long-session-name"); err != nil {
		t.Fatalf("new-session: %v", err)
	}

	argv := e.AttachArgv(120, 30)
	startInPTY(t, append([]string{e.cfg.Tmux}, argv...), 120, 30)
	waitForClientAttached(t, e, 15*time.Second)

	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v)", st, err)
	}
	strandWindow, err := e.strandWindowTargetFor(st)
	if err != nil {
		t.Fatalf("strandWindowTargetFor: %v", err)
	}
	f := &barFixture{e: e, alpha: alpha, beta: beta, selvagePane: st.SelvagePaneID, strandWindow: strandWindow, otherWindow: exactSessionWindowTarget(e.SessionName()) + "extra"}
	f.client = strings.TrimSpace(mustTmuxOutput(t, e, "list-clients", "-F", "#{client_name}"))
	return f
}

// expand returns format expanded for the attached client; callers pass T:status-format so the strftime parts are filled in as the status line does.
func (f *barFixture) expand(t *testing.T, format string) string {
	t.Helper()
	return strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-c", f.client, "-p", format))
}

func (f *barFixture) paneBorder(t *testing.T, paneID string) string {
	t.Helper()
	return strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "-t", paneID, "#{E:pane-border-format}"))
}

func TestBar_LineOneButtonsFollowTheWindowZoomAndWaitMarks(t *testing.T) {
	f := newBarFixture(t)
	e := f.e
	if err := e.tmux.run("select-window", "-t", f.strandWindow); err != nil {
		t.Fatalf("select-window: %v", err)
	}

	shellLit := "range=user|shell list=focus bg=white fg=black bold] shell "
	shellUnlit := "range=user|shell bg=colour236 fg=white] shell "
	alphaLit := "range=pane|" + f.alpha.PaneID + " list=focus bg=colour208 fg=black bold] alpha "
	alphaUnlit := "range=pane|" + f.alpha.PaneID + " bg=colour236 fg=colour208] alpha "
	betaUnlit := "range=pane|" + f.beta.PaneID + " bg=colour236 fg=cyan] beta "

	// The strand window is current and unzoomed: the shell button is lit and both strands are unlit buttons, in pane order, with no Selvage button.
	line := f.expand(t, "#{T:status-format[0]}")
	for _, want := range []string{shellLit, alphaUnlit, betaUnlit, "range=window|", " extra "} {
		if !strings.Contains(line, want) {
			t.Errorf("line 1 with the strand window current = %q, want it to contain %q", line, want)
		}
	}
	if strings.Contains(line, " VIEW ") {
		t.Errorf("line 1 = %q, want no VIEW button", line)
	}
	if strings.Index(line, " extra ") > strings.Index(line, shellLit) {
		t.Errorf("line 1 = %q, want the shell button after the extra window's button, the last visible window", line)
	}
	if strings.Index(line, alphaUnlit) > strings.Index(line, betaUnlit) {
		t.Errorf("line 1 = %q, want alpha's button before beta's (pane order)", line)
	}
	if strings.Contains(line, "range=pane|"+f.selvagePane) {
		t.Errorf("line 1 = %q, want no button for the Selvage pane", line)
	}
	if !strings.Contains(line, "range=window|1 bg=colour236 fg=white] extra ") {
		t.Errorf("line 1 = %q, want the non-current window button unlit", line)
	}
	if strings.Contains(line, " hidden ") {
		t.Errorf("line 1 = %q, want no button for the window carrying @lyx_hidden", line)
	}

	// Zooming alpha unlights the shell button and lights exactly alpha's button.
	if err := e.tmux.run("select-pane", "-t", f.alpha.PaneID); err != nil {
		t.Fatalf("select-pane: %v", err)
	}
	if err := e.tmux.run("resize-pane", "-Z", "-t", f.alpha.PaneID); err != nil {
		t.Fatalf("resize-pane -Z: %v", err)
	}
	line = f.expand(t, "#{T:status-format[0]}")
	for _, want := range []string{shellUnlit, alphaLit, betaUnlit} {
		if !strings.Contains(line, want) {
			t.Errorf("line 1 with alpha zoomed = %q, want it to contain %q", line, want)
		}
	}
	if err := e.tmux.run("resize-pane", "-Z", "-t", f.alpha.PaneID); err != nil {
		t.Fatalf("unzoom: %v", err)
	}

	// From the second window the strand buttons are still there, the shell button is unlit and the window's own button is lit.
	if err := e.tmux.run("select-window", "-t", f.otherWindow); err != nil {
		t.Fatalf("select-window extra: %v", err)
	}
	line = f.expand(t, "#{T:status-format[0]}")
	for _, want := range []string{shellUnlit, alphaUnlit, betaUnlit, "range=window|1 list=focus bg=white fg=black bold] extra "} {
		if !strings.Contains(line, want) {
			t.Errorf("line 1 from the second window = %q, want it to contain %q", line, want)
		}
	}

	// A wait mark shows on its strand's button until cleared.
	if err := e.SetWaitMark(f.alpha.GUID, "verify loom", time.Now().Add(-3*time.Minute)); err != nil {
		t.Fatalf("SetWaitMark: %v", err)
	}
	if line = f.expand(t, "#{T:status-format[0]}"); !regexp.MustCompile(`alpha ⏳verify loom [23]m `).MatchString(line) {
		t.Errorf("line 1 with a mark = %q, want alpha's button to carry the label and a two-to-three minute count", line)
	}
	if err := e.SetWaitMark(f.alpha.GUID, "", time.Time{}); err != nil {
		t.Fatalf("SetWaitMark (clear): %v", err)
	}
	if line = f.expand(t, "#{T:status-format[0]}"); strings.Contains(line, "⏳") {
		t.Errorf("line 1 after the mark was cleared = %q, want no wait text", line)
	}

	// An own window gets a button in its segment's color, underscored until the operator first selects it.
	// Follow is off first, since a session that follows the run would select the new window and leave it with no unseen mark.
	if err := e.tmux.run("set-option", "-t", exactSessionWindowTarget(e.SessionName()), followSessionOption, "off"); err != nil {
		t.Fatalf("set %s off: %v", followSessionOption, err)
	}
	scout, err := e.AddStrand(AddSpec{NameOverride: "scout", Segment: "review", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand(scout): %v", err)
	}
	scoutWindow := strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-t", scout.PaneID, "#{window_id}"))
	scoutIndex := strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-t", scout.PaneID, "#{window_index}"))
	scoutUnseen := "range=window|" + scoutIndex + " bg=colour236 fg=colour208 underscore] scout"
	scoutUnlit := "range=window|" + scoutIndex + " bg=colour236 fg=colour208] scout"
	scoutLit := "range=window|" + scoutIndex + " list=focus bg=colour208 fg=black bold] scout"
	if line = f.expand(t, "#{T:status-format[0]}"); !strings.Contains(line, scoutUnseen) {
		t.Errorf("line 1 with a new own window = %q, want it to contain the unseen button %q", line, scoutUnseen)
	}
	if strings.Contains(line, " hidden ") {
		t.Errorf("line 1 with an own window = %q, want still no button for the window carrying @lyx_hidden", line)
	}
	if strings.Index(line, "range=user|shell") < strings.Index(line, scoutUnseen) || strings.Index(line, "range=user|shell") < strings.Index(line, " extra ") {
		t.Errorf("line 1 with an own window = %q, want the shell button after every own-window button and the extra window's, the last visible window", line)
	}

	if err := e.SetWaitMark(scout.GUID, "verify loom", time.Now().Add(-3*time.Minute)); err != nil {
		t.Fatalf("SetWaitMark(scout): %v", err)
	}
	if line = f.expand(t, "#{T:status-format[0]}"); !regexp.MustCompile(`scout ⏳verify loom [23]m `).MatchString(line) {
		t.Errorf("line 1 with a mark on the own window's strand = %q, want its button to carry the label and a two-to-three minute count", line)
	}
	if err := e.SetWaitMark(scout.GUID, "", time.Time{}); err != nil {
		t.Fatalf("SetWaitMark(scout, clear): %v", err)
	}

	if err := e.tmux.run("select-window", "-t", scoutWindow); err != nil {
		t.Fatalf("select-window scout: %v", err)
	}
	if line = f.expand(t, "#{T:status-format[0]}"); !strings.Contains(line, scoutLit) {
		t.Errorf("line 1 with the own window selected = %q, want it to contain the lit button %q", line, scoutLit)
	}
	if unseen := strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-t", scoutWindow, "#{@lyx_unseen}")); unseen != "" {
		t.Errorf("own window @lyx_unseen = %q after it was selected, want it cleared", unseen)
	}

	if err := e.tmux.run("select-window", "-t", f.strandWindow); err != nil {
		t.Fatalf("select-window strands: %v", err)
	}
	if line = f.expand(t, "#{T:status-format[0]}"); !strings.Contains(line, scoutUnlit) || strings.Contains(line, scoutUnseen) {
		t.Errorf("line 1 after selecting the strands' window again = %q, want the own window's button unlit without underscore", line)
	}

	// The overview window's button is the first on line 1, in the plain unlit window style.
	status, err := e.AddStrand(AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOverview}})
	if err != nil {
		t.Fatalf("AddStrand(status): %v", err)
	}
	overviewIndex := strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-t", status.PaneID, "#{window_index}"))
	overviewButton := "range=window|" + overviewIndex + " bg=colour236 fg=white] overview"
	line = f.expand(t, "#{T:status-format[0]}")
	if first := strings.Index(line, "range="); first < 0 || !strings.HasPrefix(line[first:], overviewButton[:strings.Index(overviewButton, "]")]) {
		t.Errorf("line 1 with an overview strand = %q, want its first button to be %q", line, overviewButton)
	}
	if !strings.Contains(line, overviewButton) {
		t.Errorf("line 1 with an overview strand = %q, want the plain unlit window button %q", line, overviewButton)
	}
}

func TestBar_LineTwoListsSessionsInIDOrderAndTheBorderNamesStrands(t *testing.T) {
	f := newBarFixture(t)

	// The current session is lit with its full name and the other is cut to 12 characters, in session-id order.
	line := f.expand(t, "#{T:status-format[1]}")
	current := "list=focus bg=yellow fg=black bold] " + f.e.SessionName() + " "
	other := "bg=colour236 fg=white] a-very-long- "
	if !strings.Contains(line, current) || !strings.Contains(line, other) {
		t.Fatalf("line 2 = %q, want it to contain %q and %q", line, current, other)
	}
	if strings.Index(line, current) > strings.Index(line, other) {
		t.Errorf("line 2 = %q, want the earlier session before the later one (session-id order)", line)
	}

	if got := f.paneBorder(t, f.selvagePane); !strings.Contains(got, "selvage") || strings.Contains(got, "fg=") {
		t.Errorf("Selvage's border title = %q, want it to read selvage with no strand color", got)
	}
	got := f.paneBorder(t, f.alpha.PaneID)
	if !strings.Contains(got, "fg=colour208") || !strings.Contains(got, " alpha ") {
		t.Errorf("alpha's border title = %q, want its name in its segment color", got)
	}
	if got := f.paneBorder(t, f.beta.PaneID); !strings.Contains(got, "fg=cyan") {
		t.Errorf("beta's border title = %q, want its name in cyan", got)
	}
}
