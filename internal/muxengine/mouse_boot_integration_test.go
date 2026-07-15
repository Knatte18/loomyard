//go:build integration

// mouse_boot_integration_test.go proves the mouse boot-option contract
// against a real, running instance of the configured multiplexer binary:
// a fresh Up() boot pins "-g mouse" to the resolved Config.Mouse value in
// both directions, and an already-up session's Up() never re-applies it
// (no live toggle without a server restart). It self-skips cleanly when
// the configured binary is absent, mirroring contract_integration_test.go's
// skip/scratch-socket harness.

package muxengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// newIntegrationEngine builds an Engine rooted at a fresh t.TempDir() hub, with
// Config.Mouse overridden to mouse, and registers a t.Cleanup that always kills
// the scratch server so no server leaks across test runs. It skips the test
// cleanly when the configured multiplexer binary is not present on this box —
// this test's whole point is to validate whatever binary is actually
// configured, so an absent binary is nothing to validate here, not a failure.
func newIntegrationEngine(t *testing.T, mouse string) *Engine {
	t.Helper()

	hubDir := t.TempDir()
	worktreeDir := filepath.Join(hubDir, "worktree")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree dir: %v", err)
	}
	seedMuxConfig(t, worktreeDir)

	cfg, err := LoadConfig(worktreeDir, "mux")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}
	// Override the template-resolved default with the value this test case
	// wants to exercise, rather than relying on LYX_MUX_MOUSE env plumbing.
	cfg.Mouse = mouse

	layout := &hubgeometry.Layout{
		Cwd:          worktreeDir,
		WorktreeRoot: worktreeDir,
		Hub:          hubDir,
		RelPath:      ".",
		Prime:        worktreeDir,
	}
	e := New(cfg, layout)

	t.Cleanup(func() {
		// Always torn down, success or failure: a leaked scratch server on a
		// per-test-tempdir-derived socket is harmless to a real hub server,
		// but leaves a stray process behind if the test does not clean up.
		_ = e.tmux.run("kill-server")
	})
	return e
}

// readMouseOption reads the live "mouse" server-global option back via a raw
// show-options query, in the same command-construction/output-parsing style
// contract_integration_test.go uses for its own list-sessions/display-message
// queries (that test never reads an option back, so this models only the
// style, not a preexisting show-options assertion there).
func readMouseOption(t *testing.T, e *Engine) string {
	t.Helper()
	out, err := e.tmux.output("show-options", "-g", "mouse")
	if err != nil {
		t.Fatalf("show-options -g mouse: %v", err)
	}
	// show-options prints "mouse <value>"; the value is the second field.
	fields := strings.Fields(out)
	if len(fields) != 2 {
		t.Fatalf("show-options -g mouse = %q, want two fields (name value)", out)
	}
	return fields[1]
}

// TestMouseBootIntegration_PinsOptionAtBoot boots a fresh engine with Mouse
// set to "off" and then, on a separate fresh hub, "on", asserting the live
// server reports the matching value after Up() in both directions.
func TestMouseBootIntegration_PinsOptionAtBoot(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		e := newIntegrationEngine(t, "off")
		if _, err := e.Up(); err != nil {
			t.Fatalf("Up(): %v", err)
		}
		if got := readMouseOption(t, e); got != "off" {
			t.Errorf("show-options -g mouse after boot with Mouse=%q = %q, want %q", "off", got, "off")
		}
	})

	t.Run("on", func(t *testing.T) {
		e := newIntegrationEngine(t, "on")
		if _, err := e.Up(); err != nil {
			t.Fatalf("Up(): %v", err)
		}
		if got := readMouseOption(t, e); got != "on" {
			t.Errorf("show-options -g mouse after boot with Mouse=%q = %q, want %q", "on", got, "on")
		}
	})
}

// TestMouseBootIntegration_NoLiveToggleWithoutRestart boots once with
// Mouse="off", confirms it, then builds a second Engine on the SAME layout
// with Mouse="on" and calls Up() again without tearing the first session
// down. The already-up session must hit ensureServerAndSessionLocked's early
// return and never re-apply set-option, so the live value must stay "off" —
// proving there is no live toggle without a server restart.
func TestMouseBootIntegration_NoLiveToggleWithoutRestart(t *testing.T) {
	e1 := newIntegrationEngine(t, "off")
	if _, err := e1.Up(); err != nil {
		t.Fatalf("first Up(): %v", err)
	}
	if got := readMouseOption(t, e1); got != "off" {
		t.Fatalf("show-options -g mouse after first boot = %q, want %q", got, "off")
	}

	// Build a second Engine on the identical Config/Layout shape, differing
	// only in Mouse, and target the SAME socket/session by reusing e1's
	// layout and cfg (with Mouse overridden) rather than a fresh temp dir.
	cfg2 := e1.cfg
	cfg2.Mouse = "on"
	e2 := New(cfg2, e1.layout)
	if _, err := e2.Up(); err != nil {
		t.Fatalf("second Up() against the already-up session: %v", err)
	}

	if got := readMouseOption(t, e1); got != "off" {
		t.Errorf("show-options -g mouse after a second Up() with Mouse=on against an already-up session = %q, want still %q (no live toggle without restart)", got, "off")
	}
}
