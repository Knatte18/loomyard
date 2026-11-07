// recipe_test.go holds the tests whose subject is the recipe itself rather than New's built list.
// They cover the shape assertion against the recipe-built list and the structural check over the parsed recipe;
// the two split-argument coherence failures New performs;
// the construction-failure surface a bad Env field produces, and the seed/resume row-name pin.

package loomrecipe

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestNew_ShapeMatchesRecipe builds the embedded recipe through New from testEnv(t).
// It asserts row order and concrete Producer type against wantProducerTable.
// It also asserts the perch shape over the built rows.
// Every Bouncer's OnStuck names a review row; that review row's OnStuck names a fix row;
// that fix row's OnStuck names the Bouncer back, and the three rows have equal Segment and MaxBounces.
// No row is unreachable through OnDone/OnStuck from the first row.
func TestNew_ShapeMatchesRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	if len(shed.Producers) != len(wantProducerTable) {
		t.Fatalf("New() produced %d rows; want %d", len(shed.Producers), len(wantProducerTable))
	}

	byName := make(map[string]shedengine.ProducerDef, len(shed.Producers))
	for i, want := range wantProducerTable {
		got := shed.Producers[i]
		byName[got.Name] = got
		if got.Name != want.name {
			t.Errorf("row %d Name = %q; want %q", i, got.Name, want.name)
		}
		if gotType := reflect.TypeOf(got.Producer); gotType != want.producerType {
			t.Errorf("row %d (%s) Producer concrete type = %v; want %v", i, got.Name, gotType, want.producerType)
		}
	}

	for _, row := range shed.Producers {
		if reflect.TypeOf(row.Producer) != bouncerType {
			continue
		}
		review, ok := byName[row.OnStuck]
		if !ok || reflect.TypeOf(review.Producer) != burlerReviewType {
			t.Errorf("Bouncer %q OnStuck = %q; want a review row", row.Name, row.OnStuck)
			continue
		}
		fix, ok := byName[review.OnStuck]
		if !ok || reflect.TypeOf(fix.Producer) != burlerFixType {
			t.Errorf("review row %q OnStuck = %q; want a fix row", review.Name, review.OnStuck)
			continue
		}
		if fix.OnStuck != row.Name {
			t.Errorf("fix row %q OnStuck = %q; want %q, the Bouncer opening the ring", fix.Name, fix.OnStuck, row.Name)
		}
		for _, burler := range []shedengine.ProducerDef{review, fix} {
			if burler.Segment != row.Segment || burler.MaxBounces != row.MaxBounces {
				t.Errorf("perch %q/%q segment/max_bounces = %q/%d vs %q/%d; want equal", row.Name, burler.Name, row.Segment, row.MaxBounces, burler.Segment, burler.MaxBounces)
			}
		}
	}

	reached := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		row, ok := byName[name]
		if !ok || reached[name] {
			return
		}
		reached[name] = true
		visit(row.OnDone)
		visit(row.OnStuck)
	}
	visit(shed.Producers[0].Name)
	for _, row := range shed.Producers {
		if !reached[row.Name] {
			t.Errorf("row %q is unreachable through OnDone/OnStuck from the first row", row.Name)
		}
	}
}

// TestRecipe_StructuralCheckHasNoFindings parses each embedded recipe through shedbuild.Parse.
// It builds the parsed recipe through shedbuild.Build against testEnv(t)'s Env.
// It then asserts that shedbuild.Check(recipe, built) returns no findings.
//
// This goes through Parse/Build rather than through New: it needs the parsed Recipe value, which New does not return,
// so it must not be "simplified" back onto New.
// The tapestry recipe holds a block row, so its parsed recipe is expanded before it is built.
func TestRecipe_StructuralCheckHasNoFindings(t *testing.T) {
	tiergate.Require(t, 0)
	for name, source := range map[string][]byte{"tapestry": recipes.TapestryRecipe, "darn": recipes.DarnRecipe, "pawl": recipes.PawlRecipe, "motif": recipes.MotifRecipe} {
		t.Run(name, func(t *testing.T) {
			env, _ := testEnv(t)

			recipe, err := shedbuild.Parse(source)
			if err != nil {
				t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
			}
			recipe, err = shedbuild.Expand(recipe, embeddedRecipeSource)
			if err != nil {
				t.Fatalf("shedbuild.Expand() error = %v; want nil", err)
			}

			built, err := shedbuild.Build(recipe, env)
			if err != nil {
				t.Fatalf("shedbuild.Build() error = %v; want nil", err)
			}

			if findings := shedbuild.Check(recipe, built); len(findings) != 0 {
				t.Errorf("shedbuild.Check() = %v; want no findings", findings)
			}
		})
	}
}

