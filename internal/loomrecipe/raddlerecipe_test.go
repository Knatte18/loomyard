// raddlerecipe_test.go pins the raddle recipe's rows, their routing and the gate and budget its rows carry.

package loomrecipe

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRaddleRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeRaddle, env, paths)
	if err != nil {
		t.Fatalf("New(raddle) error = %v; want nil, which holds the loader's record-only form for the recipe", err)
	}

	wantRows := []string{loomshed.NameRecordWrite, loomshed.NameRaddleWrite, loomshed.NameRaddleFile}
	rows, err := Rows(shedrun.RecipeRaddle)
	if err != nil {
		t.Fatalf("Rows(raddle) error = %v; want nil", err)
	}
	var gotRows []string
	for _, row := range rows {
		gotRows = append(gotRows, row.Name)
	}
	if !slices.Equal(gotRows, wantRows) {
		t.Fatalf("Rows(raddle) names = %v; want %v", gotRows, wantRows)
	}

	byName := map[string]RecipeRow{}
	for _, row := range rows {
		byName[row.Name] = row
	}
	if got := byName[loomshed.NameRaddleWrite]; got.OnDone != loomshed.NameRaddleFile || got.OnStuck != "" {
		t.Errorf("Raddle-Write on_done = %q, on_stuck = %q; want %q and none, so its third failing gate attempt halts the run", got.OnDone, got.OnStuck, loomshed.NameRaddleFile)
	}
	if got := byName[loomshed.NameRaddleFile]; got.OnStuck != loomshed.NameRaddleWrite || got.OnDone != "" {
		t.Errorf("Raddle-File on_stuck = %q, on_done = %q; want %q and none", got.OnStuck, got.OnDone, loomshed.NameRaddleWrite)
	}
	if byName[loomshed.NameRaddleWrite].Segment != byName[loomshed.NameRaddleFile].Segment || byName[loomshed.NameRaddleFile].Segment == "" {
		t.Errorf("Raddle-Write segment = %q, Raddle-File segment = %q; want one shared non-empty segment", byName[loomshed.NameRaddleWrite].Segment, byName[loomshed.NameRaddleFile].Segment)
	}

	for _, producer := range shed.Producers {
		if producer.Name == loomshed.NameRaddleFile && producer.MaxBounces != 3 {
			t.Errorf("Raddle-File max_bounces = %d; want 3", producer.MaxBounces)
		}
	}

	row, found, err := EngineRow(shedrun.RecipeRaddle, "RaddleWrite")
	if err != nil || !found || row != loomshed.NameRaddleWrite {
		t.Errorf("EngineRow(raddle, RaddleWrite) = %q, %v, %v; want %q", row, found, err, loomshed.NameRaddleWrite)
	}

	parsed, err := shedbuild.Parse(recipes.RaddleRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(raddle recipe) = %v", err)
	}
	var gates []any
	for _, producer := range parsed.Producers {
		if producer.Name == loomshed.NameRaddleWrite {
			gates, _ = producer.Config["gates"].([]any)
		}
	}
	if len(gates) != 1 {
		t.Fatalf("Raddle-Write gates = %v; want the one raddle gate", gates)
	}
	if gate, _ := gates[0].(map[string]any); gate["name"] != "raddle" || gate["attempts"] != 3 {
		t.Errorf("Raddle-Write gate = %v; want the raddle gate with 3 attempts", gates[0])
	}
}
