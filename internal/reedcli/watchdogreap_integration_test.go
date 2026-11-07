// watchdogreap_integration_test.go holds the orphan-reap tier's live assertions as one TestWatchdogReap scenario.
// The scenario covers the reap end to end, healthy-sibling non-interference, and a never-entered orphan.
// It also covers the empty-shell degradation, the reap's process half, and loop liveness under a reap in flight.
//
// The reap's fixture and helpers live here, apart from watchdog_integration_test.go's discovery and re-entry cases.
//
// Every step drives runWatchdogLoop directly, in-process, against a hub built through hubforge.
// The hub follows the hubforge Fabric-Fixture Invariant, and timings come from compressedReapTiming.
// No step spawns a `lym reed watchdog` process of any kind.
// A live daemon spawn would re-exec the test binary under `go test`, which PATTERN-child-processes bars.
// suppressWatchdogSpawn exists to prevent that spawn.
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

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// reapFixture is the tagged reap tier's shared shape: a hub built through hubforge, the booted sessions of the prime worktree and one pair worktree (prime first), and the tmux binary they share.
type reapFixture struct {
	hub       *hubforge.Hub
	engines   []*reedengine.Engine
	worktrees []string
	tmuxPath  string
}

// newReapFixture adds pair pairName to hub h and boots its session, pairing it with the already-booted prime engine.
// engines and worktrees are index-aligned, prime first,
// so a step can orphan worktrees[1] and still assert against engines[0].
func newReapFixture(t *testing.T, h *hubforge.Hub, primeEngine *reedengine.Engine, pairName string) reapFixture {
	t.Helper()

	hubforge.AddPair(t, h, pairName)
	pairWorktree := h.PairCodeWorktree(pairName)
	pairEngine := watchdogIntegrationEngine(t, pairWorktree)

	return reapFixture{
		hub:       h,
		engines:   []*reedengine.Engine{primeEngine, pairEngine},
		worktrees: []string{h.PrimeWorktree(), pairWorktree},
		tmuxPath:  primeEngine.TmuxPath(),
	}
}

// compressedReapTiming returns a watchdogTiming with a short discovery cycle.
// IdleCycles and OrphanGoneCycles keep their full values,
// so every case still needs the full run of consecutive cycles.
// ReapTimeout is short too, so a descendant that ignores SIGHUP is force-killed within a second.
func compressedReapTiming() watchdogTiming {
	return watchdogTiming{
		DiscoveryCycle:   30 * time.Millisecond,
		DiscoveryCeiling: 30 * time.Millisecond,
		IdleCycles:       watchdogHubIdleCycles,
		OrphanGoneCycles: watchdogOrphanGoneCycles,
		ReapTimeout:      time.Second,
	}
}

// reapSuccessWait bounds the waits for a session to go or to appear, for steps that expect to succeed.
// Each discovery cycle spawns tmux and git.
// A host whose security agent holds new process images stretches every cycle far past the compressed cadence,
// so a window derived from that cadence fails there.
// The waits poll, so a fast host returns as soon as the condition holds and never pays the bound.
const reapSuccessWait = 30 * time.Second

// reapForceKillGrace is the wait after a force-kill before the reap gives up on a straggler.
// It mirrors the engine's force-kill grace period, so keep the two equal.
const reapForceKillGrace = 5 * time.Second

// orphanWorktree removes worktreeRoot's directory from disk while its tmux session stays live on the hub socket.
// That leaves the state the reap exists to clean up: a live session whose worktree is gone.
func orphanWorktree(t *testing.T, worktreeRoot string) {
	t.Helper()
	if err := exec.Command("rm", "-rf", worktreeRoot).Run(); err != nil {
		t.Fatalf("orphanWorktree: rm -rf %s: %v", worktreeRoot, err)
	}
}

