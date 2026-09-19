//go:build integration

// ensuresession_integration_test.go pins EnsureSession's booted return value and AddStrand's own
// attribution log line against a real, running multiplexer instance, following
// contract_integration_test.go's pattern: its own scratch socket so it can never collide with a real
// hub server, the seedReedConfig recipe for the on-disk config, and the same self-skip when the
// configured multiplexer binary is absent. Both assertions need a real server to observe — booted is
// the difference between "nothing was there to attach to" and "something already was", which only a
// real has-session/list-panes round trip can answer — so there is no hermetic half to split off and
// none of it belongs in the untagged tier.

package reedengine

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// newColdScratchEngine builds an *Engine rooted at a fresh t.TempDir(), against the real,
// LoadConfig-resolved cfg, on its own scratch socket derived from that tmpDir's own hash — the same
// shape contract_integration_test.go's own Engine-driving tests (TestRemoveStrand_SoleStrandEmptiesSessionSucceeds,
// TestDeadHeaderPaneIsHealedByUpWithoutCorruptingLayout) use, so a leaked scratch server here can
// never collide with a real hub server or another test's socket. It self-skips when the configured
// multiplexer binary is absent, and registers a t.Cleanup that tears down every session and server
// it may have created, on every path including a failed run mid-test.
func newColdScratchEngine(t *testing.T) *Engine {
	t.Helper()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	hub := filepath.Dir(tmpDir)
	geom := Geometry{
		SocketKey:    ServerName(hub),
		SessionName:  SessionName(tmpDir),
		AnchorPath:   tmpDir,
		PaneCwd:      tmpDir,
		WorktreeRoot: tmpDir,
		LogsDir:      filepath.Join(hub, "logs"),
		RepoName:     "test-repo",
		WorktreeName: filepath.Base(tmpDir),
		HubPath:      hub,
	}
	e := New(cfg, geom)

	t.Cleanup(func() {
		// Best-effort on both counts, mirroring contract_integration_test.go's own Engine fixtures:
		// Down may already have nothing left to tear down (the test under test tore it down itself,
		// or never booted at all on a failure path), and the raw kill-server afterward is the
		// belt-and-suspenders guard against a leaked scratch server on a genuine test failure that
		// never reached a clean teardown.
		_, _ = e.Down()
		_ = e.tmux.run("kill-server")
	})

	return e
}

// TestEnsureSession_BootedTrueOnColdSessionFalseOnWarm pins the primitive both this task's
// attribution log lines derive from: EnsureSession answers true against a worktree whose session has
// never been created, and false when called again against the live session it just created.
func TestEnsureSession_BootedTrueOnColdSessionFalseOnWarm(t *testing.T) {
	e := newColdScratchEngine(t)

	booted, err := e.EnsureSession()
	if err != nil {
		t.Fatalf("EnsureSession (cold) = %v, want a nil error", err)
	}
	if !booted {
		t.Errorf("EnsureSession (cold) booted = false, want true — nothing usable existed to attach to")
	}

	booted, err = e.EnsureSession()
	if err != nil {
		t.Fatalf("EnsureSession (warm) = %v, want a nil error", err)
	}
	if booted {
		t.Errorf("EnsureSession (warm) booted = true, want false — the session it just created is already usable")
	}
}

// TestAddStrand_LogsAttributionOnlyOnColdBoot pins AddStrand's own log line, using the package's
// existing captureLogOutput helper (logcapture_test.go), which sets both the logger output and its
// verbosity — neither alone captures at Info. A cold AddStrand must emit the attribution line; a
// second AddStrand against the now-live session must not.
func TestAddStrand_LogsAttributionOnlyOnColdBoot(t *testing.T) {
	e := newColdScratchEngine(t)
	logs := captureLogOutput(t)

	const attributionLine = "reed: add self-healed a cold worktree's session"

	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}}); err != nil {
		t.Fatalf("AddStrand (cold) = %v, want a nil error", err)
	}
	if !strings.Contains(logs.String(), attributionLine) {
		t.Errorf("log output after a cold AddStrand = %q; want it to contain %q", logs.String(), attributionLine)
	}

	logs.Reset()
	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}}); err != nil {
		t.Fatalf("AddStrand (warm) = %v, want a nil error", err)
	}
	if strings.Contains(logs.String(), attributionLine) {
		t.Errorf("log output after a warm AddStrand = %q; want it NOT to contain %q — the session was already up before this call", logs.String(), attributionLine)
	}
}
