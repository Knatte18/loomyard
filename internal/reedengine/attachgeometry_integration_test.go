//go:build integration && linux

// attachgeometry_integration_test.go proves the attach handover lands the client-sized layout
// verbatim against a real tmux, real pty, and real attaching client — the tier-2 assertion that
// fails before this task (tmux's own rescale rewrites AttachArgv's planned window_layout string) and
// passes after (the chained select-layout runs post-attach, once the window already matches the
// client, so the string lands unchanged).
//
// Every case before this task's growth cases below drives a 100x30 client against the fixture's
// 220x50 boot box — SHORTER than the boot box in both dimensions — so every one of them exercises a
// window SHRINK at attach time and never the growth path this task is about. And the claim that the
// chained select-layout running post-attach is what holds the header and collapsed-strip budgets is
// incomplete on its own: it lands the layout string verbatim only at attach time. tmux has no
// fixed-height pane concept and redistributes every later window-size delta evenly across the
// vertical cells, so it is the window-resized resize-pin hook (windowsize.go), not the chain, that
// holds the budgets across any resize that happens afterward — the growth cases below pin that
// directly.
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
// pty deliberately unequal to the configured boot size — a 100x30 client, SHORTER in both dimensions
// than the fixture's 220x50 boot box, so this is a window SHRINK at attach time, never the growth
// path the cases below cover — it drives a real attach-session through the chained argv AttachArgv
// builds and asserts, from OUTSIDE the pty, that the live window becomes exactly the client's told
// size and that #{window_layout} equals the argv's own planned string byte for byte — tmux's silent
// proportional rescale is what this pins against. It then asserts, from the same attached session,
// that the header pane and the collapsed strip both landed at their configured, unclamped row
// budgets — true here only at attach time; see the growth cases below for what holds those budgets
// across a later resize.
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

// assertAttachGeometryRowBudgets asserts headerPaneID and parentPaneID are, respectively, at
// e.cfg.Header.HeightRows and e.cfg.CollapsedStripRows in the live pane set, failing with step
// prefixed onto every message so a caller checking the same budgets at two points in one test can
// tell which point failed.
func assertAttachGeometryRowBudgets(t *testing.T, e *Engine, headerPaneID, parentPaneID, step string) {
	t.Helper()
	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes (%s): %v", step, err)
	}
	var sawHeader, sawParent bool
	for _, p := range live {
		switch p.ID {
		case headerPaneID:
			sawHeader = true
			if p.Height != e.cfg.Header.HeightRows {
				t.Errorf("(%s) header pane %s height = %d, want %d (cfg.Header.HeightRows)", step, p.ID, p.Height, e.cfg.Header.HeightRows)
			}
		case parentPaneID:
			sawParent = true
			if p.Height != e.cfg.CollapsedStripRows {
				t.Errorf("(%s) collapsed parent pane %s height = %d, want %d (cfg.CollapsedStripRows)", step, p.ID, p.Height, e.cfg.CollapsedStripRows)
			}
		}
	}
	if !sawHeader {
		t.Fatalf("(%s) header pane %s missing from live panes %+v", step, headerPaneID, live)
	}
	if !sawParent {
		t.Fatalf("(%s) collapsed parent pane %s missing from live panes %+v", step, parentPaneID, live)
	}
}

// TestAttachGeometry_ResizeAfterAttachHoldsRowBudgets pins the fix this task ships: unlike
// TestAttachGeometry_ExactLayoutAndRowBudgets above, whose 100x30 client is SHORTER than the fixture's
// 220x50 boot box and so never exercises anything beyond attach time, this case resizes the pty AFTER
// a healthy chained attach has already landed the header and collapsed-strip budgets, to a materially
// TALLER size, and asserts both budgets still hold. This is the case that fails before this task: tmux
// has no fixed-height pane concept and redistributes every window-size delta evenly across the
// vertical cells with no intervention, and it is the window-resized resize-pin hook installed by this
// task — not the chained select-layout, which only ever runs once, at attach time — that holds the
// budgets across the resize.
func TestAttachGeometry_ResizeAfterAttachHoldsRowBudgets(t *testing.T) {
	e := setupAttachGeometryFixture(t)

	const cols, rows = 100, 30
	argv := e.AttachArgv(cols, rows)
	if len(argv) != 10 {
		t.Fatalf("AttachArgv(%d, %d) = %v (%d argv elements), want the 10-element chained form", cols, rows, argv, len(argv))
	}

	pty := startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)

	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
	}
	if len(st.Strands) != 2 {
		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
	}
	parentPaneID := st.Strands[0].PaneID
	headerPaneID := st.HeaderPaneID

	// Confirm the budgets landed at attach time, exactly as the exact-layout case above pins.
	assertAttachGeometryRowBudgets(t, e, headerPaneID, parentPaneID, "after attach")

	// Now drive a real client resize on the pty master — materially TALLER than the attach size, and
	// taller than the 220x50 boot box's 50 rows too, so this is unambiguously the growth path, never
	// a second shrink.
	const resizedCols, resizedRows = 100, 90
	if err := unix.IoctlSetWinsize(int(pty.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(resizedCols), Row: uint16(resizedRows)}); err != nil {
		t.Fatalf("TIOCSWINSZ (%dx%d): %v", resizedCols, resizedRows, err)
	}
	waitUntil(t, 15*time.Second, "window never reported the resized height", func() bool {
		_, h := windowSizeNow(t, e)
		return h == resizedRows
	})

	assertAttachGeometryRowBudgets(t, e, headerPaneID, parentPaneID, "after resize")
}

