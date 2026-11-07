// smoke_lifecycle_test.go runs the up, add, remove, status and down verbs through RunCLIIn against a real tmux server.

package reedcli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// mustRunReed runs one reed verb in worktree and fails the calling test unless it exits zero.
// It returns the verb's output.
func mustRunReed(t *testing.T, worktree string, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, args); code != 0 {
		t.Fatalf("%v = %d; want 0, output: %s", args, code, out.String())
	}
	return out.Bytes()
}

// restartSession brings worktree's session down, if one is up, and boots a fresh one.
// The step that calls it starts with no strand and no state left from an earlier step.
func restartSession(t *testing.T, worktree string) {
	t.Helper()
	var buf bytes.Buffer
	RunCLIIn(worktree, &buf, []string{"down"})
	mustRunReed(t, worktree, "up")
}

// TestSmokeLifecycle runs the up, add, remove, status and down verbs against one hub's prime worktree.
// Each step restarts the session first, so the steps share one hub build and one tmux server.
// The steps run serially in a fixed order and do not rely on each other's results.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and its tmux server.
// Every step drives reed through RunCLIIn.
// That seeds the worktree into the execution context without moving the process.
// So the anchor reed is told to use and the test process's own working directory are two different directories.
func TestSmokeLifecycle(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)
	lymExe := lymbin.Build(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})
	stateDir := filepath.Join(prime, ".lym")

	if !t.Run("UpAddStatusDown", func(t *testing.T) {
		mustRunReed(t, prime, "up")

		guid := addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host ready")

		strand, found := statusStrand(t, mustRunReed(t, prime, "status"), guid)
		if !found {
			t.Fatalf("status strands missing guid %s", guid)
		}
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("status strand %s live = false; want true", guid)
		}

		mustRunReed(t, prime, "down")
	}) {
		return
	}

	// Successive default adds each open a window of their own.
	// With several strands added in sequence, the session-wide pane listing yields one live pane per visible strand.
	// It also holds one more for the always-present Selvage pane.
	if !t.Run("AddsKeepEverySessionPane", func(t *testing.T) {
		restartSession(t, prime)

		launch := "pwsh -NoExit -Command Write-Host ready"
		guids := []string{
			addStrandIn(t, prime, launch, "--name", "strand1"),
			addStrandIn(t, prime, launch, "--name", "strand2"),
			addStrandIn(t, prime, launch, "--name", "stack1"),
			addStrandIn(t, prime, launch, "--name", "stack2"),
		}

		socket, session := socketAndSessionIn(t, prime)
		panes := listPaneLines(t, tmuxPath, socket, session)
		wantPanes := len(guids) + 1 // +1 for the always-present Selvage pane
		if len(panes) != wantPanes {
			t.Fatalf("session holds %d panes %v; want %d (one per visible strand plus Selvage — a shortfall means a silent split failure destroyed panes)", len(panes), panes, wantPanes)
		}

		statusOut := mustRunReed(t, prime, "status")
		for _, guid := range guids {
			strand, found := statusStrand(t, statusOut, guid)
			if !found {
				t.Fatalf("status missing strand %s; output: %s", guid, statusOut)
			}
			if live, _ := strand["live"].(bool); !live {
				t.Errorf("strand %s (%v) live = false; want true", guid, strand["name"])
			}
		}
	}) {
		return
	}

	// With zero strands tracked, an alive Selvage authorizes reconcile's deterministic untracked-pane reap (reconcile.go).
	// So an up against a session holding only foreign panes leaves a usable session:
	// Selvage stays intact and every foreign pane is gone, with no zero-pane wedge.
	// A subsequent add comes up live on its own fresh pane without displacing Selvage.
	if !t.Run("UpWithOnlyForeignPanesKeepsSessionUsable", func(t *testing.T) {
		restartSession(t, prime)
		socket, session := socketAndSessionIn(t, prime)

		// Up boots the always-present Selvage pane before any strand exists.
		// This same up's reconcile already reaps the session's initial pane, which was never adopted:
		// with zero strands tracked, the newly alive Selvage authorizes the untracked-pane reap (reconcile.go),
		// so that initial pane does not survive this first up.
		// Read Selvage's pane id from reed.json so the assertions below can tell it apart from the foreign pane added next.
		selvage := selvagePaneID(t, prime)

		// A foreign pane that reed does not track, as when an operator splits a window by hand.
		// Going in, the session holds 1 pane (Selvage) and 0 strands, since the first up already reaped its initial pane.
		// This split leaves 2 panes (Selvage and the foreign one) and 0 strands.
		if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
			t.Fatalf("foreign split-window: %v", err)
		}

		// The second up has zero strands tracked and Selvage alive.
		// So reconcile's untracked reap fires again and kills the foreign pane too.
		// This up must still exit 0 and leave the session usable, not in the zero-pane wedge.
		mustRunReed(t, prime, "up")
		if panes := listPaneLines(t, tmuxPath, socket, session); len(panes) != 1 || !paneLiveOnSession(panes, selvage) {
			t.Fatalf("up with only a foreign pane must reap it, leaving exactly Selvage %s alive; got panes=%v", selvage, panes)
		}

		// The session must still be able to host a strand.
		// The add proves the substrate survived.
		// By documented policy, the add also reaps the untracked foreign pane through reconcile.
		// The strand's own pane must survive, never the foreign one:
		// psmux's positional pane reaping would pick an indeterminate victim.
		guid := addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host ready", "--name", "after-foreign")
		statusOut := mustRunReed(t, prime, "status")
		strand, found := statusStrand(t, statusOut, guid)
		if !found {
			t.Fatalf("status missing strand %s; output: %s", guid, statusOut)
		}
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("strand added after foreign-pane up: live = false; want true; status: %s", statusOut)
		}
		strandPane, _ := strand["paneId"].(string)
		panes := listPaneLines(t, tmuxPath, socket, session)
		if len(panes) != 2 || !paneLiveOnSession(panes, strandPane) || !paneLiveOnSession(panes, selvage) {
			t.Errorf("after add, session panes = %v; want exactly the strand's pane %s and Selvage %s (foreign pane must be reaped, neither pane ever displaced)", panes, strandPane, selvage)
		}
	}) {
		return
	}

	// ForeignPaneIsReapedNotAdoptedByAdd guards against adopting an operator's own split-window pane as a strand's pane.
	// Such a pane must instead be reaped by add's reconcile.
	//
	// The adoption bug fires only when the sole alive non-Selvage pane is the foreign one.
	// A session still holding its unadopted initial new-session pane has two alive non-Selvage panes.
	// So the bug cannot show there.
	// The step therefore first drives the session to Selvage plus a foreign pane only: up, add a strand, and remove it.
	// Removing it leaves its pane gone rather than corpsed, since the always-present Selvage pane keeps the session up.
	// Then it splits a foreign pane in with the real tmux binary.
	//
	// The foreign pane's #{pane_pid} is captured before the add under test and polled gone afterward.
	// The poll runs under a bounded deadline, because kill-pane terminates a pane's process asynchronously.
	// The pid check is the load-bearing assertion.
	// A pane-id check alone would pass under the adoption bug if ids were recycled.
	// Under adoption the pane pid survives, so the pid check catches what the id check misses.
	// The step asserts nothing about the foreign pid's descendants.
	// The reap is kill-pane only by decision, and the RemoveStrand and Down tests pin descendant liveness.
	if !t.Run("ForeignPaneIsReapedNotAdoptedByAdd", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)

		guid := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "throwaway")
		mustRunReed(t, prime, "remove", guid)

		socket, session := socketAndSessionIn(t, prime)

		// A foreign pane reed does not track, as when an operator splits the session by hand.
		// The session now holds Selvage, this one foreign pane, and zero strands:
		// the state in which reconcile must reap the foreign pane.
		if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
			t.Fatalf("foreign split-window: %v", err)
		}

		// The foreign pane is the only live pane that is neither Selvage nor the removed strand's pane.
		foreignPaneID := ""
		for _, line := range listPaneLines(t, tmuxPath, socket, session) {
			fields := strings.Fields(line)
			if len(fields) == 0 || fields[0] == selvage {
				continue
			}
			foreignPaneID = fields[0]
			break
		}
		if foreignPaneID == "" {
			t.Fatalf("no foreign pane found after split-window; panes=%v", listPaneLines(t, tmuxPath, socket, session))
		}
		foreignPID := paneRootPID(t, tmuxPath, socket, session, foreignPaneID)

		guid2 := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "after-foreign")

		statusOut := mustRunReed(t, prime, "status")
		strand, found := statusStrand(t, statusOut, guid2)
		if !found {
			t.Fatalf("status missing strand %s; output: %s", guid2, statusOut)
		}
		if paneID, _ := strand["paneId"].(string); paneID == foreignPaneID {
			t.Fatalf("strand %s was bound to the foreign pane %s; the foreign pane must be reaped, never adopted", guid2, foreignPaneID)
		}

		panes := listPaneLines(t, tmuxPath, socket, session)
		for _, line := range panes {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == foreignPaneID {
				t.Fatalf("foreign pane %s still present after add; want it reaped by reconcile; panes=%v", foreignPaneID, panes)
			}
		}

		pollProcessGone(t, foreignPID, 20*time.Second)
	}) {
		return
	}

	// UpSurvivesAScrubbedStateFileWhileTheSessionIsUp is an end-to-end guard, driven at the CLI seam an operator uses.
	// It checks that up recovers after the state file is scrubbed while the session is up.
	//
	// Deleting .lym/reed.json is the scrub this guards against.
	// The file is never tracked, machine-local, and disposable under the Durable-vs-Ephemeral State Invariant,
	// and `git clean -xdf` removes it.
	// The failure it guards is a wedged worktree:
	// up and resume fail on every later call with `split Selvage pane: exit status 1: no space for new pane`.
	// The bottom-most pane is then an untracked short Selvage pane that tmux cannot split.
	// Meanwhile `lym reed status` still reports the session healthy.
	// The Selvage split is a full-width `-f` split.
	// It takes its rows from the whole window, so a short bottom pane cannot refuse it.
	//
	// The recovering `up` must exit 0, and the scrubbed state must converge on that `up` itself, not one verb later.
	// So the step asserts on the recovering `up` with no intervening verb.
	// The session must hold exactly one pane, the newly persisted SelvagePaneID, distinct from the captured old one.
	// The old Selvage pane id and the old strand pane id must already be gone from list-panes.
	// The scrub erases the strand table along with SelvagePaneID.
	// The recovering up's own reconcile therefore runs with zero strands tracked and a live Selvage.
	// That authorizes reaping every other pane (reconcile.go).
	// The rebuilt Selvage pane's pane_top == 0 assertion is therefore vacuous:
	// with one pane there is nowhere else for it to be.
	//
	// kill-pane terminates asynchronously, so the captured #{pane_pid} is polled gone under a bounded deadline.
	// smokeReapLaunchCmd's launched command is a child of that pane's process, not #{pane_pid} itself.
	// This step does not assert on it.
	// The leak this pins is the pane and its own process, not the whole subtree.
	//
	// The Selvage-only, full-height end state is the accepted outcome, not a defect to "fix" by synthesizing a spacer pane.
	// The following `add` shows the session is usable again, not only that it runs without error.
	if !t.Run("UpSurvivesAScrubbedStateFileWhileTheSessionIsUp", func(t *testing.T) {
		restartSession(t, prime)
		oldSelvagePaneID := selvagePaneID(t, prime)

		guid := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "before-scrub")
		socket, session := socketAndSessionIn(t, prime)

		strand, found := statusStrand(t, mustRunReed(t, prime, "status"), guid)
		if !found {
			t.Fatalf("status missing strand %s", guid)
		}
		oldStrandPaneID, _ := strand["paneId"].(string)
		if oldStrandPaneID == "" {
			t.Fatalf("strand %s has no pane before the scrub", guid)
		}
		oldStrandPID := paneRootPID(t, tmuxPath, socket, session, oldStrandPaneID)

		statePath := filepath.Join(stateDir, "reed.json")
		if err := os.Remove(statePath); err != nil {
			t.Fatalf("remove %s: %v", statePath, err)
		}

		var out bytes.Buffer
		if code := RunCLIIn(prime, &out, []string{"up"}); code != 0 {
			t.Fatalf("up after the state file was scrubbed = %d; want 0 — a lost reed.json must not wedge the worktree, output: %s", code, out.String())
		}

		newSelvagePaneID := selvagePaneID(t, prime)
		if newSelvagePaneID == oldSelvagePaneID {
			t.Fatalf("SelvagePaneID after the recovering up = %s; want a NEW id distinct from the pre-scrub Selvage pane %s", newSelvagePaneID, oldSelvagePaneID)
		}

		panes := listPaneLines(t, tmuxPath, socket, session)
		if len(panes) != 1 || !paneLiveOnSession(panes, newSelvagePaneID) {
			t.Fatalf("panes after the recovering up = %v; want exactly the freshly rebuilt Selvage pane %s", panes, newSelvagePaneID)
		}
		for _, line := range panes {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if fields[0] == oldSelvagePaneID {
				t.Errorf("old Selvage pane %s still present after the recovering up; want it reaped", oldSelvagePaneID)
			}
			if fields[0] == oldStrandPaneID {
				t.Errorf("orphaned strand pane %s still present after the recovering up; want it reaped", oldStrandPaneID)
			}
		}
		if fields := strings.Fields(panes[0]); len(fields) < 3 || fields[2] != "0" {
			t.Errorf("rebuilt Selvage pane row %q; want pane_top 0 — as the session's sole pane, Selvage must sit at the top of the window", panes[0])
		}

		pollProcessGone(t, oldStrandPID, 20*time.Second)

		// The session must be usable again, not only free of errors.
		addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host recovered", "--name", "after-scrub")
	}) {
		return
	}

	// IfAbsentReopenIsIdempotent smokes the repeated reopen sequence that --if-absent exists for.
	// A live match no-ops, a dead match relaunches under the same guid, and status never grows a second strand.
	// It drives up, then runs add --if-absent twice while the strand is alive.
	// It kills the strand's pane and runs add --if-absent a third time.
	// It asserts that status reports exactly one strand with the same guid throughout,
	// and that the strand is live only once the third add's relaunch has run.
	if !t.Run("IfAbsentReopenIsIdempotent", func(t *testing.T) {
		restartSession(t, prime)

		launch := smokeReapLaunchCmd()
		firstGUID := addStrandIn(t, prime, launch, "--if-absent", "--name", "claude")

		// A second add --if-absent while the strand is still alive must add nothing and report the same guid.
		secondGUID := addStrandIn(t, prime, launch, "--if-absent", "--name", "claude")
		if secondGUID != firstGUID {
			t.Fatalf("second add --if-absent guid = %s; want %s (the live match, left unchanged)", secondGUID, firstGUID)
		}

		assertOneStrand := func(when string) map[string]any {
			t.Helper()
			var result map[string]any
			if err := json.Unmarshal(mustRunReed(t, prime, "status"), &result); err != nil {
				t.Fatalf("parse status result %s: %v", when, err)
			}
			strands, _ := result["strands"].([]any)
			if len(strands) != 1 {
				t.Fatalf("status %s strands = %v; want exactly 1 (a repeated add --if-absent must never stack a duplicate)", when, strands)
			}
			strand, _ := strands[0].(map[string]any)
			if guid, _ := strand["guid"].(string); guid != firstGUID {
				t.Fatalf("status %s strand guid = %q; want %q", when, guid, firstGUID)
			}
			return strand
		}

		assertOneStrand("after the second add --if-absent (still alive)")

		// Kill the strand's pane directly, not via remove: the strand must stay tracked in reed's state,
		// only its pane must die, the reopen shape that --if-absent's relaunch branch exists for.
		socket, _ := socketAndSessionIn(t, prime)
		paneID := paneIDForStrandIn(t, prime, firstGUID)
		if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", paneID).Run(); err != nil {
			t.Fatalf("kill-pane %s: %v", paneID, err)
		}

		thirdGUID := addStrandIn(t, prime, launch, "--if-absent", "--name", "claude")
		if thirdGUID != firstGUID {
			t.Fatalf("third add --if-absent guid = %s; want %s (the relaunched match, same guid, not a second strand)", thirdGUID, firstGUID)
		}

		// Assert against liveness, not mere pane presence.
		// tmux keeps a session's sole pane on screen after its process dies,
		// so a pane-presence check alone would pass without the relaunch having run.
		// The "live" field is built from the alive set, not the merely present one.
		strand := assertOneStrand("after the third add --if-absent relaunch")
		if live, _ := strand["live"].(bool); !live {
			t.Fatalf("status after the relaunch: strand %s live = false; want true", firstGUID)
		}
	}) {
		return
	}

	// StrandPaneSpawnsAtToldAnchorNotProcessCwd guards against a strand pane opening in the wrong directory.
	// launchStrandLocked must pass -c with the anchor when it creates the pane.
	// Without -c, tmux takes the new pane's cwd from the calling process, not from the anchor.
	// A process that stands in the anchor hides this defect.
	// The RunCLIIn seam seeds the cwd into the execution context without moving the process,
	// so there the strand's pane comes up in the wrong tree while reed reports success.
	//
	// Both strands here open a window of their own:
	// the first with Selvage alone in the session, the second beside the first strand's window.
	// Both are exercised identically for the -c regression.
	if !t.Run("StrandPaneSpawnsAtToldAnchorNotProcessCwd", func(t *testing.T) {
		restartSession(t, prime)
		anchor := h.Location.AnchorPath()

		launch := smokeReapLaunchCmd()
		first := addStrandIn(t, anchor, launch, "--name", "first")
		second := addStrandIn(t, anchor, launch, "--name", "second")

		socket, _ := socketAndSessionIn(t, anchor)

		tests := []struct {
			name string
			guid string
		}{
			{"first strand (window beside Selvage)", first},
			{"second strand (window beside the first)", second},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				paneID := paneIDForStrandIn(t, anchor, tt.guid)
				if got := paneCurrentPath(t, tmuxPath, socket, paneID); got != anchor {
					t.Errorf("pane %s current path = %q; want the told anchor %q", paneID, got, anchor)
				}
			})
		}
	}) {
		return
	}

	// RemoveByNameDetachedFromInsideStrand runs `lym reed remove --name <n> --detach` from inside the strand it removes.
	// The detached child must outlive the pane it kills and wait for the invoking process to exit.
	// It then removes the strand: reed.json stops listing it, and the remaining pane is laid out at full height.
	if !t.Run("RemoveByNameDetachedFromInsideStrand", func(t *testing.T) {
		restartSession(t, prime)

		keeper := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "keeper")
		selfName := "selfrm"
		self := addStrandIn(t, prime, smokeInvokeLine(lymExe, "reed", "remove", "--name", selfName, "--detach"), "--name", selfName)
		socket, session := socketAndSessionIn(t, prime)

		var statusOut []byte
		deadline := time.Now().Add(60 * time.Second)
		for {
			statusOut = mustRunReed(t, prime, "status")
			if _, listed := statusStrand(t, statusOut, self); !listed {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("strand %s still listed 60s after its own detached remove; status: %s", self, statusOut)
			}
			time.Sleep(500 * time.Millisecond)
		}

		var status struct {
			Strands []struct {
				GUID string `json:"guid"`
				Live bool   `json:"live"`
			} `json:"strands"`
		}
		if err := json.Unmarshal(statusOut, &status); err != nil {
			t.Fatalf("parse status: %v", err)
		}
		if len(status.Strands) != 1 || status.Strands[0].GUID != keeper || !status.Strands[0].Live {
			t.Fatalf("remaining strands = %+v; want only the live keeper %s", status.Strands, keeper)
		}

		// The remaining panes are laid out: no dead pane is left behind and every pane has height.
		lines := listPaneLines(t, tmuxPath, socket, session)
		for _, l := range lines {
			f := strings.Fields(l)
			if len(f) != 4 {
				t.Fatalf("unexpected list-panes row %q", l)
			}
			if f[1] != "0" {
				t.Errorf("dead pane left after removal: %q", l)
			}
			if f[3] == "0" {
				t.Errorf("pane has no height after re-layout: %q", l)
			}
		}
		if err := exec.Command(tmuxPath, "-L", socket, "has-session", "-t", session).Run(); err != nil {
			t.Errorf("reed session gone after self-removal: %v", err)
		}
	}) {
		return
	}

	// AttachRendersInsideHarnessPane drives the interactive terminal handover of `lym reed attach`.
	// It runs inside a private harness tmux server whose pane starts in the worktree,
	// so the attach command it runs resolves that worktree.
	t.Run("AttachRendersInsideHarnessPane", func(t *testing.T) {
		shellPath := harnessShellBinaryPath(t)
		restartSession(t, prime)
		addStrandIn(t, prime, smokeMarkerLaunchCmd("ATTACH-MARKER-ALPHA"), "--name", "amarker")
		reedSocket, session := socketAndSessionIn(t, prime)

		harness := tmuxkit.Socket(t, tmuxPath)
		if err := exec.Command(tmuxPath, "-L", harness, "new-session", "-d", "-s", "h", "-c", prime, "-x", "140", "-y", "42",
			shellPath).Run(); err != nil {
			t.Fatalf("boot harness server: %v", err)
		}
		t.Cleanup(func() {
			reapHarnessServer(t, tmuxPath, harness)
		})
		deadline := time.Now().Add(30 * time.Second)
		for exec.Command(tmuxPath, "-L", harness, "has-session", "-t", "h").Run() != nil {
			if time.Now().After(deadline) {
				t.Fatal("harness session did not come up within 30s")
			}
			time.Sleep(100 * time.Millisecond)
		}

		harnessPane := harnessOnlyPaneID(t, tmuxPath, harness, "h")
		sendKeysLine(t, tmuxPath, harness, harnessPane, smokeAttachInvokeLine(lymExe))
		pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-MARKER-ALPHA", 20*time.Second)

		// On Unix the attach replaces the lym process with tmux,
		// so the pane's foreground command is tmux and no lym parent is left waiting.
		if runtime.GOOS != "windows" {
			command, err := exec.Command(tmuxPath, "-L", harness, "display-message", "-p", "-t", harnessPane, "#{pane_current_command}").Output()
			if err != nil {
				t.Fatalf("read the harness pane's current command: %v", err)
			}
			if got := strings.TrimSpace(string(command)); !strings.HasPrefix(got, "tmux") {
				t.Errorf("harness pane's current command = %q; want tmux, the attach having replaced the lym process", got)
			}
		}

		if err := exec.Command(tmuxPath, "-L", harness, "send-keys", "-t", harnessPane, "C-b", "d").Run(); err != nil {
			t.Fatalf("send detach keys: %v", err)
		}
		pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-EXIT:0", 15*time.Second)

		if err := exec.Command(tmuxPath, "-L", reedSocket, "has-session", "-t", session).Run(); err != nil {
			t.Errorf("reed session %s gone after detach: %v", session, err)
		}
	})
}

