//go:build tier0

// darnrecipe_test.go covers NewDarn and DarnRouting: the darn recipe's row order, engines and producer types, its PR-Review and Code-Review routing through Describe, the perch shared with loom's recipe bar its answer key, the verify budget stamped onto both verify-gated rows, the review budget applied to the perch, and the refusal of a budget out of range.

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
	{loomshed.NameCodeBouncer, "Bouncer", bouncerType},
	{loomshed.NameCodeBurlerReview, "BurlerReview", burlerReviewType},
	{loomshed.NameCodeBurlerFix, "BurlerFix", burlerFixType},
	{loomshed.NameDescribe, "Describe", reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))},
	{loomshed.NamePRReview, "PRReview", reflect.TypeOf(&landingshed.PRReview{})},
	{loomshed.NameFinalize, "Finalize", reflect.TypeOf(&landingshed.Finalize{})},
	{loomshed.NameFrictionReflect, "FrictionReflect", frictionReflectProducerType()},
}

// TestNewDarn_RowsMatchTheDarnTable pins the darn recipe's rows against wantDarnProducerTable in order.
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

// TestDarnPerch_SharesLoomsPerchBarTheAnswerKey pins that the darn recipe's Code-Review rows carry loom's segment, run directory, role prefix, rubric and loom.yaml setting keys,
// and that only their answer key differs: the board entry snapshot and the decision record, whose fasit instructions read the record's added entries as amendments and its other sections as the writer's claim.
func TestDarnPerch_SharesLoomsPerchBarTheAnswerKey(t *testing.T) {
	rowsOf := func(source []byte) map[string]shedbuild.Row {
		t.Helper()
		recipe, err := shedbuild.Parse(source)
		if err != nil {
			t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
		}
		rows := map[string]shedbuild.Row{}
		for _, row := range recipe.Producers {
			rows[row.Name] = row
		}
		return rows
	}
	darn, loom := rowsOf(recipes.DarnRecipe), rowsOf(recipes.LoomRecipe)
	profileOf := func(row shedbuild.Row) map[string]any {
		profile, _ := row.Config["profile"].(map[string]any)
		return profile
	}

	shared := []string{"run_subdir", "subject", "role_prefix", "reviewer_fan_setting", "fixer_fan_setting", "auto_accept_setting", "rubric_stencil", "carry_over", "review_row"}
	for _, name := range []string{loomshed.NameCodeBouncer, loomshed.NameCodeBurlerReview, loomshed.NameCodeBurlerFix} {
		darnRow, loomRow := darn[name], loom[name]
		if darnRow.Segment != loomRow.Segment || darnRow.Engine != loomRow.Engine || darnRow.OnStuck != loomRow.OnStuck {
			t.Errorf("darn %s segment/engine/on_stuck = %q/%q/%q; want loom's %q/%q/%q", name, darnRow.Segment, darnRow.Engine, darnRow.OnStuck, loomRow.Segment, loomRow.Engine, loomRow.OnStuck)
		}
		for _, key := range shared {
			if !reflect.DeepEqual(darnRow.Config[key], loomRow.Config[key]) {
				t.Errorf("darn %s config %s = %v; want loom's %v", name, key, darnRow.Config[key], loomRow.Config[key])
			}
		}
		for _, key := range []string{"rubric_stencil", "fix-scope", "tool-use", "language-skills"} {
			if got, want := profileOf(darnRow)[key], profileOf(loomRow)[key]; !reflect.DeepEqual(got, want) {
				t.Errorf("darn %s profile %s = %v; want loom's %v", name, key, got, want)
			}
		}
	}

	answerKey := []any{"_lyx/discussion/board-entry.md", "_lyx/discussion/decision-record.md"}
	if got := darn[loomshed.NameCodeBouncer].Config["artifact_paths"]; !reflect.DeepEqual(got, answerKey) {
		t.Errorf("darn Code-Bouncer artifact_paths = %v; want %v", got, answerKey)
	}
	fasit, _ := profileOf(darn[loomshed.NameCodeBurlerReview])["fasit"].(map[string]any)
	if got := fasit["paths"]; !reflect.DeepEqual(got, answerKey) {
		t.Errorf("darn Code-Burler-Review fasit paths = %v; want %v", got, answerKey)
	}
	instructions, _ := fasit["instructions"].(string)
	for _, want := range []string{"Added after Discussion", "writer's own claim"} {
		if !strings.Contains(instructions, want) {
			t.Errorf("darn Code-Burler-Review fasit instructions lack %q; got %q", want, instructions)
		}
	}
}

