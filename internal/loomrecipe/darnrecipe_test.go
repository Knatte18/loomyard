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
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// wantDarnProducerTable is the darn recipe's rows in order.
var wantDarnProducerTable = []wantProducerRow{
	{loomshed.NamePreflight, "Preflight", reflect.TypeOf(preflightshed.NewPreflight("", ""))},
	{loomshed.NameDarnWrite, "DarnWrite", reflect.TypeOf(loomshed.NewDarnWrite("", nil, loomshed.DarnDeps{}))},
	{loomshed.NameDarnBouncer, "Bouncer", bouncerType},
	{loomshed.NameDarnBurlerReview, "BurlerReview", burlerReviewType},
	{loomshed.NameDarnBurlerFix, "BurlerFix", burlerFixType},
	{loomshed.NameDescribe, "Describe", reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))},
	{loomshed.NamePRReview, "PRReview", reflect.TypeOf(&landingshed.PRReview{})},
	{loomshed.NameFinalize, "Finalize", reflect.TypeOf(&landingshed.Finalize{})},
	{loomshed.NameFrictionReflect, "FrictionReflect", frictionReflectProducerType()},
}

// TestNewDarn_RowsMatchTheDarnTable pins the darn recipe's rows against wantDarnProducerTable in order, and that the Darn-Burler-Review row's fasit instructions name the amended board entry, the writer's claim and the deferral of an unrecorded parent correction.
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

	var fasit string
	for _, row := range recipe.Producers {
		if row.Name != loomshed.NameDarnBurlerReview {
			continue
		}
		profile, _ := row.Config["profile"].(map[string]any)
		fasitConfig, _ := profile["fasit"].(map[string]any)
		fasit, _ = fasitConfig["instructions"].(string)
	}
	for _, want := range []string{
		"Added after Discussion", "claim", "unrecorded parent correction", "decision", "MEDIUM", "leave the code",
	} {
		if !strings.Contains(fasit, want) {
			t.Errorf("Darn-Burler-Review fasit instructions lack %q; got %q", want, fasit)
		}
	}
}

// setDarnBudget sets the Darn-Review segment's budget in env, keeping its config key and file.
func setDarnBudget(env *shedrecipe.Env, budget int) {
	settings := env.Segments["Darn-Review"]
	settings.MaxBounces = budget
	env.Segments["Darn-Review"] = settings
}

