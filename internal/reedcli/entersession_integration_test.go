// entersession_integration_test.go pins that the watchdog's per-session engine drives the configured tmux binary.
// It runs against a hub fixture with no tmux server.

package reedcli

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestEnterSession_DrivesTheToldTmux seeds a worktree config that names one tmux path and sets watchdog to off.
// It then asserts that enterSession's engine drives a different told path and starts no watcher.
func TestEnterSession_DrivesTheToldTmux(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	location, err := lymcwd.ResolveWorktree(prime)
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: location.AnchorPath()}, "reed")
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