// pollProcessGone polls processGone until pid is gone, failing the test if timeout elapses first.
// kill-pane terminates a pane's process asynchronously.
// A caller that asserts the process is gone must poll rather than sample once right after the killing verb returns.
func pollProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if processGone(pid) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid %d still running %s after the pane that held it was reaped", pid, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite guards against booting a worktree whose name tmux would rewrite.
// It drives the CLI seam an operator uses.
//
// Without the refusal, in a hub with a sibling worktree named "svc.v2", `lym reed up` hangs until the boot timeout.
// It then leaves a session named "svc_v2" on the hub's shared per-hub server, where `lym reed status` cannot see it,
// and `lym reed down`, which targets the exact name "=svc.v2", cannot kill it.
// There is one row per rewrite class, because a single case cannot stand in for all of them.
//
// The load-bearing assertion is the last one:
// no session under the rewritten name may exist on the hub's socket after the refusal.
// A fix that merely produced a nicer error message fails there, rather than reporting a false green.
// So does a fix that sanitized the name instead of refusing it.
//
// The prime worktree is brought up first; the hub's shared tmux server is then live when the bad worktree boots.
// Each row gets its own sibling worktree, and the rows run serially.
// t.Parallel is called at the scenario level only, because the rows share the hub and its server.
func TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite(t *testing.T) {
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

	// Brings up the hub's shared tmux server, which the bad worktree boots against.
	mustRunReed(t, prime, "up")
	socket, primeSession := socketAndSessionIn(t, prime)

	tests := []struct {
		name string
		// worktreeName is the sibling worktree's directory name, and so the session name reed is told.
		worktreeName string
		// rewrittenSession is the name tmux would silently create the session under instead.
		rewrittenSession string
		// posixOnly marks a name Windows cannot hold as a directory name at all, so Windows generates no subtest for it.
		posixOnly bool
	}{
		{"dot is substituted", "rewritable.v2", "rewritable_v2", false},
		{"backslash is doubled", `rewritable\v2`, `rewritable\\v2`, true},
	}
	for _, tt := range tests {
		if tt.posixOnly && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			rewritable := materializeSibling(t, h, tt.worktreeName)
			deferHubRelease(t, rewritable)

			var out bytes.Buffer
			code := RunCLIIn(rewritable, &out, []string{"up"})
			if code == 0 {
				t.Fatalf("up in worktree %q = 0; want a non-zero refusal, output: %s", tt.worktreeName, out.String())
			}
			// Decode the envelope rather than substring-matching the raw JSON:
			// the backslash row's offending byte is JSON-escaped on the wire,
			// so a raw match would fail on a refusal that is worded correctly.
			var envelope struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("decode up refusal envelope %s: %v", out.String(), err)
			}
			if !strings.Contains(envelope.Error, tt.worktreeName) {
				t.Errorf("up refusal = %q; want it to name the offending worktree/session %q so the operator can act on it", envelope.Error, tt.worktreeName)
			}

			// Reed's own targets cannot see the stray session, so read the socket's whole session list.
			// Probing the rewritten name through reed would miss it.
			sessions, err := exec.Command(tmuxPath, "-L", socket, "list-sessions", "-F", "#{session_name}").Output()
			if err != nil {
				t.Fatalf("list-sessions on socket %s: %v", socket, err)
			}
			names := strings.Fields(strings.TrimSpace(string(sessions)))
			if slices.Contains(names, tt.rewrittenSession) {
				t.Errorf("socket %s carries session %q after the refusal (sessions: %v); reed must never create substrate it cannot address or tear down", socket, tt.rewrittenSession, names)
			}
			if !slices.Contains(names, primeSession) {
				t.Errorf("socket %s lost the prime worktree's session %q (sessions: %v); the refusal must not disturb a sibling worktree", socket, primeSession, names)
			}
		})
	}
}
