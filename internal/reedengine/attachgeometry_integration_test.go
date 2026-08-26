//go:build integration && linux

// attachgeometry_integration_test.go proves the attach handover lands the client-sized layout
// verbatim against a real tmux, real pty, and real attaching client — the tier-2 assertion that
// fails before this task (tmux's own rescale rewrites AttachArgv's planned window_layout string) and
// passes after (the chained select-layout runs post-attach, once the window already matches the
// client, so the string lands unchanged).
//
// Linux-only: the pty harness below is built directly on golang.org/x/sys/unix's /dev/ptmx ioctls
// (TIOCSPTLCK, TIOCGPTN, TIOCSWINSZ), which have no portable equivalent, and psmux's behaviour under
// a real pty is unverified anywhere in this repo — this file must not even attempt to compile off
// Linux.

package reedengine

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// attachGeometryPTY is a running child process attached to a real pty of a fixed initial size, with
// the master file descriptor kept open so a caller can drive further ioctls or reads against it.
type attachGeometryPTY struct {
	master *os.File
	cmd    *exec.Cmd
}

// startInPTY opens a fresh /dev/ptmx master sized to cols x rows and starts argv[0] with the pty
// slave as its stdin/stdout/stderr and as its controlling terminal (SysProcAttr{Setsid: true,
// Setctty: true} — Setctty's zero-value Ctty names fd 0 in the CHILD's own descriptor table, which is
// the slave since it is also wired to stdin).
//
// The master is drained continuously in a background goroutine so the child never blocks writing to
// a full pty buffer, and both the master and the started process are torn down via t.Cleanup so a
// failing assertion never leaks either.
func startInPTY(t *testing.T, argv []string, cols, rows int) *attachGeometryPTY {
	t.Helper()

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	fd := int(master.Fd())

	// Clear the slave's lock before it can be opened at all — /dev/ptmx starts every new pty locked.
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		t.Fatalf("TIOCSPTLCK (clear pty lock): %v", err)
	}
	slaveNum, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		master.Close()
		t.Fatalf("TIOCGPTN (resolve slave number): %v", err)
	}
	// Size the pty before the child ever reads/writes it: TIOCSWINSZ on either end of a pty pair sets
	// the size both sides observe, and setting it here means the child's very first #{window_width}/
	// #{window_height} — the ones AttachArgv's chained select-layout is racing to land verbatim — are
	// already the deliberately-mismatched size this test drives.
	if err := unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)}); err != nil {
		master.Close()
		t.Fatalf("TIOCSWINSZ (%dx%d): %v", cols, rows, err)
	}

	slavePath := "/dev/pts/" + strconv.Itoa(slaveNum)
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		master.Close()
		t.Fatalf("open pty slave %s: %v", slavePath, err)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.Env = append(os.Environ(), "TERM=xterm")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	startErr := cmd.Start()
	// The child holds its own copy of the slave once started (or forever, on a failed start); the
	// test only ever needs the master.
	slave.Close()
	if startErr != nil {
		master.Close()
		t.Fatalf("start %v in pty: %v", argv, startErr)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = master.Close()
	})

	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := master.Read(buf); err != nil {
				return
			}
		}
	}()

	return &attachGeometryPTY{master: master, cmd: cmd}
}

// setupAttachGeometryFixture boots a fresh integration engine and adds two strands — a
// ShrinkWhenWaitingOnChild parent and its child — so the session carries a header pane, a collapsed
// strip, and a full pane simultaneously: the three-cell shape every case below lays out against.
func setupAttachGeometryFixture(t *testing.T) *Engine {
	t.Helper()

	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	parent, err := e.AddStrand(AddSpec{
		Cmd:     "sleep 300",
		Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: true},
	})
	if err != nil {
		t.Fatalf("AddStrand(parent): %v", err)
	}
	if _, err := e.AddStrand(AddSpec{
		Cmd:     "sleep 300",
		Parent:  parent.GUID,
		Display: render.Display{Anchor: render.AnchorBelowParent},
	}); err != nil {
		t.Fatalf("AddStrand(child): %v", err)
	}

	return e
}

