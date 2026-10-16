//go:build tmux && linux

// bindings_integration_test.go drives the Alt-key and status-bar bindings against a real tmux server and an attached client.
// The terminal's own byte sequences are written into the client's pty (ESC [1;3A..D, SGR mouse clicks at the bar's cells), so the whole key path is exercised.
// The client is attached through the pty harness attachgeometry_integration_test.go defines.
// Every test swaps the package-level executablePath seam and so never runs in parallel.

package reedengine

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

const (
	bindingsClientCols = 120
	bindingsClientRows = 30
)

// styleDirective matches a tmux `#[...]` style directive in an expanded status line.
var styleDirective = regexp.MustCompile(`#\[[^\]]*\]`)

// bindingsFixture is a session with Selvage, two strand panes, a second window and a second session, with a mouse-enabled client attached.
// The executable the Alt+Left/Right bindings name is a recorder script under a directory containing `#`, which writes its arguments to recordPath.
type bindingsFixture struct {
	e                         *Engine
	pty                       *attachGeometryPTY
	client                    string
	alpha, beta               Strand
	selvagePane               string
	strandWindow, otherWindow string
	recordPath                string
}

func newBindingsFixture(t *testing.T) *bindingsFixture {
	t.Helper()

	scriptDir := filepath.Join(t.TempDir(), "dir#hash")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir recorder dir: %v", err)
	}
	recordPath := filepath.Join(t.TempDir(), "recorded-args")
	scriptPath := filepath.Join(scriptDir, "lyx-recorder")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + recordPath + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write recorder script: %v", err)
	}
	withInjectedExecutablePath(t, func() (string, error) { return scriptPath, nil })

	e := newIntegrationEngine(t, "on")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	// alpha is a top strand above the positional beta, the prime's runs-above-orch shape.
	topContent := render.Display{Anchor: render.AnchorTop}
	below := render.Display{Anchor: render.AnchorBelowParent}
	alpha, err := e.AddStrand(AddSpec{NameOverride: "alpha", Segment: "review", Cmd: "sleep 300", Display: topContent})
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
	if err := e.tmux.run("new-session", "-d", "-s", "other"); err != nil {
		t.Fatalf("new-session: %v", err)
	}

	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v)", st, err)
	}
	// The console marker is unset before attaching, so every bindings test passes only when the attach pre-flight re-asserts it.
	if err := e.tmux.run("set-option", "-p", "-u", "-t", st.SelvagePaneID, consolePaneOption); err != nil {
		t.Fatalf("unset the console marker: %v", err)
	}

	pty := startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(bindingsClientCols, bindingsClientRows)...), bindingsClientCols, bindingsClientRows)
	waitForClientAttached(t, e, 15*time.Second)
	strandWindow, err := e.strandWindowTargetFor(st)
	if err != nil {
		t.Fatalf("strandWindowTargetFor: %v", err)
	}
	f := &bindingsFixture{
		e: e, pty: pty, alpha: alpha, beta: beta, selvagePane: st.SelvagePaneID,
		strandWindow: strandWindow, otherWindow: exactSessionWindowTarget(e.SessionName()) + "extra",
		recordPath: recordPath,
	}
	f.client = strings.TrimSpace(mustTmuxOutput(t, e, "list-clients", "-F", "#{client_name}"))
	return f
}

// send writes raw bytes into the client's terminal.
func (f *bindingsFixture) send(t *testing.T, bytes string) {
	t.Helper()
	if _, err := f.pty.master.WriteString(bytes); err != nil {
		t.Fatalf("write %q to the client pty: %v", bytes, err)
	}
}

// display returns format expanded against target.
func (f *bindingsFixture) display(t *testing.T, target, format string) string {
	t.Helper()
	return strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "-t", target, format))
}

func (f *bindingsFixture) activePane(t *testing.T) string {
	t.Helper()
	return f.display(t, f.strandWindow, "#{pane_id}")
}

func (f *bindingsFixture) zoomed(t *testing.T) string {
	t.Helper()
	return f.display(t, f.strandWindow, "#{window_zoomed_flag}")
}

// currentWindow returns the name of the window the client shows.
func (f *bindingsFixture) currentWindow(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "-c", f.client, "#{window_name}"))
}

// settle gives tmux time to process a key that should have changed nothing.
func settle() { time.Sleep(doubleClickGap) }

