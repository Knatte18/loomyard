// smoke_teardown_test.go pins the teardown guarantees of `reed down` and `reed remove` against a real tmux server.
// The server is released before down returns, every pane descendant is reaped,
// and a worktree's down never touches a sibling worktree's session.

package reedcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestSmokeTeardown runs the teardown claims against one hub.
// Each step brings up its own session on the prime worktree and ends with that session down,
// so the next step starts from the same cold state.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and its tmux server.
func TestSmokeTeardown(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})

	// run runs one reed verb in worktree and fails the calling test unless it exits zero.
	run := func(t *testing.T, worktree string, args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		if code := RunCLIIn(worktree, &out, args); code != 0 {
			t.Fatalf("%v = %d; want 0, output: %s", args, code, out.String())
		}
		return out.Bytes()
	}

	// DownReleasesServerBeforeReturning pins that the tmux server is gone when down returns, over several cycles.
	if !t.Run("DownReleasesServerBeforeReturning", func(t *testing.T) {
		run(t, prime, "up")
		socket, session := socketAndSessionIn(t, prime)

		launch := "pwsh -NoExit -Command Write-Host ready"
		for cycle := 0; cycle < 3; cycle++ {
			pid := serverPID(t, tmuxPath, socket, session)
			run(t, prime, "down")
			if !processGone(pid) {
				t.Fatalf("cycle %d: tmux server (pid %d) still running immediately after down returned", cycle, pid)
			}
			run(t, prime, "up")
			addStrandIn(t, prime, launch, "--name", "churn")
		}
		run(t, prime, "down")
	}) {
		return
	}

	// DownReapsPaneChildProcesses pins that every pane descendant is gone when down returns.
	if !t.Run("DownReapsPaneChildProcesses", func(t *testing.T) {
		launch := smokeReapLaunchCmd()
		for cycle := 0; cycle < 3; cycle++ {
			run(t, prime, "up")
			addStrandIn(t, prime, launch, "--name", "reap")
			addStrandIn(t, prime, launch, "--name", "reap2")
			socket, session := socketAndSessionIn(t, prime)
			pids := paneProcessTree(t, tmuxPath, socket, session)
			if len(pids) == 0 {
				t.Fatalf("cycle %d: session reported no pane process subtree", cycle)
			}

			run(t, prime, "down")
			// No sleep: every pane descendant must already be gone when down returns.
			for _, pid := range pids {
				if !processGone(pid) {
					t.Fatalf("cycle %d: pane subtree pid %d still running immediately after down returned", cycle, pid)
				}
			}
		}
	}) {
		return
	}

	// DownForceKillsSighupImmunePaneChildren pins that down force-kills a pane child that traps SIGHUP.
	// The other reap steps cannot tell reed's reap from tmux's, because their payloads die to tmux's SIGHUP cascade.
	//
	// This payload runs a descendant that traps SIGHUP, so only reed's explicit force-kill can reap it.
	// Down must block until that descendant is gone.
	// The immune descendant also forks a late child, immune too, a few seconds after down's snapshot.
	// A pane shell does the same when down lands just after a strand's command was typed.
	// Only the reap of the pane session's remaining members reaches that child, since no snapshot holds it.
	//
	// POSIX-only: the payload needs a shell-level SIGHUP trap, which has no Windows equivalent.
	if runtime.GOOS != "windows" && !t.Run("DownForceKillsSighupImmunePaneChildren", func(t *testing.T) {
		run(t, prime, "up")

		// `exec` on the outer sleep keeps the immune child's parent pid stable, so
		// the child stays inside the pane's descendant closure reed snapshots.
		// The immune child marks itself started and outlives the snapshot in its `sleep 5`.
		// It then records its late child's pid and waits until reed force-kills it.
		readyFile := filepath.Join(t.TempDir(), "ready")
		lateFile := filepath.Join(t.TempDir(), "late.pid")
		immune := fmt.Sprintf(`bash -c 'bash -c "trap \"\" HUP; : > %s; sleep 5; sleep 300 & echo \$! > %s; wait" & exec sleep 300'`, readyFile, lateFile)
		addStrandIn(t, prime, immune, "--name", "immune")
		socket, session := socketAndSessionIn(t, prime)
		// Without this wait, down can land before the pane shell runs the payload at all,
		// and the step passes with nothing immune to reap.
		waitForCondition(t, 10*time.Second, func() bool {
			_, err := os.Stat(readyFile)
			return err == nil
		})

		pids := paneProcessTree(t, tmuxPath, socket, session)
		if len(pids) == 0 {
			t.Fatalf("session reported no pane process subtree")
		}
		if _, err := os.Stat(lateFile); err == nil {
			t.Fatalf("late child already forked before down; want it forked after down's snapshot")
		}

		run(t, prime, "down")
		// No sleep: down must not return until every snapshotted descendant is gone.
		// One still running when the graceful window expires is force-killed.
		for _, pid := range pids {
			if !processGone(pid) {
				t.Errorf("pane subtree pid %d still running immediately after down returned; reed's force-kill fallback did not run", pid)
			}
		}
		raw, err := os.ReadFile(lateFile)
		if err != nil {
			t.Fatalf("read late child pid: %v; want the immune child to have forked it during down's graceful wait", err)
		}
		latePID, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatalf("parse late child pid %q: %v", raw, err)
		}
		if !processGone(latePID) {
			t.Errorf("late pane session member pid %d still running immediately after down returned; reed did not reap the session's members beyond its snapshot", latePID)
		}
	}) {
		return
	}

	// DownLeavesNoTmuxOnSocket pins that no tmux process names this worktree's socket once down has torn the server down.
	// That covers the main server and its __warm__ helper.
	// The tmux server is spawned with the worktree as its cwd,
	// so a server that outlives down keeps the worktree directory busy.
	// Several add and down cycles give the async kill-server a chance to lag.
	if !t.Run("DownLeavesNoTmuxOnSocket", func(t *testing.T) {
		launch := smokeReapLaunchCmd()
		for cycle := 0; cycle < 3; cycle++ {
			run(t, prime, "up")
			socket, _ := socketAndSessionIn(t, prime)
			addStrandIn(t, prime, launch, "--name", "s1")
			addStrandIn(t, prime, launch, "--name", "s2")

			run(t, prime, "down")
			// No sleep: the moment down returns, the socket must be free of tmux.
			if pids := tmuxSocketPids(t, tmuxPath, socket); len(pids) != 0 {
				t.Fatalf("cycle %d: tmux still on socket %s after down returned: pids=%v", cycle, socket, pids)
			}
		}
	}) {
		return
	}

	// RemoveReapsRemovedPaneChildProcesses pins that remove reaps the removed pane's children.
	// kill-pane terminates a pane's children asynchronously.
	// On Windows the process holding the worktree directory is a deep descendant of #{pane_pid},
	// so a remove that returned early could leave it alive and keep the directory busy.
	// remove snapshots the removed pane's process subtree before kill-pane.
	// It waits for that subtree to exit, so once remove returns, every descendant of the removed pane must be gone.
	// A sibling strand is kept alive throughout so the session survives and the removed pane is never the sole pane.
	if !t.Run("RemoveReapsRemovedPaneChildProcesses", func(t *testing.T) {
		launch := smokeReapLaunchCmd()
		for cycle := 0; cycle < 3; cycle++ {
			run(t, prime, "up")
			keeper := addStrandIn(t, prime, launch, "--name", "keeper")
			victim := addStrandIn(t, prime, launch, "--name", "victim")
			socket, session := socketAndSessionIn(t, prime)

			vs, ok := statusStrand(t, run(t, prime, "status"), victim)
			if !ok {
				t.Fatalf("cycle %d: status missing victim %s", cycle, victim)
			}
			victimPane, _ := vs["paneId"].(string)
			if victimPane == "" {
				t.Fatalf("cycle %d: victim %s has no pane", cycle, victim)
			}
			pids := panePaneSubtree(t, tmuxPath, socket, session, victimPane)
			if len(pids) == 0 {
				t.Fatalf("cycle %d: victim pane %s reported no process subtree", cycle, victimPane)
			}

			run(t, prime, "remove", victim)
			// No sleep: every descendant of the removed pane must already be gone the instant remove returned.
			for _, pid := range pids {
				if !processGone(pid) {
					t.Fatalf("cycle %d: removed pane %s subtree pid %d still running immediately after remove returned", cycle, victimPane, pid)
				}
			}

			// The keeper must survive the remove untouched, and down cleans up for the next cycle.
			if ks, ok := statusStrand(t, run(t, prime, "status"), keeper); !ok {
				t.Fatalf("cycle %d: keeper %s gone after removing victim", cycle, keeper)
			} else if live, _ := ks["live"].(bool); !live {
				t.Errorf("cycle %d: keeper %s live = false after removing victim; want true", cycle, keeper)
			}
			run(t, prime, "down")
		}
	}) {
		return
	}

	// DownInOneWorktreeLeavesSiblingSessionAlive pins the cross-worktree scope of down.
	// The tmux server identity is per hub: the -L socket derives from the hub and is shared by sibling worktrees.
	// So down in one worktree must tear down only its own session,
	// and it must never touch a sibling's session, panes, or agents on that socket.
	// (The psmux port backs each session with its own `psmux.exe server -s <session> -L <socket>` process,
	// so "no duplicate server" is checked per session: one backing process per live session, none once killed.)
	// Two worktrees under one hub `up` share the socket and have distinct sessions, one backing process each.
	// Each adds a live strand.
	// Worktree A goes down.
	// Then B's session, pane, agent subtree, and single backing server must all still be live.
	// A's session and pane subtree must be gone.
	// B then goes down last, and the socket must be free of every tmux.
	// The core assertion is B's continued liveness after A's down (see `assertSiblingStaysLive`):
	// an implementation that kills the whole socket's server set fails this step rather than reporting a false green.
	t.Run("DownInOneWorktreeLeavesSiblingSessionAlive", func(t *testing.T) {
		// Named "sibling", not "hub-b": tmux's has-session and kill-session fuzzy-match an unambiguous prefix.
		// With "hub-b" still alive, has-session -t hub keeps succeeding after hub is killed,
		// so the post-down waitServerGone(sessionA) check would hang for its full timeout.
		// A name with no shared prefix avoids depending on tmux's target matching.
		sibling := materializeSibling(t, h, "sibling")

		// Release the sibling worktree dir before the framework's TempDir RemoveAll.
		// Registered before the down cleanup so it runs after it (LIFO).
		deferHubRelease(t, sibling)

		// Best-effort teardown net: down the sibling from its own cwd even if an assertion aborts the body partway,
		// so no server or session leaks.
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(sibling, &buf, []string{"down"})
		})

		launch := smokeReapLaunchCmd()

		run(t, prime, "up")
		socketA, sessionA := socketAndSessionIn(t, prime)
		aGuid := addStrandIn(t, prime, launch, "--name", "agent-a")
		aPane := paneIDForStrandIn(t, prime, aGuid)

		run(t, sibling, "up")
		socketB, sessionB := socketAndSessionIn(t, sibling)
		bGuid := addStrandIn(t, sibling, launch, "--name", "agent-b")
		bPane := paneIDForStrandIn(t, sibling, bGuid)

		if socketA != socketB {
			t.Fatalf("worktree A socket %q != worktree B socket %q; siblings on one hub must share the per-hub socket", socketA, socketB)
		}
		if sessionA == sessionB {
			t.Fatalf("worktree A and B both resolved session %q; each worktree must own a distinct session", sessionA)
		}
		socket := socketA

		// Both sessions live on the one socket.
		waitSessionUp(t, tmuxPath, socket, sessionA)
		waitSessionUp(t, tmuxPath, socket, sessionB)

		// Exactly one backing server process per session on the shared socket — no
		// duplicate spawned for either session.
		waitServerProcCountForSession(t, tmuxPath, socket, sessionA, 1)
		waitServerProcCountForSession(t, tmuxPath, socket, sessionB, 1)

		// B's backing-server pid, for the post-down stability check: it must be
		// unchanged after A's down (B's server neither killed nor restarted).
		bServerPID := serverPID(t, tmuxPath, socket, sessionB)

		// Snapshot before A's down, while both panes still exist to enumerate:
		// worktree A's whole pane process subtree, which must read gone after A's down.
		// A transient child that already exited still reads gone, so this is not flaky.
		// Worktree B's pane root pid, which must read alive after A's down.
		// Only the stable root is taken, never its come-and-go descendants, so the liveness check is robust.
		aSubtree := panePaneSubtree(t, tmuxPath, socket, sessionA, aPane)
		if len(aSubtree) == 0 {
			t.Fatalf("worktree A pane %s reported no process subtree", aPane)
		}
		bPanePID := paneRootPID(t, tmuxPath, socket, sessionB, bPane)

		// --- down in worktree A ---
		run(t, prime, "down")

		// A's own session is gone and its pane subtree reaped.
		// down reaps this session's pane children before it returns, so no sleep is needed.
		waitServerGone(t, tmuxPath, socket, sessionA)
		for _, pid := range aSubtree {
			if !processGone(pid) {
				t.Fatalf("worktree A pane subtree pid %d still running immediately after A down returned", pid)
			}
		}
		// A's own backing server process is gone too.
		waitServerProcCountForSession(t, tmuxPath, socket, sessionA, 0)

		// Worktree B must keep its session, pane, backing-server pid, and agent root process live.
		// A down that tore down the shared socket's server set would trip this check.
		// Otherwise the test would report a false green.
		assertSiblingStaysLive(t, tmuxPath, socket, sessionB, bPane, bServerPID, bPanePID, 2*time.Second)

		// And still exactly one backing server for B: no duplicate was spawned during the A up/down churn.
		// The process-table check runs once here, rather than inside the stability loop.
		if got := serverProcCountForSession(t, tmuxPath, socket, sessionB); got != 1 {
			t.Fatalf("worktree B backing-server count = %d after A down; want exactly 1 (0 = killed, 2 = duplicate)", got)
		}

		// --- down in worktree B (the last session): server torn down, socket clear ---
		run(t, sibling, "down")
		waitSocketFreeOfTmux(t, tmuxPath, socket)
	})
}
