//go:build smoke

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
)

// TestSmokeUpAddStatusDown boots the substrate, adds a strand, checks status, and tears down.
func TestSmokeUpAddStatusDown(t *testing.T) {
	tmuxBinaryPath(t)

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

	out.Reset()
	if code := RunCLI(&out, []string{"add", "--cmd", "pwsh -NoExit -Command Write-Host ready"}); code != 0 {
		t.Fatalf("add = %d; want 0, output: %s", code, out.String())
	}
	var addResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &addResult); err != nil {
		t.Fatalf("parse add result: %v", err)
	}
	guid, _ := addResult["guid"].(string)
	if guid == "" {
		t.Fatalf("add result missing guid: %v", addResult)
	}

	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	var statusResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &statusResult); err != nil {
		t.Fatalf("parse status result: %v", err)
	}
	strands, _ := statusResult["strands"].([]any)
	found := false
	for _, s := range strands {
		strand, _ := s.(map[string]any)
		if strand["guid"] != guid {
			continue
		}
		found = true
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("status strand %s live = false; want true", guid)
		}
	}
	if !found {
		t.Errorf("status strands missing guid %s; got: %v", guid, strands)
	}

	// down: tears the server down and clears state.
	out.Reset()
	if code := RunCLI(&out, []string{"down"}); code != 0 {
		t.Fatalf("down = %d; want 0, output: %s", code, out.String())
	}
}

// TestSmokeStackedAddsKeepEverySessionPane pins the composed split-path defect this round fixed:
// with several below-parent strands added in sequence, each add's session-target split-window must
// genuinely create a new pane rather than reusing an existing one — the old path could fail
// SILENTLY (exit 0, no new pane, prints an existing pane's id), binding the new strand to an
// existing pane, whose next select-layout's duplicate pane number made tmux destroy every pane in
// the session.
// The fix splits the tallest alive pane explicitly and hard-errors on a non-new reported id, so
// this sequence must now yield one live pane per visible strand, plus one more for the
// always-present header pane.
func TestSmokeStackedAddsKeepEverySessionPane(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

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
	guids := []string{
		addStrand(t, launch, "--name", "strand1"),
		addStrand(t, launch, "--name", "strand2"),
		addStrand(t, launch, "--name", "stack1"),
		addStrand(t, launch, "--name", "stack2"),
	}

	socket, session := socketAndSession(t)
	panes := listPaneLines(t, tmuxPath, socket, session)
	wantPanes := len(guids) + 1 // +1 for the always-present header pane
	if len(panes) != wantPanes {
		t.Fatalf("session holds %d panes %v; want %d (one per visible strand plus the header pane — a shortfall means a silent split failure destroyed panes)", len(panes), panes, wantPanes)
	}

	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	for _, guid := range guids {
		strand, found := statusStrand(t, out.Bytes(), guid)
		if !found {
			t.Fatalf("status missing strand %s; output: %s", guid, out.String())
		}
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("strand %s (%v) live = false; want true", guid, strand["name"])
		}
	}
}

