//go:build integration

// seed_test.go proves SeedConfig survives a redundant seed: since fabriccli.CloneAndWire now commits
// the module configs it materialises, a real hub's weft prime arrives clean, so a seed byte-identical
// to the already-committed file stages nothing -- exactly the shape SeedConfig's --allow-empty commit
// exists to tolerate.

package hubforge

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// TestSeedConfig_RedundantSeedDoesNotFatal asserts SeedConfig returns normally when handed a seed
// byte-identical to what the clone already committed, and that a genuinely different seed still
// lands on disk -- proving --allow-empty did not turn the helper into a no-op.
func TestSeedConfig_RedundantSeedDoesNotFatal(t *testing.T) {
	h := NewHub(t, ".")

	loomConfigPath := configengine.ConfigFile(h.WeftBase, "loom")
	original, err := os.ReadFile(loomConfigPath)
	if err != nil {
		t.Fatalf("read already-materialised loom config %s: %v", loomConfigPath, err)
	}

	// A byte-identical seed stages nothing; this call must return normally rather than tb.Fatalf on a
	// bare `git commit` that exits 1 over an empty stage.
	SeedConfig(t, h, map[string]string{"loom": string(original)})

	afterRedundantSeed, err := os.ReadFile(loomConfigPath)
	if err != nil {
		t.Fatalf("read loom config after redundant seed %s: %v", loomConfigPath, err)
	}
	if string(afterRedundantSeed) != string(original) {
		t.Errorf("loom config after redundant seed = %q; want unchanged %q", afterRedundantSeed, original)
	}

	// A genuinely different seed must still land on disk, proving --allow-empty did not turn
	// SeedConfig into a no-op.
	const different = "different: true\n"
	SeedConfig(t, h, map[string]string{"loom": different})

	afterDifferentSeed, err := os.ReadFile(loomConfigPath)
	if err != nil {
		t.Fatalf("read loom config after different seed %s: %v", loomConfigPath, err)
	}
	if string(afterDifferentSeed) != different {
		t.Errorf("loom config after different seed = %q; want %q", afterDifferentSeed, different)
	}
}
