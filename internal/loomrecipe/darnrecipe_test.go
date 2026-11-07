// darnrecipe_test.go covers New and Routing over the darn recipe.
// It pins the row order, engines and producer types, and the PR-Review and Code-Review routing through Describe.
// It pins the perch shared with loom's recipe bar its answer key, and the verify budget on both verify-gated rows.
// It pins the review budget on the perch, and the refusal of a budget out of range.

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
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wantDarnProducerTable is the darn recipe's rows in order.
var wantDarnProducerTable = []wantProducerRow{
	{loomshed.NamePreflight, "Preflight", reflect.TypeOf(preflightshed.NewPreflight("", ""))},
	{loomshed.NameRecordWrite, "RecordWrite", reflect.TypeOf(loomshed.NewRecordWrite("", loomshed.RecordWriteDeps{}))},
	{loomshed.NameDriftBaseline, "DriftBaseline", driftBaselineType},
	{loomshed.NameDriftAwait, "DriftAwait", driftAwaitType},
	{loomshed.NameDriftExpect, "DriftExpect", driftExpectType},
	{loomshed.NameDarnWrite, "DarnWrite", reflect.TypeOf(loomshed.NewDarnWrite("", nil, loomshed.DarnDeps{}))},
	{loomshed.NameDriftVerify, "DriftVerify", driftVerifyType},
	{loomshed.NameCodeBouncer, "Bouncer", bouncerType},
	{loomshed.NameCodeBurlerReview, "BurlerReview", burlerReviewType},
	{loomshed.NameCodeBurlerFix, "BurlerFix", burlerFixType},
	{loomshed.NameDriftRecheck, "DriftVerify", driftVerifyType},
	{loomshed.NameDescribe, "Describe", reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))},
	{loomshed.NamePRReview, "PRReview", reflect.TypeOf(&landingshed.PRReview{})},
	{loomshed.NameFinalize, "Finalize", reflect.TypeOf(&landingshed.Finalize{})},
	{loomshed.NameFrictionReflect, "FrictionReflect", frictionReflectProducerType()},
}