// TestNewDarn_Routing pins the darn tail: the Darn-Review perch, then Describe, PR-Review and Finalize after Darn-Write with the review budget on all three perch rows, Finalize's reroute to Darn-Bouncer, no Merge-In row, and a review budget of 0 routing Darn-Write's Done and Finalize's reroute straight to PR-Review.
func TestNewDarn_Routing(t *testing.T) {
	build := func(t *testing.T, reviewBudget int) map[string]shedengine.ProducerDef {
		t.Helper()
		env, paths := testEnv(t)
		setDarnBudget(&env, reviewBudget)
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

	t.Run("the perch and Describe sit between Darn-Write and PR-Review, and Finalize reroutes to the perch", func(t *testing.T) {
		rows := build(t, 4)
		darn := rows[loomshed.NameDarnWrite]
		if darn.OnDone != loomshed.NameDarnBouncer || darn.OnStuck != "" || darn.OnReroute != "" || darn.Segment != "" {
			t.Errorf("Darn-Write OnDone/OnStuck/OnReroute/Segment = %q/%q/%q/%q; want %q/empty/empty/empty", darn.OnDone, darn.OnStuck, darn.OnReroute, darn.Segment, loomshed.NameDarnBouncer)
		}
		bouncer, review, fix := rows[loomshed.NameDarnBouncer], rows[loomshed.NameDarnBurlerReview], rows[loomshed.NameDarnBurlerFix]
		if bouncer.OnDone != loomshed.NameDescribe || bouncer.OnStuck != loomshed.NameDarnBurlerReview || review.OnStuck != loomshed.NameDarnBurlerFix || fix.OnStuck != loomshed.NameDarnBouncer {
			t.Errorf("Darn-Bouncer OnDone/OnStuck = %q/%q, review OnStuck = %q, fix OnStuck = %q; want %q/%q, %q, %q", bouncer.OnDone, bouncer.OnStuck, review.OnStuck, fix.OnStuck, loomshed.NameDescribe, loomshed.NameDarnBurlerReview, loomshed.NameDarnBurlerFix, loomshed.NameDarnBouncer)
		}
		if describe := rows[loomshed.NameDescribe]; describe.OnDone != loomshed.NamePRReview || describe.OnStuck != "" {
			t.Errorf("Describe OnDone/OnStuck = %q/%q; want %q/empty", describe.OnDone, describe.OnStuck, loomshed.NamePRReview)
		}
		if prReview := rows[loomshed.NamePRReview]; prReview.OnDone != loomshed.NameFinalize || prReview.OnStuck != "" || prReview.Segment != "" {
			t.Errorf("PR-Review OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", prReview.OnDone, prReview.OnStuck, prReview.Segment, loomshed.NameFinalize)
		}
		if _, ok := rows[loomshed.NameMergeIn]; ok {
			t.Errorf("darn recipe has a %s row; want none", loomshed.NameMergeIn)
		}
		if finalize := rows[loomshed.NameFinalize]; finalize.OnReroute != loomshed.NameDarnBouncer || finalize.OnDone != loomshed.NameFrictionReflect || finalize.OnStuck != "" {
			t.Errorf("Finalize OnDone/OnStuck/OnReroute = %q/%q/%q; want %q/empty/%q", finalize.OnDone, finalize.OnStuck, finalize.OnReroute, loomshed.NameFrictionReflect, loomshed.NameDarnBouncer)
		}
		if bouncer.Segment != "Darn-Review" || review.Segment != bouncer.Segment || fix.Segment != bouncer.Segment {
			t.Errorf("segments Darn-Bouncer/review/fix = %q/%q/%q; want all Darn-Review", bouncer.Segment, review.Segment, fix.Segment)
		}
		if bouncer.MaxBounces != 4 || review.MaxBounces != 4 || fix.MaxBounces != 4 {
			t.Errorf("MaxBounces Darn-Bouncer/review/fix = %d/%d/%d; want 4/4/4", bouncer.MaxBounces, review.MaxBounces, fix.MaxBounces)
		}
	})

	t.Run("a review budget of 0 routes Darn-Write's Done and Finalize's reroute straight to PR-Review", func(t *testing.T) {
		rows := build(t, 0)
		if got := rows[loomshed.NameDarnWrite].OnDone; got != loomshed.NamePRReview {
			t.Errorf("Darn-Write OnDone = %q; want %q", got, loomshed.NamePRReview)
		}
		if got := rows[loomshed.NameFinalize].OnReroute; got != loomshed.NamePRReview {
			t.Errorf("Finalize OnReroute = %q; want %q", got, loomshed.NamePRReview)
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
			loomshed.NameDarnWrite:     {"discussion": 3, "verify": 7, "description": 3},
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
		if err := applyVerifyAttempts(&recipe, loomshed.NameFinalize, 2); err == nil {
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
		env.Segments["Darn-Review"] = shedrecipe.SegmentSettings{MaxBounces: -1, MaxBouncesKey: "review_max_bounces", ConfigFile: "darn.yaml"}
		env.Landing.OpenFabric = nil
		shed, err := NewDarn(env, paths)
		if err == nil || shed != nil {
			t.Fatalf("NewDarn() = %v, %v; want nil and an error", shed, err)
		}
		want := `loomrecipe: bounce budget of review segment "Darn-Review" = -1 from review_max_bounces in darn.yaml; want 0 or more; way forward: run "lyx config darn --set review_max_bounces=<n>" from the prime, then "lyx loom resume"`
		if err.Error() != want {
			t.Errorf("NewDarn() error = %q; want %q", err.Error(), want)
		}
	})

	t.Run("a missing Darn-Review segment is refused before any row builds", func(t *testing.T) {
		env, paths := testEnv(t)
		delete(env.Segments, "Darn-Review")
		env.Landing.OpenFabric = nil
		shed, err := NewDarn(env, paths)
		if err == nil || shed != nil {
			t.Fatalf("NewDarn() = %v, %v; want nil and an error", shed, err)
		}
		for _, want := range []string{"Darn-Review", "darn wiring", "review_max_bounces"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("NewDarn() error = %q; want it to contain %q", err, want)
			}
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

// TestDarnRouting pins the projection's rows and its review budget applied as NewDarn applies it, the routes to PR-Review under a budget of 0, and the refusal of a budget below 0.
func TestDarnRouting(t *testing.T) {
	t.Run("rows and the review budget on the perch", func(t *testing.T) {
		routing, err := DarnRouting(map[string]int{"Darn-Review": 2})
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

	t.Run("a review budget of 0 routes Darn-Write's Done and Finalize's reroute straight to PR-Review", func(t *testing.T) {
		routing, err := DarnRouting(map[string]int{"Darn-Review": 0})
		if err != nil {
			t.Fatalf("DarnRouting() error = %v; want nil", err)
		}
		for _, p := range routing.Producers {
			if p.Name == loomshed.NameDarnWrite && p.OnDone != loomshed.NamePRReview {
				t.Errorf("Darn-Write OnDone = %q; want %q", p.OnDone, loomshed.NamePRReview)
			}
			if p.Name == loomshed.NameFinalize && p.OnReroute != loomshed.NamePRReview {
				t.Errorf("Finalize OnReroute = %q; want %q", p.OnReroute, loomshed.NamePRReview)
			}
		}
	})

	t.Run("a review budget below 0 is refused with the way forward", func(t *testing.T) {
		_, err := DarnRouting(map[string]int{"Darn-Review": -1})
		if err == nil {
			t.Fatal("DarnRouting() with a negative entry error = nil; want non-nil")
		}
		for _, want := range []string{"Darn-Review", "review_max_bounces", "darn.yaml", `"lyx config darn --set review_max_bounces=<n>" from the prime, then "lyx loom resume"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("DarnRouting() error = %q; want it to contain %q", err, want)
			}
		}
	})

	t.Run("a map lacking Darn-Review is refused", func(t *testing.T) {
		_, err := DarnRouting(map[string]int{})
		if err == nil {
			t.Fatal("DarnRouting() without a Darn-Review entry error = nil; want non-nil")
		}
		for _, want := range []string{"Darn-Review", "darn wiring"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("DarnRouting() error = %q; want it to contain %q", err, want)
			}
		}
	})
}
