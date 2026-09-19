//go:build integration

// watchdogreap_integration_test.go carries the orphan-reap tier's live assertions: the end-to-end
// reap and healthy-sibling non-interference, the never-entered orphan, the empty-shell degradation,
// the reap's process half, and loop liveness under a reap in flight.
//
// It is a separate file from watchdog_integration_test.go, rather than more cases appended to it, so
// the reap's own fixture and helpers stay together rather than growing that file's unrelated
// discovery/idle-exit/re-entry fixture shape.
//
// Every case drives runWatchdogLoop directly, in-process, against a hub built through hubforge (per
// the hubforge Fabric-Fixture Invariant) with compressed timings from compressedReapTiming, and never
// spawns a `lyx reed watchdog` process of any kind: CONSTRAINTS.md's Live-Substrate Spawn
// Observability rule bars re-execing the test binary, which is exactly what a live daemon spawn would
// do under `go test` (see watchdog_integration_test.go's file-level comment), and suppressWatchdogSpawn
// exists to prevent it.
package reedcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// reapFixture is the tagged reap tier's shared shape: a hub built through hubforge, one booted
// session per worktree (prime first), and the tmux binary they all share.
type reapFixture struct {
	hub       *hubforge.Hub
	engines   []*reedengine.Engine
	worktrees []string
	tmuxPath  string
}

// newReapFixture builds a hub through hubforge.NewHub, adds one pair per name in pairNames, and
// boots a session for the prime worktree and for each pair. engines and worktrees are index-aligned,
// prime first, so a case can orphan worktrees[1] and still assert against engines[0].
func newReapFixture(t *testing.T, pairNames ...string) reapFixture {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	for _, name := range pairNames {
		hubforge.AddPair(t, h, name)
	}

	engines := make([]*reedengine.Engine, 0, len(pairNames)+1)
	worktrees := make([]string, 0, len(pairNames)+1)

	primeWorktree := h.PrimeWorktree()
	primeEngine := watchdogIntegrationEngine(t, primeWorktree)
	engines = append(engines, primeEngine)
	worktrees = append(worktrees, primeWorktree)

	for _, name := range pairNames {
		worktree := h.PairWarpWorktree(name)
		engines = append(engines, watchdogIntegrationEngine(t, worktree))
		worktrees = append(worktrees, worktree)
	}

	return reapFixture{
		hub:       h,
		engines:   engines,
		worktrees: worktrees,
		tmuxPath:  primeEngine.TmuxPath(),
	}
}

// compressedReapTiming returns a watchdogTiming whose DiscoveryCycle is compressed to tens of
// milliseconds while IdleCycles and OrphanGoneCycles stay at their production values — the cadence
// compresses, never the confirmation count, so every case still proves the three-consecutive-cycle
// rule rather than bypassing it.
func compressedReapTiming() watchdogTiming {
	return watchdogTiming{
		DiscoveryCycle:   30 * time.Millisecond,
		IdleCycles:       watchdogHubIdleCycles,
		OrphanGoneCycles: watchdogOrphanGoneCycles,
	}
}

// orphanWorktree removes worktreeRoot's directory from disk while its tmux session stays live on the
// hub socket — the exact state the reap exists to clean up.
func orphanWorktree(t *testing.T, worktreeRoot string) {
	t.Helper()
	if err := exec.Command("rm", "-rf", worktreeRoot).Run(); err != nil {
		t.Fatalf("orphanWorktree: rm -rf %s: %v", worktreeRoot, err)
	}
}

// reapSessionListed reports whether session is currently live on tmuxPath's socket.
func reapSessionListed(t *testing.T, tmuxPath, socket, session string) bool {
	t.Helper()
	names, err := reedengine.ListSessions(tmuxPath, socket)
	if err != nil {
		return false
	}
	for _, n := range names {
		if n == session {
			return true
		}
	}
	return false
}

// reapListPanePIDs lists session's live #{pane_pid} values directly, bypassing the engine entirely
// so a case can snapshot a session's pane process pids from outside any Engine — mirroring
// watchdogWindowSize's own bypass-the-engine pattern in watchdog_integration_test.go.
func reapListPanePIDs(t *testing.T, tmuxPath, socket, session string) []int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-t", "="+session+":", "-F", "#{pane_pid}").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