// TestAttachGeometry_BareAttachFromTallClientStillHoldsHeaderBudget covers the path the originally
// reported ~50-row threshold came from: a bare, unchained attach (AttachArgv(0, 0), the "no client
// size known" argv) from a client TALLER than the fixture's 220x50 boot box's 50-row height. It
// asserts the header pane still settles at e.cfg.Header.HeightRows once the attach settles.
//
// This case does NOT exercise an install of its own: AttachArgv(0, 0) returns the bare argv before
// the op lock is even taken, so it installs no window-resized hook. What holds the header here is the
// hook setupAttachGeometryFixture's own earlier AddStrand calls already installed via
// applyLayoutLocked — this case passes on that pre-existing hook, not on anything AttachArgv(0, 0)
// itself does, and must not be misread as proof that the no-client-size path installs one.
func TestAttachGeometry_BareAttachFromTallClientStillHoldsHeaderBudget(t *testing.T) {
	e := setupAttachGeometryFixture(t)

	argv := e.AttachArgv(0, 0)
	if len(argv) != 5 {
		t.Fatalf("AttachArgv(0, 0) = %v (%d argv elements), want the 5-element bare form with no chained select-layout", argv, len(argv))
	}

	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
	}
	headerPaneID := st.HeaderPaneID

	const cols, rows = 100, 80 // taller than the 220x50 boot box's 50 rows
	startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)

	waitUntil(t, 15*time.Second, "header pane never settled at cfg.Header.HeightRows after the bare attach", func() bool {
		live, err := e.tmux.listPanes(e.SessionName())
		if err != nil {
			return false
		}
		for _, p := range live {
			if p.ID == headerPaneID {
				return p.Height == e.cfg.Header.HeightRows
			}
		}
		return false
	})
}

// TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin pins the fire-time failure isolation the
// window-resized hook's array encoding buys (Shared Decision hook-body-is-one-array-entry-per-pin):
// after a healthy chained attach installs a hook pinning both the header and the collapsed parent
// strip, this kills the strip's own pane out from under the hook, leaving its array entry naming a
// destroyed pane id, then resizes and asserts the header still holds its budget — the header is
// always pin index 0, and independent array entries mean a later entry's failure cannot take an
// earlier one down with it (contract_integration_test.go's TestMultiplexerContract pins the same wire
// fact directly, at the set-hook level, with no pty involved).
func TestAttachGeometry_DeadStripPinDoesNotBreakHeaderPin(t *testing.T) {
	e := setupAttachGeometryFixture(t)

	const cols, rows = 100, 30
	argv := e.AttachArgv(cols, rows)
	if len(argv) != 10 {
		t.Fatalf("AttachArgv(%d, %d) = %v (%d argv elements), want the 10-element chained form", cols, rows, argv, len(argv))
	}

	pty := startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)

	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
	}
	if len(st.Strands) != 2 {
		t.Fatalf("st.Strands = %+v, want exactly 2 (the shrink-when-waiting parent and its child)", st.Strands)
	}
	headerPaneID := st.HeaderPaneID
	parentPaneID := st.Strands[0].PaneID

	// Kill the collapsed strip's own pane directly via e.tmux, bypassing RemoveStrand/reconcile
	// entirely: the installed hook's strip-pin entry now names a destroyed pane id, exactly the state
	// a stray operator kill or a crashed strand process would leave behind.
	if err := e.tmux.run("kill-pane", "-t", parentPaneID); err != nil {
		t.Fatalf("kill-pane -t %s: %v", parentPaneID, err)
	}

	const resizedCols, resizedRows = 100, 90
	if err := unix.IoctlSetWinsize(int(pty.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(resizedCols), Row: uint16(resizedRows)}); err != nil {
		t.Fatalf("TIOCSWINSZ (%dx%d): %v", resizedCols, resizedRows, err)
	}
	waitUntil(t, 15*time.Second, "window never reported the resized height", func() bool {
		_, h := windowSizeNow(t, e)
		return h == resizedRows
	})

	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes: %v", err)
	}
	var sawHeader bool
	for _, p := range live {
		if p.ID == headerPaneID {
			sawHeader = true
			if p.Height != e.cfg.Header.HeightRows {
				t.Errorf("header pane %s height = %d, want %d (cfg.Header.HeightRows) even though the strip pin's own pane was destroyed", p.ID, p.Height, e.cfg.Header.HeightRows)
			}
		}
	}
	if !sawHeader {
		t.Fatalf("header pane %s missing from live panes %+v", headerPaneID, live)
	}
}
