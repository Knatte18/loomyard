//go:build smoke

package muxcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestSmokeAttachRendersInsideHarnessPane drives the one verb no headless
// test could previously reach: the interactive terminal handover of
// `lyx mux attach`. A pane inside a separate harness tmux server has a
// real ConPTY terminal, so running lyx mux attach THERE (with TMUX_SESSION
// unset — tmux refuses nesting otherwise) exercises the full handover:
// pre-flight, stdio inheritance, tmux attach, and actual rendering. The
// harness pane's capture must show the mux session's strand content and
// status bar, and after a C-b d detach the attach process must exit 0.
func TestSmokeAttachRendersInsideHarnessPane(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	shellPath := harnessShellBinaryPath(t)
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

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	addStrand(t, smokeMarkerLaunchCmd("ATTACH-MARKER-ALPHA"), "--name", "amarker")
	muxSocket, session := socketAndSession(t)

	// Harness server on its own socket, spawned with cwd = the fixture hub
	// so the lyx process typed into its pane resolves the right geometry.
	harness := fmt.Sprintf("lyx-attach-harness-%d", os.Getpid())
	if err := exec.Command(tmuxPath, "-L", harness, "new-session", "-d", "-s", "h", "-x", "140", "-y", "42",
		shellPath).Run(); err != nil {
		t.Fatalf("boot harness server: %v", err)
	}
	// Reap the harness server's WHOLE process subtree before the framework's
	// TempDir cleanup runs. The harness is this test's own scaffolding, spawned
	// with cwd = the fixture hub, so its server + __warm__ helper + pane shells
	// all keep the fixture hub directory busy; mux's own down reap never covers
	// a foreign harness socket. Without this wait the harness's async teardown
	// can outlive TempDir's RemoveAll under load and fail it with a
	// worktree-dir-in-use error — a test-harness artifact, not a mux defect.
	t.Cleanup(func() {
		reapHarnessServer(t, tmuxPath, harness)
	})
	// Saturation-sized boot deadline: a quiet harness boot is ~1s, but
	// concurrent suites pegging the CPU starve it well past 10s.
	deadline := time.Now().Add(30 * time.Second)
	for exec.Command(tmuxPath, "-L", harness, "has-session", "-t", "h").Run() != nil {
		if time.Now().After(deadline) {
			t.Fatal("harness session did not come up within 30s")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The harness's own single pane, resolved rather than hardcoded as
	// "%1": pane ids are a per-SERVER counter starting at %0 on real tmux,
	// but psmux's internal "__warm__" helper pane consumes %0 first, so a
	// psmux harness's one visible pane really is %1 — a psmux-specific
	// artifact real tmux does not replicate (verified live: a fresh real
	// tmux server's first pane is %0).
	harnessPane := harnessOnlyPaneID(t, tmuxPath, harness, "h")

	// The handover under test: attach to the mux session from inside the
	// harness pane. TMUX_SESSION must be unset or tmux refuses to nest.
	sendKeysLine(t, tmuxPath, harness, harnessPane, smokeAttachInvokeLine(lyxExe))

	// The harness pane now renders the INNER session: the strand's marker
	// only ever existed inside the mux session, so seeing it here proves
	// the attach handover rendered for real.
	pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-MARKER-ALPHA", 20*time.Second)

	// Detach (prefix C-b, then d) and confirm the attach process exited 0.
	if err := exec.Command(tmuxPath, "-L", harness, "send-keys", "-t", harnessPane, "C-b", "d").Run(); err != nil {
		t.Fatalf("send detach keys: %v", err)
	}
	pollPaneContains(t, tmuxPath, harness, harnessPane, "ATTACH-EXIT:0", 15*time.Second)

	// The mux session itself must have survived the client detaching.
	if err := exec.Command(tmuxPath, "-L", muxSocket, "has-session", "-t", session).Run(); err != nil {
		t.Errorf("mux session %s gone after detach: %v", session, err)
	}
}