// TestSmokeRemoveLastStrandThenAddRunsTheNewCommand pins the corpse-pane adoption defect this round
// fixed: kill-pane on a session's SOLE pane does not remove it — under remain-on-exit psmux corpses
// it as pane_dead=1 with exit 0 — and the old adopt path then bound the next added strand to that
// corpse, silently swallowing its send-keys (the command never ran, and the next verb's reconcile
// stripped the binding again).
// The fix never adopts a dead pane, so the post-remove add must yield a strand that is live and
// STAYS live across the next reconciling verb.
//
// Caveat: this corpse-pane premise is PSMUX-SPECIFIC.
// tmux behaves oppositely — killing a session's true last pane DESTROYS the session (and, if it was
// the server's only session, the server exits) rather than corpsing it — so this test's sole-pane
// remove would never reach an "adopt or not" decision at all on tmux;
// see reedengine.RemoveStrand's emptied-session swallow (strand.go) for how that backend is
// handled.
func TestSmokeRemoveLastStrandThenAddRunsTheNewCommand(t *testing.T) {
	tmuxBinaryPath(t)

	// This test's whole premise — kill-pane on a session's sole pane corpses
	// it (pane_dead=1, exit 0) rather than destroying the session — is
	// PSMUX-SPECIFIC (see the doc comment above): on native tmux, killing a
	// session's true last pane destroys the session outright, so the remove
	// call below never reaches an "adopt a corpse or not" decision at all —
	// there is nothing left to adopt, correctly, by design (see
	// reedengine.RemoveStrand's emptied-session swallow in strand.go). Skip
	// rather than hard-fail a scenario this backend cannot reach; the
	// emptied-session path itself is covered by
	// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds
	// (contract_integration_test.go).
	if runtime.GOOS != "windows" {
		t.Skip("corpse-pane-adoption premise is PSMUX-SPECIFIC; on native tmux, removing a session's sole strand destroys the session instead of corpsing its pane, so this scenario cannot occur here (see TestRemoveStrand_SoleStrandEmptiesSessionSucceeds for the tmux-side coverage)")
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

	// The reconciling verb is the trap: with the old corpse adoption the
	// strand read live immediately after add (its binding named the corpse,
	// still present), and only the next reconcile exposed the lie by
	// clearing the binding. up reconciles; the strand must still be live.
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
		t.Errorf("strand added after remove-last: live = false; want true (adopted a dead corpse pane?); status: %s", out.String())
	}
}

// TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable pins the empty-layout defect this round fixed:
// with ZERO strands tracked and a foreign pane in the session (an operator's raw split-window), the
// old apply emitted a layout string enumerating no cells, which tmux answers (exit 0) by destroying
// EVERY pane — leaving a zero-pane zombie session in which add fails forever ("session has no panes
// to split") while up kept reporting success.
// That empty-layout hazard is now covered at the unit tier by apply_test.go's
// TestApplyLayoutLockedOpts_GuardSkipsReturnZeroResult (applyLayoutLockedOpts' anyPlacedStrand guard
// skips select-layout whenever no strand owns a present pane): this fixture's two up calls always
// arrive at apply holding exactly one pane, well under the len(live) < 2 guard applyLayoutLockedOpts
// checks first, so the anyPlacedStrand branch is never even reached from here anymore.
// What this test still proves instead: with zero strands tracked, an ALIVE header now authorizes
// reconcile's deterministic untracked-pane reap (reconcile.go), so an up against a session holding
// only foreign panes leaves a USABLE session — the header pane intact, every foreign pane gone — not
// the old zero-pane wedge, and a subsequent add comes up live on its own fresh pane without ever
// displacing the header.
func TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

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
	socket, session := socketAndSession(t)

	// up boots the always-present header pane before any strand exists, and
	// this SAME up's own reconcile (reconcileApplyPersistLocked's tail)
	// already reaps the session's not-yet-adopted initial pane: with zero
	// strands tracked, the newly-alive header authorizes the untracked-pane
	// reap (reconcile.go), so that initial pane never survives past this
	// first up at all. Read the header's pane id directly from reed.json so
	// the assertions below can tell it apart from the foreign pane added
	// next.
	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
	}
	headerPaneID := st.HeaderPaneID

	// A foreign pane reed does not track (the operator-split case): the
	// session holds 1 pane (the header) and 0 strands going in — the first
	// up above already reaped its own not-yet-adopted initial pane — so this
	// split leaves it at 2 panes (header, foreign) and 0 strands.
	if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
		t.Fatalf("foreign split-window: %v", err)
	}

	// The second up: with zero strands tracked and the header alive,
	// reconcile's untracked reap fires again and kills the foreign pane too
	// — this up must still exit 0 and leave the session usable, never the
	// zero-pane wedge the old empty-layout-apply defect produced.
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("second up = %d; want 0, output: %s", code, out.String())
	}
	if panes := listPaneLines(t, tmuxPath, socket, session); len(panes) != 1 || !paneLiveOnSession(panes, headerPaneID) {
		t.Fatalf("up with only a foreign pane must reap it, leaving exactly the header pane %s alive; got panes=%v", headerPaneID, panes)
	}

	// The session must still be able to host a strand: the add both proves
	// the substrate survived and (documented policy) deterministically reaps
	// the untracked foreign pane via reconcile — the strand's own pane must
	// be the one that survives, never the foreign one (psmux's positional
	// layout reaping would pick an indeterminate victim).
	guid := addStrand(t, "pwsh -NoExit -Command Write-Host ready", "--name", "after-foreign")
	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand, found := statusStrand(t, out.Bytes(), guid)
	if !found {
		t.Fatalf("status missing strand %s; output: %s", guid, out.String())
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("strand added after foreign-pane up: live = false; want true; status: %s", out.String())
	}
	strandPane, _ := strand["paneId"].(string)
	panes := listPaneLines(t, tmuxPath, socket, session)
	if len(panes) != 2 || !paneLiveOnSession(panes, strandPane) || !paneLiveOnSession(panes, headerPaneID) {
		t.Errorf("after add, session panes = %v; want exactly the strand's pane %s and the header pane %s (foreign pane must be reaped, neither pane ever displaced)", panes, strandPane, headerPaneID)
	}
}