// waitForClientAttached polls list-clients until this session reports at least one attached client.
func waitForClientAttached(t *testing.T, e *Engine, timeout time.Duration) {
	t.Helper()
	waitUntil(t, timeout, "no tmux client ever attached to the session", func() bool {
		out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()))
		return err == nil && strings.TrimSpace(out) != ""
	})
}

// windowSizeNow reads #{window_width} and #{window_height} back from outside the pty, via the
// engine's own TmuxCmd on the same socket, and fails the test on a malformed or errored answer.
func windowSizeNow(t *testing.T, e *Engine) (w, h int) {
	t.Helper()
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_width} #{window_height}")
	if err != nil {
		t.Fatalf("display-message #{window_width} #{window_height}: %v", err)
	}
	w, h, ok := parseWindowSize(out)
	if !ok {
		t.Fatalf("display-message #{window_width} #{window_height} = %q, want two positive integers", out)
	}
	return w, h
}

// windowLayoutNow reads #{window_layout} back from outside the pty, via the engine's own TmuxCmd.
func windowLayoutNow(t *testing.T, e *Engine) string {
	t.Helper()
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_layout}")
	if err != nil {
		t.Fatalf("display-message #{window_layout}: %v", err)
	}
	return strings.TrimSpace(out)
}

// TestAttachGeometry_ExactLayoutAndRowBudgets is the assertion that fails before this task: with a
// pty deliberately unequal to the configured boot size, it drives a real attach-session through the
// chained argv AttachArgv builds and asserts, from OUTSIDE the pty, that the live window becomes
// exactly the client's told size and that #{window_layout} equals the argv's own planned string byte
// for byte — tmux's silent proportional rescale is what this pins against. It then asserts, from the
// same attached session, that the header pane and the collapsed strip both landed at their configured,
// unclamped row budgets.
func TestAttachGeometry_ExactLayoutAndRowBudgets(t *testing.T) {
	e := setupAttachGeometryFixture(t)

	const cols, rows = 100, 30
	argv := e.AttachArgv(cols, rows)
	if len(argv) != 10 {
		t.Fatalf("AttachArgv(%d, %d) = %v (%d argv elements), want the 10-element chained form — the attach chain was unexpectedly suppressed", cols, rows, argv, len(argv))
	}
	wantLayout := argv[len(argv)-1]

	startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)

	// Case 1: the exact-layout assertion, evaluated from outside the pty on the same socket so the
	// attaching client's own view of the window cannot influence the answer.
	gotW, gotH := windowSizeNow(t, e)
	if gotW != cols {
		t.Errorf("#{window_width} after attach = %d, want %d (the client's told cols)", gotW, cols)
	}
	if gotH != rows {
		t.Errorf("#{window_height} after attach = %d, want exactly %d (status is off, so the window is the client's full rows, not rows-1)", gotH, rows)
	}
	if gotLayout := windowLayoutNow(t, e); gotLayout != wantLayout {
		t.Errorf("#{window_layout} after attach = %q, want %q byte for byte — a mismatch here means tmux rescaled the planned string instead of applying it verbatim", gotLayout, wantLayout)
	}

	// Case 2: the row budgets survived, read from the same attached session.
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
	}
	if len(st.Strands) != 2 {
		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
	}
	parentPaneID := st.Strands[0].PaneID
	headerPaneID := st.HeaderPaneID

	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes: %v", err)
	}
	var sawHeader, sawParent bool
	for _, p := range live {
		switch p.ID {
		case headerPaneID:
			sawHeader = true
			if p.Height != e.cfg.Header.HeightRows {
				t.Errorf("header pane %s height = %d, want %d (cfg.Header.HeightRows)", p.ID, p.Height, e.cfg.Header.HeightRows)
			}
		case parentPaneID:
			sawParent = true
			if p.Height != e.cfg.CollapsedStripRows {
				t.Errorf("collapsed parent pane %s height = %d, want %d (cfg.CollapsedStripRows)", p.ID, p.Height, e.cfg.CollapsedStripRows)
			}
		}
	}
	if !sawHeader {
		t.Fatalf("header pane %s missing from live panes %+v", headerPaneID, live)
	}
	if !sawParent {
		t.Fatalf("collapsed parent pane %s missing from live panes %+v", parentPaneID, live)
	}
}

