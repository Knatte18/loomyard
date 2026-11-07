// mouse_boot_integration_test.go proves the mouse boot-option contract:
// it runs against a real, running instance of the configured multiplexer binary.
// A fresh Up() boot pins "-g mouse" to the resolved Config.Mouse value in both directions.
// An already-up session's Up() never re-applies it: there is no live toggle without a server restart.
// The test skips when the configured binary is absent.

package reedengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// newIntegrationEngine builds an Engine rooted at a fresh t.TempDir() hub, with Config.Mouse overridden to mouse.
// It registers a t.Cleanup that always kills the scratch server, so no server leaks across test runs.
// It skips the test when the configured multiplexer binary is absent.
// The test validates whatever binary is configured, so an absent one leaves nothing to validate.
func newIntegrationEngine(t *testing.T, mouse string) *Engine {
	t.Helper()

	hubDir := t.TempDir()
	worktreeDir := filepath.Join(hubDir, "worktree")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree dir: %v", err)
	}
	seedReedConfig(t, worktreeDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: worktreeDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}
	// Override the template-resolved default with this case's value.
	// It does not rely on the LYM_REED_MOUSE environment variable.
	cfg.Mouse = mouse

	geom := Geometry{
		SocketKey:     ServerName(hubDir),
		SessionName:   SessionName(worktreeDir),
		AnchorPath:    worktreeDir,
		PaneCwd:       worktreeDir,
		WorktreeRoot:  worktreeDir,
		LogsDir:       filepath.Join(hubDir, "logs"),
		HubPath:       hubDir,
		WorktreeName:  filepath.Base(worktreeDir),
		NameShortname: "tc",
		NameSlug:      "tslug",
	}
	e := New(cfg, geom, testScrub)

	// Always torn down, on success or failure: a scratch server that leaks leaves a stray process behind.
	tmuxkit.KillOnCleanup(t, cfg.Tmux, geom.SocketKey)
	return e
}

// readMouseOption reads the live "mouse" server-global option back with a raw show-options query.
// It returns the option's value without the name.
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

// TestMouseBootIntegration_PinsOptionAtBoot pins that Up boots the live server with the configured mouse option.
func TestMouseBootIntegration_PinsOptionAtBoot(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
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

// TestMouseBootIntegration_NoLiveToggleWithoutRestart pins that the live mouse option stays put without a restart.
// It boots once with Mouse="off" and confirms it.
// A second Engine on the same geometry with Mouse="on" then calls Up without tearing the first session down.
// The already-up session hits ensureServerAndSessionLocked's early return and never re-applies set-option,
// so the live value stays "off", since there is no live toggle without a server restart.
func TestMouseBootIntegration_NoLiveToggleWithoutRestart(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	e1 := newIntegrationEngine(t, "off")
	if _, err := e1.Up(); err != nil {
		t.Fatalf("first Up(): %v", err)
	}
	if got := readMouseOption(t, e1); got != "off" {
		t.Fatalf("show-options -g mouse after first boot = %q, want %q", got, "off")
	}

	// Build a second Engine from e1's geometry and cfg, with only Mouse overridden,
	// so it targets the same socket and session rather than a fresh temp dir.
	cfg2 := e1.cfg
	cfg2.Mouse = "on"
	e2 := New(cfg2, e1.geom, testScrub)
	if _, err := e2.Up(); err != nil {
		t.Fatalf("second Up() against the already-up session: %v", err)
	}

	if got := readMouseOption(t, e1); got != "off" {
		t.Errorf("show-options -g mouse after a second Up() with Mouse=on against an already-up session = %q, want still %q (no live toggle without restart)", got, "off")
	}
}
