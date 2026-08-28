//go:build integration && linux

// watchdog_integration_test.go is the live reproduction of the M7 resize defect: it drives a real
// pty client's terminal size against a real tmux session and proves, from outside the pty, that the
// resize watch loop restores the planned layout in both directions with no lyx command run.
//
// Linux-only for the same reason attachgeometry_integration_test.go is: the pty harness this file
// reuses is built directly on golang.org/x/sys/unix's /dev/ptmx ioctls, which have no portable
// equivalent, and psmux's behaviour under a real pty is unverified anywhere in this repo.

package reedengine

import (
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// fastWatchTiming shortens watchDefaultTiming's SignalTick, PollCycle, and Quiet durations, which
// would otherwise make every test in this file needlessly slow; MaxAttempts and BaseDelay stay at
// their production values since no case here drives a failure streak.
func fastWatchTiming() watchTiming {
	t := watchDefaultTiming()
	t.SignalTick = 20 * time.Millisecond
	t.PollCycle = 150 * time.Millisecond
	t.Quiet = 120 * time.Millisecond
	return t
}

// watchdogFixture bundles the live pieces a resize self-heal scenario drives: the booted engine and
// the pty client currently attached to it.
type watchdogFixture struct {
	e   *Engine
	pty *attachGeometryPTY
}

// attachWatchdogClient attaches a fresh pty client to e at cols x rows via the chained AttachArgv and
// waits for tmux to report it attached.
// AttachArgv's own pre-flight is what actually calls pinGeometryOptionsLocked under the op lock, so
// this is "the one op that reaches it" this file's card requires — by the time this returns, the
// window-resized hook is genuinely installed against the live session, not merely configured.
func attachWatchdogClient(t *testing.T, e *Engine, cols, rows int) *attachGeometryPTY {
	t.Helper()
	argv := e.AttachArgv(cols, rows)
	pty := startInPTY(t, append([]string{e.cfg.Tmux}, argv...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)
	return pty
}

// bootWatchdogFixture boots setupAttachGeometryFixture's two-strand session (a header pane, a
// collapsed parent, and a full child), turns the watchdog on, and attaches a pty client at
// cols x rows.
func bootWatchdogFixture(t *testing.T, cols, rows int) *watchdogFixture {
	t.Helper()
	e := setupAttachGeometryFixture(t)
	e.cfg.Watchdog = "on"

	pty := attachWatchdogClient(t, e, cols, rows)
	return &watchdogFixture{e: e, pty: pty}
}

// resizePTY drives unix.TIOCSWINSZ on pty's master to cols x rows, then delivers SIGWINCH to the
// attached client process — the same two-step sequence a real terminal emulator performs on a live
// resize, and the one that fires tmux's window-resized hook (doc.go: client-resized reports the stale
// pre-resize size and window-layout-changed is self-triggering, so neither serves this loop).
func resizePTY(t *testing.T, pty *attachGeometryPTY, cols, rows int) {
	t.Helper()
	fd := int(pty.master.Fd())
	if err := unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)}); err != nil {
		t.Fatalf("TIOCSWINSZ (%dx%d): %v", cols, rows, err)
	}
	if err := pty.cmd.Process.Signal(syscall.SIGWINCH); err != nil {
		t.Fatalf("SIGWINCH: %v", err)
	}
}

// headerPaneHeightNow reports the live header pane's current row count and whether it could be
// determined at all — false while state or the live pane list cannot be read, or the header pane is
// momentarily absent from either.
func headerPaneHeightNow(t *testing.T, e *Engine) (height int, ok bool) {
	t.Helper()
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil || st.HeaderPaneID == "" {
		return 0, false
	}
	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		return 0, false
	}
	for _, p := range live {
		if p.ID == st.HeaderPaneID {
			return p.Height, true
		}
	}
	return 0, false
}