// setCodeBudget sets the Code-Review segment's budget in env, keeping its config key and file.
func setCodeBudget(env *shedrecipe.Env, budget int) {
	settings := env.Segments[codeReviewSegment]
	settings.MaxBounces = budget
	env.Segments[codeReviewSegment] = settings
}

// TestNewDarn_Routing pins the darn tail: the Code-Review perch, then Describe, PR-Review and Finalize after Darn-Write with the review budget on all three perch rows, Finalize's reroute to Code-Bouncer, no Merge-In row, and a review budget of 0 keeping every edge and gating Describe off.
func TestNewDarn_Routing(t *testing.T) {
	build := func(t *testing.T, reviewBudget int) map[string]shedengine.ProducerDef {
		t.Helper()
		env, paths := testEnv(t)
		setCodeBudget(&env, reviewBudget)
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
		if darn.OnDone != loomshed.NameCodeBouncer || darn.OnStuck != "" || darn.OnReroute != "" || darn.Segment != "" {
			t.Errorf("Darn-Write OnDone/OnStuck/OnReroute/Segment = %q/%q/%q/%q; want %q/empty/empty/empty", darn.OnDone, darn.OnStuck, darn.OnReroute, darn.Segment, loomshed.NameCodeBouncer)
		}
		bouncer, review, fix := rows[loomshed.NameCodeBouncer], rows[loomshed.NameCodeBurlerReview], rows[loomshed.NameCodeBurlerFix]
		if bouncer.OnDone != loomshed.NameDescribe || bouncer.OnStuck != loomshed.NameCodeBurlerReview || review.OnStuck != loomshed.NameCodeBurlerFix || fix.OnStuck != loomshed.NameCodeBouncer {
			t.Errorf("Code-Bouncer OnDone/OnStuck = %q/%q, review OnStuck = %q, fix OnStuck = %q; want %q/%q, %q, %q", bouncer.OnDone, bouncer.OnStuck, review.OnStuck, fix.OnStuck, loomshed.NameDescribe, loomshed.NameCodeBurlerReview, loomshed.NameCodeBurlerFix, loomshed.NameCodeBouncer)
		}
		describe := rows[loomshed.NameDescribe]
		if describe.OnDone != loomshed.NamePRReview || describe.OnStuck != "" {
			t.Errorf("Describe OnDone/OnStuck = %q/%q; want %q/empty", describe.OnDone, describe.OnStuck, loomshed.NamePRReview)
		}
		if describe.EntryGate == nil {
			t.Fatal("Describe EntryGate = nil; want a gate on the Code-Review budget")
		}
		if holds, _, err := describe.EntryGate(); err != nil || !holds {
			t.Errorf("Describe EntryGate() with a review budget of 4 = %v, %v; want true, nil", holds, err)
		}
		if prReview := rows[loomshed.NamePRReview]; prReview.OnDone != loomshed.NameFinalize || prReview.OnStuck != "" || prReview.Segment != "" {
			t.Errorf("PR-Review OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", prReview.OnDone, prReview.OnStuck, prReview.Segment, loomshed.NameFinalize)
		}
		if _, ok := rows[loomshed.NameParentAlign]; ok {
			t.Errorf("darn recipe has a %s row; want none", loomshed.NameParentAlign)
		}
		if finalize := rows[loomshed.NameFinalize]; finalize.OnReroute != loomshed.NameCodeBouncer || finalize.OnDone != loomshed.NameFrictionReflect || finalize.OnStuck != "" {
			t.Errorf("Finalize OnDone/OnStuck/OnReroute = %q/%q/%q; want %q/empty/%q", finalize.OnDone, finalize.OnStuck, finalize.OnReroute, loomshed.NameFrictionReflect, loomshed.NameCodeBouncer)
		}
		if bouncer.Segment != codeReviewSegment || review.Segment != bouncer.Segment || fix.Segment != bouncer.Segment {
			t.Errorf("segments Code-Bouncer/review/fix = %q/%q/%q; want all %s", bouncer.Segment, review.Segment, fix.Segment, codeReviewSegment)
		}
		if bouncer.MaxBounces != 4 || review.MaxBounces != 4 || fix.MaxBounces != 4 {
			t.Errorf("MaxBounces Code-Bouncer/review/fix = %d/%d/%d; want 4/4/4", bouncer.MaxBounces, review.MaxBounces, fix.MaxBounces)
		}
	})

	t.Run("a review budget of 0 keeps every edge and gates Describe off", func(t *testing.T) {
		rows := build(t, 0)
		if got := rows[loomshed.NameDarnWrite].OnDone; got != loomshed.NameCodeBouncer {
			t.Errorf("Darn-Write OnDone = %q; want %q", got, loomshed.NameCodeBouncer)
		}
		if got := rows[loomshed.NameFinalize].OnReroute; got != loomshed.NameCodeBouncer {
			t.Errorf("Finalize OnReroute = %q; want %q", got, loomshed.NameCodeBouncer)
		}
		describe := rows[loomshed.NameDescribe]
		if describe.EntryGate == nil {
			t.Fatal("Describe EntryGate = nil; want a gate on the Code-Review budget")
		}
		if holds, _, err := describe.EntryGate(); err != nil || holds {
			t.Errorf("Describe EntryGate() with a review budget of 0 = %v, %v; want false, nil", holds, err)
		}
	})

	t.Run("a review budget below 0 is refused with the way forward before any row builds", func(t *testing.T) {
		env, paths := testEnv(t)
		env.Segments[codeReviewSegment] = shedrecipe.SegmentSettings{MaxBounces: -1, MaxBouncesKey: "code_max_bounces", ConfigFile: "loom.yaml"}
		env.Landing.OpenFabric = nil
		shed, err := NewDarn(env, paths)
		if err == nil || shed != nil {
			t.Fatalf("NewDarn() = %v, %v; want nil and an error", shed, err)
		}
		want := `loomrecipe: bounce budget of review segment "Code-Review" = -1 from code_max_bounces in loom.yaml; want 0 or more; way forward: set code_max_bounces in loom.yaml`
		if err.Error() != want {
			t.Errorf("NewDarn() error = %q; want %q", err.Error(), want)
		}
	})

	t.Run("a missing Code-Review segment is refused before any row builds", func(t *testing.T) {
		env, paths := testEnv(t)
		delete(env.Segments, codeReviewSegment)
		env.Landing.OpenFabric = nil
		shed, err := NewDarn(env, paths)
		if err == nil || shed != nil {
			t.Fatalf("NewDarn() = %v, %v; want nil and an error", shed, err)
		}
		for _, want := range []string{codeReviewSegment, loomSegmentTableWayForward} {
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

func TestDarnVerifyBudget(t *testing.T) {
	t.Run("stamped onto the verify gates of Darn-Write and Code-Burler-Fix only", func(t *testing.T) {
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
			loomshed.NameCodeBurlerFix: {"verify": 7},
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
}

// TestDarnRouting pins the projection's rows and its review budget applied as NewDarn applies it, the kept edges under a budget of 0, and the refusal of a budget below 0.
func TestDarnRouting(t *testing.T) {
	t.Run("rows and the review budget on the perch", func(t *testing.T) {
		routing, err := DarnRouting(map[string]int{codeReviewSegment: 2})
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
		for _, row := range []string{loomshed.NameCodeBouncer, loomshed.NameCodeBurlerReview, loomshed.NameCodeBurlerFix} {
			if budgets[row] != 2 {
				t.Errorf("DarnRouting() MaxBounces %s = %d; want 2", row, budgets[row])
			}
		}
	})

	t.Run("a review budget of 0 keeps every edge and applies the budget to the perch rows", func(t *testing.T) {
		routing, err := DarnRouting(map[string]int{codeReviewSegment: 0})
		if err != nil {
			t.Fatalf("DarnRouting() error = %v; want nil", err)
		}
		for _, p := range routing.Producers {
			if p.Name == loomshed.NameDarnWrite && p.OnDone != loomshed.NameCodeBouncer {
				t.Errorf("Darn-Write OnDone = %q; want %q", p.OnDone, loomshed.NameCodeBouncer)
			}
			if p.Name == loomshed.NameFinalize && p.OnReroute != loomshed.NameCodeBouncer {
				t.Errorf("Finalize OnReroute = %q; want %q", p.OnReroute, loomshed.NameCodeBouncer)
			}
			if p.Segment == codeReviewSegment && p.MaxBounces != 0 {
				t.Errorf("%s MaxBounces = %d; want the budget 0", p.Name, p.MaxBounces)
			}
		}
	})

	t.Run("a review budget below 0 is refused naming the segment", func(t *testing.T) {
		_, err := DarnRouting(map[string]int{codeReviewSegment: -1})
		if err == nil {
			t.Fatal("DarnRouting() with a negative entry error = nil; want non-nil")
		}
		for _, want := range []string{codeReviewSegment, "loom.yaml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("DarnRouting() error = %q; want it to contain %q", err, want)
			}
		}
	})

	t.Run("a map lacking Code-Review is refused", func(t *testing.T) {
		_, err := DarnRouting(map[string]int{})
		if err == nil {
			t.Fatal("DarnRouting() without a Code-Review entry error = nil; want non-nil")
		}
		for _, want := range []string{codeReviewSegment, loomSegmentTableWayForward} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("DarnRouting() error = %q; want it to contain %q", err, want)
			}
		}
	})
}
