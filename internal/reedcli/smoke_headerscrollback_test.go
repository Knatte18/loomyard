//go:build smoke

// smoke_headerscrollback_test.go holds the two scrollback assertions this batch adds, both built on
// capturePaneScrollback.
// TestSmokeHeaderPayloadClearsPaneScrollback is the direct proof that the ED 3 backstop actually
// clears a real multiplexer's scrollback — the one claim the composite test below can never show,
// since it goes green either way once the source fixes land.
// TestSmokeHeaderPaneScrollbackIsClean is the composite backstop B: it pins the end-to-end outcome
// across boot, resume, and heal, and pins none of the individual source fixes — P1, P2, and P3 are
// the pins for those, and they live in internal/reedengine/lifecycle_test.go,
// internal/reedcli/smoke_headerseed_test.go, and internal/reedcli/header_test.go respectively.

package reedcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// TestSmokeHeaderPayloadClearsPaneScrollback proves ED 3 takes effect against a real multiplexer
// rather than being a silent no-op: it fills a pane's scrollback with real junk lines, then emits
// headerBlockingPayload's exact bytes into that same pane, and asserts the resulting scrollback
// holds the header line and nothing else.
// On a Windows/psmux host this claim is asserted, not verified — the discussion records both, but
// this worktree is Linux and cannot execute a Windows run.
func TestSmokeHeaderPayloadClearsPaneScrollback(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	tempDir := t.TempDir()
	headerLine := "hub: " + tempDir

	var payload strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&payload, "smoke-junk-line-%03d\n", i)
	}
	payload.WriteString(headerBlockingPayload(headerLine))

	payloadFile := filepath.Join(tempDir, "payload.txt")
	if err := os.WriteFile(payloadFile, []byte(payload.String()), 0o644); err != nil {
		t.Fatalf("write payload file %s: %v", payloadFile, err)
	}

	socket := fmt.Sprintf("lyx-headerscrollback-harness-%d", os.Getpid())
	sessionCmd := fmt.Sprintf("cat %s; sleep 300", payloadFile)
	if err := exec.Command(tmuxPath, "-L", socket, "new-session", "-d", "-s", "h",
		"sh", "-c", sessionCmd).Run(); err != nil {
		t.Fatalf("boot harness server: %v", err)
	}
	t.Cleanup(func() {
		reapHarnessServer(t, tmuxPath, socket)
	})

	var capture string
	deadline := time.Now().Add(20 * time.Second)
	for {
		capture = capturePaneScrollback(t, tmuxPath, socket, "h")
		if strings.Contains(capture, headerLine) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane never showed %q within 20s; last scrollback:\n%s", headerLine, capture)
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !strings.Contains(capture, headerLine) {
		t.Errorf("scrollback missing header line %q; full capture:\n%s", headerLine, capture)
	}
	for i := 0; i < 50; i++ {
		junk := fmt.Sprintf("smoke-junk-line-%03d", i)
		if strings.Contains(capture, junk) {
			t.Errorf("scrollback still contains junk line %q that ED 3 should have cleared; full capture:\n%s", junk, capture)
		}
	}
}

// TestSmokeHeaderPaneScrollbackIsClean is the composite backstop B: it pins the end-to-end
// outcome — the live header pane's scrollback holds the rendered header line and no other
// non-empty line — across boot, resume, and heal.
// It pins no individual source fix: ED 3 runs after everything else and would keep this test green
// even if a source fix regressed, which is exactly why it is landed only alongside the direct proof
// above and the three per-mechanism pins (P1, P2, P3) elsewhere in this package and reedengine.
func TestSmokeHeaderPaneScrollbackIsClean(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := buildLyxBinaryWithLDFlags(t, "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev")

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	// Plant the same stale-but-untouched board stencil TestSmokeHeaderDeclinesStencilSeedPass
	// plants, so the arrangement that made the noise non-deterministic in the field is forced
	// rather than hoped for.
	registry := stencils.Registry()
	names := registry.Names()
	if len(names) == 0 {
		t.Fatalf("stencils.Registry().Names() returned no names")
	}
	name := names[0]
	shipped, known := registry.Default(name)
	if !known {
		t.Fatalf("registry has no default for its own first name %q", name)
	}
	driftedBody := append(append([]byte{}, shipped...), []byte("\nsmoke-drift-line\n")...)
	boardPath := stencilstore.Path(fabricengine.StencilsDir(h.Path), name)
	if err := os.MkdirAll(filepath.Dir(boardPath), 0o755); err != nil {
		t.Fatalf("create board stencil parent dir: %v", err)
	}
	stamped := stencilstore.ApplyStamp(driftedBody, stencilstore.BodyHash(driftedBody))
	if err := os.WriteFile(boardPath, stamped, 0o644); err != nil {
		t.Fatalf("write board stencil %s: %v", boardPath, err)
	}

	headerLine := "hub: " + h.Location.HubPath
	assertScrollbackClean := func(when, socket, paneID string) {
		t.Helper()
		capture := capturePaneScrollback(t, tmuxPath, socket, paneID)
		if !strings.Contains(capture, headerLine) {
			t.Errorf("%s: header pane scrollback missing %q; full capture:\n%s", when, headerLine, capture)
		}
		for _, line := range strings.Split(capture, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) == "" || strings.Contains(line, headerLine) {
				continue
			}
			t.Errorf("%s: header pane scrollback carries an unexpected non-empty line %q; full capture:\n%s", when, line, capture)
		}
	}
	pollHeaderLine := func(socket, paneID string) {
		t.Helper()
		pollPaneContains(t, tmuxPath, socket, paneID, headerLine, 20*time.Second)
	}

	// Boot.
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
	pollHeaderLine(socket, st.HeaderPaneID)
	assertScrollbackClean("after up", socket, st.HeaderPaneID)

	// Resume: already-live header pane must be left untouched, and its scrollback must stay clean.
	resumeCmd := exec.Command(lyxExe, "reed", "resume")
	resumeCmd.Dir = h.PrimeWorktree()
	if out, err := resumeCmd.CombinedOutput(); err != nil {
		t.Fatalf("built-binary resume: %v\n%s", err, out)
	}
	assertScrollbackClean("after resume", socket, st.HeaderPaneID)

	// Heal: kill the header pane directly through tmux, then re-run up, which is the retried-split
	// heal path — the one most likely to regress, since it re-runs the same launch code from a
	// different entry point.
	if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", st.HeaderPaneID).Run(); err != nil {
		t.Fatalf("kill-pane %s: %v", st.HeaderPaneID, err)
	}
	healCmd := exec.Command(lyxExe, "reed", "up")
	healCmd.Dir = h.PrimeWorktree()
	if out, err := healCmd.CombinedOutput(); err != nil {
		t.Fatalf("built-binary up (heal): %v\n%s", err, out)
	}
	healedSt, err := reedengine.LoadState(filepath.Join(h.PrimeWorktree(), ".lyx"))
	if err != nil || healedSt == nil || healedSt.HeaderPaneID == "" {
		t.Fatalf("LoadState after heal up = (%+v, %v), want a fresh persisted HeaderPaneID", healedSt, err)
	}
	pollHeaderLine(socket, healedSt.HeaderPaneID)
	assertScrollbackClean("after heal", socket, healedSt.HeaderPaneID)
}