// activePaneID reports the tmux pane id display-message resolves as the window's active pane — the
// target-pane default for a window-scoped -t, per tmux's own resolution rules.
func activePaneID(t *testing.T, e *Engine) string {
	t.Helper()
	out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{pane_id}")
	if err != nil {
		t.Fatalf("display-message #{pane_id}: %v", err)
	}
	return strings.TrimSpace(out)
}

// expectedLayoutForCurrentBox recomputes the layout string the engine would plan for the session's
// current persisted state, live pane set, and live window box — the same inputs
// applyLayoutLockedOpts itself uses — so a test can assert the watcher's own #{window_layout} answer
// against it. ok is false when state or the live pane list cannot be read.
func expectedLayoutForCurrentBox(t *testing.T, e *Engine) (layout string, ok bool) {
	t.Helper()
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		return "", false
	}
	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		return "", false
	}
	w, h := windowSizeNow(t, e)
	layout, _, err = e.planLayout(st, live, render.Box{W: w, H: h})
	if err != nil {
		return "", false
	}
	return layout, true
}

// assertLayoutSelfHeals resizes fx's pty client to newCols x newRows and asserts that, within a
// bounded wait, the live #{window_layout} becomes exactly what the engine plans for the new box and
// the header pane is back to exactly cfg.Header.HeightRows rows — the M7 assertion, driven in
// whichever direction the caller resizes.
func assertLayoutSelfHeals(t *testing.T, fx *watchdogFixture, newCols, newRows int) {
	t.Helper()
	e := fx.e
	resizePTY(t, fx.pty, newCols, newRows)

	waitUntil(t, 15*time.Second, "layout never self-healed after the resize", func() bool {
		w, h := windowSizeNow(t, e)
		if w != newCols || h != newRows {
			return false
		}
		wantLayout, ok := expectedLayoutForCurrentBox(t, e)
		if !ok || windowLayoutNow(t, e) != wantLayout {
			return false
		}
		height, ok := headerPaneHeightNow(t, e)
		return ok && height == e.cfg.Header.HeightRows
	})
}

// TestWatchdogSelfHeal_GrowsBackToPlannedLayout drives a live window growth and asserts the layout
// re-applies to exactly the planned string for the new, larger box.
func TestWatchdogSelfHeal_GrowsBackToPlannedLayout(t *testing.T) {
	fx := bootWatchdogFixture(t, 100, 30)
	startWatchLoop(t, fx.e, fastWatchTiming())
	assertLayoutSelfHeals(t, fx, 140, 45)
}

// TestWatchdogSelfHeal_ShrinksBackToPlannedLayout drives a live window shrink and asserts the same.
// This case is non-negotiable: it is the one SIGWINCH misses entirely, and a watcher passing only the
// grow case above is the failure mode this task must not ship.
func TestWatchdogSelfHeal_ShrinksBackToPlannedLayout(t *testing.T) {
	fx := bootWatchdogFixture(t, 100, 30)
	startWatchLoop(t, fx.e, fastWatchTiming())
	assertLayoutSelfHeals(t, fx, 100, 16)
}

