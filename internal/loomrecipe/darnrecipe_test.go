// darnrecipe_test.go covers NewDarn and DarnRouting: the darn recipe's row order, engines and producer types, its PR-Review and Darn-Review routing through Describe, the verify budget stamped onto both verify-gated rows, the review budget applied to the perch, and the refusal of a budget out of range.

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
	{loomshed.NameDarnWrite, "DarnWrite", reflect.TypeOf(loomshed.NewDarnWrite("", nil, loomshed.DarnDeps{}))},
	{loomshed.NameDarnBouncer, "Bouncer", bouncerType},
	{loomshed.NameDarnBurlerReview, "BurlerReview", burlerReviewType},
	{loomshed.NameDarnBurlerFix, "BurlerFix", burlerFixType},
	{loomshed.NameDescribe, "Describe", reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))},
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

// TestNewDarn_Routing pins PR-Gate's bounce to Darn-Write inside PR-Review, the Darn-Review perch and then Describe between Darn-Write and Publish with the review budget on all three perch rows, and a review budget of 0 routing Darn-Write straight to Publish.
func TestNewDarn_Routing(t *testing.T) {
	build := func(t *testing.T, reviewBudget int) map[string]shedengine.ProducerDef {
		t.Helper()
		env, paths := testEnv(t)
		env.DarnReviewMaxBounces = reviewBudget
		shed, err := NewDarn(env, paths)
		if err != nil {
			t.Fatalf("NewDarn() error = %v; want nil", err)
		}
		rows := map[string]shedengine.ProducerDef{}
		for _, p := range shed.Producers {
			rows[p.Name] = p
		}
		return rows
	}

	t.Run("PR-Gate bounces to Darn-Write inside its segment and the perch and Describe sit before Publish", func(t *testing.T) {
		rows := build(t, 4)
		gate, darn := rows[loomshed.NamePRGate], rows[loomshed.NameDarnWrite]
		if gate.OnStuck != loomshed.NameDarnWrite || gate.OnDone != loomshed.NameFinalize {
			t.Errorf("PR-Gate OnDone/OnStuck = %q/%q; want %q/%q", gate.OnDone, gate.OnStuck, loomshed.NameFinalize, loomshed.NameDarnWrite)
		}
		if gate.Segment != "PR-Review" || darn.Segment != gate.Segment {
			t.Errorf("segments PR-Gate/Darn-Write = %q/%q; want both PR-Review", gate.Segment, darn.Segment)
		}
		if darn.OnDone != loomshed.NameDarnBouncer || darn.OnStuck != "" {
			t.Errorf("Darn-Write OnDone/OnStuck = %q/%q; want %q/empty", darn.OnDone, darn.OnStuck, loomshed.NameDarnBouncer)
		}
		bouncer, review, fix := rows[loomshed.NameDarnBouncer], rows[loomshed.NameDarnBurlerReview], rows[loomshed.NameDarnBurlerFix]
		if bouncer.OnDone != loomshed.NameDescribe || bouncer.OnStuck != loomshed.NameDarnBurlerReview || review.OnStuck != loomshed.NameDarnBurlerFix || fix.OnStuck != loomshed.NameDarnBouncer {
			t.Errorf("Darn-Bouncer OnDone/OnStuck = %q/%q, review OnStuck = %q, fix OnStuck = %q; want %q/%q, %q, %q", bouncer.OnDone, bouncer.OnStuck, review.OnStuck, fix.OnStuck, loomshed.NameDescribe, loomshed.NameDarnBurlerReview, loomshed.NameDarnBurlerFix, loomshed.NameDarnBouncer)
		}
		if describe := rows[loomshed.NameDescribe]; describe.OnDone != loomshed.NamePublish || describe.OnStuck != "" {
			t.Errorf("Describe OnDone/OnStuck = %q/%q; want %q/empty", describe.OnDone, describe.OnStuck, loomshed.NamePublish)
		}
		if bouncer.Segment != "Darn-Review" || review.Segment != bouncer.Segment || fix.Segment != bouncer.Segment {
			t.Errorf("segments Darn-Bouncer/review/fix = %q/%q/%q; want all Darn-Review", bouncer.Segment, review.Segment, fix.Segment)
		}
		if bouncer.MaxBounces != 4 || review.MaxBounces != 4 || fix.MaxBounces != 4 || gate.MaxBounces != 5 {
			t.Errorf("MaxBounces Darn-Bouncer/review/fix/PR-Gate = %d/%d/%d/%d; want 4/4/4/5", bouncer.MaxBounces, review.MaxBounces, fix.MaxBounces, gate.MaxBounces)
		}
	})

	t.Run("a review budget of 0 routes Darn-Write straight to Publish", func(t *testing.T) {
		rows := build(t, 0)
		if got := rows[loomshed.NameDarnWrite].OnDone; got != loomshed.NamePublish {
			t.Errorf("Darn-Write OnDone = %q; want %q", got, loomshed.NamePublish)
		}
	})
}

