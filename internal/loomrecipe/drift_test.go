// drift_test.go covers the entry gate of the Drift rows in loom's and darn's recipes.

package loomrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDriftRows_EntryGate asserts every Drift row of both recipes is left out under the template's drift.enabled and entered when the setting answers true, and that a row the run already entered is called when the setting is false.
func TestDriftRows_EntryGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	recipes := []struct {
		name   string
		recipe string
		rows   []string
	}{
		{"loom", shedrun.RecipeTapestry, []string{loomshed.NameDriftBaseline, loomshed.NameDriftAwait, loomshed.NameDriftExpect, loomshed.NameDriftVerify, loomshed.NameDriftRecheck}},
		{"darn", shedrun.RecipeDarn, []string{loomshed.NameDriftBaseline, loomshed.NameDriftAwait, loomshed.NameDriftExpect, loomshed.NameDriftVerify, loomshed.NameDriftRecheck}},
	}
	for _, recipe := range recipes {
		t.Run(recipe.name, func(t *testing.T) {
			t.Parallel()
			for _, enabled := range []bool{false, true} {
				env, paths := testEnv(t)
				if enabled {
					env.Setting = func(module, key string) (any, error) {
						if module == "drift" && key == "enabled" {
							return true, nil
						}
						return envkit.TemplateSetting(module, key)
					}
				}
				shed, err := New(recipe.recipe, env, paths)
				if err != nil {
					t.Fatalf("%s build error = %v; want nil", recipe.name, err)
				}
				for _, row := range recipe.rows {
					def := producerDef(t, shed, row)
					if def.EntryGate == nil {
						t.Fatalf("%s EntryGate = nil; want a gate on drift.enabled", row)
					}
					if holds, _, err := def.EntryGate(); err != nil || holds != enabled {
						t.Errorf("%s EntryGate() with drift.enabled %v = %v, %v; want %v, nil", row, enabled, holds, err, enabled)
					}
				}
			}
		})
	}

	// A flag turned off at a row the run already entered, waiting or halted, does not skip the row.
	for _, row := range []string{loomshed.NameDriftAwait, loomshed.NameDriftVerify} {
		t.Run("entered "+row, func(t *testing.T) {
			t.Parallel()
			env, paths := testEnv(t)
			shed, err := New(shedrun.RecipeTapestry, env, paths)
			if err != nil {
				t.Fatalf("New() error = %v; want nil", err)
			}
			seed := shedengine.Status{CurrentProducer: row, EnteredRow: row, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{}}
			if err := state.WriteJSON(paths.StatusPath, paths.StatusLockPath, seed); err != nil {
				t.Fatalf("WriteJSON() error = %v; want nil", err)
			}

			result, err := shed.Step(context.Background())

			if err != nil {
				t.Fatalf("Step() error = %v; want nil", err)
			}
			if result.Producer != row || result.Outcome != shedengine.Done {
				t.Errorf("Step() called %q with outcome %q; want %q called with outcome %q rather than skipped", result.Producer, result.Outcome, row, shedengine.Done)
			}
		})
	}
}

// producerDef returns the row of shed named name, failing the test when there is none.
func producerDef(t *testing.T, shed *shedengine.Shed, name string) shedengine.ProducerDef {
	t.Helper()
	for _, def := range shed.Producers {
		if def.Name == name {
			return def
		}
	}
	t.Fatalf("shed has no row %q", name)
	return shedengine.ProducerDef{}
}
