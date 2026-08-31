//go:build smoke

// smoke_dotfill_test.go reproduces the tmux client-side dot-fill render artifact described by reed's
// root-cause-model decision: tmux itself, not reed, paints a run of dot-fill glyphs (see dotFillGlyphs)
// into the region of an attached client's terminal that its own window geometry does not (yet, or
// ever) cover. Those dots live entirely in what tmux paints to a client's terminal — they are in no
// pane's grid, so they never
// show up in a `capture-pane` of a strand's own pane. Capturing the *harness* pane that hosts the
// attach client is therefore the only way to observe them: this file's harness boots a second, private
// tmux server whose own pane renders the attach client's terminal, and every dot-run assertion here
// captures that harness pane, never a reed-managed strand pane.
//
// This file adds no production code and changes no production behaviour: it is the measuring
// instrument the next two batches build on.

package reedcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
)

// dotRunFloor is the minimum number of consecutive dot-fill characters on one captured line that
// counts as the dot-fill artifact.
// This value is fixed, not tunable: it sits far above anything reed's own rendered content produces on
// one line, and far below the width of any pane region tmux would pad with the artifact. Card 6 (below)
// validates it once against a clean capture, and that validation is a gate rather than a licence to
// retune it — a future failure of that gate is news about reed's own output, not a reason to raise the
// floor.
const dotRunFloor = 20

// dotFillGlyphs are the tmux uncovered-cell padding characters lineHasDotRun matches against.
//
// Verified live on tmux 3.6: the operator's field report describes the artifact as "dots", but the
// byte tmux 3.6 actually writes into an uncovered cell — confirmed here via a hex dump of a captured
// cross-client-control line — is U+00B7 MIDDLE DOT ("\xc2\xb7" in UTF-8), never the ASCII U+002E FULL
// STOP a literal `strings.Repeat(".", dotRunFloor)` would match. A middle dot renders visually close
// enough to a period, at typical terminal font sizes, that the field report's informal "dots" is
// accurate prose despite naming the wrong code point. Older tmux builds — and clients that failed to
// negotiate a UTF-8 terminal — fall back to the ASCII period for the same padding role, so both glyphs
// are matched here rather than only the one this build was observed to emit.
var dotFillGlyphs = []string{".", "·"}

// lineHasDotRun reports whether line contains a run of at least dotRunFloor consecutive occurrences of
// one of dotFillGlyphs.
func lineHasDotRun(line string) bool {
	for _, glyph := range dotFillGlyphs {
		if strings.Contains(line, strings.Repeat(glyph, dotRunFloor)) {
			return true
		}
	}
	return false
}

// captureHasDotRun reports whether any single line of capture contains a dot run, per lineHasDotRun.
// This is checked per line rather than as a whole-capture substring test, because a whole-capture test
// would join dots across line boundaries and could report a hit built from unrelated content on two
// separate lines.
func captureHasDotRun(capture string) bool {
	for _, line := range strings.Split(capture, "\n") {
		if lineHasDotRun(line) {
			return true
		}
	}
	return false
}

