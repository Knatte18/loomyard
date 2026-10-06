// hubreservedroutes_test.go covers the filterHubReserved wiring guard's live surface: a hub-reserved
// name (_board, _portals, _launchers) must never appear in the routes that drive junction wiring or
// the weft commit pathspec, over the config a hub is seeded with (the fabric config template).
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.
package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestHubReserved_BoardExcludedFromPathspecRoutes guards the wiring guard's live surface against a
// later "simplification": _board must appear in neither WiredNames' output (the exported wrapper
// over filterHubReserved) nor ScopedPathspec's output over the loaded config's raw Dirs() — the
// two routes that respectively drive junction wiring and the weft commit pathspec.
// junctionnames_test.go's TestFilterHubReserved covers filterHubReserved itself at unit level; this
// case is the half that exercises it through a loaded config.
func TestHubReserved_BoardExcludedFromPathspecRoutes(t *testing.T) {
	t.Parallel()

	boardDir := t.TempDir()
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(fabricengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("seed fabric config: %v", err)
	}

	names, err := fabricengine.WiredNames(boardDir)
	if err != nil {
		t.Fatalf("WiredNames: %v", err)
	}
	if slices.Contains(names, fabricengine.BoardDirName) {
		t.Errorf("WiredNames() = %v; want it to never include %q", names, fabricengine.BoardDirName)
	}

	cfg, err := fabricengine.LoadConfig(boardDir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	const anchorRel = "."
	scoped := fabricengine.ScopedPathspec(anchorRel, cfg.Dirs())
	for _, entry := range scoped {
		if filepath.Base(entry) == fabricengine.BoardDirName {
			t.Errorf("ScopedPathspec(%q, %v) = %v; want no entry named %q", anchorRel, cfg.Dirs(), scoped, fabricengine.BoardDirName)
		}
	}
}