// TestNew_StatusPathCoherence protects the split: ShedPaths holds duplicates of Env.StatusPath and Env.StatusLockPath.
// A divergent fill between the Env copy and the ShedPaths copy (read by Shed) is silent:
// Shed persists to one file while a row reads another, so New checks the pair itself, ahead of shedbuild.Build.
func TestNew_StatusPathCoherence(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("StatusPath", func(t *testing.T) {
		env, paths := testEnv(t)
		paths.StatusPath = paths.StatusPath + ".other"

		shed, err := New(shedrun.RecipeTapestry, env, paths)
		if err == nil {
			t.Fatalf("New() error = nil; want non-nil error for a divergent StatusPath pair")
		}
		if shed != nil {
			t.Errorf("New() shed = %+v; want nil alongside a non-nil error", shed)
		}
		if !strings.Contains(err.Error(), env.StatusPath) || !strings.Contains(err.Error(), paths.StatusPath) {
			t.Errorf("New() error = %q; want it to name both divergent values %q and %q", err.Error(), env.StatusPath, paths.StatusPath)
		}
	})

	t.Run("StatusLockPath", func(t *testing.T) {
		env, paths := testEnv(t)
		paths.StatusLockPath = paths.StatusLockPath + ".other"

		shed, err := New(shedrun.RecipeTapestry, env, paths)
		if err == nil {
			t.Fatalf("New() error = nil; want non-nil error for a divergent StatusLockPath pair")
		}
		if shed != nil {
			t.Errorf("New() shed = %+v; want nil alongside a non-nil error", shed)
		}
		if !strings.Contains(err.Error(), env.StatusLockPath) || !strings.Contains(err.Error(), paths.StatusLockPath) {
			t.Errorf("New() error = %q; want it to name both divergent values %q and %q", err.Error(), env.StatusLockPath, paths.StatusLockPath)
		}
	})
}

// TestNew_ConstructionFailureNamesOffendingRow covers a construction failure that names the offending row.
// An Env with an empty Cwd makes New return a non-nil error and a nil *shedengine.Shed:
// preflightEntry's requireAbsRoot("Preflight", "Cwd", …) rejects it.
// shedbuild wraps that error with the row's zero-based index and quoted name.
func TestNew_ConstructionFailureNamesOffendingRow(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	env.Cwd = ""

	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err == nil {
		t.Fatalf("New() error = nil; want non-nil error for an Env with an empty Cwd")
	}
	if shed != nil {
		t.Errorf("New() shed = %+v; want nil alongside a non-nil error", shed)
	}
	if !strings.Contains(err.Error(), "Preflight") {
		t.Errorf("New() error = %q; want it to name the offending row %q", err.Error(), "Preflight")
	}
}

// TestRecipeEngines_ReportsExactlyLoomsOwnEngineSet asserts RecipeEngines() returns exactly the engines loom reaches.
// The expected set is the literal below plus wantProducerTable's engine column, sorted and de-duplicated.
// A silently empty return would disable the cross-consumer coverage guard rather than fail it.
func TestRecipeEngines_ReportsExactlyLoomsOwnEngineSet(t *testing.T) {
	tiergate.Require(t, 0)
	// The literal lists the engines a loom row can reach from other recipes:
	// the darn recipe's record row and writer, the sampler recipe's session row, and the pawl recipe's rows;
	// the raddle recipe's writer and file follow.
	// The motif recipe's rows are all tapestry's.
	want := []string{"DarnWrite", "RecordWrite", "SamplerSession", "BeamUp", "BeamAwait", "BeamDown", "RaddleWrite", "RaddleFile"}
	seen := map[string]bool{}
	for _, engine := range want {
		seen[engine] = true
	}
	for _, row := range wantProducerTable {
		engine := row.engine
		if seen[engine] {
			continue
		}
		seen[engine] = true
		want = append(want, engine)
	}
	sort.Strings(want)

	got := RecipeEngines()

	if len(got) != len(want) {
		t.Fatalf("RecipeEngines() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("RecipeEngines()[%d] = %q; want %q", i, got[i], w)
		}
	}
}

