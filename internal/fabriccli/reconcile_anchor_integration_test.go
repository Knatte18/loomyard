//go:build tier1

// reconcile_anchor_integration_test.go drives `lym fabric reconcile` from a pair whose anchor differs from the prime's.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestRunCLI_ReconcileFromAPairAtAnotherAnchor asserts a reconcile run from a pair at another anchor reseeds a missing hub-wide file from the prime's own base directory.
// The prime sits at "." and holds a non-template unit of the board module; a read at <prime>/sub finds nothing and would seed the template only.
func TestRunCLI_ReconcileFromAPairAtAnotherAnchor(t *testing.T) {
	t.Parallel()

	const slug = "anchored-pair"
	const primeBoard = "types:\n  spike: A time-boxed investigation\n"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	hubforge.ReanchorPair(t, h, slug, "sub")
	hubforge.SeedConfig(t, h, map[string]string{"board": primeBoard})
	boardPath := configengine.ConfigFile(h.BoardDir(), "board")
	if err := os.Remove(boardPath); err != nil {
		t.Fatalf("remove the hub-wide board config: %v", err)
	}

	code, output := runFabric(t, filepath.Join(h.PairCodeWorktree(slug), "sub"), "reconcile")
	if code != 0 {
		t.Fatalf("RunCLI(reconcile) from the pair = %d; want 0\noutput: %s", code, output)
	}

	boardYAML, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("hub-wide board config not reseeded at %s: %v", boardPath, err)
	}
	if !strings.Contains(string(boardYAML), "spike:") {
		t.Errorf("board.yaml lacks the prime's unit, so it was not read from the prime's base directory:\n%s", boardYAML)
	}
}
