//go:build tier0

// tacklerfor_test.go pins tacklerFor's reading of the tackler keys from shed.yaml alone, over temp config layers.

package loomcli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestTacklerFor verifies a darn, a tapestry and a pawl run all take shed.yaml's tackler_max_spawns, each with the recipe line its rows select: a Plan-Write row the phased line, none the one-writer line, and pawl its own.
func TestTacklerFor(t *testing.T) {
	t.Parallel()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	tests := []struct {
		name       string
		recipe     string
		wantRecipe string
	}{
		{name: "darn", recipe: shedrun.RecipeDarn, wantRecipe: darnTacklerRecipe},
		{name: "tapestry", recipe: shedrun.RecipeTapestry, wantRecipe: loomTacklerRecipe},
		{name: "pawl", recipe: shedrun.RecipePawl, wantRecipe: pawlTacklerRecipe},
		{name: "empty reads as tapestry", recipe: "", wantRecipe: loomTacklerRecipe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ConfigDir(dir), err)
			}
			const wantSpawns = 2
			document := strings.Replace(sheddriver.ConfigTemplate(), "tackler_max_spawns: 4", "tackler_max_spawns: 2", 1)
			if err := os.WriteFile(configengine.ConfigFile(dir, "shed"), []byte(document), 0o644); err != nil {
				t.Fatalf("WriteFile(shed.yaml) = %v; want nil", err)
			}

			settings, recipe, err := tacklerFor(configengine.Dirs{Anchor: dir}, tt.recipe, reg)
			if err != nil {
				t.Fatalf("tacklerFor(%q) = _, _, %v; want nil error", tt.recipe, err)
			}
			if settings.MaxSpawns != wantSpawns {
				t.Errorf("tacklerFor(%q) MaxSpawns = %d; want %d", tt.recipe, settings.MaxSpawns, wantSpawns)
			}
			if recipe != tt.wantRecipe {
				t.Errorf("tacklerFor(%q) recipe = %q; want %q", tt.recipe, recipe, tt.wantRecipe)
			}
		})
	}
}