// TestRowNames_FollowEachRecipesRowOrder pins that each recipe's names and rows equal its expanded shedbuild rows.
// They must match in order, block fields included, and an unknown recipe name is refused.
func TestRowNames_FollowEachRecipesRowOrder(t *testing.T) {
	tiergate.Require(t, 0)
	for _, tt := range []struct {
		recipe string
		source []byte
	}{
		{shedrun.RecipeTapestry, recipes.TapestryRecipe},
		{shedrun.RecipeDarn, recipes.DarnRecipe},
		{shedrun.RecipeBatten, recipes.BattenRecipe},
		{shedrun.RecipePawl, recipes.PawlRecipe},
		{shedrun.RecipeSampler, recipes.SamplerRecipe},
		{shedrun.RecipeMotif, recipes.MotifRecipe},
		{shedrun.RecipeRaddle, recipes.RaddleRecipe},
	} {
		t.Run(tt.recipe, func(t *testing.T) {
			parsed, err := shedbuild.Parse(tt.source)
			if err != nil {
				t.Fatalf("shedbuild.Parse(%s recipe) = %v", tt.recipe, err)
			}
			parsed, err = shedbuild.Expand(parsed, embeddedRecipeSource)
			if err != nil {
				t.Fatalf("shedbuild.Expand(%s recipe) = %v", tt.recipe, err)
			}
			var want []string
			for _, row := range parsed.Producers {
				want = append(want, row.Name)
			}

			got, err := RowNames(tt.recipe)
			if err != nil {
				t.Fatalf("RowNames(%q) error = %v; want nil", tt.recipe, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("RowNames(%q) = %v; want %v", tt.recipe, got, want)
			}

			rows, err := Rows(tt.recipe)
			if err != nil {
				t.Fatalf("Rows(%q) error = %v; want nil", tt.recipe, err)
			}
			if len(rows) != len(parsed.Producers) {
				t.Fatalf("Rows(%q) has %d rows; want %d", tt.recipe, len(rows), len(parsed.Producers))
			}
			for i, row := range parsed.Producers {
				wantRow := RecipeRow{
					Name: row.Name, Segment: row.Segment, OnStuck: row.OnStuck, OnDone: row.OnDone, Bouncer: row.Engine == "Bouncer",
					BlockPath: row.BlockPath, SkipTo: row.SkipTo, IsBlock: row.Engine == "Block",
				}
				if rows[i] != wantRow {
					t.Errorf("Rows(%q)[%d] = %+v; want %+v", tt.recipe, i, rows[i], wantRow)
				}
			}
		})
	}

	for name, call := range map[string]func(string) error{
		"RowNames": func(recipe string) error { _, err := RowNames(recipe); return err },
		"Rows":     func(recipe string) error { _, err := Rows(recipe); return err },
	} {
		if err := call("no-such-recipe"); err == nil || !strings.Contains(err.Error(), "no-such-recipe") {
			t.Errorf("%s(%q) error = %v; want a refusal naming the recipe", name, "no-such-recipe", err)
		}
	}
}

// TestAmendmentTarget pins that loom's recipe names Plan-Burler-Review and that darn's and batten's name none;
// an unknown recipe name is refused.
func TestAmendmentTarget(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, tt := range []struct {
		recipe string
		want   string
	}{
		{shedrun.RecipeTapestry, "Plan-Burler-Review"},
		{shedrun.RecipeDarn, ""},
		{shedrun.RecipeBatten, ""},
	} {
		t.Run(tt.recipe, func(t *testing.T) {
			t.Parallel()
			got, err := AmendmentTarget(tt.recipe)
			if err != nil {
				t.Fatalf("AmendmentTarget(%q) error = %v; want nil", tt.recipe, err)
			}
			if got != tt.want {
				t.Errorf("AmendmentTarget(%q) = %q; want %q", tt.recipe, got, tt.want)
			}
		})
	}

	if _, err := AmendmentTarget("no-such-recipe"); err == nil || !strings.Contains(err.Error(), "no-such-recipe") {
		t.Errorf("AmendmentTarget(%q) error = %v; want a refusal naming the recipe", "no-such-recipe", err)
	}
}

// TestEntryNumbers pins the entry numbering on the shipped recipes and on a constructed one.
func TestEntryNumbers(t *testing.T) {
	tiergate.Require(t, 0)
	numbersOf := func(t *testing.T, recipe string) map[string]int {
		t.Helper()
		rows, err := Rows(recipe)
		if err != nil {
			t.Fatalf("Rows(%q) = %v", recipe, err)
		}
		return EntryNumbers(rows)
	}
	for _, tt := range []struct {
		name   string
		recipe string
		want   map[string]int
	}{
		{"loom", shedrun.RecipeTapestry, map[string]int{
			"Preflight": 1, "Motif": 2, "Discussion-Write": 3,
			"Discussion-Bouncer": 4, "Discussion-Burler-Review": 4, "Discussion-Burler-Fix": 4,
			"Drift-Baseline": 5,
			"Parts-Write":    6, "Parts-Plan-Write": 7,
			"Plan-Write":   8,
			"Plan-Bouncer": 9, "Plan-Burler-Review": 9, "Plan-Burler-Fix": 9,
			"Drift-Await": 10, "Drift-Expect": 11,
			"Webster": 12, "Parent-Align": 13,
			"Drift-Verify": 14,
			"Code-Bouncer": 15, "Code-Burler-Review": 15, "Code-Burler-Fix": 15,
			"Drift-Recheck": 16, "Describe": 17, "PR-Review": 18, "Finalize": 19, "Friction-Reflect": 20,
		}},
		{"motif", shedrun.RecipeMotif, map[string]int{
			"Discussion-Write": 1, "Discussion-Bouncer": 2, "Discussion-Burler-Review": 2, "Discussion-Burler-Fix": 2,
		}},
		{"batten", shedrun.RecipeBatten, map[string]int{"Run-Shed": 6, "Land": 7}},
		{"darn", shedrun.RecipeDarn, map[string]int{"Record-Write": 2, "Drift-Baseline": 3, "Drift-Await": 4, "Drift-Expect": 5, "Darn-Write": 6, "Drift-Verify": 7, "Code-Bouncer": 8, "Code-Burler-Review": 8, "Code-Burler-Fix": 8, "Drift-Recheck": 9, "Describe": 10, "PR-Review": 11, "Finalize": 12, "Friction-Reflect": 13}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := numbersOf(t, tt.recipe)
			for row, want := range tt.want {
				if got[row] != want {
					t.Errorf("EntryNumbers(%s)[%q] = %d; want %d", tt.recipe, row, got[row], want)
				}
			}
		})
	}

	constructed := []RecipeRow{
		{Name: "A"},
		{Name: "B", Segment: "S"},
		{Name: "C", Segment: "S"},
		{Name: "D", Segment: "R", Bouncer: true},
		{Name: "E", Segment: "R"},
	}
	want := map[string]int{"A": 1, "B": 2, "C": 3, "D": 4, "E": 4}
	if got := EntryNumbers(constructed); !reflect.DeepEqual(got, want) {
		t.Errorf("EntryNumbers(constructed) = %v; want %v", got, want)
	}
	if _, ok := EntryNumbers(constructed)["missing"]; ok {
		t.Error("EntryNumbers(constructed) has a key for a row not among the rows")
	}
	if got := EntryNumbers(nil); len(got) != 0 {
		t.Errorf("EntryNumbers(nil) = %v; want empty", got)
	}
}

