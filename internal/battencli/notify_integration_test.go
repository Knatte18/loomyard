//go:build integration

// notify_integration_test.go proves the wired Notify seam over a real hub, whose prime NotifyPrime resolves through git.

package battencli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/orchcli"
)

func TestWire_NotifyWritesNothingWithoutAnOrchStrand(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	c := wireForHub(t, h, "a-slug", nil)
	orchDir := orchcli.PrimePaths(h.Location).Dir

	if posted, err := c.env.InnerRun.Notify(context.Background(), "batten a-slug: child left running"); err != nil || posted {
		t.Fatalf("Notify() = %v, %v; want the notice reported not posted, without an error", posted, err)
	}

	// Reading the orch state leaves its lock file; nothing else may appear.
	entries, err := os.ReadDir(orchDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read %s: %v", orchDir, err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".lock") {
			t.Errorf("%s holds %s; want only lock files", orchDir, entry.Name())
		}
	}
}
