//go:build integration

// hubreservedroutes_integration_test.go covers the filterHubReserved wiring guard's live surface: a
// hub-reserved name (_board, _portals, _launchers) must never appear in the routes that drive
// junction wiring or the weft commit pathspec, over the repo's real loaded config.
//
// Package fabricengine_test to reuse newFabricFixture (reconcile_stale_registration_test.go);
// shares the single TestMain in testmain_test.go — no new TestMain is added here.
package fabricengine_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestHubReserved_BoardExcludedFromPathspecRoutes guards the wiring guard's live surface against a
// later "simplification": _board must appear in neither WiredNames' output (the exported wrapper
// over filterHubReserved) nor ScopedPathspec's output over the real loaded config's raw Dirs() — the
// two routes that respectively drive junction wiring and the weft commit pathspec.
// junctionnames_test.go's TestFilterHubReserved covers filterHubReserved itself at unit level; this
// case is the half that exercises it through the real loaded config.
func TestHubReserved_BoardExcludedFromPathspecRoutes(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	boardDir := fabricengine.BoardDir(l.HubPath)

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
	scoped := fabricengine.ScopedPathspec(l.AnchorRel, cfg.Dirs())
	for _, entry := range scoped {
		if filepath.Base(entry) == fabricengine.BoardDirName {
			t.Errorf("ScopedPathspec(%q, %v) = %v; want no entry named %q", l.AnchorRel, cfg.Dirs(), scoped, fabricengine.BoardDirName)
		}
	}
}