// pollPaneHasDotRun captures the target pane via capturePane every 100 ms until captureHasDotRun holds
// (returning true) or timeout elapses (returning false).
// It samples at 100 ms rather than reusing pollPaneContains, and it never fails the test itself: the
// caller decides whether a hit or a miss is the expected outcome, so a control scenario and a treatment
// scenario can share this one poller.
func pollPaneHasDotRun(t *testing.T, tmuxPath, socket, target string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if captureHasDotRun(capturePane(t, tmuxPath, socket, target)) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// pollPaneDotRunClears captures the target pane via capturePane every 100 ms until a capture free of
// any dot run is observed (returning true) or timeout elapses (returning false).
// It is pollPaneHasDotRun's inverse, not paneStaysCleanOfDotRun's: it waits for the FIRST clean
// capture rather than requiring every sample to be clean, so a caller can assert that an artifact it
// just proved present clears once its trigger condition is removed, without racing the clear itself.
// Like the other pollers here it never fails the test — the caller owns the verdict.
func pollPaneDotRunClears(t *testing.T, tmuxPath, socket, target string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if !captureHasDotRun(capturePane(t, tmuxPath, socket, target)) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// paneStaysCleanOfDotRun samples the target pane via capturePane every 100 ms for the whole of window,
// returning false on the first sample where captureHasDotRun holds and true only if every sample was
// clean.
// It must never return early on a clean sample: an absence assertion that did so would pass before the
// artifact had a chance to appear, which is exactly the failure mode this helper exists to avoid.
//
// Both pollers in this file sample at 100 ms rather than reusing pollPaneContains: pollPaneContains
// takes a plain substring, and legitimate harness-pane content contains dots (file paths, ellipses, the
// header template), so reusing it would ship a test that proves nothing. Its 500 ms cadence is also a
// quarter of the window the artifact would occupy under watchdog: on, too coarse to characterise it.
func paneStaysCleanOfDotRun(t *testing.T, tmuxPath, socket, target string, window time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(window)
	clean := true
	for time.Now().Before(deadline) {
		if captureHasDotRun(capturePane(t, tmuxPath, socket, target)) {
			clean = false
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return clean
}

// dotFillHarness is one harness-in-harness scenario's live fixture: a booted reed session (reedSocket,
// reedSession) inside a hub worktree, plus a second, private tmux server (harnessSocket) whose own pane
// hosts the attach client that lets a dot-fill scenario observe reed's session from the outside.
type dotFillHarness struct {
	tmuxPath      string
	lyxExe        string
	harnessSocket string
	reedSocket    string
	reedSession   string
}

// newDotFillHarness boots a dot-fill scenario's full fixture: a reed session carrying a header pane
// plus two strand panes, and a private harness tmux server sized cols x rows to host the attach
// client(s) that observe it.
func newDotFillHarness(t *testing.T, cols, rows int) *dotFillHarness {
	t.Helper()

	// This is the watchdog-off-in-every-smoke-scenario Shared Decision: it must land before any
	// RunCLI call, so every config load in this process resolves the watchdog key to "off". Its
	// mechanical consequence is that resizeSignalHookCommand answers "", so reed's window-resized
	// array holds the resize-pane pins and no signal entry — the array shape every rewrite and
	// readback helper below is written against.
	t.Setenv("LYX_REED_WATCHDOG", "off")

	tmuxPath := tmuxBinaryPath(t)
	shellPath := harnessShellBinaryPath(t)
	lyxExe := buildLyxBinary(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var up bytes.Buffer
	if code := RunCLI(&up, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, up.String())
	}

	// Two strands, not one, is a fidelity choice rather than a pin-count requirement:
	// render.FixedHeightPins emits the header pin whenever a header is placed and the layout is not
	// the sole-header case, so a single strand already yields a non-empty pin set. What two strands
	// buy is a taller stack for the resize round-robin to distribute rows across, so the
	// mid-relayout region is larger and the artifact reproduces more reliably, and it keeps the
	// scenario clear of AttachArgv's len(live) < 2 guard boundary rather than sitting exactly on it.
	addStrand(t, smokeMarkerLaunchCmd("DOTFILL-MARKER-ALPHA"), "--name", "dfalpha")
	addStrand(t, smokeMarkerLaunchCmd("DOTFILL-MARKER-BETA"), "--name", "dfbeta")

	reedSocket, reedSession := socketAndSession(t)

	harnessSocket := fmt.Sprintf("lyx-dotfill-harness-%d", os.Getpid())
	if err := exec.Command(tmuxPath, "-L", harnessSocket, "new-session", "-d", "-s", "h",
		"-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows), shellPath).Run(); err != nil {
		t.Fatalf("boot harness server: %v", err)
	}
	t.Cleanup(func() {
		reapHarnessServer(t, tmuxPath, harnessSocket)
	})
	deadline := time.Now().Add(30 * time.Second)
	for exec.Command(tmuxPath, "-L", harnessSocket, "has-session", "-t", "h").Run() != nil {
		if time.Now().After(deadline) {
			t.Fatal("harness session did not come up within 30s")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Pin the harness window's geometry so a later resize is deterministic: with the default
	// "latest" window-size policy the harness window would itself track its most-recently-active
	// client, which is exactly the moving target a scenario driving the harness's OWN geometry
	// cannot tolerate.
	if err := exec.Command(tmuxPath, "-L", harnessSocket, "set-option", "-t", "h", "-w", "window-size", "manual").Run(); err != nil {
		t.Fatalf("pin harness window-size manual: %v", err)
	}

	return &dotFillHarness{
		tmuxPath:      tmuxPath,
		lyxExe:        lyxExe,
		harnessSocket: harnessSocket,
		reedSocket:    reedSocket,
		reedSession:   reedSession,
	}
}

// attachIn sends the attach invocation into paneID and waits for the attach to have rendered by
// polling for marker, one of the harness's strand markers.
// Waiting on a marker rather than a fixed sleep is what makes the scenarios deterministic.
func (h *dotFillHarness) attachIn(t *testing.T, paneID, marker string) {
	t.Helper()
	sendKeysLine(t, h.tmuxPath, h.harnessSocket, paneID, smokeAttachInvokeLine(h.lyxExe))
	pollPaneContains(t, h.tmuxPath, h.harnessSocket, paneID, marker, 20*time.Second)
}

// windowResizedEntries runs `show-options -v` against reed's own socket for the window-resized array
// and splits the answer into its entries, mirroring hookArrayEntries exactly.
//
// show-options -v prints every entry of an array option one per line, in index order, with the
// "window-resized[N]" prefix suppressed; a trailing newline yields a trailing empty entry, which
// callers must tolerate rather than special-case.
//
// The "=<session>:" window form is required here because window-resized is a window-scoped option; the
// bare "=<session>" session form must not be used for this call.
func windowResizedEntries(t *testing.T, tmuxPath, socket, session string) []string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "show-options", "-v", "-t", "="+session+":", "window-resized").Output()
	if err != nil {
		t.Fatalf("show-options -v window-resized: %v", err)
	}
	return strings.Split(string(out), "\n")
}

// pinOnlyEntries returns only the entries of entries carrying the "resize-pane " prefix.
//
// This is exactly "reed's own array minus the repaint entry" in both eras this plan spans: before this
// task reed's array under watchdog: off is pins alone, and after batch 3 it is pins plus one repaint
// entry, so this filter needs no revision when the repaint entry ships.
func pinOnlyEntries(entries []string) []string {
	var pins []string
	for _, entry := range entries {
		if strings.HasPrefix(entry, "resize-pane ") {
			pins = append(pins, entry)
		}
	}
	return pins
}

// rewriteWindowResizedArray rebuilds session's window-resized array on socket from entries, issuing the
// clear first and then one set-hook per entry — the plain replacing form for index 0, the -a appending
// form for every entry after it. Empty entries are skipped.
//
// This reproduces resizePinHookArgvs's own plain-first/-a-after rebuild pattern. Rewriting the array
// from the test rather than adding a production seam is deliberate: it needs no build-tagged env knob,
// no exported test hook, and no branch in shipping code.
//
// Sequencing rule the control scenarios depend on: every AttachArgv pre-flight rebuilds the array from
// scratch, so this rewrite must be the LAST setup step, after every attach the scenario performs, and
// immediately before the trigger. An attach performed after the rewrite would re-install reed's own
// array and the control would silently assert against the wrong one.
func rewriteWindowResizedArray(t *testing.T, tmuxPath, socket, session string, entries []string) {
	t.Helper()
	target := "=" + session + ":"
	if err := exec.Command(tmuxPath, "-L", socket, "set-hook", "-u", "-w", "-t", target, "window-resized").Run(); err != nil {
		t.Fatalf("set-hook -u -w window-resized: %v", err)
	}
	first := true
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		args := []string{"-L", socket, "set-hook"}
		if !first {
			args = append(args, "-a")
		}
		args = append(args, "-w", "-t", target, "window-resized", entry)
		if err := exec.Command(tmuxPath, args...).Run(); err != nil {
			t.Fatalf("set-hook window-resized %q: %v", entry, err)
		}
		first = false
	}
}

// assertOnlyPinEntries fails the test unless every non-empty entry of entries carries the
// "resize-pane " prefix, and unless at least one such entry is present.
// This is the control's proof that it fired against the array it wrote, converting "we think no attach
// intervened" into an assertion. It performs the same per-entry matching hookInstalledLocked performs,
// never a match against the whole answer.
func assertOnlyPinEntries(t *testing.T, entries []string) {
	t.Helper()
	sawPin := false
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		if !strings.HasPrefix(entry, "resize-pane ") {
			t.Fatalf("window-resized entry %q is not a resize-pane pin; an attach must have re-installed reed's own array after the rewrite", entry)
		}
		sawPin = true
	}
	if !sawPin {
		t.Fatalf("window-resized array carried no resize-pane pin entries after the rewrite")
	}
}

// TestSmokeDotFillFloorIsCleanOnASettledAttach validates dotRunFloor against a settled attach with no
// trigger fired: no resize, no input delivered beyond the attach itself.
//
// A failure here means something in reed's own rendered output produces a run of at least dotRunFloor
// dots on one line. That is itself news, and the remedy is to report it — never to raise the floor
// until this test goes quiet.
func TestSmokeDotFillFloorIsCleanOnASettledAttach(t *testing.T) {
	h := newDotFillHarness(t, 140, 42)
	paneID := harnessOnlyPaneID(t, h.tmuxPath, h.harnessSocket, "h")
	h.attachIn(t, paneID, "DOTFILL-MARKER-ALPHA")

	if capture := capturePane(t, h.tmuxPath, h.harnessSocket, paneID); captureHasDotRun(capture) {
		t.Fatalf("settled attach capture already carries a run of >= %d dots — reed's own rendered output produces the dot-fill floor's own pattern; report this, do not raise dotRunFloor:\n%s", dotRunFloor, capture)
	}
	if !paneStaysCleanOfDotRun(t, h.tmuxPath, h.harnessSocket, paneID, 2*time.Second) {
		t.Fatalf("settled attach pane developed a run of >= %d dots with no trigger fired — reed's own rendered output produces the dot-fill floor's own pattern; report this, do not raise dotRunFloor", dotRunFloor)
	}
}

// TestSmokeDotFillResizeControl is the resize-trigger control scenario: it proves the harness still
// reproduces the dot-fill artifact on a real window-dimension change (root-cause-model decision) by
// resizing reed's own window in both directions after rewriting reed's own window-resized array to
// pins only, and asserting the padding paint appears while the window stands smaller than the
// attached client and clears once the window grows back past it.
//
// A control that does not hit means the harness can no longer reproduce the bug, and every companion
// absence assertion in the batches built on this one has become vacuous — so a miss here is a run
// failure, not a skip. This test does not assert the artifact appears on every size or on every run;
// that asymmetry is exactly why this control exists as an executable assertion rather than as a note in
// a commit message.
func TestSmokeDotFillResizeControl(t *testing.T) {
	h := newDotFillHarness(t, 140, 42)
	paneID := harnessOnlyPaneID(t, h.tmuxPath, h.harnessSocket, "h")
	h.attachIn(t, paneID, "DOTFILL-MARKER-ALPHA")

	// Last setup step, after the attach: rewrite reed's own array to pins only, then prove the
	// rewrite stuck.
	pins := pinOnlyEntries(windowResizedEntries(t, h.tmuxPath, h.reedSocket, h.reedSession))
	rewriteWindowResizedArray(t, h.tmuxPath, h.reedSocket, h.reedSession, pins)
	assertOnlyPinEntries(t, windowResizedEntries(t, h.tmuxPath, h.reedSocket, h.reedSession))

	// Fire the trigger's shrink half: resize reed's own window directly, on reed's own socket, to a
	// size distinctly smaller than the attached client. resize-window flips the window's window-size
	// option to manual (verified live on tmux 3.6), so the shrunken window STANDS against the
	// still-140x42 client rather than snapping back under the latest policy — and a standing
	// window-smaller-than-client mismatch is exactly the state root-cause-model says tmux pads with
	// dot-fill glyphs in the client region the window's geometry does not cover.
	//
	// The hit assertion must run while the window is still shrunk; both prior rounds of this batch
	// missed here by sampling only after a back-to-back shrink-then-grow pair. Measured live in this
	// container's tmux 3.6 build: the padding appears within ~10ms of the shrink and stands for as
	// long as the mismatch does, while the grow clears it instantly and permanently (zero hits
	// across ~390 tight no-sleep capture samples over 2.5s, three runs) — so a poll started after
	// the grow samples only the clean fully-covered regime and can never hit. A real SIGWINCH
	// cascade (resizing the outer harness window so the attached client's own pty changes size and
	// window-size latest follows it) was also measured live, in both directions and in 1-column
	// drag steps, each with a concurrent tight sampling loop: zero hits — in this build the client
	// repaint after a followed resize is atomic from capture-pane's viewpoint, so the transient
	// stale-paint smear the field report describes is not observable headlessly, and the standing
	// mismatch below is this environment's reliable reproduction of the same client-side padding
	// paint.
	if err := exec.Command(h.tmuxPath, "-L", h.reedSocket, "resize-window", "-t", h.reedSession, "-x", "80", "-y", "24").Run(); err != nil {
		t.Fatalf("resize-window shrink: %v", err)
	}
	if !pollPaneHasDotRun(t, h.tmuxPath, h.harnessSocket, paneID, 5*time.Second) {
		t.Fatalf("resize control did not reproduce the dot-fill artifact within 5s of the shrink — the harness can no longer reproduce the bug, so every companion absence assertion built on this control has become vacuous; this is a run failure, not a skip")
	}

	// Fire the trigger's grow half: grow the window back past the original. The client is fully
	// covered again, so the padding must clear — the pair of assertions is what proves the dots
	// track the window-versus-client mismatch itself, not anything in reed's rendered content.
	if err := exec.Command(h.tmuxPath, "-L", h.reedSocket, "resize-window", "-t", h.reedSession, "-x", "160", "-y", "50").Run(); err != nil {
		t.Fatalf("resize-window grow: %v", err)
	}
	if !pollPaneDotRunClears(t, h.tmuxPath, h.harnessSocket, paneID, 5*time.Second) {
		t.Fatalf("dot-fill artifact still present 5s after the grow covered the client again — the dots did not track the window-versus-client mismatch, so this scenario measured something other than the padding paint root-cause-model describes")
	}
}

// TestSmokeDotFillResizeTreatment is the resize-trigger treatment scenario, the fix-side companion to
// TestSmokeDotFillResizeControl above. It shares the control's setup exactly — newDotFillHarness,
// harnessOnlyPaneID, attachIn — and fires the same shrink-then-grow resize-window trigger. It differs
// in one way: it leaves reed's own window-resized array untouched rather than rewriting it, so this
// scenario always observes whatever reed itself installs.
//
// Per the Measurement record (repaint candidates) block in internal/reedengine/doc.go's package doc
// comment, neither measured repaint candidate was accepted: both cleared the dot-fill artifact but both
// were rejected on the repaint-must-not-self-retrigger decision's exactly-one-fire criterion. No repaint
// entry ships from this task, so this scenario is INVERTED rather than skipped or deleted: it asserts
// the artifact still appears on the resize trigger. This makes the scenario a live tripwire — if a
// future tmux release or a future reed change makes the artifact stop appearing on its own (for
// instance because a repaint mechanism is added later without updating this test), this scenario fails
// and someone finds out, which a t.Skip would never do.
func TestSmokeDotFillResizeTreatment(t *testing.T) {
	h := newDotFillHarness(t, 140, 42)
	paneID := harnessOnlyPaneID(t, h.tmuxPath, h.harnessSocket, "h")
	h.attachIn(t, paneID, "DOTFILL-MARKER-ALPHA")

	// No array readback and no rewrite here — unlike the control, this scenario's whole point is to
	// observe reed's own array exactly as installed. There is also no repaint entry to assert the
	// presence of, since no candidate was accepted.
	if err := exec.Command(h.tmuxPath, "-L", h.reedSocket, "resize-window", "-t", h.reedSession, "-x", "80", "-y", "24").Run(); err != nil {
		t.Fatalf("resize-window shrink: %v", err)
	}
	if err := exec.Command(h.tmuxPath, "-L", h.reedSocket, "resize-window", "-t", h.reedSession, "-x", "160", "-y", "50").Run(); err != nil {
		t.Fatalf("resize-window grow: %v", err)
	}

	if !pollPaneHasDotRun(t, h.tmuxPath, h.harnessSocket, paneID, 5*time.Second) {
		t.Fatalf("inverted resize treatment did not reproduce the dot-fill artifact within 5s — this is a live tripwire per the no-candidate-accepted disposition in internal/reedengine/doc.go's Measurement record (repaint candidates) block: either the environment changed or a repaint mechanism has shipped without updating this scenario")
	}
}

// TestSmokeDotFillCrossClientControl is the cross-client-trigger control scenario. It is control-only —
// there is no cross-client treatment scenario, in any branch of the measurement gate — per the
// uncovered-subset-is-documented-not-fixed decision: this is a documentation-of-behaviour test, not a
// fix test.
//
// Under root-cause-model, these dots are the UNCOVERED subset: the window shrank to the toucher
// client's size, so the taller observed client has real estate with nothing behind it and tmux is
// padding it correctly. No repaint mechanism can remove them.
func TestSmokeDotFillCrossClientControl(t *testing.T) {
	h := newDotFillHarness(t, 140, 42)

	if err := exec.Command(h.tmuxPath, "-L", h.harnessSocket, "split-window", "-v", "-t", "h").Run(); err != nil {
		t.Fatalf("split-window -v: %v", err)
	}
	panes := strings.Fields(strings.TrimSpace(mustOutput(t, h.tmuxPath, h.harnessSocket, "list-panes", "-t", "h", "-F", "#{pane_id}")))
	if len(panes) != 2 {
		t.Fatalf("harness window has %d panes after split; want 2 (panes=%v)", len(panes), panes)
	}
	observedPane, toucherPane := panes[0], panes[1]

	// Size them deliberately unequally: the observed pane distinctly taller (about 30 rows), the
	// toucher pane distinctly shorter (about 8 rows). This is the configuration the field report
	// describes — a VS Code integrated terminal is smaller than a standalone Konsole window — and it
	// is the only configuration this trigger is known to reproduce in.
	if err := exec.Command(h.tmuxPath, "-L", h.harnessSocket, "resize-pane", "-t", observedPane, "-y", "30").Run(); err != nil {
		t.Fatalf("resize-pane observed: %v", err)
	}
	if err := exec.Command(h.tmuxPath, "-L", h.harnessSocket, "resize-pane", "-t", toucherPane, "-y", "8").Run(); err != nil {
		t.Fatalf("resize-pane toucher: %v", err)
	}

	// Both attaches complete before anything else.
	h.attachIn(t, observedPane, "DOTFILL-MARKER-ALPHA")
	h.attachIn(t, toucherPane, "DOTFILL-MARKER-BETA")

	// Last setup step, after both attaches: rewrite reed's own array to pins only, then prove the
	// rewrite stuck.
	pins := pinOnlyEntries(windowResizedEntries(t, h.tmuxPath, h.reedSocket, h.reedSession))
	rewriteWindowResizedArray(t, h.tmuxPath, h.reedSocket, h.reedSession, pins)
	assertOnlyPinEntries(t, windowResizedEntries(t, h.tmuxPath, h.reedSocket, h.reedSession))

	// Fire the trigger: deliver input to the toucher client. Any client input suffices — a keystroke,
	// a mouse report, a focus report — because what the trigger needs is only to make that client the
	// most-recently-used one, which is what `window-size latest` keys on. No resize is fired and no
	// reed code path runs.
	if err := exec.Command(h.tmuxPath, "-L", h.harnessSocket, "send-keys", "-t", toucherPane, "Escape").Run(); err != nil {
		t.Fatalf("send-keys Escape to toucher: %v", err)
	}

	if !pollPaneHasDotRun(t, h.tmuxPath, h.harnessSocket, observedPane, 5*time.Second) {
		t.Fatalf("cross-client control did not reproduce the dot-fill artifact within 5s on the observed pane — the harness can no longer reproduce the bug, so every companion absence assertion built on this control has become vacuous; this is a run failure, not a skip")
	}
	if paneStaysCleanOfDotRun(t, h.tmuxPath, h.harnessSocket, observedPane, 2*time.Second) {
		t.Fatalf("cross-client artifact on the observed pane cleared within 2s — this trigger's dots are the uncovered subset and must stand, not self-heal; a clean window here means this scenario measured the wrong thing")
	}
}

// mustOutput runs a tmux command against socket and fails the test on a non-zero exit, returning its
// stdout.
func mustOutput(t *testing.T, tmuxPath, socket string, args ...string) string {
	t.Helper()
	full := append([]string{"-L", socket}, args...)
	out, err := exec.Command(tmuxPath, full...).Output()
	if err != nil {
		t.Fatalf("tmux -L %s %v: %v", socket, args, err)
	}
	return string(out)
}
