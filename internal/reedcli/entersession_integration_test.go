//go:build integration

// entersession_integration_test.go pins that the watchdog daemon's per-session engine drives the tmux binary the daemon was told, against a hub fixture and with no tmux server.

package reedcli

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestEnterSession_DrivesTheToldTmux seeds a worktree config naming one tmux path and watchdog: off, then asserts enterSession's engine drives a different told path and starts no watcher.
func TestEnterSession_DrivesTheToldTmux(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	location, err := lyxcwd.ResolveWorktree(prime)
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	scratch := t.TempDir()
	cfg.Tmux = filepath.Join(scratch, "configured-tmux")
	cfg.Watchdog = "off"
	seeded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	hubforge.SeedConfig(t, h, map[string]string{"reed": string(seeded)})

	told := filepath.Join(scratch, "told-tmux")
	ws, err := enterSession(h.Path, told, reedengine.SessionName(prime))
	if err != nil {
		t.Fatalf("enterSession: %v", err)
	}
	if got := ws.eng.TmuxPath(); got != told {
		t.Errorf("engine TmuxPath() = %q, want the told path %q", got, told)
	}
	if ws.cancel != nil {
		t.Error("enterSession with watchdog: off returned a non-nil cancel; want no watcher started")
	}
}
