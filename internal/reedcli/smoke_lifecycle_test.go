//go:build tmux

// smoke_lifecycle_test.go drives the composed up/add/remove/status/down behaviors through RunCLIIn against a real tmux server: the basic round-trip, layout survival under stacked below-parent adds, foreign-pane reaping, recovery from a scrubbed state file, the --if-absent reopen sequence, the told pane cwd, the detached self-removal, and the interactive attach handover.

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
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// mustRunReed runs one reed verb in worktree and fails the calling test unless it exits zero; it returns the verb's output.
func mustRunReed(t *testing.T, worktree string, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, args); code != 0 {
		t.Fatalf("%v = %d; want 0, output: %s", args, code, out.String())
	}
	return out.Bytes()
}

// restartSession brings worktree's session down, if one is up, and boots a fresh one, so a scenario step starts with a session that holds no strand and no state from an earlier step.
func restartSession(t *testing.T, worktree string) {
	t.Helper()
	var buf bytes.Buffer
	RunCLIIn(worktree, &buf, []string{"down"})
	mustRunReed(t, worktree, "up")
}

// TestSmokeLifecycle runs the composed up/add/remove/status/down behaviors against one hub's prime worktree, each step starting from a freshly restarted session, so the steps share the one hub build and the one tmux server instead of building their own.
// The steps run serially in a fixed order and do not rely on each other's results.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and its tmux server.
// Every step drives reed through RunCLIIn, which seeds the worktree into the execution context without moving the process, so the told anchor and the test process's own cwd are two different directories.
func TestSmokeLifecycle(t *testing.T) {
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := lyxbin.Build(t)

	h := hubforge.NewHub(t, ".")
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})
	stateDir := filepath.Join(prime, ".lyx")

	// UpAddStatusDown boots the substrate, adds a strand, checks status, and tears down.
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

		// down: tears the server down and clears state.
		mustRunReed(t, prime, "down")
	}) {
		return
	}

	// StackedAddsKeepEverySessionPane pins the composed split-path defect this round fixed:
	// with several below-parent strands added in sequence, each add's session-target split-window must genuinely create a new pane rather than reusing an existing one — the old path could fail SILENTLY (exit 0, no new pane, prints an existing pane's id), binding the new strand to an existing pane, whose next select-layout's duplicate pane number made tmux destroy every pane in the session.
	// The fix splits the tallest alive pane explicitly and hard-errors on a non-new reported id, so this sequence must now yield one live pane per visible strand, plus one more for the always-present Selvage pane.
	if !t.Run("StackedAddsKeepEverySessionPane", func(t *testing.T) {
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

	// UpWithOnlyForeignPanesKeepsSessionUsable pins the empty-layout defect this round fixed:
	// with ZERO strands tracked and a foreign pane in the session (an operator's raw split-window), the old apply emitted a layout string enumerating no cells, which tmux answers (exit 0) by destroying EVERY pane — leaving a zero-pane zombie session in which add fails forever ("session has no panes to split") while up kept reporting success.
	// That empty-layout hazard is now covered at the unit tier by apply_test.go's TestApplyLayoutLockedOpts_GuardSkipsReturnZeroResult (applyLayoutLockedOpts' anyPlacedStrand guard skips select-layout whenever no strand owns a present pane): this fixture's two up calls always arrive at apply holding exactly one pane, well under the len(live) < 2 guard applyLayoutLockedOpts checks first, so the anyPlacedStrand branch is never even reached from here anymore.
	// What this step still proves instead: with zero strands tracked, an ALIVE Selvage now authorizes reconcile's deterministic untracked-pane reap (reconcile.go), so an up against a session holding only foreign panes leaves a USABLE session — Selvage intact, every foreign pane gone — not the old zero-pane wedge, and a subsequent add comes up live on its own fresh pane without ever displacing Selvage.
	if !t.Run("UpWithOnlyForeignPanesKeepsSessionUsable", func(t *testing.T) {
		restartSession(t, prime)
		socket, session := socketAndSessionIn(t, prime)

		// up boots the always-present Selvage pane before any strand exists, and
		// this SAME up's own reconcile (reconcileApplyPersistLocked's tail)
		// already reaps the session's not-yet-adopted initial pane: with zero
		// strands tracked, the newly-alive Selvage authorizes the untracked-pane
		// reap (reconcile.go), so that initial pane never survives past this
		// first up at all. Read Selvage's pane id directly from reed.json so
		// the assertions below can tell it apart from the foreign pane added
		// next.
		selvage := selvagePaneID(t, prime)

		// A foreign pane reed does not track (the operator-split case): the
		// session holds 1 pane (Selvage) and 0 strands going in — the first
		// up above already reaped its own not-yet-adopted initial pane — so this
		// split leaves it at 2 panes (Selvage, foreign) and 0 strands.
		if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
			t.Fatalf("foreign split-window: %v", err)
		}

		// The second up: with zero strands tracked and Selvage alive,
		// reconcile's untracked reap fires again and kills the foreign pane too
		// — this up must still exit 0 and leave the session usable, never the
		// zero-pane wedge the old empty-layout-apply defect produced.
		mustRunReed(t, prime, "up")
		if panes := listPaneLines(t, tmuxPath, socket, session); len(panes) != 1 || !paneLiveOnSession(panes, selvage) {
			t.Fatalf("up with only a foreign pane must reap it, leaving exactly Selvage %s alive; got panes=%v", selvage, panes)
		}

		// The session must still be able to host a strand: the add both proves
		// the substrate survived and (documented policy) deterministically reaps
		// the untracked foreign pane via reconcile — the strand's own pane must
		// be the one that survives, never the foreign one (psmux's positional
		// layout reaping would pick an indeterminate victim).
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

	// ForeignPaneIsReapedNotAdoptedByAdd is the faithful M16 regression: an operator's own manually-created split-window pane must never be adopted as a strand's pane, and must instead be reaped by add's reconcile.
	//
	// M16 fires only when the sole alive non-Selvage pane is the foreign one — a session that still holds its unadopted initial new-session pane has TWO alive non-Selvage panes, which is why UpWithOnlyForeignPanesKeepsSessionUsable passed even before this round's fix.
	// So the step first drives the session to a Selvage-plus-foreign-pane-only state: up, add a strand, remove that strand (its pane is gone, not corpsed — the always-present Selvage pane keeps the session up), then split a foreign pane in with the real tmux binary, exactly as UpWithOnlyForeignPanesKeepsSessionUsable does.
	//
	// The foreign pane's own #{pane_pid} is captured before the add under test and polled gone afterward, under a bounded deadline rather than sampled once immediately — kill-pane terminates a pane's process asynchronously.
	// That pid check is the load-bearing assertion, not decoration: a pane-id-only assertion would have passed for the adoption bug had ids been recycled, whereas under adoption the pane pid provably survives — that identity is exactly what M16 recorded.
	// Assert nothing about the foreign pid's descendants: the reap is kill-pane-only by decision, and descendant liveness is pinned by RemoveStrand's and Down's own tests, not here.
	if !t.Run("ForeignPaneIsReapedNotAdoptedByAdd", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)

		guid := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "throwaway")
		mustRunReed(t, prime, "remove", guid)

		socket, session := socketAndSessionIn(t, prime)

		// A foreign pane reed does not track (the operator-split case): the session now holds Selvage plus this one foreign pane, and zero strands — the sole-alive-non-Selvage-pane precondition M16 requires.
		if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
			t.Fatalf("foreign split-window: %v", err)
		}

		// The foreign pane is the one live pane that is neither Selvage nor
		// the (already-gone, kill-pane-removed-outright) removed strand's pane.
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

	// UpSurvivesAScrubbedStateFileWhileTheSessionIsUp is the end-to-end regression guard for the R4 review's R4-F4 and the M22 regression, driven at the CLI seam a real operator uses.
	//
	// Reproduced live before the R4-F4 fix: with the session up, a strand added and the default one-row Selvage band laid out, deleting .lyx/reed.json — a never-tracked machine-local tree the Durable-vs-Ephemeral State Invariant makes disposable, and exactly what `git clean -xdf` in the worktree removes — permanently wedged the worktree.
	// `lyx reed up` and `lyx reed resume` both failed, on every subsequent invocation, with `split Selvage pane: exit status 1: no space for new pane`, because the physically bottom-most pane was now an UNTRACKED one-row Selvage band that tmux cannot split, while `lyx reed status` kept reporting the session healthy and nothing named the one escape (`down`, then `up`).
	// A strand is added first so applyLayoutLocked actually applies reed's layout and the Selvage band is squeezed down to its configured one row — the whole precondition for the wedge.
	//
	// The recovering `up` must exit 0, and the M22 regression is that the scrubbed state converges on that `up` itself, not one verb later.
	// The pre-fix defect converged one verb late — the recovering up left the old Selvage pane and the orphaned strand pane both untracked-but-alive alongside the freshly split Selvage pane, and only a FOLLOW-UP verb's reconcile cleared them.
	// An assertion placed after that follow-up would pass either way, which is why this step asserts on the recovering `up` itself, with no intervening verb: the session must hold exactly one pane (the newly persisted SelvagePaneID, distinct from the captured old one), and both the old Selvage pane id and the old strand pane id must already be gone from list-panes.
	// The scrub erases the strand table along with SelvagePaneID, so the recovering up's own reconcile runs with zero strands tracked and a freshly-alive Selvage, which authorizes reaping every other pane (reconcile.go).
	// That makes the rebuilt Selvage pane's pane_top == 0 assertion TRIVIALLY true — with one pane there is nowhere else for it to be — so it is vacuous rather than live coverage.
	//
	// The orphaned strand pane's captured #{pane_pid} is polled gone too, under a bounded deadline (kill-pane terminates asynchronously) — smokeReapLaunchCmd's launched command is a CHILD of that pane's own process, not #{pane_pid} itself, so it is deliberately not what this step asserts on:
	// the leak this pins is the pane and its own process, not the whole subtree (RemoveStrand's and Down's own tests pin subtree reaping).
	//
	// The Selvage-only, full-height end state is the accepted outcome, not a layout defect to "fix" by synthesizing a spacer pane: applyLayoutLockedOpts deliberately skips select-layout when no strand owns a present pane (anyPlacedStrand, apply.go).
	// The following `add` proves the session is genuinely usable again, not merely non-erroring.
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
			t.Errorf("rebuilt Selvage pane row %q; want pane_top 0 — as the session's sole pane, Selvage must land at the origin or select-layout misassigns its cell", panes[0])
		}

		pollProcessGone(t, oldStrandPID, 20*time.Second)

		// The session must be genuinely usable again, not merely non-erroring.
		addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host recovered", "--name", "after-scrub")
	}) {
		return
	}

	// IfAbsentReopenIsIdempotent smokes the real repeated-reopen sequence --if-absent exists for: a live match no-ops, a dead match relaunches under the same guid, and status never grows a second strand across either reopen.
	// It drives up -> add --if-absent -> a second identical add --if-absent while the strand is alive -> kill its pane -> a third add --if-absent, asserting status reports exactly one strand carrying the same guid throughout, and live only once the third add's relaunch has run.
	if !t.Run("IfAbsentReopenIsIdempotent", func(t *testing.T) {
		restartSession(t, prime)

		launch := smokeReapLaunchCmd()
		firstGUID := addStrandIn(t, prime, launch, "--if-absent", "--name", "claude")

		// Second add --if-absent while the strand is still alive: must add nothing and report the same
		// guid.
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

		// Kill the strand's PANE directly, not "remove" -- the strand must stay tracked in reed's state,
		// only its pane must die, which is the reopen shape --if-absent's relaunch branch exists for.
		socket, _ := socketAndSessionIn(t, prime)
		paneID := paneIDForStrandIn(t, prime, firstGUID)
		if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", paneID).Run(); err != nil {
			t.Fatalf("kill-pane %s: %v", paneID, err)
		}

		thirdGUID := addStrandIn(t, prime, launch, "--if-absent", "--name", "claude")
		if thirdGUID != firstGUID {
			t.Fatalf("third add --if-absent guid = %s; want %s (the relaunched match, same guid, not a second strand)", thirdGUID, firstGUID)
		}

		// Assert against liveness, never mere pane presence: tmux keeps a session's sole pane on screen
		// after its process dies, so a check for pane presence alone would pass without the relaunch ever
		// having run. The "live" field is already built from the alive-not-merely-present set.
		strand := assertOneStrand("after the third add --if-absent relaunch")
		if live, _ := strand["live"].(bool); !live {
			t.Fatalf("status after the relaunch: strand %s live = false; want true", firstGUID)
		}
	}) {
		return
	}

	// StrandPaneSpawnsAtToldAnchorNotProcessCwd is the regression guard for the pane-cwd defect this round's R1 review found: launchStrandLocked's split-window carried no -c, so tmux resolved the new pane's cwd from the invoking CLIENT rather than from the anchor reed was told.
	// Verified live (tmux 3.6) that a client-issued split lands in the calling process's cwd — neither the target pane's cwd nor the session's — so a strand's command ran wherever lyx happened to stand.
	// Under a process that stands in the anchor that is accidentally right; under the RunCLIIn seam, which seeds the cwd into the execution context WITHOUT moving the process, it is not, and every strand pane came up in the wrong tree while reed reported success.
	//
	// Both strands here are splits — there is no other way a strand gets a pane — but they still take genuinely different planPaneTarget branches, worth asserting as two distinct cases rather than one duplicated twice: by the time this step's first add runs, the preceding up's own reconcile has already reaped the session down to Selvage alone (the same zero-strands-plus-alive-Selvage reap UpWithOnlyForeignPanesKeepsSessionUsable pins), so the FIRST strand's split targets Selvage itself (planPaneTarget's Selvage-as-last-resort fallback, since no non-Selvage pane exists to split otherwise).
	// The SECOND strand then targets the tallest alive non-Selvage pane — the first strand's own pane, once it exists.
	// Both are exercised for the -c regression identically: the split path is the one the defect broke, on either branch.
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
			{"first strand (splits off Selvage)", first},
			{"second strand (splits off the first)", second},
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

	// RemoveByNameDetachedFromInsideStrand runs `lyx reed remove --name <n> --detach` from inside the strand it removes.
	// The detached child must outlive the pane it kills, wait for the invoking process to exit, then remove the strand: reed.json stops listing it and the remaining pane is laid out at full height.
	if !t.Run("RemoveByNameDetachedFromInsideStrand", func(t *testing.T) {
		restartSession(t, prime)

		keeper := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "keeper")
		selfName := "selfrm"
		self := addStrandIn(t, prime, smokeInvokeLine(lyxExe, "reed", "remove", "--name", selfName, "--detach"), "--name", selfName)
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

	// AttachRendersInsideHarnessPane drives the interactive terminal handover of `lyx reed attach` inside a private harness tmux server whose pane starts in the worktree, so the attach command it runs resolves that worktree.
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
		sendKeysLine(t, tmuxPath, harness, harnessPane, smokeAttachInvokeLine(lyxExe))
		pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-MARKER-ALPHA", 20*time.Second)

		if err := exec.Command(tmuxPath, "-L", harness, "send-keys", "-t", harnessPane, "C-b", "d").Run(); err != nil {
			t.Fatalf("send detach keys: %v", err)
		}
		pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-EXIT:0", 15*time.Second)

		if err := exec.Command(tmuxPath, "-L", reedSocket, "has-session", "-t", session).Run(); err != nil {
			t.Errorf("reed session %s gone after detach: %v", session, err)
		}
	})
}

