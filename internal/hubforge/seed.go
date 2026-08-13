// seed.go implements hubforge's two seeding entry points, SeedConfig and SeedFabricConfig, which
// override a real hub's already-materialized config after NewHub has produced it.

package hubforge

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
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
	gitkit.MustRun(tb, h.PrimeWeft(), "git", "commit", "-m", "hubforge: seed config")
}
