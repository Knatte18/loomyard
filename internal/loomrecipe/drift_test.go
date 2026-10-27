//go:build tier0

// drift_test.go covers the entry gate of the Drift rows in loom's and darn's recipes.

package loomrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// TestDriftRows_EntryGate asserts every Drift row of both recipes is left out under the template's drift.enabled and entered when the setting answers true, and that a row the run already entered is called when the setting is false.
func TestDriftRows_EntryGate(t *testing.T) {
	t.Parallel()

	recipes := []struct {
		name  string
		build func(shedrecipe.Env, shedbuild.ShedPaths) (*shedengine.Shed, error)
		rows  []string
	}{
		{"loom", New, []string{loomshed.NameDriftBaseline, loomshed.NameDriftAwait, loomshed.NameDriftVerify}},
		{"darn", NewDarn, []string{loomshed.NameDriftBaseline, loomshed.NameDriftAwait, loomshed.NameDriftVerify}},
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
				shed, err := recipe.build(env, paths)
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
			shed, err := New(env, paths)
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
