//go:build llm

// smoke_llm_test.go proves creel's delivery into a real Claude session, which the fake provider of the tmux tier cannot reach.

package creelcli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
)

// TestSmokeCreelClaudeStrand adds a strand running claude to a hub and drives creel into it.
// The adapter reads the booted session idle, a posted message's carried line and an inject are typed at idle, and the agent's pane shows both.
func TestSmokeCreelClaudeStrand(t *testing.T) {
	claudePath := llmkit.Claude(t, "LYX_REED_CLAUDE")
	// The strand name in the environment is process-global state, set by newSmokeHub.
	smoke := newSmokeHub(t)
	name, guid := smoke.addStrand(t, "smokeclaude", claudePath)
	smoke.startDaemon(t, shuttleengines.New)

	delivery := newHubDelivery(smoke.hub.Path, shuttleengines.New)
	strands, err := delivery.Strands()
	if err != nil {
		t.Fatalf("Strands() = %v", err)
	}
	var strand creelengine.Strand
	for _, candidate := range strands {
		if candidate.GUID == guid {
			strand = candidate
		}
	}
	if strand.GUID != guid || !strand.Live {
		t.Fatalf("Strands() = %+v; want the claude strand %s live", strands, guid)
	}
	// A fresh temp hub shows claude's folder-trust gate, which covers the input box.
	// The pane has no event to wait on, so it is polled on a tick under a real-time deadline until the gate is accepted or the box shows.
	// Accepting the gate adds the temp hub's path to the operator's Claude trust list.
	deadline := time.Now().Add(smokePaneWindow)
	for {
		ops, err := wireShuttle(smoke.hub.Location, shuttleengines.New)
		if err != nil {
			t.Fatalf("wireShuttle() = %v", err)
		}
		accepted, err := ops.runner.AcceptStartupGate(guid)
		if err != nil {
			t.Fatalf("AcceptStartupGate() = %v", err)
		}
		answer, err := delivery.Idle(strand)
		if err != nil {
			t.Fatalf("Idle() = %v", err)
		}
		if accepted || answer.Stall != creelengine.StallNoInputBox {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the claude pane showed no input box and no startup gate; pane:\n%s", smoke.lastPane(t, guid))
		}
		time.Sleep(time.Second)
	}
	// The booted session has no event to wait on, so the idle check is polled on a tick under a real-time deadline.
	awaitIdle := func() {
		t.Helper()
		deadline := time.Now().Add(smokePaneWindow)
		for {
			answer, err := delivery.Idle(strand)
			if err != nil {
				t.Fatalf("Idle() = %v", err)
			}
			if answer.Idle {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("the adapter never read the session idle; pane:\n%s", smoke.lastPane(t, guid))
			}
			time.Sleep(time.Second)
		}
	}
	awaitIdle()

	if code, out := smoke.creel("post", name, "creel smoke message"); code != 0 {
		t.Fatalf("lyx creel post = %d; output: %s", code, out)
	}
	// The test process posts as EXTERNAL, whose mail is carried in its own line rather than reminded.
	if capture, ok := smoke.awaitPane(t, guid, "[creel] #1 from "); !ok {
		t.Fatalf("the carried line was not typed into the agent's pane; pane:\n%s", capture)
	}
	code, out := smoke.creel("inject", "--wait", "--timeout", "120s", name, "creel smoke inject")
	if code != 0 || !strings.Contains(out, `"outcome":"injected"`) {
		t.Fatalf("lyx creel inject --wait = %d, %s; want exit 0 injected", code, out)
	}
	if capture, ok := smoke.awaitPane(t, guid, "creel smoke inject"); !ok {
		t.Fatalf("the agent's pane does not show the injected line; pane:\n%s", capture)
	}

	// Each command either lands or is typed without a confirmed landing; neither busy nor any other error is a result the watcher can act on.
	for _, command := range []creelengine.Command{
		{Name: creelengine.CommandReloadPlugins},
		{Name: creelengine.CommandColor, Argument: "blue"},
		{Name: creelengine.CommandLoadSkills, Argument: "scribe:prose"},
		{Name: creelengine.CommandCompact, Argument: "keep the smoke test"},
		{Name: creelengine.CommandClear},
	} {
		awaitIdle()
		err := delivery.Command(strand, command)
		switch {
		case err == nil:
			t.Logf("command %s landed", command.Name)
		case errors.Is(err, creelengine.ErrNotLanded):
			t.Logf("command %s was typed and not confirmed", command.Name)
		default:
			t.Fatalf("Command(%s) = %v; want nil or ErrNotLanded", command.Name, err)
		}
	}
}
