//go:build linux

// remotecontrol_integration_test.go drives ClientAgentPane against a real tmux server with two clients in two sessions.
// It also holds the helper that attaches one client to each of several sessions.
// The helper uses the pty harness that attachgeometry_integration_test.go defines.

package reedengine

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// attachedSession is one session of a test server with one client attached to it.
type attachedSession struct {
	name   string
	paneID string
	client string
}

// attachOneClientPerSession attaches one client to each named session, in the order given, and waits for each to attach.
// It returns each session's active pane and the name of its client.
// The last session's client attaches last, so it is the server's most recently active client.
func attachOneClientPerSession(t *testing.T, e *Engine, names ...string) []attachedSession {
	t.Helper()
	sessions := make([]attachedSession, len(names))
	for i, name := range names {
		s := &sessions[i]
		s.name = name
		s.paneID = strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-t", exactSessionWindowTarget(name), "#{pane_id}"))
		startInPTY(t, []string{e.cfg.Tmux, "-L", e.Socket(), "attach-session", "-t", exactSessionTarget(name)}, 100, 30)
		waitUntil(t, 15*time.Second, "no client ever attached to session "+name, func() bool {
			out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(name), "-F", "#{client_name}")
			s.client = strings.TrimSpace(out)
			return err == nil && s.client != ""
		})
	}
	return sessions
}

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

	strandNames := map[string]string{e.SessionName(): "tc:orch", otherSession: "tc:a-task:agent"}
	sessions := attachOneClientPerSession(t, e, e.SessionName(), otherSession)
	for _, s := range sessions {
		if err := e.tmux.run("set-option", "-p", "-t", s.paneID, strandNamePaneOption, strandNames[s.name]); err != nil {
			t.Fatalf("set the strand name on %s: %v", s.paneID, err)
		}
	}

	for _, s := range sessions {
		got, err := ClientAgentPane(e.cfg.Tmux, socketPath, s.client)
		if err != nil {
			t.Errorf("ClientAgentPane(%q): %v", s.client, err)
			continue
		}
		if want := (AgentPane{PaneID: s.paneID, StrandName: strandNames[s.name]}); got != want {
			t.Errorf("ClientAgentPane(%q) in session %s = %+v; want %+v", s.client, s.name, got, want)
		}
	}
}