// setFollow writes @lyx_follow on the session named session.
func (f *bindingsFixture) setFollow(t *testing.T, session, value string) {
	t.Helper()
	if err := f.e.tmux.run("set-option", "-t", "="+session+":", followSessionOption, value); err != nil {
		t.Fatalf("set %s %s on %s: %v", followSessionOption, value, session, err)
	}
}

// follow returns @lyx_follow of the session named session, empty when unset.
func (f *bindingsFixture) follow(t *testing.T, session string) string {
	t.Helper()
	return f.display(t, "="+session+":", "#{"+followSessionOption+"}")
}

// assertFollow fails unless @lyx_follow of the fixture's own session reads want.
func (f *bindingsFixture) assertFollow(t *testing.T, want string) {
	t.Helper()
	if got := f.follow(t, f.e.SessionName()); got != want {
		t.Errorf("%s = %q, want %q", followSessionOption, got, want)
	}
}

// awaitRecord waits for the recorder lyx to write its eight arguments of a follow jump and returns them.
func (f *bindingsFixture) awaitRecord(t *testing.T) []string {
	t.Helper()
	var args []string
	waitUntil(t, 5*time.Second, "the recorder never finished writing", func() bool {
		raw, err := os.ReadFile(f.recordPath)
		args = strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		return err == nil && len(args) == 8
	})
	return args
}

// removeRecord drops a stale recorder file.
func (f *bindingsFixture) removeRecord(t *testing.T) {
	t.Helper()
	if err := os.Remove(f.recordPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove stale record: %v", err)
	}
}

// wantFollowRecord returns the arguments the recorder lyx writes for a follow jump of session on the told socket, with the tmux path checked apart.
func wantFollowRecord(socketPath, session string) []string {
	return []string{"reed", "follow", "--socket", socketPath, "--session", session, "--tmux"}
}

// selectStrandsWindow makes the strands' window current with pane the active one, unzoomed.
func (f *bindingsFixture) selectStrandsWindow(t *testing.T, pane string) {
	t.Helper()
	if err := f.e.tmux.run("select-window", "-t", f.strandWindow); err != nil {
		t.Fatalf("select-window: %v", err)
	}
	if err := f.e.tmux.run("select-pane", "-t", pane); err != nil {
		t.Fatalf("select-pane: %v", err)
	}
	if f.zoomed(t) == "1" {
		if err := f.e.tmux.run("resize-pane", "-Z", "-t", pane); err != nil {
			t.Fatalf("unzoom: %v", err)
		}
	}
}

// doubleClickGap is longer than tmux's own double-click window, so two clicks of a test never merge into a DoubleClick1Status event the bindings do not take.
const doubleClickGap = 400 * time.Millisecond

// click presses and releases the left mouse button on the status line row (0 or 1) at the first cell of label in that line's text.
func (f *bindingsFixture) click(t *testing.T, row int, label string) {
	t.Helper()
	time.Sleep(doubleClickGap)
	line := strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-c", f.client, "-p", "#{T:status-format["+strconv.Itoa(row)+"]}"))
	text := styleDirective.ReplaceAllString(line, "")
	at := strings.Index(text, label)
	if at < 0 {
		t.Fatalf("status line %d reads %q, want it to contain %q", row, text, label)
	}
	x := len([]rune(text[:at])) + 2
	y := bindingsClientRows - 1 + row
	f.send(t, "\x1b[<0;"+strconv.Itoa(x)+";"+strconv.Itoa(y)+"M\x1b[<0;"+strconv.Itoa(x)+";"+strconv.Itoa(y)+"m")
}

