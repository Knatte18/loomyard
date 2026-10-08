//go:build tmux && linux

// bindings_integration_test.go drives the Alt-key and status-bar bindings against a real tmux server and an attached client.
// The terminal's own byte sequences are written into the client's pty (ESC z, ESC [1;3A..D, SGR mouse clicks at the bar's cells), so the whole key path is exercised.
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
	if err := e.tmux.run("new-session", "-d", "-s", "other"); err != nil {
		t.Fatalf("new-session: %v", err)
	}

	pty := startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(bindingsClientCols, bindingsClientRows)...), bindingsClientCols, bindingsClientRows)
	waitForClientAttached(t, e, 15*time.Second)

	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v)", st, err)
	}
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

func TestBindings_AltZTogglesAStrandsZoomAndLeavesSelvageAndOtherWindowsAlone(t *testing.T) {
	f := newBindingsFixture(t)
	f.selectStrandsWindow(t, f.alpha.PaneID)

	f.send(t, "\x1bz")
	waitUntil(t, 5*time.Second, "Alt+z never zoomed the strand", func() bool { return f.zoomed(t) == "1" })
	f.send(t, "\x1bz")
	waitUntil(t, 5*time.Second, "Alt+z never unzoomed the strand", func() bool { return f.zoomed(t) == "0" })

	f.selectStrandsWindow(t, f.selvagePane)
	f.send(t, "\x1bz")
	settle()
	if got := f.zoomed(t); got != "0" {
		t.Errorf("Alt+z on Selvage left zoom flag %q, want 0", got)
	}

	if err := f.e.tmux.run("select-window", "-t", f.otherWindow); err != nil {
		t.Fatalf("select-window extra: %v", err)
	}
	f.send(t, "\x1bz")
	settle()
	if got := f.display(t, f.otherWindow, "#{window_zoomed_flag}"); got != "0" {
		t.Errorf("Alt+z in the second window zoomed it: flag %q, want 0", got)
	}
	if got := f.zoomed(t); got != "0" {
		t.Errorf("Alt+z in the second window zoomed the strands' window: flag %q, want 0", got)
	}
}

func TestBindings_AltUpDownStepThroughStrandPanesOnlyWrappingAndKeepingTheZoom(t *testing.T) {
	f := newBindingsFixture(t)
	f.selectStrandsWindow(t, f.alpha.PaneID)

	steps := []struct {
		key  string
		want string
	}{
		{"\x1b[1;3B", f.beta.PaneID},
		{"\x1b[1;3B", f.alpha.PaneID},
		{"\x1b[1;3A", f.beta.PaneID},
		{"\x1b[1;3A", f.alpha.PaneID},
	}
	for _, step := range steps {
		f.send(t, step.key)
		waitUntil(t, 5*time.Second, "the Alt step never reached "+step.want, func() bool { return f.activePane(t) == step.want })
	}

	if err := f.e.tmux.run("resize-pane", "-Z", "-t", f.alpha.PaneID); err != nil {
		t.Fatalf("zoom: %v", err)
	}
	f.send(t, "\x1b[1;3B")
	waitUntil(t, 5*time.Second, "the zoomed Alt+Down never reached beta", func() bool { return f.activePane(t) == f.beta.PaneID })
	if got := f.zoomed(t); got != "1" {
		t.Errorf("zoom flag after a step in a zoomed window = %q, want 1", got)
	}

	if err := f.e.tmux.run("select-window", "-t", f.otherWindow); err != nil {
		t.Fatalf("select-window extra: %v", err)
	}
	before := f.activePane(t)
	f.send(t, "\x1b[1;3A")
	settle()
	if got := f.activePane(t); got != before {
		t.Errorf("Alt+Up in the second window moved the strands' window to %q, want it left at %q", got, before)
	}
}

func TestBindings_StatusBarClicksSelectAndZoomByRangeType(t *testing.T) {
	f := newBindingsFixture(t)

	// A strand button selects its pane and zooms it, and a second click keeps the zoom on.
	f.selectStrandsWindow(t, f.alpha.PaneID)
	f.click(t, 0, " alpha ")
	waitUntil(t, 5*time.Second, "a click on alpha's button never zoomed it", func() bool {
		return f.activePane(t) == f.alpha.PaneID && f.zoomed(t) == "1"
	})
	f.click(t, 0, " beta ")
	waitUntil(t, 5*time.Second, "a click on beta's button never moved the zoom to it", func() bool {
		return f.activePane(t) == f.beta.PaneID && f.zoomed(t) == "1"
	})
	f.click(t, 0, " beta ")
	settle()
	if got := f.zoomed(t); got != "1" {
		t.Errorf("a second click on beta's button left zoom flag %q, want 1 (never toggled off)", got)
	}

	// VIEW unzooms the strands' window.
	f.click(t, 0, " VIEW ")
	waitUntil(t, 5*time.Second, "a click on VIEW never unzoomed the strands' window", func() bool { return f.zoomed(t) == "0" })

	// A window button selects that window, and VIEW from there returns to the strands' window.
	f.click(t, 0, " extra ")
	waitUntil(t, 5*time.Second, "a click on the extra window's button never selected it", func() bool { return f.currentWindow(t) == "extra" })
	f.click(t, 0, " VIEW ")
	waitUntil(t, 5*time.Second, "a click on VIEW from the second window never returned to the strands' window", func() bool {
		return f.currentWindow(t) != "extra"
	})

	// A strand button from the second window selects and zooms the strand in its own window.
	if err := f.e.tmux.run("select-window", "-t", f.otherWindow); err != nil {
		t.Fatalf("select-window extra: %v", err)
	}
	f.click(t, 0, " alpha ")
	waitUntil(t, 5*time.Second, "a click on alpha's button from the second window never zoomed it", func() bool {
		return f.currentWindow(t) != "extra" && f.activePane(t) == f.alpha.PaneID && f.zoomed(t) == "1"
	})

	// A run button switches the client to that session.
	f.click(t, 1, " other ")
	waitUntil(t, 5*time.Second, "a click on the other session's button never switched the client", func() bool {
		return strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "-c", f.client, "#{session_name}")) == "other"
	})
}

func TestBindings_AltLeftRightRunTheSwitchCommandWithTheToldPaths(t *testing.T) {
	f := newBindingsFixture(t)
	socketPath := strings.TrimSpace(mustTmuxOutput(t, f.e, "display-message", "-p", "#{socket_path}"))

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
}