// waitProcessGone polls until pid is no longer alive (per proc.IsAlive) or timeout elapses, failing
// the test on timeout — the tagged tier's own confirmed-exit check, mirroring
// internal/reedengine/lifecycle.go's waitProcessExit but usable from this package.
func waitProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	waitForCondition(t, timeout, func() bool {
		return !proc.IsAlive(pid)
	})
}

// startReapLoop starts runWatchdogLoop in the background against fx's hub/tmux and returns its
// cancel func plus a channel receiving its return error once it stops.
func startReapLoop(fx reapFixture, shellPath string, timing watchdogTiming) (context.CancelFunc, chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runWatchdogLoop(ctx, fx.hub.Path, fx.tmuxPath, shellPath, timing)
	}()
	return cancel, done
}

// TestWatchdogReap_EndToEndAndSiblingSurvives boots two worktree sessions on one hub, orphans one,
// drives the loop with compressed timing, and asserts in one test that the orphan is fully reaped
// (session gone, pane processes confirmed exited) while the healthy sibling on the same hub socket is
// untouched — a broken exact-match kill target takes out the prefix-sharing sibling, and that must
// fail loudly in the same run that proves the reap works.
func TestWatchdogReap_EndToEndAndSiblingSurvives(t *testing.T) {
	fx := newReapFixture(t, "reap-e2e-sibling")
	socket := reedengine.ServerName(fx.hub.Path)

	orphanEng := fx.engines[1]
	siblingEng := fx.engines[0]
	orphanSession := orphanEng.SessionName()
	siblingSession := siblingEng.SessionName()

	beforePIDs := reapListPanePIDs(t, fx.tmuxPath, socket, orphanSession)
	if len(beforePIDs) == 0 {
		t.Fatalf("reapListPanePIDs(%s) before reap = empty; want at least one live pane pid", orphanSession)
	}

	orphanWorktree(t, fx.worktrees[1])

	timing := compressedReapTiming()
	cancel, loopDone := startReapLoop(fx, orphanEng.ShellPath(), timing)
	defer cancel()

	// The timing rule: the session must still be live before OrphanGoneCycles affirmative cycles
	// have elapsed. A reap firing on the first observation must fail this check.
	time.Sleep(time.Duration(timing.OrphanGoneCycles) * timing.DiscoveryCycle / 2)
	if !reapSessionListed(t, fx.tmuxPath, socket, orphanSession) {
		t.Errorf("orphan session %s was reaped before %d affirmative cycles elapsed; want it still live", orphanSession, timing.OrphanGoneCycles)
	}

	waitForCondition(t, timing.DiscoveryCycle*time.Duration(timing.OrphanGoneCycles)*20, func() bool {
		return !reapSessionListed(t, fx.tmuxPath, socket, orphanSession)
	})

	for _, pid := range beforePIDs {
		waitProcessGone(t, pid, 20*time.Second)
	}

	if !reapSessionListed(t, fx.tmuxPath, socket, siblingSession) {
		t.Errorf("sibling session %s did not survive the orphan's reap; want it still listed", siblingSession)
	}

	cancel()
	<-loopDone
}

// TestWatchdogReap_NeverEnteredOrphanIsStillReaped covers the case enterSession can structurally
// never reach: the worktree directory is already gone before runWatchdogLoop's first cycle runs, so
// resolveWatchedSession's git spawn fails every cycle and the name is never in known. It must still
// be reaped, because the reap reads the live session-name list rather than the daemon's own known
// map.
func TestWatchdogReap_NeverEnteredOrphanIsStillReaped(t *testing.T) {
	fx := newReapFixture(t, "reap-never-entered")
	socket := reedengine.ServerName(fx.hub.Path)

	orphanEng := fx.engines[1]
	orphanSession := orphanEng.SessionName()

	// Gone before the loop is ever started: the daemon's first cycle finds a live session name
	// whose worktree already does not exist, so enterSession can never succeed for it.
	orphanWorktree(t, fx.worktrees[1])

	timing := compressedReapTiming()
	cancel, loopDone := startReapLoop(fx, orphanEng.ShellPath(), timing)
	defer cancel()

	waitForCondition(t, timing.DiscoveryCycle*time.Duration(timing.OrphanGoneCycles)*20, func() bool {
		return !reapSessionListed(t, fx.tmuxPath, socket, orphanSession)
	})

	cancel()
	<-loopDone
}

