// tacklerfor_test.go pins tacklerFor's pick of the module file whose tackler keys a run takes, over temp config layers.

package loomcli

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestTacklerFor verifies a darn run takes darn.yaml's tackler_max_spawns and the darn recipe line, a loom run loom.yaml's and the loom line, so one module's choice never leaks into the other's run.
func TestTacklerFor(t *testing.T) {
	t.Parallel()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	tests := []struct {
		name       string
		recipe     string
		loomSpawns string
		darnSpawns string
		wantSpawns int
		wantRecipe string
	}{
		{name: "darn takes darn.yaml's 0 over loom.yaml's 4", recipe: shedrun.RecipeDarn, loomSpawns: "4", darnSpawns: "0", wantSpawns: 0, wantRecipe: darnTacklerRecipe},
		{name: "darn takes darn.yaml's 4 over loom.yaml's 0", recipe: shedrun.RecipeDarn, loomSpawns: "0", darnSpawns: "4", wantSpawns: 4, wantRecipe: darnTacklerRecipe},
		{name: "loom takes loom.yaml's value", recipe: shedrun.RecipeLoom, loomSpawns: "2", darnSpawns: "0", wantSpawns: 2, wantRecipe: loomTacklerRecipe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ConfigDir(dir), err)
			}
			documents := map[string]string{
				"loom": strings.Replace(loomengine.ConfigTemplate(), "tackler_max_spawns: ~", "tackler_max_spawns: "+tt.loomSpawns, 1),
				"darn": strings.Replace(darnengine.ConfigTemplate(), "tackler_max_spawns: ~", "tackler_max_spawns: "+tt.darnSpawns, 1),
				"shed": sheddriver.ConfigTemplate(),
			}
			for module, document := range documents {
				if err := os.WriteFile(configengine.ConfigFile(dir, module), []byte(document), 0o644); err != nil {
					t.Fatalf("WriteFile(%s.yaml) = %v; want nil", module, err)
				}
			}

			settings, recipe, err := tacklerFor(configengine.Dirs{Anchor: dir}, tt.recipe, reg)
			if err != nil {
				t.Fatalf("tacklerFor(%q) = _, _, %v; want nil error", tt.recipe, err)
			}
			if settings.MaxSpawns != tt.wantSpawns {
				t.Errorf("tacklerFor(%q) MaxSpawns = %d; want %d", tt.recipe, settings.MaxSpawns, tt.wantSpawns)
			}
			if recipe != tt.wantRecipe {
				t.Errorf("tacklerFor(%q) recipe = %q; want %q", tt.recipe, recipe, tt.wantRecipe)
			}
		})
	}
}
