// childrecipe_test.go pins the child-recipe record's place, its relative form and its round trip.

package shedrun

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestChildRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	l := syntheticLocation(t)

	if want := filepath.Join(l.AnchorPath(), lymdirs.LymDirName, "shed", "slug", "child-recipe"); ChildRecipeFile(l, "slug") != want {
		t.Errorf("ChildRecipeFile() = %q; want %q", ChildRecipeFile(l, "slug"), want)
	}
	if want := filepath.Join(lymdirs.LymDirName, "shed", "slug", "child-recipe"); ChildRecipeRel(l, "slug") != want {
		t.Errorf("ChildRecipeRel() = %q; want %q", ChildRecipeRel(l, "slug"), want)
	}
	if got, err := ReadChildRecipe(l, "slug"); err != nil || got != "" {
		t.Errorf("ReadChildRecipe() before a write = %q, %v; want empty and nil", got, err)
	}
	for _, recipe := range []string{RecipeMotif, RecipeDarn} {
		if err := WriteChildRecipe(l, "slug", recipe); err != nil {
			t.Fatalf("WriteChildRecipe(%q) = %v; want nil", recipe, err)
		}
		if got, err := ReadChildRecipe(l, "slug"); err != nil || got != recipe {
			t.Errorf("ReadChildRecipe() = %q, %v; want %q and nil", got, err, recipe)
		}
	}
}
