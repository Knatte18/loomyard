//go:build tmux && linux

// activity_integration_test.go pins that a client's #{pane_id} in list-clients is its active pane.
// It runs against a real tmux server and a client attached in a pty, whose keystroke makes OperatorActive read the pane active.

package reedengine

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

func TestOperatorActive_ReadsAnAttachedClientsKeystroke(t *testing.T) {
	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	strand, err := e.AddStrand(AddSpec{Cmd: "cat", Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}
	other, err := e.AddStrand(AddSpec{Cmd: "cat", Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand(other): %v", err)
	}
	if err := e.tmux.run("new-window", "-d", "-t", exactSessionWindowTarget(e.SessionName()), "-n", "extra"); err != nil {
		t.Fatalf("new-window: %v", err)
	}

	client := startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(120, 30)...), 120, 30)
	waitForClientAttached(t, e, 15*time.Second)
	// The attach pre-flight selects the newest agent window, which is other's; move the client onto strand's window afterwards.
	if err := e.tmux.run("select-window", "-t", windowOfPane(t, e, strand.PaneID)); err != nil {
		t.Fatalf("select-window: %v", err)
	}
	if err := e.tmux.run("select-pane", "-t", strand.PaneID); err != nil {
		t.Fatalf("select-pane: %v", err)
	}

	if _, err := client.master.Write([]byte("x")); err != nil {
		t.Fatalf("write keystroke to the client pty: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		active, quiet, err := e.OperatorActive(strand.GUID)
		if err != nil {
			t.Fatalf("OperatorActive: %v", err)
		}
		if quiet != 5*time.Second {
			t.Fatalf("quiet = %v, want the 5s default", quiet)
		}
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("OperatorActive never read the keystroked pane as active")
		}
		time.Sleep(50 * time.Millisecond)
	}

	active, _, err := e.OperatorActive(other.GUID)
	if err != nil {
		t.Fatalf("OperatorActive(other): %v", err)
	}
	if active {
		t.Errorf("OperatorActive(%s) = true, want false for a pane the client is not on", other.GUID)
	}
}