// TestWatchdogReap_EmptyShellStillReaps proves the discussion's degrade-never-refuse decision at the
// live tier: runWatchdogLoop is driven directly with an empty shellPath against a live hub socket and
// an orphaned worktree, and the session's pane ROOT pids (never their descendants — see this test's
// doc, and card 18's requirements) are confirmed exited. Asserting only the roots keeps this case
// asserting the guarantee an empty shell makes on every platform: descendantClosurePIDs' Windows body
// degrades to returning the roots unchanged when its probe cannot spawn, while its Linux body (this
// suite's own platform) reads no Engine field at all and would in fact walk descendants regardless of
// shellPath — so asserting descendants here would pass for a reason specific to Linux, not the
// cross-platform guarantee this case exists to pin.
func TestWatchdogReap_EmptyShellStillReaps(t *testing.T) {
	fx := newReapFixture(t, "reap-empty-shell")
	socket := reedengine.ServerName(fx.hub.Path)

	orphanEng := fx.engines[1]
	orphanSession := orphanEng.SessionName()

	rootPIDs := reapListPanePIDs(t, fx.tmuxPath, socket, orphanSession)
	if len(rootPIDs) == 0 {
		t.Fatalf("reapListPanePIDs(%s) before reap = empty; want at least one live pane root pid", orphanSession)
	}

	orphanWorktree(t, fx.worktrees[1])

	timing := compressedReapTiming()
	cancel, loopDone := startReapLoop(fx, "", timing)
	defer cancel()

	waitForCondition(t, timing.DiscoveryCycle*time.Duration(timing.OrphanGoneCycles)*20, func() bool {
		return !reapSessionListed(t, fx.tmuxPath, socket, orphanSession)
	})

	for _, pid := range rootPIDs {
		waitProcessGone(t, pid, 20*time.Second)
	}

	cancel()
	<-loopDone
}

// writeReapScript writes a small executable bash script to path, failing the test on error.
func writeReapScript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writeReapScript(%s): %v", path, err)
	}
}

// reapDescendantScript builds, on disk under dir, a three-level process chain and returns an
// AddSpec.Cmd that launches it: the pane's own process (root), a child detached into its own session
// via setsid (a pane child), and a grandchild that child forks in turn (a descendant of a pane
// child). Each level writes its own pid to its own file via "echo $$" as its first act.
//
// setsid is what makes this prove the wait-then-force-kill sequence rather than an ordinary hangup:
// a plain backgrounded job stays in the pane's own process group and dies from the same SIGHUP
// tmux's kill-session delivers to that whole group, which would make this case pass even with a
// reordered implementation that computes the descendant closure after the kill. Detaching into a
// brand-new session is what makes the child (and through it, the grandchild) immune to that group
// signal, surviving the pane's own death exactly the way an orphaned agent descendant would — so only
// the explicit descendant-closure kill reaps them.
//
// setsid is spawned from its own separate, non-interactive script file (level1.sh calling
// level1b.sh), rather than inline in the pane's own top-level script: setsid() fails EPERM against a
// caller that is already a process-group leader, which an inline foreground fork under the pane's own
// interactive, job-control-enabled shell can be, silently forcing setsid's own fallback double-fork
// and exiting its immediate child right away. A plain "bash <file>" invocation with job control off
// never assigns the forked setsid call its own process group first, so setsid() succeeds directly and
// every level here stays exactly one real process, with no premature exit.
func reapDescendantScript(t *testing.T, dir, rootFile, childFile, grandchildFile string) string {
	t.Helper()

	level2 := filepath.Join(dir, "level2.sh")
	level1b := filepath.Join(dir, "level1b.sh")
	level1 := filepath.Join(dir, "level1.sh")
	level0 := filepath.Join(dir, "level0.sh")

	writeReapScript(t, level2, fmt.Sprintf("#!/bin/bash\necho $$ > %s\nsleep 300\n", grandchildFile))
	writeReapScript(t, level1b, fmt.Sprintf("#!/bin/bash\necho $$ > %s\nbash %s &\nGPID=$!\nwait $GPID\n", childFile, level2))
	writeReapScript(t, level1, fmt.Sprintf("#!/bin/bash\nsetsid bash %s &\nCPID=$!\nwait $CPID\n", level1b))
	writeReapScript(t, level0, fmt.Sprintf("#!/bin/bash\necho $$ > %s\nbash %s\n", rootFile, level1))

	return "bash " + level0
}

