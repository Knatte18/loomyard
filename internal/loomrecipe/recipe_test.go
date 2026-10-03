// recipe_test.go carries the tests whose subject is the recipe itself rather than New's built
// list: the shape assertion against the recipe-built list, the structural check over the parsed
// recipe, the two split-argument coherence failures New itself performs, the construction-failure
// surface a bad Env field produces, and the seed/resume row-name pin.

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
)

// TestNew_ShapeMatchesRecipe builds the embedded recipe through New from testEnv(t) and asserts row order and concrete Producer type against wantProducerTable.
// It also asserts the perch shape over the built rows: every Bouncer's OnStuck names a Burler whose OnStuck names it back, with equal Segment and MaxBounces, and no row is unreachable through OnDone/OnStuck from the first row.
func TestNew_ShapeMatchesRecipe(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
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
		burler, ok := byName[row.OnStuck]
		if !ok || reflect.TypeOf(burler.Producer) != burlerType {
			t.Errorf("Bouncer %q OnStuck = %q; want a Burler row", row.Name, row.OnStuck)
			continue
		}
		if burler.OnStuck != row.Name {
			t.Errorf("Burler %q OnStuck = %q; want %q, the Bouncer naming it", burler.Name, burler.OnStuck, row.Name)
		}
		if burler.Segment != row.Segment || burler.MaxBounces != row.MaxBounces {
			t.Errorf("perch %q/%q segment/max_bounces = %q/%d vs %q/%d; want equal", row.Name, burler.Name, row.Segment, row.MaxBounces, burler.Segment, burler.MaxBounces)
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

// TestRecipe_StructuralCheckHasNoFindings parses recipes.LoomRecipe through shedbuild.Parse, builds
// it through shedbuild.Build against testEnv(t)'s Env, and asserts shedbuild.Check(recipe, built)
// returns no findings.
//
// This goes through Parse/Build rather than through New deliberately: it needs the parsed Recipe
// value, which New does not return, so it must not be "simplified" back onto New.
func TestRecipe_StructuralCheckHasNoFindings(t *testing.T) {
	env, _ := testEnv(t)

	recipe, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse() error = %v; want nil", err)
	}

	built, err := shedbuild.Build(recipe, env)
	if err != nil {
		t.Fatalf("shedbuild.Build() error = %v; want nil", err)
	}

	if findings := shedbuild.Check(recipe, built); len(findings) != 0 {
		t.Errorf("shedbuild.Check() = %v; want no findings", findings)
	}
}

// TestNew_StatusPathCoherence protects the split ShedPaths carries a duplicate of Env.StatusPath and
// Env.StatusLockPath needs: internal/loomshed.Deps carried a single field feeding both consumers, so
// the two could not disagree. Splitting it into an Env copy (read by loomPreflightEntry) and a
// ShedPaths copy (read by Shed) makes a divergent fill possible for the first time, and its
// consequence is silent -- Shed persisting to one file while Loom-Preflight reads another -- which
// is exactly why New checks it itself, ahead of shedbuild.Build.
func TestNew_StatusPathCoherence(t *testing.T) {
	t.Run("StatusPath", func(t *testing.T) {
		env, paths := testEnv(t)
		paths.StatusPath = paths.StatusPath + ".other"

		shed, err := New(env, paths)
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

		shed, err := New(env, paths)
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

// TestNew_ConstructionFailureNamesOffendingRow is the replacement for the deleted
// TestNew_NilPreflightReturnsError and covers the same class of failure at the layer that now owns
// it: an Env with an empty Cwd makes New return a non-nil error and a nil *shedengine.Shed, because
// preflightEntry's requireAbsRoot("Preflight", "Cwd", …) rejects it, wrapped by shedbuild with the
// row's zero-based index and quoted name.
func TestNew_ConstructionFailureNamesOffendingRow(t *testing.T) {
	env, paths := testEnv(t)
	env.Cwd = ""

	shed, err := New(env, paths)
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

// TestRecipe_SeedAndResumeRowNamesExist protects the two row names loom's seed and resume paths
// hard-code outside the recipe: loomshed.NamePreflight, which Seed writes as CurrentProducer into a
// fresh status file, and loomshed.NameLoomPreflight, which loomPreflightProducer.Call passes to
// loomengine.CheckSeed as the expected name alongside []string{NamePreflight, NameLoomPreflight} as
// the tolerated history set.
//
// Once the recipe is the row-name source, a recipe row renamed from Preflight would leave Seed
// writing a current_producer naming no row and CheckSeed's tolerated set no longer matching -- a
// failure that is silent at build time and surfaces as a broken resume for an in-flight task.
func TestRecipe_SeedAndResumeRowNamesExist(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	var haveNames []string
	for _, p := range shed.Producers {
		haveNames = append(haveNames, p.Name)
	}

	for _, want := range []string{loomshed.NamePreflight, loomshed.NameLoomPreflight} {
		found := false
		for _, name := range haveNames {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("recipe row names %v do not contain %q", haveNames, want)
		}
	}
}

// TestRecipeEngines_ReportsExactlyLoomsOwnEngineSet asserts RecipeEngines() reports exactly loom's
// own recipe's engine set, sorted and de-duplicated -- derived from wantProducerTable's own engine
// column rather than a second hand-written literal, for the same reason its battenrecipe twin
// gets this test: a silently empty return would disable the cross-consumer coverage guard rather
// than fail it.
func TestRecipeEngines_ReportsExactlyLoomsOwnEngineSet(t *testing.T) {
	seen := make(map[string]bool, len(wantProducerTable))
	var want []string
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

// TestRecipe_PublishRoutesThroughPRGate pins the review fix-back routing: Publish hands to the gate, the gate lands on Finalize or bounces to the rework row, the rework row re-enters Webster, both rows share segment PR-Review, and the main line still ends at Friction-Reflect.
func TestRecipe_PublishRoutesThroughPRGate(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	rows := make(map[string]int, len(shed.Producers))
	for i, p := range shed.Producers {
		rows[p.Name] = i
	}
	get := func(name string) int {
		i, ok := rows[name]
		if !ok {
			t.Fatalf("New() has no row %q", name)
		}
		return i
	}
	publish := shed.Producers[get(loomshed.NamePublish)]
	gate := shed.Producers[get(loomshed.NamePRGate)]
	rework := shed.Producers[get(loomshed.NamePRRework)]

	if publish.OnDone != loomshed.NamePRGate {
		t.Errorf("Publish.OnDone = %q; want %q", publish.OnDone, loomshed.NamePRGate)
	}
	if gate.OnDone != loomshed.NameFinalize || gate.OnStuck != loomshed.NamePRRework {
		t.Errorf("PR-Gate OnDone/OnStuck = %q/%q; want %q/%q", gate.OnDone, gate.OnStuck, loomshed.NameFinalize, loomshed.NamePRRework)
	}
	if rework.OnDone != loomshed.NamePlanBouncer || rework.OnStuck != "" {
		t.Errorf("PR-Rework OnDone/OnStuck = %q/%q; want %q/empty", rework.OnDone, rework.OnStuck, loomshed.NamePlanBouncer)
	}
	if gate.Segment != "PR-Review" || rework.Segment != "PR-Review" {
		t.Errorf("segments = %q/%q; want both PR-Review", gate.Segment, rework.Segment)
	}
	last := shed.Producers[len(shed.Producers)-1]
	if last.Name != loomshed.NameFrictionReflect || last.OnDone != "" {
		t.Errorf("main line ends at %q (OnDone %q); want %q with empty OnDone", last.Name, last.OnDone, loomshed.NameFrictionReflect)
	}
}