// TestWatchdogSelfHeal_BurstCoalesces drives a rapid succession of resizes and asserts the layout
// converges to the final size while the number of distinct #{window_layout} values observed across
// the burst stays far below one per resize event — the live counterpart of the debounce's
// trailing-edge coalescing.
func TestWatchdogSelfHeal_BurstCoalesces(t *testing.T) {
	timing := fastWatchTiming()
	fx := bootWatchdogFixture(t, 100, 30)
	e := fx.e
	startWatchLoop(t, e, timing)

	sizes := []struct{ cols, rows int }{
		{100, 32}, {100, 34}, {100, 36}, {100, 38}, {100, 40}, {100, 42}, {100, 44}, {100, 46},
	}
	finalCols, finalRows := sizes[len(sizes)-1].cols, sizes[len(sizes)-1].rows

	// Sample #{window_layout} throughout the burst and the settle window, counting distinct values
	// seen. The sampling goroutine never touches t beyond the read-only closure variables it
	// captures — a failing sample is simply not counted, never a fatal test failure, since Fatal must
	// only ever be called from the test's own goroutine.
	var mu sync.Mutex
	seen := map[string]bool{}
	stopSampling := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(15 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopSampling:
				return
			case <-ticker.C:
				out, err := e.tmux.output("display-message", "-p", "-t", exactSessionWindowTarget(e.SessionName()), "#{window_layout}")
				if err != nil {
					continue
				}
				mu.Lock()
				seen[strings.TrimSpace(out)] = true
				mu.Unlock()
			}
		}
	}()

	for _, s := range sizes {
		resizePTY(t, fx.pty, s.cols, s.rows)
		time.Sleep(20 * time.Millisecond)
	}

	waitUntil(t, 15*time.Second, "burst never converged to the final size's layout", func() bool {
		w, h := windowSizeNow(t, e)
		if w != finalCols || h != finalRows {
			return false
		}
		height, ok := headerPaneHeightNow(t, e)
		return ok && height == e.cfg.Header.HeightRows
	})

	close(stopSampling)
	wg.Wait()

	mu.Lock()
	distinct := len(seen)
	mu.Unlock()
	if distinct > len(sizes)/2 {
		t.Errorf("observed %d distinct #{window_layout} values across the burst, want far fewer than the %d resize events driven — the debounce should have coalesced them into a small number of settled applies", distinct, len(sizes))
	}
}

// TestWatchdogSelfHeal_DegradedPathStillConverges makes the window-resized hook uninstallable after
// boot and scripts nothing else, so the loop's own first probe finds it absent and never promotes out
// of poll mode; the poll fallback must still heal the layout after a resize, within a bounded wait
// proportional to PollCycle.
func TestWatchdogSelfHeal_DegradedPathStillConverges(t *testing.T) {
	fx := bootWatchdogFixture(t, 100, 30)
	e := fx.e

	target := exactSessionWindowTarget(e.SessionName())
	if err := e.tmux.run("set-hook", "-u", "-t", target, windowResizedHookName); err != nil {
		t.Fatalf("set-hook -u (make the hook uninstallable): %v", err)
	}

	timing := fastWatchTiming()
	startWatchLoop(t, e, timing)

	resizePTY(t, fx.pty, 130, 42)

	waitUntil(t, 10*timing.PollCycle+5*time.Second, "poll-mode fallback never healed the layout after the resize", func() bool {
		w, h := windowSizeNow(t, e)
		if w != 130 || h != 42 {
			return false
		}
		height, ok := headerPaneHeightNow(t, e)
		return ok && height == e.cfg.Header.HeightRows
	})
}

// TestWatchdogSelfHeal_SurvivesInducedTmuxFailure kills the session out from under the running loop,
// waits past several ticks, asserts the loop's own goroutine has not returned, then boots a fresh
// session and asserts the loop is still functioning.
func TestWatchdogSelfHeal_SurvivesInducedTmuxFailure(t *testing.T) {
	timing := fastWatchTiming()
	fx := bootWatchdogFixture(t, 100, 30)
	e := fx.e
	_, loopDone := startWatchLoop(t, e, timing)

	if err := e.tmux.run("kill-session", "-t", exactSessionTarget(e.SessionName())); err != nil {
		t.Fatalf("kill-session (induced failure): %v", err)
	}

	// Every reapplyLayout call in this window fails requireSessionLocked and is only ever logged,
	// never fatal (Shared Decision watch-loop-failures-are-never-fatal) — wait past several such
	// ticks before checking the loop is still alive.
	time.Sleep(8 * timing.PollCycle)

	select {
	case <-loopDone:
		t.Fatal("watch loop returned after the session was killed out from under it — it must keep running and stay responsive to the next signal")
	default:
	}

	if _, err := e.Resume(); err != nil {
		t.Fatalf("Resume (rebuild after the induced failure): %v", err)
	}
	fx.pty = attachWatchdogClient(t, e, 100, 30)

	assertLayoutSelfHeals(t, fx, 125, 38)
}

