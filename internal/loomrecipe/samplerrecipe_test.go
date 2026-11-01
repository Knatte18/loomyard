// samplerrecipe_test.go covers New over the sampler recipe: its row path, the entry gate that leaves the Code-Review perch out, the perch-gate accessor, Finalize's lack of a reroute edge and the engine-row accessor.

package loomrecipe

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSamplerRecipe asserts the sampler recipe builds under the loom contract and walks Preflight to Friction-Reflect on the Done edges.
// It also asserts the three Code-Review rows are left out unless sampler.code_review holds while Describe always runs, Finalize declares no reroute, CodePerchGate answers that gate, and the session row is found by engine.
func TestSamplerRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("on_done path", func(t *testing.T) {
		t.Parallel()
		rows, err := Rows(shedrun.RecipeSampler)
		if err != nil {
			t.Fatalf("Rows(%q) error = %v; want nil", shedrun.RecipeSampler, err)
		}
		next := map[string]string{}
		for _, row := range rows {
			next[row.Name] = row.OnDone
		}
		var got []string
		for row := rows[0].Name; row != ""; row = next[row] {
			got = append(got, row)
		}
		want := []string{
			loomshed.NamePreflight, loomshed.NameRecordWrite, loomshed.NameSamplerSession, loomshed.NameCodeBouncer,
			loomshed.NameDescribe, loomshed.NamePRReview, loomshed.NameFinalize, loomshed.NameFrictionReflect,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("sampler on_done path = %v; want %v", got, want)
		}
	})

	t.Run("code review perch gate", func(t *testing.T) {
		t.Parallel()
		perch := []string{loomshed.NameCodeBouncer, loomshed.NameCodeBurlerReview, loomshed.NameCodeBurlerFix}
		for _, enabled := range []bool{false, true} {
			env, paths := testEnv(t)
			if enabled {
				env.Setting = func(module, key string) (any, error) {
					if module == "sampler" && key == "code_review" {
						return true, nil
					}
					return envkit.TemplateSetting(module, key)
				}
			}
			setCodeBudget(&env, 0)
			shed, err := New(shedrun.RecipeSampler, env, paths)
			if err != nil {
				t.Fatalf("New(sampler) with code_review %v error = %v; want nil", enabled, err)
			}
			for _, row := range perch {
				def := producerDef(t, shed, row)
				if def.EntryGate == nil {
					t.Fatalf("%s EntryGate = nil; want a gate on sampler.code_review", row)
				}
				if holds, _, err := def.EntryGate(); err != nil || holds != enabled {
					t.Errorf("%s EntryGate() with sampler.code_review %v = %v, %v; want %v, nil", row, enabled, holds, err, enabled)
				}
			}
			if def := producerDef(t, shed, loomshed.NameDescribe); def.EntryGate != nil {
				t.Errorf("%s EntryGate is set; want nil so Describe runs with the perch off", loomshed.NameDescribe)
			}
			if def := producerDef(t, shed, loomshed.NameFinalize); def.OnReroute != "" {
				t.Errorf("%s OnReroute = %q; want none", loomshed.NameFinalize, def.OnReroute)
			}
		}
	})

	t.Run("code perch gate accessor", func(t *testing.T) {
		t.Parallel()
		env, _ := testEnv(t)
		for _, enabled := range []bool{false, true} {
			env.Setting = func(module, key string) (any, error) {
				if module == "sampler" && key == "code_review" {
					return enabled, nil
				}
				return envkit.TemplateSetting(module, key)
			}
			gate, err := CodePerchGate(shedrun.RecipeSampler, env)
			if err != nil || gate == nil {
				t.Fatalf("CodePerchGate(sampler) = %v, %v; want a gate, nil", gate == nil, err)
			}
			if holds, _, err := gate(); err != nil || holds != enabled {
				t.Errorf("CodePerchGate(sampler) with sampler.code_review %v holds = %v, %v; want %v, nil", enabled, holds, err, enabled)
			}
		}
		if gate, err := CodePerchGate(shedrun.RecipeDarn, env); err != nil || gate != nil {
			t.Errorf("CodePerchGate(darn) = gate set %v, %v; want nil, nil", gate != nil, err)
		}
		if _, err := CodePerchGate("no-such-recipe", env); err == nil {
			t.Errorf("CodePerchGate(no-such-recipe) error = nil; want a refusal")
		}
	})

	t.Run("engine row", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			recipe  string
			wantRow string
			wantOK  bool
		}{
			{shedrun.RecipeSampler, loomshed.NameSamplerSession, true},
			{shedrun.RecipeDarn, "", false},
		}
		for _, tt := range tests {
			row, ok, err := EngineRow(tt.recipe, "SamplerSession")
			if err != nil || row != tt.wantRow || ok != tt.wantOK {
				t.Errorf("EngineRow(%q, SamplerSession) = %q, %v, %v; want %q, %v, nil", tt.recipe, row, ok, err, tt.wantRow, tt.wantOK)
			}
		}
		if _, _, err := EngineRow("no-such-recipe", "SamplerSession"); err == nil {
			t.Errorf("EngineRow(no-such-recipe) error = nil; want a refusal")
		}
	})
}