func TestBindings_AltUpDownStepWindowsInIndexOrderPastHiddenOnes(t *testing.T) {
	f := newBindingsFixture(t)
	if _, err := f.e.AddStrand(AddSpec{NameOverride: "agent", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}}); err != nil {
		t.Fatalf("AddStrand(agent): %v", err)
	}
	overview, err := f.e.AddStrand(AddSpec{NameOverride: "status", Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOverview}})
	if err != nil {
		t.Fatalf("AddStrand(overview): %v", err)
	}
	// The recorder lyx exits at once, which would close an OpenWindow window, so each hidden window is a plain detached one marked by hand.
	for _, name := range []string{"hidden-one", "hidden-two"} {
		windows, err := f.e.tmux.listSessionWindows(f.e.SessionName())
		if err != nil {
			t.Fatalf("list windows: %v", err)
		}
		last, _ := highestWindow(windows, false)
		id := strings.TrimSpace(mustTmuxOutput(t, f.e, "new-window", "-d", "-a", "-t", last.ID, "-n", name, "-P", "-F", "#{window_id}"))
		if err := f.e.tmux.run("set-option", "-w", "-t", id, "@lyx_hidden", "1"); err != nil {
			t.Fatalf("mark %s hidden: %v", name, err)
		}
	}
	if err := f.e.tmux.run("select-window", "-t", windowOfPane(t, f.e, overview.PaneID)); err != nil {
		t.Fatalf("select-window overview: %v", err)
	}

	f.setFollow(t, f.e.SessionName(), "on")

	for i, step := range []struct {
		key  string
		want string
	}{
		{"\x1b[1;3B", "extra"},
		{"\x1b[1;3B", "agent"},
		{"\x1b[1;3B", "shell"},
		{"\x1b[1;3B", "overview"},
		{"\x1b[1;3A", "shell"},
		{"\x1b[1;3A", "agent"},
		{"\x1b[1;3A", "extra"},
		{"\x1b[1;3A", "overview"},
		{"\x1b[1;3A", "shell"},
	} {
		f.send(t, step.key)
		waitUntil(t, 5*time.Second, "the Alt step never reached the "+step.want+" window", func() bool { return f.currentWindow(t) == step.want })
		if i == 0 {
			f.assertFollow(t, "off")
		}
	}
}

func TestBindings_StatusBarClicksSelectAndZoomByRangeType(t *testing.T) {
	f := newBindingsFixture(t)
	session := f.e.SessionName()
	socketPath := strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "#{socket_path}"))
	// The shell and window clicks write follow off, so each starts from follow on.
	clickFollowingOn := func(row int, label string) {
		t.Helper()
		f.setFollow(t, session, "on")
		f.click(t, row, label)
	}

	// The shell button selects the strands' window and leaves its zoom as it is.
	f.selectStrandsWindow(t, f.alpha.PaneID)
	if err := f.e.tmux.run("resize-pane", "-Z", "-t", f.alpha.PaneID); err != nil {
		t.Fatalf("zoom alpha: %v", err)
	}
	clickFollowingOn(0, " shell ")
	settle()
	if got := f.zoomed(t); got != "1" {
		t.Errorf("a click on shell left zoom flag %q, want 1 (never unzoomed)", got)
	}
	f.assertFollow(t, "off")
	if err := f.e.tmux.run("resize-pane", "-Z", "-t", f.alpha.PaneID); err != nil {
		t.Fatalf("unzoom alpha: %v", err)
	}

	// A window button selects that window, and a click on the current window's own button writes follow off too.
	clickFollowingOn(0, " extra ")
	waitUntil(t, 5*time.Second, "a click on the extra window's button never selected it", func() bool { return f.currentWindow(t) == "extra" })
	f.assertFollow(t, "off")
	clickFollowingOn(0, " extra ")
	settle()
	if got := f.currentWindow(t); got != "extra" {
		t.Errorf("a click on the current window's button left window %q current, want extra", got)
	}
	f.assertFollow(t, "off")

	// The shell button from the second window returns to the strands' window.
	clickFollowingOn(0, " shell ")
	waitUntil(t, 5*time.Second, "a click on shell from the second window never returned to the strands' window", func() bool {
		return f.currentWindow(t) != "extra"
	})
	f.assertFollow(t, "off")

	// With @lyx_strands unset on every window, the shell click's command leaves the current window where it is.
	if err := f.e.tmux.run("select-window", "-t", f.otherWindow); err != nil {
		t.Fatalf("select-window extra: %v", err)
	}
	if err := f.e.tmux.run("set-option", "-w", "-u", "-t", f.strandWindow, "@lyx_strands"); err != nil {
		t.Fatalf("unset @lyx_strands: %v", err)
	}
	if err := f.e.tmux.run("if-shell", "-F", "1", shellClickCommand); err != nil {
		t.Fatalf("run the shell click command: %v", err)
	}
	settle()
	if got := f.currentWindow(t); got != "extra" {
		t.Errorf("the shell click with no strands' window left window %q current, want extra", got)
	}

	// The follow button on line 2 toggles the state: a click from on writes off, keeps the window and runs no recorder.
	// A click from off writes on and runs the jump line with the told socket, this session and the tmux path.
	f.setFollow(t, session, "on")
	f.removeRecord(t)
	windowBefore := f.currentWindow(t)
	f.click(t, 1, " follow ")
	waitUntil(t, 5*time.Second, "a click on the lit follow button never turned follow off", func() bool { return f.follow(t, session) == "off" })
	settle()
	if got := f.currentWindow(t); got != windowBefore {
		t.Errorf("a click on the lit follow button left window %q current, want %q", got, windowBefore)
	}
	if _, err := os.Stat(f.recordPath); err == nil {
		t.Errorf("a click on the lit follow button ran the recorder, want no jump")
	}
	f.click(t, 1, " follow ")
	waitUntil(t, 5*time.Second, "a click on the unlit follow button never turned follow on", func() bool { return f.follow(t, session) == "on" })
	args := f.awaitRecord(t)
	if want := wantFollowRecord(socketPath, session); !slices.Equal(args[:7], want) {
		t.Errorf("recorded arguments = %q, want %q followed by the tmux path", args, want)
	}
	if filepath.Base(args[7]) != filepath.Base(f.e.cfg.Tmux) || !filepath.IsAbs(args[7]) {
		t.Errorf("recorded tmux path = %q, want the absolute path of %q", args[7], f.e.cfg.Tmux)
	}

	// A run button switches the client to that session and leaves the follow state of both sessions as it was.
	f.setFollow(t, session, "on")
	f.setFollow(t, "other", "off")
	f.click(t, 1, " other ")
	waitUntil(t, 5*time.Second, "a click on the other session's button never switched the client", func() bool {
		return strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "-c", f.client, "#{session_name}")) == "other"
	})
	if got := f.follow(t, session); got != "on" {
		t.Errorf("%s of the left session = %q after the session click, want on unchanged", followSessionOption, got)
	}
	if got := f.follow(t, "other"); got != "off" {
		t.Errorf("%s of the entered session = %q after the session click, want off unchanged", followSessionOption, got)
	}

	// The jump line names the clicking client's session as one argument, however the name is spelled.
	// The session shows two status lines only because it is given them, since reed pins that on its own session alone.
	const oddSession = "sp ace's #1"
	if err := f.e.tmux.run("new-session", "-d", "-s", oddSession); err != nil {
		t.Fatalf("new-session %q: %v", oddSession, err)
	}
	if err := f.e.tmux.run("set-option", "-t", "="+oddSession+":", "status", "2"); err != nil {
		t.Fatalf("status 2 on %q: %v", oddSession, err)
	}
	if err := f.e.tmux.run("switch-client", "-c", f.client, "-t", "="+oddSession); err != nil {
		t.Fatalf("switch-client to %q: %v", oddSession, err)
	}
	f.setFollow(t, oddSession, "off")
	f.removeRecord(t)
	f.click(t, 1, " follow ")
	odd := f.awaitRecord(t)
	if odd[5] != oddSession {
		t.Errorf("recorded session = %q, want %q intact (full record %q)", odd[5], oddSession, odd)
	}
}