// TestWatchdogSelfHeal_FocusNeverStolen selects a specific pane as live-active that is not the
// persisted table's own focus resolution, resizes, and asserts that pane is still active afterwards —
// only a real session can demonstrate this, since the watcher's re-apply always suppresses the focus
// half (SkipFocus: true) and this proves that suppression actually holds against real tmux.
func TestWatchdogSelfHeal_FocusNeverStolen(t *testing.T) {
	fx := bootWatchdogFixture(t, 100, 30)
	e := fx.e
	startWatchLoop(t, e, fastWatchTiming())

	st, err := LoadState(e.stateDir())
	if err != nil || st == nil || len(st.Strands) < 2 {
		t.Fatalf("LoadState = (%+v, %v), want at least 2 strands", st, err)
	}
	// The child strand, last in the fixture's build order, not the persisted table's own focus
	// resolution — selecting it makes a stolen focus visibly detectable.
	childPaneID := st.Strands[len(st.Strands)-1].PaneID
	if err := e.tmux.run("select-pane", "-t", childPaneID); err != nil {
		t.Fatalf("select-pane -t %s: %v", childPaneID, err)
	}

	resizePTY(t, fx.pty, 150, 44)
	waitUntil(t, 15*time.Second, "layout never settled after the resize", func() bool {
		height, ok := headerPaneHeightNow(t, e)
		return ok && height == e.cfg.Header.HeightRows
	})

	if got := activePaneID(t, e); got != childPaneID {
		t.Errorf("active pane after resize = %q, want %q (the watcher's re-apply must never steal focus)", got, childPaneID)
	}
}

// TestWatchdogSelfHeal_NoSelfTriggerLoop asserts that once the watcher's own apply has settled,
// #{window_layout} stops changing with no further client resize — the live counterpart of the
// box-equality guard and of doc.go's select-layout-does-not-fire-window-resized probe.
func TestWatchdogSelfHeal_NoSelfTriggerLoop(t *testing.T) {
	timing := fastWatchTiming()
	fx := bootWatchdogFixture(t, 100, 30)
	e := fx.e
	startWatchLoop(t, e, timing)

	resizePTY(t, fx.pty, 145, 41)
	waitUntil(t, 15*time.Second, "layout never settled after the resize", func() bool {
		height, ok := headerPaneHeightNow(t, e)
		return ok && height == e.cfg.Header.HeightRows
	})

	settled := windowLayoutNow(t, e)

	// No further client resize happens here — select-layout does not itself fire window-resized
	// (doc.go), so a self-triggering watcher would show up as the layout drifting with nothing new to
	// react to.
	time.Sleep(10 * timing.SignalTick)

	if got := windowLayoutNow(t, e); got != settled {
		t.Errorf("#{window_layout} drifted from %q to %q with no new client resize — the watcher must never self-trigger", settled, got)
	}
}

// TestWatchdogSelfHeal_HookProbeMatchesLiveTmux asserts hookInstalledLocked reports (true, true)
// after pinGeometryOptionsLocked has already run against the real session via AttachArgv's own
// pre-flight. This is the assertion that catches any tmux-side normalisation of the hook command
// string that would silently pin every watcher into poll mode on Linux, which no tier-1 test can see.
func TestWatchdogSelfHeal_HookProbeMatchesLiveTmux(t *testing.T) {
	fx := bootWatchdogFixture(t, 100, 30)
	e := fx.e

	var installed, known bool
	if err := e.withOpLock(func() error {
		installed, known = e.hookInstalledLocked()
		return nil
	}); err != nil {
		t.Fatalf("withOpLock(hookInstalledLocked): %v", err)
	}
	if !known {
		t.Fatal("hookInstalledLocked() known = false, want true — pinGeometryOptionsLocked already ran against the real session via AttachArgv's own pre-flight")
	}
	if !installed {
		t.Error("hookInstalledLocked() installed = false, want true — reed's own hook command must round-trip byte-identically through show-options against a live tmux")
	}
}