// reapListPanePIDs lists session's live #{pane_pid} values by querying tmux directly.
// A case uses it to snapshot a session's pane process pids independently of the engine under test.
func reapListPanePIDs(t *testing.T, tmuxPath, socket, session string) []int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-s", "-t", "="+session+":", "-F", "#{pane_pid}").Output()
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

// waitProcessGone polls until pid is no longer alive, per proc.IsAlive.
// It fails the test if timeout elapses first.
// Use it to confirm a process exited, not only that its session disappeared.
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
		done <- runWatchdogLoop(ctx, fx.hub.Path, fx.tmuxPath, shellPath, timing, reedengine.OpenFileWatch, reedengine.ListSessions, nil)
	}()
	return cancel, done
}

// TestWatchdogReap runs the reap steps against one hub whose prime worktree stays booted and healthy throughout.
// Each step orphans its own pair worktree.
// The steps run serially in a fixed order and do not rely on each other's results.
// The scenario calls t.Parallel, but no step does: every step shares the hub, its tmux server and the prime session.
func TestWatchdogReap(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	socket := reedengine.ServerName(h.Path)
	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	tmuxkit.KillOnCleanup(t, watchdogIntegrationTmux(t, cfg), socket)
	primeEng := watchdogIntegrationEngine(t, h.PrimeWorktree())

	// EndToEndAndSiblingSurvives orphans a pair session and drives the loop with compressed timing.
	// It asserts that the orphan is fully reaped, with its session gone and its pane processes confirmed exited.
	// It also asserts that the healthy sibling on the same hub socket is untouched.
	// The sibling's session name shares a prefix with the orphan's,
	// so a kill target that matches by prefix instead of by exact name also takes the sibling down.
	// This step fails loudly in that case, in the same run that proves the reap works.
	if !t.Run("EndToEndAndSiblingSurvives", func(t *testing.T) {
		fx := newReapFixture(t, h, primeEng, "reap-e2e-sibling")

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

		// The session must still be live before OrphanGoneCycles affirmative cycles have elapsed.
		// A reap that fires on the first observation fails this check.
		time.Sleep(time.Duration(timing.OrphanGoneCycles) * timing.DiscoveryCycle / 2)
		if !sessionListed(fx.tmuxPath, socket, orphanSession) {
			t.Errorf("orphan session %s was reaped before %d affirmative cycles elapsed; want it still live", orphanSession, timing.OrphanGoneCycles)
		}

		waitForCondition(t, reapSuccessWait, func() bool {
			return !sessionListed(fx.tmuxPath, socket, orphanSession)
		})

		for _, pid := range beforePIDs {
			waitProcessGone(t, pid, 20*time.Second)
		}

		if !sessionListed(fx.tmuxPath, socket, siblingSession) {
			t.Errorf("sibling session %s did not survive the orphan's reap; want it still listed", siblingSession)
		}

		cancel()
		<-loopDone
	}) {
		return
	}

	// NeverEnteredOrphanIsStillReaped covers an orphan the daemon can never enter.
	// Its worktree is gone before the loop's first cycle,
	// so the git spawn that resolves the session fails every cycle and its name never reaches the known map.
	// The reap must still catch it, because the reap reads the live session-name list, not the known map.
	if !t.Run("NeverEnteredOrphanIsStillReaped", func(t *testing.T) {
		fx := newReapFixture(t, h, primeEng, "reap-never-entered")

		orphanEng := fx.engines[1]
		orphanSession := orphanEng.SessionName()

		// The worktree is removed before the loop starts,
		// so the daemon's first cycle finds a live session whose worktree does not exist.
		// Entering that session can never succeed.
		orphanWorktree(t, fx.worktrees[1])

		timing := compressedReapTiming()
		cancel, loopDone := startReapLoop(fx, orphanEng.ShellPath(), timing)
		defer cancel()

		waitForCondition(t, reapSuccessWait, func() bool {
			return !sessionListed(fx.tmuxPath, socket, orphanSession)
		})

		cancel()
		<-loopDone
	}) {
		return
	}

	// EmptyShellStillReaps checks that runWatchdogLoop still reaps an orphan with an empty shellPath.
	// It drives the loop against a live hub socket and an orphaned worktree.
	// It asserts that the session's pane root pids are confirmed exited.
	// It asserts only the roots, not their descendants, because descendant handling differs by platform.
	// Asserting descendants would pass for a platform-specific reason, not for the cross-platform guarantee this step pins.
	if !t.Run("EmptyShellStillReaps", func(t *testing.T) {
		fx := newReapFixture(t, h, primeEng, "reap-empty-shell")

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

		waitForCondition(t, reapSuccessWait, func() bool {
			return !sessionListed(fx.tmuxPath, socket, orphanSession)
		})

		for _, pid := range rootPIDs {
			waitProcessGone(t, pid, 20*time.Second)
		}

		cancel()
		<-loopDone
	}) {
		return
	}

	// DescendantClosureConfirmedExited covers the descendant closure and the wait-then-force-kill sequence.
	// It drives an end-to-end reap against a session whose pane grew a background descendant.
	// That descendant never receives kill-session's hangup.
	// It asserts via proc.IsAlive that the descendant is confirmed exited, not merely signalled, after the reap returns.
	// This proves the closure is computed before the kill.
	// A closure computed after kill-session collapses to the pane roots alone and would leave this descendant alive forever:
	// nothing else in this step signals it directly.
	if !t.Run("DescendantClosureConfirmedExited", func(t *testing.T) {
		fx := newReapFixture(t, h, primeEng, "reap-descendant")

		orphanEng := fx.engines[1]
		orphanSession := orphanEng.SessionName()

		dir := t.TempDir()
		rootFile := filepath.Join(dir, "root.pid")
		childFile := filepath.Join(dir, "child.pid")
		grandchildFile := filepath.Join(dir, "grandchild.pid")
		script := reapDescendantScript(t, dir, rootFile, childFile, grandchildFile)

		if _, err := orphanEng.AddStrand(reedengine.AddSpec{Cmd: script, Display: render.Display{Anchor: render.AnchorOwnWindow}}); err != nil {
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

		waitForCondition(t, reapSuccessWait, func() bool {
			return !sessionListed(fx.tmuxPath, socket, orphanSession)
		})

		// A resistant background descendant survives the reap for at most timing.ReapTimeout plus reapForceKillGrace.
		// The wait below runs generously beyond both.
		goneWithin := timing.ReapTimeout + reapForceKillGrace + 5*time.Second
		waitProcessGone(t, childPID, goneWithin)
		waitProcessGone(t, grandchildPID, goneWithin)

		cancel()
		<-loopDone
	}) {
		return
	}

	// LoopStaysLiveDuringReap guards the reap running off the discovery loop.
	// With a reap dispatched and still in flight, it asserts the following.
	// The discovery loop keeps ticking and enters a session that appears after the dispatch.
	// Cancelling the daemon's context returns promptly, not tied to the reap's graceful-plus-force budget.
	// The reaped name leaves the in-flight set on a later tick.
	// A session re-created under the same name is reaped again when orphaned a second time.
	// planReapCycle skips any name still marked in-flight, so a leaked mark would block that second reap.
	//
	// Both this step and DescendantClosureConfirmedExited run under -race.
	// The in-flight set, the gone-counter map and the known map are written only on the loop goroutine.
	// The reap goroutines' only cross-goroutine act is a non-blocking channel send,
	// so a correct implementation leaves the race detector nothing to find here.
	t.Run("LoopStaysLiveDuringReap", func(t *testing.T) {
		fx := newReapFixture(t, h, primeEng, "reap-inflight-a")

		orphanEng := fx.engines[1]
		orphanSession := orphanEng.SessionName()
		orphanWorktreeRoot := fx.worktrees[1]

		// Renamed aside, not removed: the worktree is restored with its git worktree admin data untouched.
		// That lets this step orphan it a second time under the identical session name.
		restore := renameWorktreeAway(t, orphanWorktreeRoot)

		timing := compressedReapTiming()
		cancel, loopDone := startReapLoop(fx, orphanEng.ShellPath(), timing)
		defer cancel()

		dispatchStart := time.Now()
		waitForCondition(t, reapSuccessWait, func() bool {
			return !sessionListed(fx.tmuxPath, socket, orphanSession)
		})

		// A session that appears while the first reap is in flight must still be discovered.
		// The loop must not stall behind the dispatched reap goroutine.
		hubforge.AddPair(t, fx.hub, "reap-inflight-b")
		freshWorktree := fx.hub.PairCodeWorktree("reap-inflight-b")
		freshEng := watchdogIntegrationEngine(t, freshWorktree)
		waitForCondition(t, reapSuccessWait, func() bool {
			return sessionListed(fx.tmuxPath, socket, freshEng.SessionName())
		})

		select {
		case err := <-loopDone:
			t.Fatalf("runWatchdogLoop exited while the fresh sibling was still live (err=%v); want it still running", err)
		default:
		}

		// Re-create a session under the same orphan name, against the still-running loop, and orphan it again.
		// A second reap succeeding proves the first reap's in-flight entry was cleared on a later tick.
		// planReapCycle skips any name still marked in-flight, so a leaked entry would stop this second reap from happening.
		restore()
		secondEng := watchdogIntegrationEngine(t, orphanWorktreeRoot)
		if secondEng.SessionName() != orphanSession {
			t.Fatalf("re-created session name = %q, want %q (same worktree, same session identity)", secondEng.SessionName(), orphanSession)
		}
		orphanWorktree(t, orphanWorktreeRoot)

		waitForCondition(t, reapSuccessWait, func() bool {
			return !sessionListed(fx.tmuxPath, socket, orphanSession)
		})

		// Cancelling is not tied to either reap's graceful-plus-force budget.
		// The loop's select observes ctx.Done() on its next iteration, whether or not a reap is in flight,
		// so the call returns in a small fraction of that budget.
		cancelDeadline := 5 * time.Second
		cancel()
		select {
		case <-loopDone:
		case <-time.After(cancelDeadline):
			t.Fatalf("runWatchdogLoop did not return within %s of cancel (first dispatch was %s earlier); want cancellation independent of either reap's own budget", cancelDeadline, time.Since(dispatchStart))
		}
	})
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
// setsid makes this prove the wait-then-force-kill sequence rather than an ordinary hangup.
// A plain backgrounded job stays in the pane's process group,
// and the SIGHUP that tmux's kill-session sends to that group kills it,
// so this case would pass even with a reordered implementation that computes the descendant closure after the kill.
// A brand-new session makes the child and, through it, the grandchild immune to that group signal,
// so they survive the pane's death the way an orphaned agent descendant would.
// Only the explicit descendant-closure kill reaps them.
//
// setsid runs from its own non-interactive script file, level1.sh calling level1b.sh,
// so the fork does not run inline in the pane's interactive shell.
// There, an inline foreground fork can already be a process-group leader, and setsid() fails with EPERM.
// setsid then takes its fallback double-fork path and exits its immediate child right away.
// A plain "bash <file>" invocation with job control off never assigns the forked setsid call its own process group.
// So setsid() succeeds directly, and every level stays exactly one real process, with no premature exit.
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

// readPIDFile reads the int pid that a script's "echo $$ > file" step wrote to path.
// The pane's shell writes that file asynchronously after AddStrand returns, so this waits for it to appear.
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

// renameWorktreeAway moves worktreeRoot's directory aside, so the worktree reads as gone at its original path.
// It returns a restore func that moves the directory back to that exact path.
// The git worktree admin data is left untouched throughout, so the restored worktree is intact.
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
