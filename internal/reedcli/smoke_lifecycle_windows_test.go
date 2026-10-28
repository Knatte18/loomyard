//go:build tier2

// smoke_lifecycle_windows_test.go holds the lifecycle smoke test that runs only on the psmux backend.

package reedcli

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestSmokeRemoveLastStrandThenAddRunsTheNewCommand exercises removing a session's last STRAND and then adding a new one, with the always-present Selvage pane in play:
// removeStrandLocked collects pane ids from strands only, and Selvage is never a strand,
// so the removed strand's pane is never the session's actual last pane — kill-pane removes it outright, on either backend, rather than corpsing it under remain-on-exit, and Selvage alone is left holding the session up.
// The following add must split a fresh pane whose command genuinely runs and STAYS live across the next reconciling verb.
//
// This premise holds identically on tmux — see reedengine.RemoveStrand's emptied-session swallow (strand.go) for how that path is handled at the engine level — so the test runs only on Windows for coverage economy, not backend-specific behavior:
// the equivalent removing-the-last-strand-then-add shape is already exercised on the tmux backend by TestRemoveStrand_SoleStrandEmptiesSessionSucceeds (contract_integration_test.go), so running this real-tmux-session variant there too would be redundant.
func TestSmokeRemoveLastStrandThenAddRunsTheNewCommand(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.PrimeWorktree())
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	launch := "pwsh -NoExit -Command Write-Host ready"
	first := addStrand(t, launch, "--name", "first")
	out.Reset()
	if code := RunCLI(&out, []string{"remove", first}); code != 0 {
		t.Fatalf("remove = %d; want 0, output: %s", code, out.String())
	}

	second := addStrand(t, launch, "--name", "second")

	// A genuine fresh split has no reason to wobble across a reconcile,
	// but this is the shape a corpse-bound strand would have failed under:
	// up reconciles; the strand must still be live.
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("post-add up = %d; want 0, output: %s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand, found := statusStrand(t, out.Bytes(), second)
	if !found {
		t.Fatalf("status missing strand %s; output: %s", second, out.String())
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("strand added after remove-last: live = false; want true (bound to a pane reconcile then cleared?); status: %s", out.String())
	}
}