// readPIDFile reads an int pid written by reapDescendantScript's "echo $$ > file" steps, waiting for
// the file to appear since the pane's shell writes it asynchronously after AddStrand returns.
func readPIDFile(t *testing.T, path string) int {
	t.Helper()
	var pid int
	waitForCondition(t, 10*time.Second, func() bool {
		out, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return false
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return false
		}
		pid = parsed
		return true
	})
	return pid
}

// TestWatchdogReap_DescendantClosureConfirmedExited is the only tier that exercises the descendant
// closure and the wait-then-force-kill sequence at all: it drives an end-to-end reap against a
// session whose pane grew a background descendant that never receives kill-session's own hangup, and
// asserts the descendant is confirmed exited (via proc.IsAlive, not merely signalled) after the reap
// returns. This is what proves the closure was computed before the kill: a closure computed after
// kill-session collapses to the pane roots alone and would leave this descendant alive forever, since
// nothing else in this test ever signals it directly.
func TestWatchdogReap_DescendantClosureConfirmedExited(t *testing.T) {
	fx := newReapFixture(t, "reap-descendant")
	socket := reedengine.ServerName(fx.hub.Path)

	orphanEng := fx.engines[1]
	orphanSession := orphanEng.SessionName()

	dir := t.TempDir()
	rootFile := filepath.Join(dir, "root.pid")
	childFile := filepath.Join(dir, "child.pid")
	grandchildFile := filepath.Join(dir, "grandchild.pid")
	script := reapDescendantScript(t, dir, rootFile, childFile, grandchildFile)

	if _, err := orphanEng.AddStrand(reedengine.AddSpec{Cmd: script, Display: render.Display{Anchor: render.AnchorBelowParent}}); err != nil {
		t.Fatalf("AddStrand(descendant script): %v", err)
	}

	childPID := readPIDFile(t, childFile)
	grandchildPID := readPIDFile(t, grandchildFile)
	if !proc.IsAlive(childPID) {
		t.Fatalf("pane child pid %d not alive before reap", childPID)
	}
	if !proc.IsAlive(grandchildPID) {
		t.Fatalf("descendant-of-a-pane-child pid %d not alive before reap", grandchildPID)
	}

	orphanWorktree(t, fx.worktrees[1])

	timing := compressedReapTiming()
	cancel, loopDone := startReapLoop(fx, orphanEng.ShellPath(), timing)
	defer cancel()

	waitForCondition(t, timing.DiscoveryCycle*time.Duration(timing.OrphanGoneCycles)*20, func() bool {
		return !reapSessionListed(t, fx.tmuxPath, socket, orphanSession)
	})

	// The graceful wait (reapExitTimeout, 15s) plus the force-kill grace (forceKillExitGrace, 5s)
	// bound how long a resistant background descendant can survive the reap; wait generously beyond
	// both.
	waitProcessGone(t, childPID, 25*time.Second)
	waitProcessGone(t, grandchildPID, 25*time.Second)

	cancel()
	<-loopDone
}

