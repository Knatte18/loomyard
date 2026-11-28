// languageskills_test.go checks which shipped recipe rows carry the Burler profile key language-skills.
// Only the code rounds carry it.

package loomrecipe

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestShippedRecipes_LanguageSkillsOnCodeRounds asserts that each shipped recipe sets language-skills.
// It sets the key to the boolean true on exactly the recipe's code-round rows.
// A code row that loses the key fails, and so does a Discussion or Plan row that gains it.
func TestShippedRecipes_LanguageSkillsOnCodeRounds(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		recipe   string
		wantRows []string
	}{
		{
			name:     "loom",
			recipe:   shedrun.RecipeTapestry,
			wantRows: []string{"Code-Burler-Fix", "Code-Burler-Review"},
		},
		{
			name:     "darn",
			recipe:   shedrun.RecipeDarn,
			wantRows: []string{"Code-Burler-Fix", "Code-Burler-Review"},
		},
		{
			name:   "pawl",
			recipe: shedrun.RecipePawl,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recipe, err := parseEmbeddedRecipe(tt.recipe)
			if err != nil {
				t.Fatalf("parseEmbeddedRecipe(%s) error = %v; want nil", tt.recipe, err)
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
