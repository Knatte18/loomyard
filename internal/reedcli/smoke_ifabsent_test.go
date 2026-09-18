//go:build smoke

// smoke_ifabsent_test.go smokes the real repeated-reopen sequence --if-absent exists for: a live
// match no-ops, a dead match relaunches under the same guid, and status never grows a second strand
// across either reopen. See smoke_test.go for the shared harness this file builds on.

package reedcli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestSmokeIfAbsentReopenIsIdempotent drives up -> add --if-absent -> a second identical add
// --if-absent while the strand is alive -> kill its pane -> a third add --if-absent, asserting status
// reports exactly one strand carrying the same guid throughout, and live only once the third add's
// relaunch has run.
func TestSmokeIfAbsentReopenIsIdempotent(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	launch := smokeReapLaunchCmd()
	firstGUID := addStrand(t, launch, "--if-absent", "--name", "claude")

	// Second add --if-absent while the strand is still alive: must add nothing and report the same
	// guid.
	secondGUID := addStrand(t, launch, "--if-absent", "--name", "claude")
	if secondGUID != firstGUID {
		t.Fatalf("second add --if-absent guid = %s; want %s (the live match, left unchanged)", secondGUID, firstGUID)
	}

	assertOneStrand := func(when string) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		if code := RunCLI(&buf, []string{"status"}); code != 0 {
			t.Fatalf("status %s = %d; want 0, output: %s", when, code, buf.String())
		}
		var result map[string]any
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("parse status result %s: %v", when, err)
		}
		strands, _ := result["strands"].([]any)
		if len(strands) != 1 {
			t.Fatalf("status %s strands = %v; want exactly 1 (a repeated add --if-absent must never stack a duplicate)", when, strands)
		}
		strand, _ := strands[0].(map[string]any)
		if guid, _ := strand["guid"].(string); guid != firstGUID {
			t.Fatalf("status %s strand guid = %q; want %q", when, guid, firstGUID)
		}
		return strand
	}

	assertOneStrand("after the second add --if-absent (still alive)")

	// Kill the strand's PANE directly, not "remove" -- the strand must stay tracked in reed's state,
	// only its pane must die, which is the reopen shape --if-absent's relaunch branch exists for.
	socket, _ := socketAndSession(t)
	paneID := paneIDForStrand(t, firstGUID)
	if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", paneID).Run(); err != nil {
		t.Fatalf("kill-pane %s: %v", paneID, err)
	}

	thirdGUID := addStrand(t, launch, "--if-absent", "--name", "claude")
	if thirdGUID != firstGUID {
		t.Fatalf("third add --if-absent guid = %s; want %s (the relaunched match, same guid, not a second strand)", thirdGUID, firstGUID)
	}

	// Assert against liveness, never mere pane presence: tmux keeps a session's sole pane on screen
	// after its process dies, so a check for pane presence alone would pass without the relaunch ever
	// having run. The "live" field is already built from the alive-not-merely-present set.
	strand := assertOneStrand("after the third add --if-absent relaunch")
	if live, _ := strand["live"].(bool); !live {
		t.Fatalf("status after the relaunch: strand %s live = false; want true", firstGUID)
	}
}