// TestWatchdogReap_LoopStaysLiveDuringReap is the regression guard for running the reap off-loop
// rather than inline. With a reap dispatched and still in flight, it asserts the discovery loop keeps
// ticking and enters a session that appears after the dispatch, that cancelling the daemon's context
// returns promptly rather than being tied to the reap's own graceful-plus-force budget, and that the
// reaped name leaves the in-flight set on a later tick: a session re-created under the exact same
// name is reaped again when orphaned a second time, which an implementation leaking the name in the
// in-flight set forever could never do (planReapCycle skips any name still marked in-flight,
// permanently).
//
// Both this case and TestWatchdogReap_DescendantClosureConfirmedExited run under the -race flag the
// batch verify carries: the in-flight set, the gone-counter map and the known map are all written
// only on the loop goroutine, with the reap goroutines' sole cross-goroutine act being a non-blocking
// channel send, so a correct implementation leaves nothing for the race detector to find here.
func TestWatchdogReap_LoopStaysLiveDuringReap(t *testing.T) {
	fx := newReapFixture(t, "reap-inflight-a")
	socket := reedengine.ServerName(fx.hub.Path)

	orphanEng := fx.engines[1]
	orphanSession := orphanEng.SessionName()
	orphanWorktreeRoot := fx.worktrees[1]

	// Renamed aside rather than removed outright (unlike the shared orphanWorktree helper): this
	// case needs to restore the exact same worktree, with its git worktree admin data untouched, so
	// it can be re-orphaned a second time under the identical session name later in this same test.
	restore := renameWorktreeAway(t, orphanWorktreeRoot)

	timing := compressedReapTiming()
	cancel, loopDone := startReapLoop(fx, orphanEng.ShellPath(), timing)
	defer cancel()

	dispatchStart := time.Now()
	waitForCondition(t, timing.DiscoveryCycle*time.Duration(timing.OrphanGoneCycles)*20, func() bool {
		return !reapSessionListed(t, fx.tmuxPath, socket, orphanSession)
	})

	// A newly-appeared session, added right as the first reap is in flight, must still be
	// discovered — the loop must not stall behind the dispatched reap goroutine.
	hubforge.AddPair(t, fx.hub, "reap-inflight-b")
	freshWorktree := fx.hub.PairWarpWorktree("reap-inflight-b")
	freshEng := watchdogIntegrationEngine(t, freshWorktree)
	waitForCondition(t, timing.DiscoveryCycle*10, func() bool {
		return reapSessionListed(t, fx.tmuxPath, socket, freshEng.SessionName())
	})

	select {
	case err := <-loopDone:
		t.Fatalf("runWatchdogLoop exited while the fresh sibling was still live (err=%v); want it still running", err)
	default:
	}

	// Re-create a session under the exact same orphan name, against the SAME still-running loop,
	// and orphan it again: a second reap succeeding here proves the first reap's in-flight entry
	// was cleared on a later tick rather than left set forever — planReapCycle skips any name still
	// marked in-flight, permanently, so a leak here would make this second reap never happen.
	restore()
	secondEng := watchdogIntegrationEngine(t, orphanWorktreeRoot)
	if secondEng.SessionName() != orphanSession {
		t.Fatalf("re-created session name = %q, want %q (same worktree, same session identity)", secondEng.SessionName(), orphanSession)
	}
	orphanWorktree(t, orphanWorktreeRoot)

	waitForCondition(t, timing.DiscoveryCycle*time.Duration(timing.OrphanGoneCycles)*20, func() bool {
		return !reapSessionListed(t, fx.tmuxPath, socket, orphanSession)
	})

	// Cancelling must not be tied to either reap's own graceful-plus-force budget: the loop's
	// select observes ctx.Done() on its very next iteration regardless of a reap in flight, so the
	// whole call returns in a small fraction of that budget rather than only after it elapses.
	cancelDeadline := 5 * time.Second
	cancel()
	select {
	case <-loopDone:
	case <-time.After(cancelDeadline):
		t.Fatalf("runWatchdogLoop did not return within %s of cancel (first dispatch was %s earlier); want cancellation independent of either reap's own budget", cancelDeadline, time.Since(dispatchStart))
	}
}

// renameWorktreeAway renames worktreeRoot's directory aside so lyxcwd resolution and
// worktreeRootGone both see it as gone at its original path, and returns a restore func that moves
// it back to the exact same path — leaving its git worktree admin data untouched throughout, unlike
// the shared orphanWorktree helper's irreversible rm -rf.
func renameWorktreeAway(t *testing.T, worktreeRoot string) func() {
	t.Helper()
	backup := worktreeRoot + ".reap-backup"
	if err := os.Rename(worktreeRoot, backup); err != nil {
		t.Fatalf("renameWorktreeAway: rename %s aside: %v", worktreeRoot, err)
	}
	return func() {
		t.Helper()
		if err := os.Rename(backup, worktreeRoot); err != nil {
			t.Fatalf("renameWorktreeAway: restore %s: %v", worktreeRoot, err)
		}
	}
}