func TestBindings_AltLeftRightRunTheSwitchCommandWithTheToldPaths(t *testing.T) {
	f := newBindingsFixture(t)
	socketPath := strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "#{socket_path}"))
	f.setFollow(t, f.e.SessionName(), "on")

	for _, step := range []struct{ key, direction string }{
		{"\x1b[1;3C", "--next"},
		{"\x1b[1;3D", "--prev"},
	} {
		if err := os.Remove(f.recordPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove stale record: %v", err)
		}
		f.send(t, step.key)
		waitUntil(t, 5*time.Second, "the recorder never ran for "+step.direction, func() bool {
			_, err := os.Stat(f.recordPath)
			return err == nil
		})
		var args []string
		waitUntil(t, 5*time.Second, "the recorder never finished writing", func() bool {
			raw, err := os.ReadFile(f.recordPath)
			args = strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
			return err == nil && len(args) == 9
		})
		want := []string{"reed", "switch", step.direction, "--socket", socketPath, "--client", f.client, "--tmux"}
		if !slices.Equal(args[:8], want) {
			t.Errorf("recorded arguments = %q, want %q followed by the tmux path", args, want)
		}
		if filepath.Base(args[8]) != filepath.Base(f.e.cfg.Tmux) || !filepath.IsAbs(args[8]) {
			t.Errorf("recorded tmux path = %q, want the absolute path of %q", args[8], f.e.cfg.Tmux)
		}
	}
	f.assertFollow(t, "on")
}
