//go:build tier0

// recipeoverride_test.go pins RecipeOverride's pick of the module file that holds a run's shed override.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestRecipeOverride verifies the loom recipe and the empty recipe read loom.yaml's override and the darn recipe reads darn.yaml's, over files that set different values so a picker reading the wrong one fails.
func TestRecipeOverride(t *testing.T) {
	t.Parallel()
	loomSpawns, darnSpawns := 4, 0

	tests := []struct {
		name       string
		recipe     string
		wantKind   string
		wantSpawns int
		// darnTreeScope replaces the template's empty tree_scope in darn.yaml; wantErr then lists what the read's refusal names.
		darnTreeScope string
		wantErr       []string
	}{
		{name: "the loom recipe reads loom.yaml", recipe: shedrun.RecipeLoom, wantKind: "llm", wantSpawns: loomSpawns},
		{name: "the empty recipe reads loom.yaml", recipe: "", wantKind: "llm", wantSpawns: loomSpawns},
		{name: "the darn recipe reads darn.yaml", recipe: shedrun.RecipeDarn, wantKind: "go", wantSpawns: darnSpawns},
		{
			name: "the darn recipe refuses a malformed tree key with the editor way forward", recipe: shedrun.RecipeDarn,
			darnTreeScope: `tree_scope: {"./tools": ""}`,
			wantErr:       []string{"./tools", "lyx config darn's editor", "then re-run the lyx command that refused"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			configDir := configengine.ConfigDir(dir)
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
			}
			loomDocument := strings.NewReplacer(`driver_kind: ""`, "driver_kind: llm", "tackler_max_spawns: ~", "tackler_max_spawns: 4").Replace(ConfigTemplate())
			darnReplacements := []string{`driver_kind: ""`, "driver_kind: go", "tackler_max_spawns: ~", "tackler_max_spawns: 0"}
			if tt.darnTreeScope != "" {
				darnReplacements = append(darnReplacements, "tree_scope: {}", tt.darnTreeScope)
			}
			darnDocument := strings.NewReplacer(darnReplacements...).Replace(darnengine.ConfigTemplate())
			for module, document := range map[string]string{"loom": loomDocument, "darn": darnDocument} {
				if err := os.WriteFile(filepath.Join(configDir, module+".yaml"), []byte(document), 0o644); err != nil {
					t.Fatalf("WriteFile(%s.yaml) = %v; want nil", module, err)
				}
			}

			got, err := RecipeOverride(configengine.Dirs{Anchor: dir}, tt.recipe)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("RecipeOverride(%q) = _, nil; want a refusal", tt.recipe)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("RecipeOverride(%q) error = %q; want it to contain %q", tt.recipe, err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("RecipeOverride(%q) = _, %v; want nil error", tt.recipe, err)
			}
			if got.DriverKind != tt.wantKind {
				t.Errorf("RecipeOverride(%q).DriverKind = %q; want %q", tt.recipe, got.DriverKind, tt.wantKind)
			}
			if got.TacklerMaxSpawns == nil || *got.TacklerMaxSpawns != tt.wantSpawns {
				t.Errorf("RecipeOverride(%q).TacklerMaxSpawns = %v; want a set %d", tt.recipe, got.TacklerMaxSpawns, tt.wantSpawns)
			}
		})
	}
}
