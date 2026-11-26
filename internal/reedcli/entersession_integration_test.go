// entersession_integration_test.go pins which tmux binary the watchdog's per-session engine drives,
// and that a test can wait for an entered session's goroutines to stop.
// It runs against a hub fixture with no tmux server.

package reedcli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestEnterSession_DrivesTheToldTmux seeds a worktree config that names one tmux path and sets watchdog to off.
// It then asserts that enterSession's engine drives a different told path and starts no name-repair loop,
// and that the OOM watch's first read is done once the wait for the session's goroutines returns.
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

	// The OOM watch's first strand read creates this file under the worktree.
	opLock := filepath.Join(location.AnchorPath(), lymdirs.DotLymDirName, "reed.lock")
	if _, err := os.Stat(opLock); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("os.Stat(%q) before enterSession = %v; want fs.ErrNotExist", opLock, err)
	}

	told := filepath.Join(scratch, "told-tmux")
	ws, err := enterSession(h.Path, told, reedengine.SessionName(prime), watchdogDefaultTiming())
	if err != nil {
		t.Fatalf("enterSession: %v", err)
	}
	t.Cleanup(func() { stopWatchedSession(t, ws) })
	if got := ws.eng.TmuxPath(); got != told {
		t.Errorf("engine TmuxPath() = %q, want the told path %q", got, told)
	}
	if ws.repairsNames {
		t.Error("enterSession with watchdog: off started the name-repair loop; want none")
	}
	stopWatchedSession(t, ws)
	if _, err := os.Stat(opLock); err != nil {
		t.Errorf("os.Stat(%q) after the wait = %v; want the file the OOM watch's first read creates", opLock, err)
	}
}

// stopWatchedSession cancels ws and waits for every goroutine it started to return.
// A goroutine still reading the session writes under the worktree,
// so a test that returns without this wait can race its temp directory's removal.
func stopWatchedSession(t *testing.T, ws watchedSession) {
	t.Helper()
	ws.cancel()
	stopped := make(chan struct{})
	go func() {
		ws.running.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(30 * time.Second):
		t.Error("the watched session's goroutines did not return within 30s of its cancel")
	}
}
