// hubreservedroutes_test.go covers the wiring guard's live surface.
// Hub-reserved names (the Bolt checkout, _portals, _launchers) must stay out of two routes.
// The first drives junction wiring, and the second builds the weft commit pathspec.
// The check runs over the config a hub is seeded with, the fabric config template.
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHubReserved_BoardExcludedFromPathspecRoutes asserts that the Bolt checkout appears in neither:
// WiredNames' output nor ScopedPathspec's output over the loaded config's raw Dirs().
// Those routes drive junction wiring and the weft commit pathspec.
//
//testtiming:keep the Bolt checkout appearing in neither WiredNames nor ScopedPathspec over a loaded config, guarding the wiring guard's live surface; coverage of its blocks by other tests does not show an assertion of this
func TestHubReserved_BoardExcludedFromPathspecRoutes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boardDir := t.TempDir()
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(fabricengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("seed fabric config: %v", err)
	}

	names, err := fabricengine.WiredNames(configengine.Dirs{Board: boardDir})
	if err != nil {
		t.Fatalf("WiredNames: %v", err)
	}
	if slices.Contains(names, fabricengine.BoardDirName) {
		t.Errorf("WiredNames() = %v; want it to never include %q", names, fabricengine.BoardDirName)
	}

	cfg, err := fabricengine.LoadConfig(configengine.Dirs{Board: boardDir})
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
