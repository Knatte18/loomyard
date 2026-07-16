//go:build smoke

package muxcli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestSmokeUpAddStatusDown boots the substrate, adds one strand with a cheap
// placeholder command, verifies status reports it tracked and live, then
// tears the substrate back down. Skipped when tmux is not found in PATH or
// LYX_MUX_TMUX so a -tags=smoke run never hard-fails on a machine without
// the tool installed.
func TestSmokeUpAddStatusDown(t *testing.T) {
	tmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)

	// Always attempt to tear the server down, even if an assertion below
	// fails partway through, so a failed run does not leak a live server.
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	// up: boots the substrate (server + session), no strand command runs yet.
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	// add: a cheap placeholder command instead of a real Claude session.
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

	// status: the added strand must be tracked and reported live.
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

// TestSmokeStackedAddsKeepEverySessionPane pins the composed split-path
// defect this round fixed: with several below-parent strands added in
// sequence, each add's session-target split-window must genuinely create a
// new pane rather than reusing an existing one — the old path could fail
// SILENTLY (exit 0, no new pane, prints an existing pane's id), binding the
// new strand to an existing pane, whose next select-layout's duplicate pane
// number made tmux destroy every pane in the session. The fix splits the
// tallest alive pane explicitly and hard-errors on a non-new reported id,
// so this sequence must now yield one live pane per visible strand, plus
// one more for the always-present header pane.
func TestSmokeStackedAddsKeepEverySessionPane(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
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

// TestSmokeRemoveLastStrandThenAddRunsTheNewCommand pins the corpse-pane
// adoption defect this round fixed: kill-pane on a session's SOLE pane does
// not remove it — under remain-on-exit psmux corpses it as pane_dead=1 with
// exit 0 — and the old adopt path then bound the next added strand to that
// corpse, silently swallowing its send-keys (the command never ran, and the
// next verb's reconcile stripped the binding again). The fix never adopts a
// dead pane, so the post-remove add must yield a strand that is live and
// STAYS live across the next reconciling verb.
//
// Caveat: this corpse-pane premise is PSMUX-SPECIFIC. tmux behaves
// oppositely — killing a session's true last pane DESTROYS the session
// (and, if it was the server's only session, the server exits) rather than
// corpsing it — so this test's sole-pane remove would never reach an
// "adopt or not" decision at all on tmux; see muxengine.RemoveStrand's
// emptied-session swallow (strand.go) for how that backend is handled.
func TestSmokeRemoveLastStrandThenAddRunsTheNewCommand(t *testing.T) {
	tmuxBinaryPath(t)

	// This test's whole premise — kill-pane on a session's sole pane corpses
	// it (pane_dead=1, exit 0) rather than destroying the session — is
	// PSMUX-SPECIFIC (see the doc comment above): on native tmux, killing a
	// session's true last pane destroys the session outright, so the remove
	// call below never reaches an "adopt a corpse or not" decision at all —
	// there is nothing left to adopt, correctly, by design (see
	// muxengine.RemoveStrand's emptied-session swallow in strand.go). Skip
	// rather than hard-fail a scenario this backend cannot reach; the
	// emptied-session path itself is covered by
	// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds
	// (contract_integration_test.go).
	if runtime.GOOS != "windows" {
		t.Skip("corpse-pane-adoption premise is PSMUX-SPECIFIC; on native tmux, removing a session's sole strand destroys the session instead of corpsing its pane, so this scenario cannot occur here (see TestRemoveStrand_SoleStrandEmptiesSessionSucceeds for the tmux-side coverage)")
	}

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
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

// TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable pins the empty-layout
// defect this round fixed: with ZERO strands tracked and a foreign pane in
// the session (an operator's raw split-window — 2+ panes, none mux's), the
// old apply emitted a layout string enumerating no cells, which tmux
// answers (exit 0) by destroying EVERY pane — leaving a zero-pane zombie
// session in which add fails forever ("session has no panes to adopt or
// split") while up keeps reporting success. Now (a) apply is skipped when no
// strand owns a present pane, so the foreign panes survive an up, and (b)
// even a zero-pane husk (simulated separately below via the same foreign
// route) is healed by the next up's fresh boot.
func TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	socket, session := socketAndSession(t)

	// up already booted the always-present header pane before any strand
	// exists, alongside the session's not-yet-adopted initial pane (2
	// panes, 0 strands). Read the header's pane id directly from mux.json
	// so the assertions below can tell it apart from the foreign pane.
	st, err := muxengine.LoadState(fixture.Layout.DotLyxDir())
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
	}
	headerPaneID := st.HeaderPaneID

	// A foreign pane mux does not track (the operator-split case): the
	// session now has 3 panes (header, the not-yet-adopted initial pane,
	// and this foreign one) and 0 strands.
	if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
		t.Fatalf("foreign split-window: %v", err)
	}

	// The trap: up with zero placeable strands must NOT apply an empty
	// layout. Every pane must survive it.
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("second up = %d; want 0, output: %s", code, out.String())
	}
	if panes := listPaneLines(t, tmuxPath, socket, session); len(panes) == 0 {
		t.Fatalf("up with only foreign panes destroyed the session's pane set (zero panes remain)")
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

// TestSmokeHeaderPaneDisplaysRenderedHeaderText pins the header pane's
// actual OUTPUT — the rendered "hub: <hub path>" line from the embedded
// default template — not merely its liveness. This is the regression test
// for the header-cwd defect the fable-header-r1 round found: the pane used
// to be split with -c layout.Hub, a container directory that is by
// definition not a git repo, so its "lyx mux header --blocking" command
// died at geometry resolution ({"ok":false,"error":"not a git repository"})
// and the operator console showed a JSON error over a bash prompt forever —
// while every liveness-only assertion stayed green, because the pane's
// parent shell survived the failed command. Two things make content
// assertable here where the other smoke tests cannot: up must run as a
// SUBPROCESS of the built lyx binary (the header pane boots
// os.Executable() + " mux header --blocking", and an in-process RunCLI's
// executable is this TEST binary, whose header invocation is nonsense), and
// the assertion polls capture-pane for the rendered text rather than
// list-panes for presence.
func TestSmokeHeaderPaneDisplaysRenderedHeaderText(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := buildLyxBinary(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	upCmd := exec.Command(lyxExe, "mux", "up")
	upCmd.Dir = fixture.Hub
	if out, err := upCmd.CombinedOutput(); err != nil {
		t.Fatalf("built-binary up: %v\n%s", err, out)
	}

	st, err := muxengine.LoadState(fixture.Layout.DotLyxDir())
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after up = (%+v, %v), want a persisted HeaderPaneID", st, err)
	}

	socket, _ := socketAndSession(t)
	// The embedded default template renders "hub: {{.hub}}"; the fixture's
	// hub is its temp container. A JSON error body in the pane (the pre-fix
	// symptom) can never contain this line.
	pollPaneContains(t, tmuxPath, socket, st.HeaderPaneID, "hub: "+fixture.Layout.Hub, 20*time.Second)

	// The 1-row regression (fable-header-r1 F10): once a strand exists the
	// header clamps to its configured single row (height_rows: 1), and
	// capture-pane's default output is the VISIBLE area only — so this
	// second poll proves the rendered text sits ON that one visible row.
	// Pre-fix, the pane's echoed launch line plus a trailing newline left
	// the cursor on a fresh empty row, which was the only row the 1-row
	// pane showed; the text existed solely in scrollback.
	addCmd := exec.Command(lyxExe, "mux", "add", "--cmd", smokeReapLaunchCmd(), "--name", "clamps-header")
	addCmd.Dir = fixture.Hub
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("built-binary add: %v\n%s", err, out)
	}
	pollPaneContains(t, tmuxPath, socket, st.HeaderPaneID, "hub: "+fixture.Layout.Hub, 20*time.Second)
}

// TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile pins the header-pane
// keepalive guarantee this batch adds: the always-present header pane must
// survive a full up -> add -> remove -> add cycle and every reconcile along
// the way, never adopted/reaped as a strand's pane, and — the whole point —
// still alive even when the strand table momentarily drops to zero after a
// remove. Mirrors TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable's
// tmux-driven verification style (list-panes via the real binary, not mux's
// own reporting) but for the header instead of a foreign pane.
func TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	// up boots the header pane before any strand exists (card 17). Read the
	// persisted pane id directly from mux.json (RunCLI/status carries no
	// header field) rather than assuming which of the session's panes it is.
	st, err := muxengine.LoadState(fixture.Layout.DotLyxDir())
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
