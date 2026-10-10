// darnrecipe_test.go covers NewDarn and DarnRouting: the darn recipe's row order, engines and producer types, its PR-Review routing, the verify budget stamped onto the Darn row, and the refusal of a budget below 1.

package loomrecipe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// wantDarnProducerTable is the darn recipe's rows in order.
var wantDarnProducerTable = []wantProducerRow{
	{loomshed.NamePreflight, "Preflight", reflect.TypeOf(preflightshed.NewPreflight("", ""))},
	{loomshed.NameDarn, "DarnWrite", reflect.TypeOf(loomshed.NewDarnWrite("", nil, loomshed.DarnDeps{}))},
	{loomshed.NamePublish, "Publish", reflect.TypeOf(&landingshed.Publish{})},
	{loomshed.NamePRGate, "PRGate", reflect.TypeOf(&landingshed.PRGate{})},
	{loomshed.NameFinalize, "Finalize", reflect.TypeOf(&landingshed.Finalize{})},
	{loomshed.NameFrictionReflect, "FrictionReflect", frictionReflectProducerType()},
}

func TestNewDarn_RowsMatchTheDarnTable(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := NewDarn(env, paths)
	if err != nil {
		t.Fatalf("NewDarn() error = %v; want nil", err)
	}
	if len(shed.Producers) != len(wantDarnProducerTable) {
		t.Fatalf("NewDarn() built %d rows; want %d", len(shed.Producers), len(wantDarnProducerTable))
	}
	for i, want := range wantDarnProducerTable {
		got := shed.Producers[i]
		if got.Name != want.name {
			t.Errorf("row %d name = %q; want %q", i, got.Name, want.name)
		}
		if gotType := reflect.TypeOf(got.Producer); gotType != want.producerType {
			t.Errorf("row %q producer type = %v; want %v", want.name, gotType, want.producerType)
		}
	}

	recipe, err := shedbuild.Parse(recipes.DarnRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
	}
	for i, want := range wantDarnProducerTable {
		if recipe.Producers[i].Engine != want.engine {
			t.Errorf("recipe row %q engine = %q; want %q", want.name, recipe.Producers[i].Engine, want.engine)
		}
	}
}

func TestNewDarn_PRGateBouncesToTheDarnRowInsideItsSegment(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := NewDarn(env, paths)
	if err != nil {
		t.Fatalf("NewDarn() error = %v; want nil", err)
	}
	rows := map[string]shedengine.ProducerDef{}
	for _, p := range shed.Producers {
		rows[p.Name] = p
	}
	gate, darn := rows[loomshed.NamePRGate], rows[loomshed.NameDarn]
	if gate.OnStuck != loomshed.NameDarn || gate.OnDone != loomshed.NameFinalize {
		t.Errorf("PR-Gate OnDone/OnStuck = %q/%q; want %q/%q", gate.OnDone, gate.OnStuck, loomshed.NameFinalize, loomshed.NameDarn)
	}
	if gate.Segment != "PR-Review" || darn.Segment != gate.Segment {
		t.Errorf("segments PR-Gate/Darn = %q/%q; want both PR-Review", gate.Segment, darn.Segment)
	}
	if darn.OnDone != loomshed.NamePublish || darn.OnStuck != "" {
		t.Errorf("Darn OnDone/OnStuck = %q/%q; want %q/empty", darn.OnDone, darn.OnStuck, loomshed.NamePublish)
	}
}

func TestDarnVerifyBudget(t *testing.T) {
	t.Run("stamped onto the Darn row's verify gate only", func(t *testing.T) {
		recipe, err := shedbuild.Parse(recipes.DarnRecipe)
		if err != nil {
			t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
		}
		if err := applyVerifyAttempts(&recipe, loomshed.NameDarn, 7); err != nil {
			t.Fatalf("applyVerifyAttempts() error = %v; want nil", err)
		}
		for _, row := range recipe.Producers {
			if row.Name != loomshed.NameDarn {
				continue
			}
			gates := row.Config["gates"].([]any)
			got := map[any]any{}
			for _, g := range gates {
				entry := g.(map[string]any)
				got[entry["name"]] = entry["attempts"]
			}
			if got["verify"] != 7 || got["description"] != 3 {
				t.Errorf("Darn gates attempts = %v; want verify 7, description 3", got)
			}
		}
	})

	t.Run("a missing row or gate entry is an error", func(t *testing.T) {
		recipe, err := shedbuild.Parse(recipes.DarnRecipe)
		if err != nil {
			t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
		}
		if err := applyVerifyAttempts(&recipe, "No-Such-Row", 2); err == nil {
			t.Error("applyVerifyAttempts() on a missing row: error = nil; want non-nil")
		}
		if err := applyVerifyAttempts(&recipe, loomshed.NamePublish, 2); err == nil {
			t.Error("applyVerifyAttempts() on a row with no gates: error = nil; want non-nil")
		}
	})

	t.Run("a budget below 1 is refused with the way forward before any row builds", func(t *testing.T) {
		env, paths := testEnv(t)
		env.DarnVerifyAttempts = 0
		env.Landing.OpenFabric = nil
		shed, err := NewDarn(env, paths)
		if err == nil || shed != nil {
			t.Fatalf("NewDarn() = %v, %v; want nil and an error", shed, err)
		}
		want := `loomrecipe: verify_attempts = 0; want at least 1; way forward: run "lyx config darn --set verify_attempts=<n>" from the prime, then "lyx loom resume"`
		if err.Error() != want {
			t.Errorf("NewDarn() error = %q; want %q", err.Error(), want)
		}
	})

	t.Run("a diverging status path is refused first", func(t *testing.T) {
		env, paths := testEnv(t)
		paths.StatusPath += ".other"
		if _, err := NewDarn(env, paths); err == nil || !strings.Contains(err.Error(), "StatusPath") {
			t.Errorf("NewDarn() error = %v; want it to name StatusPath", err)
		}
	})
}

func TestDarnRouting_ReadsNoBudget(t *testing.T) {
	routing, err := DarnRouting()
	if err != nil {
		t.Fatalf("DarnRouting() error = %v; want nil", err)
	}
	var names []string
	for _, p := range routing.Producers {
		names = append(names, p.Name)
	}
	var want []string
	for _, row := range wantDarnProducerTable {
		want = append(want, row.name)
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("DarnRouting() rows = %v; want %v", names, want)
	}
}