// TestSmokeHeaderPaneDisplaysRenderedHeaderText pins the header pane's actual OUTPUT — the rendered
// "hub: <hub path>" line from the embedded default template — not merely its liveness.
// This is the regression test for the header-cwd defect the fable-header-r1 round found: the pane
// used to be split with -c set to the HUB path, a container directory that is by definition not a
// git repo (the engine field that then held it is long gone; today the anchor arrives as the told
// Geometry.AnchorPath),
// so its "lyx reed header --blocking" command died at geometry resolution ({"ok":false,"error":"not
// a git repository"}) and the operator console showed a JSON error over a bash prompt forever —
// while every liveness-only assertion stayed green, because the pane's parent shell survived the
// failed command.
// Two things make content assertable here where the other smoke tests cannot: up must run as a
// SUBPROCESS of the built lyx binary (the header pane boots os.Executable() + " reed header
// --blocking", and an in-process RunCLI's executable is this TEST binary, whose header invocation
// is nonsense), and the assertion polls capture-pane for the rendered text rather than list-panes
// for presence.
func TestSmokeHeaderPaneDisplaysRenderedHeaderText(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := buildLyxBinary(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	upCmd := exec.Command(lyxExe, "reed", "up")
	upCmd.Dir = h.PrimeWorktree()
	if out, err := upCmd.CombinedOutput(); err != nil {
		t.Fatalf("built-binary up: %v\n%s", err, out)
	}

	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
	}

	socket, _ := socketAndSession(t)
	// The embedded default template renders "hub: {{.hub}}"; the fixture's
	// hub is its temp container. A JSON error body in the pane (the pre-fix
	// symptom) can never contain this line.
	pollPaneContains(t, tmuxPath, socket, st.HeaderPaneID, "hub: "+h.Location.HubPath, 20*time.Second)

	// The 1-row regression (fable-header-r1 F10): once a strand exists the
	// header clamps to its configured single row (height_rows: 1), and
	// capture-pane's default output is the VISIBLE area only — so this
	// second poll proves the rendered text sits ON that one visible row.
	// Pre-fix, the pane's echoed launch line plus a trailing newline left
	// the cursor on a fresh empty row, which was the only row the 1-row
	// pane showed; the text existed solely in scrollback.
	addCmd := exec.Command(lyxExe, "reed", "add", "--cmd", smokeReapLaunchCmd(), "--name", "clamps-header")
	addCmd.Dir = h.PrimeWorktree()
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("built-binary add: %v\n%s", err, out)
	}
	pollPaneContains(t, tmuxPath, socket, st.HeaderPaneID, "hub: "+h.Location.HubPath, 20*time.Second)
}

// TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile pins the header-pane keepalive guarantee this
// batch adds: the always-present header pane must survive a full up -> add -> remove -> add cycle
// and every reconcile along the way, never adopted/reaped as a strand's pane, and — the whole point
// — still alive even when the strand table momentarily drops to zero after a remove.
// Mirrors TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable's tmux-driven verification style
// (list-panes via the real binary, not reed's own reporting) but for the header instead of a
// foreign pane.
func TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

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

	// up boots the header pane before any strand exists (card 17). Read the
	// persisted pane id directly from reed.json (RunCLI/status carries no
	// header field) rather than assuming which of the session's panes it is.
	st, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
	}
	headerPaneID := st.HeaderPaneID

	socket, session := socketAndSession(t)
	requireHeaderAlive := func(when string) {
		t.Helper()
		lines := listPaneLines(t, tmuxPath, socket, session)
		if !paneLiveOnSession(lines, headerPaneID) {
			t.Fatalf("header pane %s not alive %s; panes=%v", headerPaneID, when, lines)
		}
	}
	requireHeaderAlive("right after up (zero strands)")

	// add: the header must never be adopted as this first strand's pane —
	// planPaneTarget excludes it from adoption, so the strand lands on the
	// session's other (pre-header) pane instead.
	guid := addStrand(t, "pwsh -NoExit -Command Write-Host ready", "--name", "first")
	requireHeaderAlive("after add")

	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand, found := statusStrand(t, out.Bytes(), guid)
	if !found {
		t.Fatalf("status missing strand %s; output: %s", guid, out.String())
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("strand %s live = false; want true", guid)
	}
	strandPaneID, _ := strand["paneId"].(string)
	if strandPaneID == "" || strandPaneID == headerPaneID {
		t.Fatalf("strand %s paneId = %q, want a real, non-header pane id", guid, strandPaneID)
	}

	// remove: the session's only strand is gone, but the header pane — a
	// permanent second pane — must keep the session (and itself) alive,
	// exactly the invariant contract_integration_test.go's
	// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds pins at the engine
	// level.
	out.Reset()
	if code := RunCLI(&out, []string{"remove", guid}); code != 0 {
		t.Fatalf("remove = %d; want 0, output: %s", code, out.String())
	}
	if !sessionAlive(tmuxPath, socket, session) {
		t.Fatalf("session %s died after removing its sole strand; the header pane must have kept it alive", session)
	}
	requireHeaderAlive("after removing the sole strand (zero strands tracked)")

	// A reconciling verb (up) with zero strands must not disturb the header
	// either — mirrors TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable's
	// same-shaped assertion for a foreign pane.
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("post-remove up = %d; want 0, output: %s", code, out.String())
	}
	requireHeaderAlive("after a reconciling up with zero strands")

	// add again: the header must still never be adopted, and the new
	// strand must come up live — the substrate the header keeps alive is
	// still genuinely usable, not a wedged husk.
	second := addStrand(t, "pwsh -NoExit -Command Write-Host ready", "--name", "second")
	requireHeaderAlive("after a second add with strands now bound")

	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand2, found := statusStrand(t, out.Bytes(), second)
	if !found {
		t.Fatalf("status missing strand %s; output: %s", second, out.String())
	}
	if live, _ := strand2["live"].(bool); !live {
		t.Errorf("strand %s live = false; want true", second)
	}
	if paneID, _ := strand2["paneId"].(string); paneID == headerPaneID {
		t.Errorf("strand %s was bound to the header pane %s; the header must never be adopted", second, headerPaneID)
	}
}

// TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp is the end-to-end regression guard for
// the R4 review's R4-F4, driven at the CLI seam a real operator uses.
//
// Reproduced live before the fix: with the session up, a strand added and the default one-row header
// band laid out, deleting .lyx/reed.json — a never-tracked machine-local tree the
// Durable-vs-Ephemeral State Invariant makes disposable, and exactly what `git clean -xdf` in the
// worktree removes — permanently wedged the worktree. `lyx reed up` and `lyx reed resume` both
// failed, on every subsequent invocation, with
// `split header pane: exit status 1: no space for new pane`, because the physically topmost pane was
// now an UNTRACKED one-row header band that tmux cannot split, while `lyx reed status` kept
// reporting the session healthy and nothing named the one escape (`down`, then `up`).
//
// The load-bearing assertions are that the recovering `up` exits 0 AND that the rebuilt header pane
// really is at pane_top 0 — a fix that recovered by splitting somewhere else would let the next
// select-layout assign the header's one-row cell to a strand positionally.
func TestSmokeUpSurvivesAScrubbedStateFileWhileTheSessionIsUp(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

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
	// A strand, so applyLayoutLocked actually applies reed's layout and the
	// header band is squeezed down to its configured one row — the whole
	// precondition for the wedge.
	addStrand(t, "pwsh -NoExit -Command Write-Host ready", "--name", "before-scrub")
	socket, session := socketAndSession(t)

	stateDir := filepath.Join(h.PrimeWorktree(), ".lyx")
	statePath := filepath.Join(stateDir, "reed.json")
	if err := os.Remove(statePath); err != nil {
		t.Fatalf("remove %s: %v", statePath, err)
	}

	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up after the state file was scrubbed = %d; want 0 — a lost reed.json must not wedge the worktree, output: %s", code, out.String())
	}

	st, err := reedengine.LoadState(stateDir)
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after the recovering up = (%+v, %v); want a freshly persisted HeaderPaneID", st, err)
	}
	headerTop := ""
	for _, line := range listPaneLines(t, tmuxPath, socket, session) {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == st.HeaderPaneID {
			headerTop = fields[2]
		}
	}
	if headerTop != "0" {
		t.Errorf("rebuilt header pane %s has pane_top %q; want %q — the header must land physically topmost or select-layout misassigns its cell", st.HeaderPaneID, headerTop, "0")
	}

	// The session must be genuinely usable again, not merely non-erroring.
	addStrand(t, "pwsh -NoExit -Command Write-Host recovered", "--name", "after-scrub")
}

// TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite is the end-to-end regression guard for the R2
// review's BLOCKING finding and the R4 review's R4-F1, driven at the CLI seam a real operator uses.
//
// Reproduced live before the R2 fix, in a hub with a sibling worktree named "svc.v2":
// `lyx reed up` hung for the full 20s bootAttemptTimeout, failed with
// `tmux server is up but session "svc.v2" did not materialize within 20s` (a message naming neither
// cause nor remedy), and left a session named "svc_v2" running on the SHARED per-hub server — which
// `lyx reed status` could not see, `lyx reed down` could not kill (it targets the exact "=svc.v2"),
// and which then kept the shared server alive so no sibling worktree's `down` could tidy it either.
// R4 reproduced the identical shape for the backslash class ("bs\slash" creating "bs\\slash",
// verified live on tmux 3.6), which the R2/R3 pre-flight let straight through — hence one row per
// rewrite class here rather than a single case standing in for all of them.
//
// The load-bearing assertion is the LAST one: no session under the rewritten name may exist on the
// hub's socket after the refusal. A fix that merely produced a nicer error message, or that
// sanitized the name instead of refusing it, fails there rather than reporting a false green.
// The prime worktree is brought up first on purpose, so the hub's shared tmux server is genuinely
// live when the bad worktree tries to boot — the exact shape that produced the stray.
func TestSmokeUpRefusesAWorktreeNameTmuxWouldRewrite(t *testing.T) {
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
			tmuxPath := tmuxBinaryPath(t)

			h := hubforge.NewHub(t, ".")
			rewritable := materializeSibling(t, h, tt.worktreeName)

			deferHubRelease(t, h.PrimeWorktree())
			deferHubRelease(t, rewritable)
			t.Cleanup(func() {
				_ = os.Chdir(h.PrimeWorktree())
				var buf bytes.Buffer
				RunCLI(&buf, []string{"down"})
			})

			// A genuinely live shared per-hub server for the bad worktree to boot against.
			mustChdir(t, h.PrimeWorktree())
			var out bytes.Buffer
			if code := RunCLI(&out, []string{"up"}); code != 0 {
				t.Fatalf("prime up = %d; want 0, output: %s", code, out.String())
			}
			socket, primeSession := socketAndSession(t)

			mustChdir(t, rewritable)
			out.Reset()
			code := RunCLI(&out, []string{"up"})
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