// TestRecipe_FinalizeDeclaresNoReroute pins that the tapestry recipe has a Finalize row;
// it declares no reroute edge.
func TestRecipe_FinalizeDeclaresNoReroute(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	found := false
	for _, p := range shed.Producers {
		if p.Name != loomshed.NameFinalize {
			continue
		}
		found = true
		if p.OnReroute != "" {
			t.Errorf("Finalize.OnReroute = %q; want none", p.OnReroute)
		}
	}
	if !found {
		t.Errorf("tapestry recipe has no %s row", loomshed.NameFinalize)
	}
}

// TestRecipe_WebsterReroutesToPlanReview pins the edge a Webster plan conflict travels.
// The Webster row reroutes to Plan-Burler-Review.
// Its fasit instructions name the conflict record by the path that websterengine declares.
func TestRecipe_WebsterReroutesToPlanReview(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	for _, p := range shed.Producers {
		if p.Name == loomshed.NameWebster && p.OnReroute != loomshed.NamePlanBurlerReview {
			t.Errorf("Webster.OnReroute = %q; want %q", p.OnReroute, loomshed.NamePlanBurlerReview)
		}
	}

	recipe, err := shedbuild.Parse(recipes.TapestryRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
	}
	for _, row := range recipe.Producers {
		if row.Name != loomshed.NamePlanBurlerReview {
			continue
		}
		profile, _ := row.Config["profile"].(map[string]any)
		fasit, _ := profile["fasit"].(map[string]any)
		instructions, _ := fasit["instructions"].(string)
		if !strings.Contains(instructions, websterengine.ConflictRecordRel()) {
			t.Errorf("Plan-Burler-Review fasit instructions = %q; want them to name %q", instructions, websterengine.ConflictRecordRel())
		}
		return
	}
	t.Fatalf("recipe has no %q row", loomshed.NamePlanBurlerReview)
}
