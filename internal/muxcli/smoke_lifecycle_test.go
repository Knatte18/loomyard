//go:build smoke

package muxcli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestSmokeUpAddStatusDown boots the substrate, adds one strand with a cheap
// placeholder command, verifies status reports it tracked and live, then
// tears the substrate back down. Skipped when psmux is not found at the
// configured/default path so a -tags=smoke run never hard-fails on a
// machine without the tool installed.
func TestSmokeUpAddStatusDown(t *testing.T) {
	psmuxBinaryPath(t)

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
// number made psmux destroy every pane in the session. The fix splits the
// tallest alive pane explicitly and hard-errors on a non-new reported id,
// so this sequence must now yield one live pane per visible strand.
func TestSmokeStackedAddsKeepEverySessionPane(t *testing.T) {
	psmuxPath := psmuxBinaryPath(t)

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
	panes := listPaneLines(t, psmuxPath, socket, session)
	if len(panes) != len(guids) {
		t.Fatalf("session holds %d panes %v; want %d (one per visible strand — a shortfall means a silent split failure destroyed panes)", len(panes), panes, len(guids))
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
	psmuxBinaryPath(t)

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
// old apply emitted a layout string enumerating no cells, which psmux
// answers (exit 0) by destroying EVERY pane — leaving a zero-pane zombie
// session in which add fails forever ("session has no panes to adopt or
// split") while up keeps reporting success. Now (a) apply is skipped when no
// strand owns a present pane, so the foreign panes survive an up, and (b)
// even a zero-pane husk (simulated separately below via the same foreign
// route) is healed by the next up's fresh boot.
func TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable(t *testing.T) {
	psmuxPath := psmuxBinaryPath(t)

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

	// A foreign pane mux does not track (the operator-split case): the
	// session now has 2 panes and 0 strands.
	if err := exec.Command(psmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
		t.Fatalf("foreign split-window: %v", err)
	}

	// The trap: up with zero placeable strands must NOT apply an empty
	// layout. Every pane must survive it.
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("second up = %d; want 0, output: %s", code, out.String())
	}
	if panes := listPaneLines(t, psmuxPath, socket, session); len(panes) == 0 {
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
	panes := listPaneLines(t, psmuxPath, socket, session)
	if len(panes) != 1 || !strings.HasPrefix(panes[0], strandPane+" ") {
		t.Errorf("after add, session panes = %v; want exactly the strand's pane %s (foreign pane must be reaped, strand pane never displaced)", panes, strandPane)
	}
}
