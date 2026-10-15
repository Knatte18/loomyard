// languageskills_test.go pins which shipped recipe rows carry the Burler profile key language-skills: the code rounds only.

package loomrecipe

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestShippedRecipes_LanguageSkillsOnCodeRounds asserts each shipped recipe sets language-skills, as the boolean true, on exactly its code-round rows, so a code row losing the key or a Discussion or Plan row gaining it fails.
func TestShippedRecipes_LanguageSkillsOnCodeRounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		wantRows []string
	}{
		{
			name:     "loom",
			data:     recipes.LoomRecipe,
			wantRows: []string{"Merge-Burler-Fix", "Merge-Burler-Review", "Webster-Burler-Fix", "Webster-Burler-Review"},
		},
		{
			name:     "darn",
			data:     recipes.DarnRecipe,
			wantRows: []string{"Darn-Burler-Fix", "Darn-Burler-Review"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recipe, err := shedbuild.Parse(tt.data)
			if err != nil {
				t.Fatalf("Parse() error = %v; want nil", err)
			}

			var gotRows []string
			for _, row := range recipe.Producers {
				profile, _ := row.Config["profile"].(map[string]any)
				value, ok := profile["language-skills"]
				if !ok {
					continue
				}
				if value != true {
					t.Errorf("row %q language-skills = %#v; want the boolean true", row.Name, value)
				}
				gotRows = append(gotRows, row.Name)
			}
			slices.Sort(gotRows)
			if !slices.Equal(gotRows, tt.wantRows) {
				t.Errorf("rows carrying language-skills = %v; want %v", gotRows, tt.wantRows)
			}
		})
	}
}