func TestDarnVerifyBudget(t *testing.T) {
	t.Run("stamped onto the verify gates of Darn-Write and Darn-Burler-Fix only", func(t *testing.T) {
		recipe, err := shedbuild.Parse(recipes.DarnRecipe)
		if err != nil {
			t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
		}
		for _, row := range darnVerifyGatedRows {
			if err := applyVerifyAttempts(&recipe, row, 7); err != nil {
				t.Fatalf("applyVerifyAttempts(%q) error = %v; want nil", row, err)
			}
		}
		want := map[string]map[any]any{
			loomshed.NameDarnWrite:     {"verify": 7, "description": 3},
			loomshed.NameDarnBurlerFix: {"verify": 7},
			loomshed.NameDescribe:      {"description": 3},
		}
		for _, row := range recipe.Producers {
			gates, _ := row.Config["gates"].([]any)
			got := map[any]any{}
			for _, g := range gates {
				entry := g.(map[string]any)
				got[entry["name"]] = entry["attempts"]
			}
			if wantGates, ok := want[row.Name]; ok && !reflect.DeepEqual(got, wantGates) {
				t.Errorf("%s gates attempts = %v; want %v", row.Name, got, wantGates)
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

	t.Run("a review budget below 0 is refused with the way forward before any row builds", func(t *testing.T) {
		env, paths := testEnv(t)
		env.DarnReviewMaxBounces = -1
		env.Landing.OpenFabric = nil
		shed, err := NewDarn(env, paths)
		if err == nil || shed != nil {
			t.Fatalf("NewDarn() = %v, %v; want nil and an error", shed, err)
		}
		want := `loomrecipe: darn review_max_bounces = -1; want 0 or more; way forward: run "lyx config darn --set review_max_bounces=<n>" from the prime, then "lyx loom resume"`
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

// TestDarnRouting pins the projection's rows and its review budget applied as NewDarn applies it, Darn-Write's route to Publish under a budget of 0, and the refusal of a budget below 0.
func TestDarnRouting(t *testing.T) {
	t.Run("rows and the review budget on the perch", func(t *testing.T) {
		routing, err := DarnRouting(2)
		if err != nil {
			t.Fatalf("DarnRouting() error = %v; want nil", err)
		}
		var names []string
		budgets := map[string]int{}
		for _, p := range routing.Producers {
			names = append(names, p.Name)
			budgets[p.Name] = p.MaxBounces
		}
		var want []string
		for _, row := range wantDarnProducerTable {
			want = append(want, row.name)
		}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("DarnRouting() rows = %v; want %v", names, want)
		}
		for _, row := range []string{loomshed.NameDarnBouncer, loomshed.NameDarnBurlerReview, loomshed.NameDarnBurlerFix} {
			if budgets[row] != 2 {
				t.Errorf("DarnRouting() MaxBounces %s = %d; want 2", row, budgets[row])
			}
		}
	})

	t.Run("a review budget of 0 routes Darn-Write straight to Publish", func(t *testing.T) {
		routing, err := DarnRouting(0)
		if err != nil {
			t.Fatalf("DarnRouting() error = %v; want nil", err)
		}
		for _, p := range routing.Producers {
			if p.Name == loomshed.NameDarnWrite && p.OnDone != loomshed.NamePublish {
				t.Errorf("Darn-Write OnDone = %q; want %q", p.OnDone, loomshed.NamePublish)
			}
		}
	})

	t.Run("a review budget below 0 is refused", func(t *testing.T) {
		if _, err := DarnRouting(-1); err == nil {
			t.Error("DarnRouting(-1) error = nil; want non-nil")
		}
	})
}
