//go:build tmux

// smoke_selvage_keepalive_test.go pins Selvage's keepalive guarantee — the job the header pane's `--blocking` process once served before this task, now served by a permanent, deliberately ordinary shell pane instead of a custom blocking mechanism.
// There is no analogue of the old blockForever/deadlock-avoidance test here: Selvage runs the operator's configured shell (e.cfg.Shell) directly, so there is no reed-authored blocking loop left to pin against a runtime deadlock detector.
// What survives is the guarantee itself — a session with Selvage in it never dies just because every strand died — pinned instead against the real mechanism: an ordinary shell surviving a killed sibling pane, surviving Ctrl-C at its own idle prompt, and surviving Ctrl-C to a foreground job running inside it.
// Tagged smoke because untagged reedcli tests must not spawn processes (Test Tier Purity Invariant).
// The scenario also pins Selvage's placement (physically bottom-most, never a strand's pane) and the status-line options `up` pins into the session.

package reedcli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// selvagePaneID reads the persisted SelvagePaneID for worktree, failing the test if it is empty —
// RunCLI/status carries no Selvage field of its own, so every caller here reads it straight from
// reed.json exactly as the header-pane tests once did for HeaderPaneID.
func selvagePaneID(t *testing.T, worktree string) string {
	t.Helper()
	st, err := reedengine.LoadState(filepath.Join(worktree, ".lyx"))
	if err != nil || st == nil || st.SelvagePaneID == "" {
		t.Fatalf("LoadState(%s) = (%+v, %v), want a persisted SelvagePaneID", worktree, st, err)
	}
	return st.SelvagePaneID
}

