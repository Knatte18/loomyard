//go:build tmux && linux

// switch_integration_test.go drives SwitchClient against a real tmux server with a real attached client:
// it steps the client across three sessions in id order, wraps at both ends, and errors on an unknown client or a socket nothing listens on.
// The client is attached through the pty harness attachgeometry_integration_test.go defines.

package reedengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSwitchClient_StepsAnAttachedClientThroughSessionsInIDOrder(t *testing.T) {
	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	// The booted session holds the lowest id; these two follow it.
	for _, name := range []string{"second", "third"} {
		if err := e.tmux.run("new-session", "-d", "-s", name); err != nil {
			t.Fatalf("new-session %s: %v", name, err)
		}
	}
	socketPath := strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "#{socket_path}"))

	startInPTY(t, []string{e.cfg.Tmux, "-L", e.Socket(), "attach-session", "-t", exactSessionTarget(e.SessionName())}, 100, 30)
	waitForClientAttached(t, e, 15*time.Second)
	client := strings.TrimSpace(mustTmuxOutput(t, e, "list-clients", "-F", "#{client_name}"))

	steps := []struct {
		next bool
		want string
	}{
		{true, "second"},
		{true, "third"},
		{true, filepath.Base(e.geom.WorktreeRoot)},
		{false, "third"},
		{false, "second"},
	}
	for _, step := range steps {
		if err := SwitchClient(e.cfg.Tmux, socketPath, client, step.next); err != nil {
			t.Fatalf("SwitchClient(next=%v): %v", step.next, err)
		}
		waitUntil(t, 5*time.Second, "the client never reached session "+step.want, func() bool {
			return strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-c", client, "#{session_name}")) == step.want
		})
	}

	if err := SwitchClient(e.cfg.Tmux, socketPath, "/dev/pts/no-such-client", true); err == nil {
		t.Error("SwitchClient with an unknown client = nil error, want one")
	}
	if err := SwitchClient(e.cfg.Tmux, filepath.Join(t.TempDir(), "no-server.sock"), client, true); err == nil {
		t.Error("SwitchClient on a socket nothing listens on = nil error, want one")
	}
}

// mustTmuxOutput runs one tmux command on the engine's server and fails the test on error.
func mustTmuxOutput(t *testing.T, e *Engine, args ...string) string {
	t.Helper()
	out, err := e.tmux.output(args...)
	if err != nil {
		t.Fatalf("tmux %v: %v", args, err)
	}
	return out
}
