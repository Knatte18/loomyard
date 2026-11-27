// seed.go holds SeedConfig and SeedFabricConfig, which override a hub's config after NewHub builds it.

package hubforge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// SeedConfig writes a sparse worktree layer over the board's repo defaults for one or more modules.
// It writes the layer into h's weft side and commits it.
// It writes to h.WeftBase, the anchor-joined weft directory, never h.PrimeRecords().
// At a non-"." anchor, h.WeftBase is a subdirectory of the weft worktree root.
// Every module loader reads from the anchored path, so the layer belongs there.
// The commit runs at h.PrimeRecords(), the weft worktree root, not at h.WeftBase.
// h.WeftBase can be a subdirectory, so the files it stages and commits are named by their path from that root.
// Only the files written here are staged and committed.
// In a one-repository hub the records worktree is the code worktree.
// So nothing else staged there joins the seed's commit.
//
// The base is inferred from h, not passed in.
// A caller that guesses it from the path shape gets it silently wrong.
//
// Seeding the warp side is impossible, so this function does not offer it.
// <worktree>/_lym is a weft junction excluded from the warp's index via .git/info/exclude.
// There, staging stages nothing and the commit errors.
//
// The commit uses --allow-empty because the base clone leaves the weft prime clean.
// A seeded layer byte-identical to a file the prime already holds stages nothing, and a bare `git commit` exits 1.
// That would fail the test.
// The cost is an occasional empty fixture commit that nothing observes.
func SeedConfig(tb testing.TB, h *Hub, configByModule map[string]string) {
	tb.Helper()

	configDir := configengine.ConfigDir(h.WeftBase)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		tb.Fatalf("hubforge.SeedConfig: mkdir config dir: %v", err)
	}

	var seeded []string
	for module, content := range configByModule {
		configPath := configengine.ConfigFile(h.WeftBase, module)
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			tb.Fatalf("hubforge.SeedConfig: write config file %s: %v", module, err)
		}
		relPath, err := filepath.Rel(h.PrimeRecords(), configPath)
		if err != nil {
			tb.Fatalf("hubforge.SeedConfig: relate config file %s to %s: %v", configPath, h.PrimeRecords(), err)
		}
		seeded = append(seeded, filepath.ToSlash(relPath))
	}

	commitArgs := []string{"git", "commit", "--allow-empty", "-m", "hubforge: seed config"}
	if len(seeded) > 0 {
		gitkit.MustRun(tb, h.PrimeRecords(), append([]string{"git", "add", "--"}, seeded...)...)
		commitArgs = append(commitArgs, append([]string{"--"}, seeded...)...)
	}
	gitkit.MustRun(tb, h.PrimeRecords(), commitArgs...)
}

// SeedFabricConfig writes an override for the repo-wide fabric.yaml into h's Bolt checkout and commits it.
// The commit leaves the Bolt checkout in the state a real clone leaves it in.
//
// It writes to configengine.ConfigFile of the Bolt checkout, for "fabric", the repo-wide fabric config base.
//
// Leaving the seed uncommitted would be unsafe.
// The Bolt checkout is the weft:main checkout that the destruction gate's dirtiness check observes.
// An uncommitted seed would therefore silently change verb outcomes in fabric's live-state cells.
func SeedFabricConfig(tb testing.TB, h *Hub, yaml string) {
	tb.Helper()

	boltDir := fabricengine.BoltDir(h.Path)
	if err := os.MkdirAll(configengine.ConfigDir(boltDir), 0o755); err != nil {
		tb.Fatalf("hubforge.SeedFabricConfig: mkdir config dir: %v", err)
	}

	fabricConfigPath := configengine.ConfigFile(boltDir, "fabric")
	if err := os.WriteFile(fabricConfigPath, []byte(yaml), 0o644); err != nil {
		tb.Fatalf("hubforge.SeedFabricConfig: write %s: %v", fabricConfigPath, err)
	}

	if _, _, err := fabricengine.NewBolt(boltDir).Commit("hubforge: seed repo-wide fabric config", fabricengine.SyncOptions{}); err != nil {
		tb.Fatalf("hubforge.SeedFabricConfig: commit: %v", err)
	}
}
