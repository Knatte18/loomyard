// childrecipe_test.go pins which recipes a task worktree's bootstrap verb can run as the child's own run.
// It also pins the refusal that names those recipes.

package battencli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestChildRecipeSupported(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		recipe string
		want   bool
	}{
		{shedrun.RecipeTapestry, true},
		{shedrun.RecipeMotif, true},
		{shedrun.RecipeDarn, true},
		{shedrun.RecipePawl, true},
		{shedrun.RecipeSampler, true},
		{shedrun.RecipeRaddle, true},
		{shedrun.RecipeBatten, false},
		{"no-such-recipe", false},
	}
	for _, tt := range tests {
		t.Run(tt.recipe, func(t *testing.T) {
			t.Parallel()
			if got := childRecipeSupported(tt.recipe); got != tt.want {
				t.Errorf("childRecipeSupported(%q) = %v; want %v", tt.recipe, got, tt.want)
			}
		})
	}

	err := unsupportedChildRecipeRefusal("no-such-recipe")
	if !errors.Is(err, battenshed.ErrUnsupportedChildRecipe) {
		t.Fatalf("unsupportedChildRecipeRefusal() error = %v; want it to wrap ErrUnsupportedChildRecipe", err)
	}
	for _, want := range []string{shedrun.RecipeRaddle, "no-such-recipe"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("unsupportedChildRecipeRefusal() error = %q; want it to contain %q", err, want)
		}
	}
}