// TestAttachGeometry_DegradedPathStillAttaches asserts AttachArgv(0, 0)'s bare-argv degradation still
// lands a working attach with no chained layout: attach is the operator's escape hatch, and this pins
// that the degradation path is a strictly-no-worse attach, not a broken one.
func TestAttachGeometry_DegradedPathStillAttaches(t *testing.T) {
	e := setupAttachGeometryFixture(t)

	argv := e.AttachArgv(0, 0)
	if len(argv) != 5 {
		t.Fatalf("AttachArgv(0, 0) = %v (%d argv elements), want the 5-element bare form with no chained select-layout", argv, len(argv))
	}

	pty := startInPTY(t, append([]string{e.cfg.Tmux}, argv...), 80, 24)
	waitForClientAttached(t, e, 15*time.Second)

	if !proc.IsAlive(pty.cmd.Process.Pid) {
		t.Fatalf("attach-session process (pid %d) is not alive after attach was confirmed via list-clients", pty.cmd.Process.Pid)
	}
	if pty.cmd.ProcessState != nil && pty.cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("attach-session process exited with code %d, want it still running (or a clean 0)", pty.cmd.ProcessState.ExitCode())
	}
}

// TestAttachGeometry_StaleLayoutRaceIsSafe pins the stale-layout race the accepted build-vs-apply
// window rests on: the planned argv is built first, then the live pane set is mutated (a new strand
// is added) before the argv is ever exec'd, reproducing the window between AttachArgv's op-locked
// plan and the shell's later, lock-free exec. tmux must refuse the now-mismatched chained
// select-layout rather than obey it, the attach itself must still succeed, and no pane may be
// destroyed.
func TestAttachGeometry_StaleLayoutRaceIsSafe(t *testing.T) {
	e := setupAttachGeometryFixture(t)

	const cols, rows = 100, 30
	argv := e.AttachArgv(cols, rows)
	if len(argv) != 10 {
		t.Fatalf("AttachArgv(%d, %d) = %v (%d argv elements), want the 10-element chained form", cols, rows, argv, len(argv))
	}
	plannedLayout := argv[len(argv)-1]

	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes before mutating the pane set: %v", err)
	}
	beforeIDs := liveIDSet(live)

	// Mutate the live pane set AFTER the argv (and its embedded layout string) has already been
	// planned — a third, root-level strand added here makes the string's cell count disagree with
	// the live pane count by the time the argv below actually runs.
	if _, err := e.AddStrand(AddSpec{
		Cmd:     "sleep 300",
		Display: render.Display{Anchor: render.AnchorBelowParent},
	}); err != nil {
		t.Fatalf("AddStrand (stale-race third strand): %v", err)
	}

	live, err = e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes after mutating the pane set: %v", err)
	}
	if len(live) != len(beforeIDs)+1 {
		t.Fatalf("listPanes after the mutating AddStrand = %d pane(s), want %d (one more than before)", len(live), len(beforeIDs)+1)
	}

	startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)

	if gotLayout := windowLayoutNow(t, e); gotLayout == plannedLayout {
		t.Errorf("#{window_layout} after the stale-race attach = %q, want it to DIFFER from the planned string %q — tmux should have refused the pane-count mismatch rather than applying it", gotLayout, plannedLayout)
	}

	afterLive, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes after the stale-race attach: %v", err)
	}
	afterIDs := liveIDSet(afterLive)
	for id := range beforeIDs {
		if !afterIDs[id] {
			t.Errorf("pane %s present before the mutating AddStrand is missing after the stale-race attach — the refused select-layout must destroy nothing", id)
		}
	}
	// The mutation added exactly one pane and the refused select-layout must not have destroyed or
	// added any further pane of its own.
	if len(afterLive) != len(live) {
		t.Errorf("listPanes after the stale-race attach = %d pane(s), want %d (unchanged from just before the attach)", len(afterLive), len(live))
	}
}