// TestSmokeSelvage runs the Selvage claims against one hub's prime worktree, each step starting from a freshly restarted session, so the steps share the one hub build and the one tmux server.
// The steps run serially in a fixed order and do not rely on each other's results.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and its tmux server.
func TestSmokeSelvage(t *testing.T) {
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})

	// SurvivesUpAddRemoveAndReconcile pins Selvage's keepalive guarantee: the always-present Selvage pane must survive a full up -> add -> remove -> add cycle and every reconcile along the way, and it is never a strand's pane — because a strand's pane is always a fresh split (planPaneTarget never targets Selvage while any non-Selvage pane exists), and Selvage is exempt from both halves of reconcile's kill schedule — and, the whole point, still alive even when the strand table momentarily drops to zero after a remove.
	// It mirrors TestSmokeLifecycle/UpWithOnlyForeignPanesKeepsSessionUsable's tmux-driven verification style (list-panes via the real binary, not reed's own reporting) but for Selvage instead of a foreign pane.
	if !t.Run("SurvivesUpAddRemoveAndReconcile", func(t *testing.T) {
		restartSession(t, prime)

		// up boots Selvage before any strand exists.
		// Read the persisted pane id directly from reed.json (status carries no Selvage field) rather than assuming which of the session's panes it is.
		selvage := selvagePaneID(t, prime)

		socket, session := socketAndSessionIn(t, prime)
		requireSelvageAlive := func(when string) {
			t.Helper()
			lines := listPaneLines(t, tmuxPath, socket, session)
			if !paneLiveOnSession(lines, selvage) {
				t.Fatalf("Selvage pane %s not alive %s; panes=%v", selvage, when, lines)
			}
		}
		requireSelvageAlive("right after up (zero strands)")

		// add: the preceding up's own reconcile already reaped the session's pre-Selvage pane (the same zero-strands-plus-alive-Selvage reap TestSmokeLifecycle/UpWithOnlyForeignPanesKeepsSessionUsable pins), so Selvage is the session's only pane when this first strand is added.
		// planPaneTarget's Selvage-as-last-resort fallback then splits off Selvage itself — Selvage stays alive as the split TARGET, and the strand lands on the freshly split pane, never on Selvage.
		guid := addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host ready", "--name", "first")
		requireSelvageAlive("after add")

		strand, found := statusStrand(t, mustRunReed(t, prime, "status"), guid)
		if !found {
			t.Fatalf("status missing strand %s", guid)
		}
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("strand %s live = false; want true", guid)
		}
		strandPaneID, _ := strand["paneId"].(string)
		if strandPaneID == "" || strandPaneID == selvage {
			t.Fatalf("strand %s paneId = %q, want a real, non-Selvage pane id", guid, strandPaneID)
		}

		// remove: the session's only strand is gone, but Selvage — a
		// permanent second pane — must keep the session (and itself) alive,
		// exactly the invariant contract_integration_test.go's
		// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds pins at the engine
		// level.
		mustRunReed(t, prime, "remove", guid)
		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after removing its sole strand; Selvage must have kept it alive", session)
		}
		requireSelvageAlive("after removing the sole strand (zero strands tracked)")

		// A reconciling verb (up) with zero strands must not disturb Selvage either — the same-shaped assertion the foreign-pane step makes for a foreign pane.
		mustRunReed(t, prime, "up")
		requireSelvageAlive("after a reconciling up with zero strands")

		// add again: Selvage must still never become the new strand's own
		// pane, and the new strand must come up live — the substrate Selvage
		// keeps alive is still genuinely usable, not a wedged husk.
		second := addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host ready", "--name", "second")
		requireSelvageAlive("after a second add with strands now bound")

		strand2, found := statusStrand(t, mustRunReed(t, prime, "status"), second)
		if !found {
			t.Fatalf("status missing strand %s", second)
		}
		if live, _ := strand2["live"].(bool); !live {
			t.Errorf("strand %s live = false; want true", second)
		}
		if paneID, _ := strand2["paneId"].(string); paneID == selvage {
			t.Errorf("strand %s was bound to Selvage %s; Selvage must never become a strand's own pane", second, selvage)
		}
	}) {
		return
	}

	// SurvivesKillingEveryStrandPane pins the keepalive job itself: tmux destroys a session once its last window loses its last pane, so with every strand pane force-killed via tmux (not reed's own remove, which would already tear its bookkeeping down cleanly), Selvage alone must be enough to keep the session — and a subsequent reed verb usable — alive.
	if !t.Run("SurvivesKillingEveryStrandPane", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)

		guids := []string{
			addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "first"),
			addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "second"),
		}

		socket, session := socketAndSessionIn(t, prime)
		for _, guid := range guids {
			paneID := paneIDForStrandIn(t, prime, guid)
			if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", paneID).Run(); err != nil {
				t.Fatalf("kill-pane %s (strand %s): %v", paneID, guid, err)
			}
		}

		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after killing every strand pane; Selvage must have kept it alive", session)
		}
		if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
			t.Fatalf("Selvage pane %s not alive after killing every strand pane; panes=%v", selvage, panes)
		}

		// status must still answer after the strand panes are gone.
		mustRunReed(t, prime, "status")
	}) {
		return
	}

	// StaysPhysicallyBottomMostAcrossAddsAndRemoves pins the physical-position rule module-local to this package (see internal/reedengine/doc.go): Selvage is always the pane with the largest pane_top, in every state a series of adds and removes can leave the window in.
	if !t.Run("StaysPhysicallyBottomMostAcrossAddsAndRemoves", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)
		socket, session := socketAndSessionIn(t, prime)

		requireBottomMost := func(when string) {
			t.Helper()
			panes := listPaneLines(t, tmuxPath, socket, session)
			bottomTop := -1
			bottomID := ""
			for _, line := range panes {
				fields := strings.Fields(line)
				if len(fields) < 3 {
					continue
				}
				top, err := strconv.Atoi(fields[2])
				if err != nil {
					t.Fatalf("parse pane_top %q: %v", fields[2], err)
				}
				if top > bottomTop {
					bottomTop = top
					bottomID = fields[0]
				}
			}
			if bottomID != selvage {
				t.Errorf("%s: physically bottom-most pane = %s; want Selvage %s; panes=%v", when, bottomID, selvage, panes)
			}
		}
		requireBottomMost("right after up")

		first := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "first")
		requireBottomMost("after adding the first strand")

		second := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "second")
		requireBottomMost("after adding a second strand")

		mustRunReed(t, prime, "remove", first)
		requireBottomMost("after removing the first strand")

		addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "third")
		requireBottomMost("after adding a third strand")

		mustRunReed(t, prime, "remove", second)
		requireBottomMost("after removing the second strand")
	}) {
		return
	}

	// SurvivesCtrlCAtIdlePrompt pins the first of the two ordinary-shell survival claims in internal/reedengine/doc.go's design record: interactive bash ignores SIGINT while waiting at its own idle prompt, so a Ctrl-C sent directly to Selvage must never take the pane — or the session it anchors — down.
	if !t.Run("SurvivesCtrlCAtIdlePrompt", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)
		socket, session := socketAndSessionIn(t, prime)

		// Bash ignores SIGINT only once it sits at its prompt; a Ctrl-C that lands during startup kills it, so wait for the prompt to render.
		waitForCondition(t, 10*time.Second, func() bool {
			return strings.TrimSpace(capturePane(t, tmuxPath, socket, selvage)) != ""
		})

		if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", selvage, "C-c").Run(); err != nil {
			t.Fatalf("send-keys C-c to idle Selvage %s: %v", selvage, err)
		}
		time.Sleep(1 * time.Second)

		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after Ctrl-C to Selvage's idle prompt", session)
		}
		if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
			t.Fatalf("Selvage pane %s not alive after Ctrl-C at its idle prompt; panes=%v", selvage, panes)
		}
	}) {
		return
	}

	// SurvivesCtrlCKillingAForegroundJob pins the second ordinary-shell survival claim: Ctrl-C to a foreground job running inside Selvage kills only that job, never the shell pane itself.
	if !t.Run("SurvivesCtrlCKillingAForegroundJob", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)
		socket, session := socketAndSessionIn(t, prime)

		// Baseline: Selvage's own shell and whatever it has spawned so far, before the foreground job
		// starts — panePaneSubtree is the same cross-platform (linux/proc, windows/WMI) descendant-closure
		// helper the pane-reap smoke tests already use.
		before := map[int]bool{}
		for _, pid := range panePaneSubtree(t, tmuxPath, socket, session, selvage) {
			before[pid] = true
		}

		// A foreground job typed literally into Selvage's own shell — never reed's doing, exactly like
		// an operator running a long command directly at the prompt.
		sendKeysLine(t, tmuxPath, socket, selvage, smokeReapLaunchCmd())

		var jobPIDs []int
		deadline := time.Now().Add(10 * time.Second)
		for {
			for _, pid := range panePaneSubtree(t, tmuxPath, socket, session, selvage) {
				if !before[pid] {
					jobPIDs = append(jobPIDs, pid)
				}
			}
			if len(jobPIDs) > 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("Selvage's foreground job never showed up as a new descendant within 10s")
			}
			time.Sleep(200 * time.Millisecond)
		}

		if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", selvage, "C-c").Run(); err != nil {
			t.Fatalf("send-keys C-c to Selvage %s running a foreground job: %v", selvage, err)
		}

		for _, pid := range jobPIDs {
			pollProcessGone(t, pid, 10*time.Second)
		}

		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after Ctrl-C killed Selvage's foreground job", session)
		}
		if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
			t.Fatalf("Selvage pane %s not alive after Ctrl-C killed its foreground job; panes=%v", selvage, panes)
		}
	}) {
		return
	}

	// StatusLinePinsIdentityAndPosition pins pinGeometryOptionsLocked's status-bar pins after `up`:
	// `#{status}` reads "2" (the two-line bar) and `#{status-position}` reads "bottom".
	// The bar's content is not a pane, so the assertion is a `display-message -p` readback rather than a pane-content poll.
	//
	// On Windows the identity values are not asserted directly — per the windows-status-line-is-an-unbranched-accepted-degrade Shared Decision, psmux may refuse some or all of the status-bar set-option calls, and reed does not branch to compensate.
	// What is asserted there instead is the SELF-CORRECTING half: whatever `#{status}` reads back, the reserved-row count it implies must match the window the layout was actually planned against, so a psmux that refuses the options fails the identity assertion loudly (an operator watching the status-line notices) rather than silently corrupting the layout.
	t.Run("StatusLinePinsIdentityAndPosition", func(t *testing.T) {
		restartSession(t, prime)
		socket, session := socketAndSessionIn(t, prime)
		target := "=" + session + ":"

		readOption := func(option string) string {
			t.Helper()
			out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", target, "#{"+option+"}").Output()
			if err != nil {
				t.Fatalf("display-message #{%s}: %v", option, err)
			}
			return strings.TrimSpace(string(out))
		}

		if runtime.GOOS == "windows" {
			// reservedRowsFromStatus's own mapping, inlined here since it is unexported in reedengine:
			// "off" -> 0, "on" -> 1, a non-negative integer string -> that integer verbatim.
			status := strings.ToLower(readOption("status"))
			reserved := 0
			switch status {
			case "off":
				reserved = 0
			case "on":
				reserved = 1
			default:
				n, err := strconv.Atoi(status)
				if err != nil || n < 0 {
					t.Fatalf("#{status} = %q; want off, on, or a non-negative integer", status)
				}
				reserved = n
			}

			windowHeight, err := strconv.Atoi(readOption("window_height"))
			if err != nil {
				t.Fatalf("parse #{window_height}: %v", err)
			}
			lines := listPaneLines(t, tmuxPath, socket, session)
			paneRowsSum := 0
			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) < 4 {
					continue
				}
				height, err := strconv.Atoi(fields[3])
				if err != nil {
					t.Fatalf("parse pane_height %q: %v", fields[3], err)
				}
				paneRowsSum += height
			}
			// tmux draws one border row between each pair of vertically stacked panes.
			borders := len(lines) - 1
			if borders < 0 {
				borders = 0
			}
			if paneRowsSum+borders+reserved != windowHeight {
				t.Errorf("panes(%d) + borders(%d) + status-reserved(%d) = %d; want the live window height %d — whatever #{status} reads back must match the window the layout was actually planned against", paneRowsSum, borders, reserved, paneRowsSum+borders+reserved, windowHeight)
			}
			return
		}

		if got := readOption("status"); got != "2" {
			t.Errorf("#{status} = %q; want \"2\"", got)
		}
		if got := readOption("status-position"); got != "bottom" {
			t.Errorf("#{status-position} = %q; want \"bottom\"", got)
		}
	})
}
