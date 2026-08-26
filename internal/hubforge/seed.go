// seed.go implements hubforge's two seeding entry points, SeedConfig and SeedFabricConfig, which
// override a real hub's already-materialized config after NewHub has produced it.

package hubforge

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// SeedConfig writes an override for one or more modules' config into h's weft side and commits it.
// It writes to h.WeftBase — the anchor-joined weft directory, never h.PrimeWeft() — because at a
// non-"." anchor h.WeftBase is a subdirectory of the weft worktree root, and every module loader
// reads from the anchored path.
// The commit itself runs at the weft worktree root h.PrimeWeft(), not at h.WeftBase, because
// h.WeftBase can be a subdirectory of the worktree that "git add ." must be run above to stage.
//
// A single seeding entry point that infers its base from h is deliberate: it is exactly what makes
// the three ad-hoc call sites this migration replaces silently wrong, each guessing a base from the
// path shape in front of it.
//
// Seeding the warp side is impossible and this function does not offer it: <worktree>/_lyx is a weft
// junction excluded from the warp's index via .git/info/exclude, so `git add .` there stages nothing
// and the commit errors.
//
// The commit is run with --allow-empty because, once fabriccli.CloneAndWire commits the module
// configs it materialises, the base clone leaves the weft prime clean, so a seeded override that is
// byte-identical to the just-committed reconciled file stages nothing and a bare `git commit` exits
// 1 — which gitkit.MustRun turns into a tb.Fatalf in a test that did nothing wrong. The cost is an
// occasional empty fixture commit that nothing observes.
func SeedConfig(tb testing.TB, h *Hub, configByModule map[string]string) {
	tb.Helper()

	configDir := configengine.ConfigDir(h.WeftBase)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		tb.Fatalf("hubforge.SeedConfig: mkdir config dir: %v", err)
	}

	for module, content := range configByModule {
		configPath := configengine.ConfigFile(h.WeftBase, module)
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			tb.Fatalf("hubforge.SeedConfig: write config file %s: %v", module, err)
		}
	}

	gitkit.MustRun(tb, h.PrimeWeft(), "git", "add", ".")
	gitkit.MustRun(tb, h.PrimeWeft(), "git", "commit", "--allow-empty", "-m", "hubforge: seed config")
}

// SeedFabricConfig writes an override for the repo-wide fabric.yaml into h's board and commits it
// through fabricengine.NewBolt, matching what fabriccli.CloneAndWire itself does after
// ReconcileFabricAt — the same NewBolt(res.BoardDir).Commit(...) call — so the fixture leaves the
// board in the same state a real clone does.
//
// It writes to configengine.ConfigFile(h.BoardDir(), "fabric"), the repo-wide fabric config base,
// matching configsync.ReconcileFabricAt's own boardDir argument.
//
// Leaving it uncommitted would be unsafe rather than merely untidy: h.BoardDir() is the weft:main
// checkout the destruction gate's dirtiness check observes, so an uncommitted seed would silently
// change verb outcomes in fabric's own live-state cells.
func SeedFabricConfig(tb testing.TB, h *Hub, yaml string) {
	tb.Helper()

	boardDir := h.BoardDir()
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		tb.Fatalf("hubforge.SeedFabricConfig: mkdir config dir: %v", err)
	}

	fabricConfigPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(fabricConfigPath, []byte(yaml), 0o644); err != nil {
		tb.Fatalf("hubforge.SeedFabricConfig: write %s: %v", fabricConfigPath, err)
	}

	if _, _, err := fabricengine.NewBolt(boardDir).Commit("hubforge: seed repo-wide fabric config", fabricengine.SyncOptions{}); err != nil {
		tb.Fatalf("hubforge.SeedFabricConfig: commit: %v", err)
	}
}