// TestNewDarn_RowsMatchTheDarnTable pins the darn recipe's rows against wantDarnProducerTable in order.
func TestNewDarn_RowsMatchTheDarnTable(t *testing.T) {
	tiergate.Require(t, 0)
	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeDarn, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	if len(shed.Producers) != len(wantDarnProducerTable) {
		t.Fatalf("New() built %d rows; want %d", len(shed.Producers), len(wantDarnProducerTable))
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

// TestDarnPerch_SharesLoomsPerchBarTheAnswerKey pins the darn recipe's Code-Review rows against loom's perch.
// They carry loom's segment, run directory, role prefix, rubric and loom.yaml setting keys.
// Only their answer key differs:
// the decision record alone, whose fasit instructions read it as the task's scope with its added entries as amendments.
func TestDarnPerch_SharesLoomsPerchBarTheAnswerKey(t *testing.T) {
	tiergate.Require(t, 0)
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
	darn, loom := rowsOf(recipes.DarnRecipe), rowsOf(recipes.TapestryRecipe)
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

	answerKey := []any{"_lym/discussion/decision-record.md"}
	if got := darn[loomshed.NameCodeBouncer].Config["artifact_paths"]; !reflect.DeepEqual(got, answerKey) {
		t.Errorf("darn Code-Bouncer artifact_paths = %v; want %v", got, answerKey)
	}
	fasit, _ := profileOf(darn[loomshed.NameCodeBurlerReview])["fasit"].(map[string]any)
	if got := fasit["paths"]; !reflect.DeepEqual(got, answerKey) {
		t.Errorf("darn Code-Burler-Review fasit paths = %v; want %v", got, answerKey)
	}
	instructions, _ := fasit["instructions"].(string)
	for _, want := range []string{"Added after Discussion", "## Goal"} {
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

// TestNewDarn_Routing pins the darn tail.
// The Code-Review perch comes first, then Describe, PR-Review and Finalize after Darn-Write.
// The review budget applies to all three perch rows.
// Finalize has no reroute edge and no Merge-In row.
// A review budget of 0 keeps every edge and gates Describe off.
func TestNewDarn_Routing(t *testing.T) {
	tiergate.Require(t, 0)
	build := func(t *testing.T, reviewBudget int) map[string]shedengine.ProducerDef {
		t.Helper()
		env, paths := testEnv(t)
		setCodeBudget(&env, reviewBudget)
		shed, err := New(shedrun.RecipeDarn, env, paths)
		if err != nil {
			t.Fatalf("New() error = %v; want nil", err)
		}
		rows := map[string]shedengine.ProducerDef{}
		for _, p := range shed.Producers {
			rows[p.Name] = p
		}
		return rows
	}

	t.Run("Drift-Baseline follows Preflight and Drift-Await follows it, Drift-Expect follows Drift-Await, Darn-Write follows Drift-Expect, Drift-Verify and the perch follow Darn-Write, and Finalize declares no reroute", func(t *testing.T) {
		rows := build(t, 4)
		if preflight := rows[loomshed.NamePreflight]; preflight.OnDone != loomshed.NameRecordWrite {
			t.Errorf("Preflight OnDone = %q; want %q", preflight.OnDone, loomshed.NameRecordWrite)
		}
		if record := rows[loomshed.NameRecordWrite]; record.OnDone != loomshed.NameDriftBaseline || record.OnStuck != "" || record.Segment != "" {
			t.Errorf("Record-Write OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", record.OnDone, record.OnStuck, record.Segment, loomshed.NameDriftBaseline)
		}
		if baseline := rows[loomshed.NameDriftBaseline]; baseline.OnDone != loomshed.NameDriftAwait || baseline.OnStuck != "" || baseline.Segment != "" {
			t.Errorf("Drift-Baseline OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", baseline.OnDone, baseline.OnStuck, baseline.Segment, loomshed.NameDriftAwait)
		}
		if await := rows[loomshed.NameDriftAwait]; await.OnDone != loomshed.NameDriftExpect || await.OnStuck != "" || await.Segment != "" {
			t.Errorf("Drift-Await OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", await.OnDone, await.OnStuck, await.Segment, loomshed.NameDriftExpect)
		}
		if expect := rows[loomshed.NameDriftExpect]; expect.OnDone != loomshed.NameDarnWrite || expect.OnStuck != "" || expect.Segment != "" {
			t.Errorf("Drift-Expect OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", expect.OnDone, expect.OnStuck, expect.Segment, loomshed.NameDarnWrite)
		}
		darn := rows[loomshed.NameDarnWrite]
		if darn.OnDone != loomshed.NameDriftVerify || darn.OnStuck != "" || darn.OnReroute != "" || darn.Segment != "" {
			t.Errorf("Darn-Write OnDone/OnStuck/OnReroute/Segment = %q/%q/%q/%q; want %q/empty/empty/empty", darn.OnDone, darn.OnStuck, darn.OnReroute, darn.Segment, loomshed.NameDriftVerify)
		}
		if verify := rows[loomshed.NameDriftVerify]; verify.OnDone != loomshed.NameCodeBouncer || verify.OnStuck != "" || verify.Segment != "" {
			t.Errorf("Drift-Verify OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", verify.OnDone, verify.OnStuck, verify.Segment, loomshed.NameCodeBouncer)
		}
		bouncer, review, fix := rows[loomshed.NameCodeBouncer], rows[loomshed.NameCodeBurlerReview], rows[loomshed.NameCodeBurlerFix]
		if bouncer.OnDone != loomshed.NameDriftRecheck || bouncer.OnStuck != loomshed.NameCodeBurlerReview || review.OnStuck != loomshed.NameCodeBurlerFix || fix.OnStuck != loomshed.NameCodeBouncer {
			t.Errorf("Code-Bouncer OnDone/OnStuck = %q/%q, review OnStuck = %q, fix OnStuck = %q; want %q/%q, %q, %q", bouncer.OnDone, bouncer.OnStuck, review.OnStuck, fix.OnStuck, loomshed.NameDriftRecheck, loomshed.NameCodeBurlerReview, loomshed.NameCodeBurlerFix, loomshed.NameCodeBouncer)
		}
		if recheck := rows[loomshed.NameDriftRecheck]; recheck.OnDone != loomshed.NameDescribe || recheck.OnStuck != "" || recheck.Segment != "" {
			t.Errorf("Drift-Recheck OnDone/OnStuck/Segment = %q/%q/%q; want %q/empty/empty", recheck.OnDone, recheck.OnStuck, recheck.Segment, loomshed.NameDescribe)
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
		if prReview := rows[loomshed.NamePRReview]; prReview.OnDone != loomshed.NameFinalize || prReview.OnStuck != "" || prReview.Segment != "" || prReview.EntryGate != nil {
			t.Errorf("PR-Review OnDone/OnStuck/Segment/EntryGate set = %q/%q/%q/%v; want %q/empty/empty/false", prReview.OnDone, prReview.OnStuck, prReview.Segment, prReview.EntryGate != nil, loomshed.NameFinalize)
		}
		if _, ok := rows[loomshed.NameParentAlign]; ok {
			t.Errorf("darn recipe has a %s row; want none", loomshed.NameParentAlign)
		}
		if finalize := rows[loomshed.NameFinalize]; finalize.OnReroute != "" || finalize.OnDone != loomshed.NameFrictionReflect || finalize.OnStuck != "" {
			t.Errorf("Finalize OnDone/OnStuck/OnReroute = %q/%q/%q; want %q/empty/empty", finalize.OnDone, finalize.OnStuck, finalize.OnReroute, loomshed.NameFrictionReflect)
		}
		if bouncer.Segment != codeReviewSegment || review.Segment != bouncer.Segment || fix.Segment != bouncer.Segment {
			t.Errorf("segments Code-Bouncer/review/fix = %q/%q/%q; want all %s", bouncer.Segment, review.Segment, fix.Segment, codeReviewSegment)
		}
		if bouncer.MaxBounces != 4 || review.MaxBounces != 4 || fix.MaxBounces != 4 {
			t.Errorf("MaxBounces Code-Bouncer/review/fix = %d/%d/%d; want 4/4/4", bouncer.MaxBounces, review.MaxBounces, fix.MaxBounces)
		}
	})

	t.Run("on_done from Record-Write walks Drift-Baseline, Drift-Await, Drift-Expect and Darn-Write, the route a goto back to Record-Write takes", func(t *testing.T) {
		rows := build(t, 4)
		row := loomshed.NameRecordWrite
		for _, want := range []string{loomshed.NameDriftBaseline, loomshed.NameDriftAwait, loomshed.NameDriftExpect, loomshed.NameDarnWrite} {
			row = rows[row].OnDone
			if row != want {
				t.Fatalf("on_done walk from Record-Write reached %q; want %q", row, want)
			}
		}
	})

	t.Run("New refuses the batten recipe naming lym shed", func(t *testing.T) {
		env, paths := testEnv(t)
		_, err := New(shedrun.RecipeBatten, env, paths)
		if err == nil || !strings.Contains(err.Error(), "lym shed") {
			t.Errorf("New(batten) error = %v; want a refusal naming lym shed", err)
		}
	})

	t.Run("a review budget of 0 keeps every edge and gates Describe off", func(t *testing.T) {
		rows := build(t, 0)
		if got := rows[loomshed.NameDriftVerify].OnDone; got != loomshed.NameCodeBouncer {
			t.Errorf("Drift-Verify OnDone = %q; want %q", got, loomshed.NameCodeBouncer)
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
		shed, err := New(shedrun.RecipeDarn, env, paths)
		if err == nil || shed != nil {
			t.Fatalf("New() = %v, %v; want nil and an error", shed, err)
		}
		want := `loomrecipe: bounce budget of review segment "Code-Review" = -1 from code_max_bounces in loom.yaml; want 0 or more; way forward: set code_max_bounces in loom.yaml`
		if err.Error() != want {
			t.Errorf("New() error = %q; want %q", err.Error(), want)
		}
	})

	t.Run("a missing Code-Review segment is refused before any row builds", func(t *testing.T) {
		env, paths := testEnv(t)
		delete(env.Segments, codeReviewSegment)
		env.Landing.OpenFabric = nil
		shed, err := New(shedrun.RecipeDarn, env, paths)
		if err == nil || shed != nil {
			t.Fatalf("New() = %v, %v; want nil and an error", shed, err)
		}
		for _, want := range []string{codeReviewSegment, loomSegmentTableWayForward} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("New() error = %q; want it to contain %q", err, want)
			}
		}
	})

	t.Run("a diverging status path is refused first", func(t *testing.T) {
		env, paths := testEnv(t)
		paths.StatusPath += ".other"
		if _, err := New(shedrun.RecipeDarn, env, paths); err == nil || !strings.Contains(err.Error(), "StatusPath") {
			t.Errorf("New() error = %v; want it to name StatusPath", err)
		}
	})
}

func TestDarnVerifyBudget(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("stamped onto the verify gates of Darn-Write and Code-Burler-Fix only", func(t *testing.T) {
		recipe, err := shedbuild.Parse(recipes.DarnRecipe)
		if err != nil {
			t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
		}
		gatedRows := verifyGatedRows(recipe)
		if want := []string{loomshed.NameDarnWrite, loomshed.NameCodeBurlerFix}; !reflect.DeepEqual(gatedRows, want) {
			t.Fatalf("verifyGatedRows() = %v; want %v", gatedRows, want)
		}
		for _, row := range gatedRows {
			if err := applyVerifyAttempts(&recipe, row, 7); err != nil {
				t.Fatalf("applyVerifyAttempts(%q) error = %v; want nil", row, err)
			}
		}
		want := map[string]map[any]any{
			loomshed.NameDarnWrite:     {"clean": 3, "verify": 7, "drift": 3, "description": 3},
			loomshed.NameCodeBurlerFix: {"clean": 3, "verify": 7},
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

	t.Run("on the sampler recipe only Code-Burler-Fix is stamped and Describe keeps its gate", func(t *testing.T) {
		recipe, err := shedbuild.Parse(recipes.SamplerRecipe)
		if err != nil {
			t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
		}
		gatedRows := verifyGatedRows(recipe)
		if want := []string{loomshed.NameCodeBurlerFix}; !reflect.DeepEqual(gatedRows, want) {
			t.Fatalf("verifyGatedRows() = %v; want %v", gatedRows, want)
		}
		for _, row := range gatedRows {
			if err := applyVerifyAttempts(&recipe, row, 7); err != nil {
				t.Fatalf("applyVerifyAttempts(%q) error = %v; want nil", row, err)
			}
		}
		want := map[string]map[any]any{
			loomshed.NameCodeBurlerFix: {"clean": 3, "verify": 7},
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

	t.Run("on the tapestry recipe Code-Burler-Fix runs the clean gate before verify", func(t *testing.T) {
		recipe, err := shedbuild.Parse(recipes.TapestryRecipe)
		if err != nil {
			t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
		}
		var names []any
		for _, row := range recipe.Producers {
			if row.Name != loomshed.NameCodeBurlerFix {
				continue
			}
			gates, _ := row.Config["gates"].([]any)
			for _, g := range gates {
				names = append(names, g.(map[string]any)["name"])
			}
		}
		if want := []any{"clean", "verify"}; !reflect.DeepEqual(names, want) {
			t.Errorf("tapestry Code-Burler-Fix gate names = %v; want %v", names, want)
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
		shed, err := New(shedrun.RecipeDarn, env, paths)
		if err == nil || shed != nil {
			t.Fatalf("New() = %v, %v; want nil and an error", shed, err)
		}
		want := `loomrecipe: verify_attempts = 0; want at least 1; way forward: run "lym config darn --set verify_attempts=<n>" from the prime, then "lym loom resume"`
		if err.Error() != want {
			t.Errorf("New() error = %q; want %q", err.Error(), want)
		}
	})
}

// TestDarnRouting pins the projection's rows and the review budget as New applies it.
// It also pins the kept edges under a budget of 0, and the refusal of a budget below 0.
func TestDarnRouting(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("rows and the review budget on the perch", func(t *testing.T) {
		routing, err := Routing(shedrun.RecipeDarn, map[string]int{codeReviewSegment: 2})
		if err != nil {
			t.Fatalf("Routing() error = %v; want nil", err)
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
			t.Errorf("Routing() rows = %v; want %v", names, want)
		}
		for _, row := range []string{loomshed.NameCodeBouncer, loomshed.NameCodeBurlerReview, loomshed.NameCodeBurlerFix} {
			if budgets[row] != 2 {
				t.Errorf("Routing() MaxBounces %s = %d; want 2", row, budgets[row])
			}
		}
	})

	t.Run("a review budget of 0 keeps every edge and applies the budget to the perch rows", func(t *testing.T) {
		routing, err := Routing(shedrun.RecipeDarn, map[string]int{codeReviewSegment: 0})
		if err != nil {
			t.Fatalf("Routing() error = %v; want nil", err)
		}
		for _, p := range routing.Producers {
			if p.Name == loomshed.NameDriftVerify && p.OnDone != loomshed.NameCodeBouncer {
				t.Errorf("Drift-Verify OnDone = %q; want %q", p.OnDone, loomshed.NameCodeBouncer)
			}
			if p.Segment == codeReviewSegment && p.MaxBounces != 0 {
				t.Errorf("%s MaxBounces = %d; want the budget 0", p.Name, p.MaxBounces)
			}
		}
	})

	t.Run("a review budget below 0 is refused naming the segment", func(t *testing.T) {
		_, err := Routing(shedrun.RecipeDarn, map[string]int{codeReviewSegment: -1})
		if err == nil {
			t.Fatal("Routing() with a negative entry error = nil; want non-nil")
		}
		for _, want := range []string{codeReviewSegment, "loom.yaml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Routing() error = %q; want it to contain %q", err, want)
			}
		}
	})

	t.Run("a map lacking Code-Review is refused", func(t *testing.T) {
		_, err := Routing(shedrun.RecipeDarn, map[string]int{})
		if err == nil {
			t.Fatal("Routing() without a Code-Review entry error = nil; want non-nil")
		}
		for _, want := range []string{codeReviewSegment, loomSegmentTableWayForward} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Routing() error = %q; want it to contain %q", err, want)
			}
		}
	})
}
