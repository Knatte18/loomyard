//go:build !windows

// stopserver_integration_test.go runs StopServer against a real tmux server.
// It proves the hub's server is stopped by the hub's path alone.

package reedengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestStopServer(t *testing.T) {
	tiergate.Require(t, 2)
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("tmux not found: %v", err)
	}

	// The hub's directory never exists: the server is reached by the hub's path alone.
	hub := filepath.Join(t.TempDir(), "gone-hub")
	key := ServerName(hub)
	tmuxkit.KillOnCleanup(t, tmux, key)

	stopped, err := StopServer(tmux, hub)
	if err != nil || !stopped {
		t.Fatalf("StopServer() = %v, %v; want true, nil for a live server", stopped, err)
	}
	if _, err := os.Lstat(filepath.Join(socketDirFromEnv(), key)); !os.IsNotExist(err) {
		t.Errorf("socket file of %s after the stop: err = %v; want it removed", key, err)
	}

	stopped, err = StopServer(tmux, hub)
	if err != nil || stopped {
		t.Errorf("second StopServer() = %v, %v; want false, nil with no live server", stopped, err)
	}
}
