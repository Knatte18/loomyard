//go:build linux

// remotecontrol_integration_test.go drives ClientAgentPane against a real tmux server.
// Two clients are attached to two sessions.
// The clients are attached through the pty harness that attachgeometry_integration_test.go defines.

package reedengine

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestClientAgentPane_ReadsEachClientsOwnSession pins that each client's read names the active pane of its own session.
// The result does not depend on which client was active last.
func TestClientAgentPane_ReadsEachClientsOwnSession(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	const otherSession = "other"
	if err := e.tmux.run("new-session", "-d", "-s", otherSession); err != nil {
		t.Fatalf("new-session %s: %v", otherSession, err)
	}
	socketPath := strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "#{socket_path}"))

	sessions := []struct {
		name       string
		strandName string
		paneID     string
		client     string
	}{
		{name: e.SessionName(), strandName: "tc:orch"},
		{name: otherSession, strandName: "tc:a-task:agent"},
	}
	for i := range sessions {
		s := &sessions[i]
		s.paneID = strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-t", exactSessionWindowTarget(s.name), "#{pane_id}"))
		if err := e.tmux.run("set-option", "-p", "-t", s.paneID, strandNamePaneOption, s.strandName); err != nil {
			t.Fatalf("set the strand name on %s: %v", s.paneID, err)
		}
		startInPTY(t, []string{e.cfg.Tmux, "-L", e.Socket(), "attach-session", "-t", exactSessionTarget(s.name)}, 100, 30)
		waitUntil(t, 15*time.Second, "no client ever attached to session "+s.name, func() bool {
			out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(s.name), "-F", "#{client_name}")
			s.client = strings.TrimSpace(out)
			return err == nil && s.client != ""
		})
	}

	for _, s := range sessions {
		got, err := ClientAgentPane(e.cfg.Tmux, socketPath, s.client)
		if err != nil {
			t.Errorf("ClientAgentPane(%q): %v", s.client, err)
			continue
		}
		if want := (AgentPane{PaneID: s.paneID, StrandName: s.strandName}); got != want {
			t.Errorf("ClientAgentPane(%q) in session %s = %+v; want %+v", s.client, s.name, got, want)
		}
	}
}
