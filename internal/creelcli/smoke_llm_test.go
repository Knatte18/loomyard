//go:build llm

// smoke_llm_test.go proves creel's delivery into a real Claude session, which the fake provider of the tmux tier cannot reach.

package creelcli

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
)

// TestSmokeCreelClaudeStrand adds a strand running claude to a hub and drives creel into it.
// The adapter reads the booted session idle, a posted message's reminder and an inject are typed at idle, and the agent's pane shows both.
func TestSmokeCreelClaudeStrand(t *testing.T) {
	claudePath := llmkit.Claude(t, "LYX_REED_CLAUDE")
	// The strand name in the environment is process-global state, set by newSmokeHub.
	smoke := newSmokeHub(t)
	name, guid := smoke.addStrand(t, "smokeclaude", claudePath)
	engineFor := func(cfg shuttleengine.Config) shuttleengine.Engine { return claudeengine.NewFromConfig(cfg) }
	smoke.startDaemon(t, engineFor)

	delivery := newHubDelivery(smoke.hub.Path, engineFor)
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
	// The booted session has no event to wait on, so the idle check is polled on a tick under a real-time deadline.
	deadline := time.Now().Add(smokePaneWindow)
	for {
		idle, err := delivery.Idle(strand)
		if err != nil {
			t.Fatalf("Idle() = %v", err)
		}
		if idle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the adapter never read the booted session idle")
		}
		time.Sleep(time.Second)
	}

	if code, out := smoke.creel("post", name, "creel smoke message"); code != 0 {
		t.Fatalf("lyx creel post = %d; output: %s", code, out)
	}
	if capture, ok := smoke.awaitPane(t, guid, "[creel] 1 unread"); !ok {
		t.Fatalf("the reminder was not typed into the agent's pane; pane:\n%s", capture)
	}
	code, out := smoke.creel("inject", "--wait", "--timeout", "120s", name, "creel smoke inject")
	if code != 0 || !strings.Contains(out, `"outcome":"injected"`) {
		t.Fatalf("lyx creel inject --wait = %d, %s; want exit 0 injected", code, out)
	}
	if capture, ok := smoke.awaitPane(t, guid, "creel smoke inject"); !ok {
		t.Fatalf("the agent's pane does not show the injected line; pane:\n%s", capture)
	}
}
