// recipe_test.go carries the tests whose subject is the recipe itself rather than New's built
// list: the shape assertion against the recipe-built list, the structural check over the parsed
// recipe, the two split-argument coherence failures New itself performs, the construction-failure
// surface a bad Env field produces, and the seed/resume row-name pin.

package loomrecipe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestNew_ShapeMatchesRecipe is internal/shedbuild/equivalence_test.go's assertion loop with its
// loomshed.New side replaced by wantProducerTable, the package's single authoritative row table
// (declared in shape_test.go, extended there with a reflect.Type column for exactly this test): it
// builds the embedded recipe through New from testEnv(t), asserts exactly seventeen rows, and for
// each row asserts Name, OnDone, OnStuck, Segment, MaxBounces, and the expected concrete Producer
// type.
func TestNew_ShapeMatchesRecipe(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	if len(shed.Producers) != 17 {
		t.Fatalf("New() produced %d rows; want 17", len(shed.Producers))
	}
	if len(wantProducerTable) != 17 {
		t.Fatalf("wantProducerTable has %d rows; want 17", len(wantProducerTable))
	}

	for i, want := range wantProducerTable {
		got := shed.Producers[i]
		if got.Name != want.name {
			t.Errorf("row %d Name = %q; want %q", i, got.Name, want.name)
		}
		if got.OnDone != want.onDone {
			t.Errorf("row %d (%s) OnDone = %q; want %q", i, got.Name, got.OnDone, want.onDone)
		}
		if got.OnStuck != want.onStuck {
			t.Errorf("row %d (%s) OnStuck = %q; want %q", i, got.Name, got.OnStuck, want.onStuck)
		}
		if got.Segment != want.segment {
			t.Errorf("row %d (%s) Segment = %q; want %q", i, got.Name, got.Segment, want.segment)
		}
		if got.MaxBounces != want.maxBounces {
			t.Errorf("row %d (%s) MaxBounces = %d; want %d", i, got.Name, got.MaxBounces, want.maxBounces)
		}
		if gotType := reflect.TypeOf(got.Producer); gotType != want.producerType {
			t.Errorf("row %d (%s) Producer concrete type = %v; want %v", i, got.Name, gotType, want.producerType)
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