// TestSmokeRemoveLastStrandThenAddRunsTheNewCommand exercises removing a session's last STRAND and
// then adding a new one, with the always-present Selvage pane in play: removeStrandLocked collects
// pane ids from strands only, and Selvage is never a strand, so the removed strand's pane is never
// the session's actual last pane — kill-pane removes it outright, on either backend, rather than
// corpsing it under remain-on-exit, and Selvage alone is left holding the session up. The
// following add must split a fresh pane whose command genuinely runs and STAYS live across the next
// reconciling verb.
//
// This premise holds identically on tmux — see reedengine.RemoveStrand's emptied-session swallow
// (strand.go) for how that path is handled at the engine level — so the Windows-only skip below is
// kept for coverage economy, not backend-specific behavior: the equivalent
// removing-the-last-strand-then-add shape is already exercised on the tmux backend by
// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds (contract_integration_test.go), so running this
// real-tmux-session variant there too would be redundant.
func TestSmokeRemoveLastStrandThenAddRunsTheNewCommand(t *testing.T) {
	tmuxBinaryPath(t)

	// Windows-only for coverage economy, not because this backend behaves
	// differently (see the doc comment above): the equivalent
	// removing-the-last-strand-then-add shape is already exercised on the
	// tmux backend by TestRemoveStrand_SoleStrandEmptiesSessionSucceeds
	// (contract_integration_test.go), so skip here rather than duplicate it.
	if runtime.GOOS != "windows" {
		t.Skip("removing-the-last-strand-then-add is already exercised on the tmux backend by TestRemoveStrand_SoleStrandEmptiesSessionSucceeds; this real-tmux-session variant runs only on the psmux (Windows) backend to avoid redundant coverage")
	}

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	launch := "pwsh -NoExit -Command Write-Host ready"
	first := addStrand(t, launch, "--name", "first")
	out.Reset()
	if code := RunCLI(&out, []string{"remove", first}); code != 0 {
		t.Fatalf("remove = %d; want 0, output: %s", code, out.String())
	}

	second := addStrand(t, launch, "--name", "second")

	// A genuine fresh split has no reason to wobble across a reconcile, but
	// this is the shape a corpse-bound strand would have failed under: up
	// reconciles; the strand must still be live.
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("post-add up = %d; want 0, output: %s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand, found := statusStrand(t, out.Bytes(), second)
	if !found {
		t.Fatalf("status missing strand %s; output: %s", second, out.String())
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("strand added after remove-last: live = false; want true (bound to a pane reconcile then cleared?); status: %s", out.String())
	}
}

// pollProcessGone polls processGone until pid is gone or timeout elapses, failing the test on
// timeout. kill-pane terminates a pane's process asynchronously, so a caller that needs to assert a
// killed pane's process is truly gone must poll rather than sample once immediately after the killing
// verb returns.
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

// TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite is the end-to-end regression guard for the R2 review's BLOCKING finding and the R4 review's R4-F1, driven at the CLI seam a real operator uses.
//
// Reproduced live before the R2 fix, in a hub with a sibling worktree named "svc.v2":
// `lyx reed up` hung for the full 20s bootAttemptTimeout, failed with `tmux server is up but session "svc.v2" did not materialize within 20s` (a message naming neither cause nor remedy), and left a session named "svc_v2" running on the SHARED per-hub server — which `lyx reed status` could not see, `lyx reed down` could not kill (it targets the exact "=svc.v2"), and which then kept the shared server alive so no sibling worktree's `down` could tidy it either.
// R4 reproduced the identical shape for the backslash class ("bs\slash" creating "bs\\slash", verified live on tmux 3.6), which the R2/R3 pre-flight let straight through — hence one row per rewrite class here rather than a single case standing in for all of them.
//
// The load-bearing assertion is the LAST one: no session under the rewritten name may exist on the hub's socket after the refusal.
// A fix that merely produced a nicer error message, or that sanitized the name instead of refusing it, fails there rather than reporting a false green.
// The prime worktree is brought up first on purpose, so the hub's shared tmux server is genuinely live when the bad worktree tries to boot — the exact shape that produced the stray.
//
// The scenario brings the prime worktree up once, so the hub's shared tmux server is live for every row, and gives each row its own sibling worktree; the rows run serially and call t.Parallel at the scenario level only, because they share the hub and its server.
func TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite(t *testing.T) {
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})

	// A genuinely live shared per-hub server for the bad worktree to boot against.
	mustRunReed(t, prime, "up")
	socket, primeSession := socketAndSessionIn(t, prime)

	tests := []struct {
		name string
		// worktreeName is the sibling worktree directory name, and therefore the told session name.
		worktreeName string
		// rewrittenSession is the name tmux would silently create the session under instead.
		rewrittenSession string
		// posixOnly marks a name Windows cannot hold as a directory name at all.
		posixOnly bool
	}{
		{"dot is substituted", "rewritable.v2", "rewritable_v2", false},
		{"backslash is doubled", `rewritable\v2`, `rewritable\\v2`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.posixOnly && runtime.GOOS == "windows" {
				t.Skip("the offending byte is a path separator on Windows, so no directory can carry it")
			}
			rewritable := materializeSibling(t, h, tt.worktreeName)
			deferHubRelease(t, rewritable)

			var out bytes.Buffer
			code := RunCLIIn(rewritable, &out, []string{"up"})
			if code == 0 {
				t.Fatalf("up in worktree %q = 0; want a non-zero refusal, output: %s", tt.worktreeName, out.String())
			}
			// Decode the envelope rather than substring-matching the raw JSON:
			// the backslash row's own offending byte is JSON-escaped on the
			// wire, so a raw match would fail on a refusal that is in fact
			// perfectly worded.
			var envelope struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("decode up refusal envelope %s: %v", out.String(), err)
			}
			if !strings.Contains(envelope.Error, tt.worktreeName) {
				t.Errorf("up refusal = %q; want it to name the offending worktree/session %q so the operator can act on it", envelope.Error, tt.worktreeName)
			}

			// The stray. Read the socket's whole session list rather than probing
			// the rewritten name through reed, since reed's own targets are
			// exactly what cannot see it.
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
