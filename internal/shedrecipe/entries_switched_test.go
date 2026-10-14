// entries_switched_test.go covers the switched entries: each builds over its own Env seam pair and refuses either seam nil, naming it.

package shedrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
)

// newTestEnvWithSwitches returns newTestEnv(t) with the three switched seam pairs filled with flags off and no-op actions.
func newTestEnvWithSwitches(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	off := shedadapters.SwitchedAction{
		Enabled: func() (bool, error) { return false, nil },
		Act:     func(context.Context) error { return nil },
	}
	env.OpenWindow, env.SeedIDE, env.OpenIDE = off, off, off
	return env
}

func TestSwitchedEntries_BuildOverTheirOwnSeamsAndRefuseANilOne(t *testing.T) {
	t.Parallel()
	cases := []struct {
		engine string
		seams  func(env *Env) *shedadapters.SwitchedAction
	}{
		{"OpenWindow", func(env *Env) *shedadapters.SwitchedAction { return &env.OpenWindow }},
		{"SeedIDE", func(env *Env) *shedadapters.SwitchedAction { return &env.SeedIDE }},
		{"OpenIDE", func(env *Env) *shedadapters.SwitchedAction { return &env.OpenIDE }},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			t.Parallel()
			constructor, err := Lookup(tc.engine)
			if err != nil {
				t.Fatalf("Lookup(%q) error = %v; want nil", tc.engine, err)
			}

			env := newTestEnvWithSwitches(t)
			tc.seams(&env).Enabled = nil
			_, err = constructor("row-name", Config{}, env)
			assertErrContains(t, err, tc.engine+".Enabled")

			env = newTestEnvWithSwitches(t)
			tc.seams(&env).Act = nil
			_, err = constructor("row-name", Config{}, env)
			assertErrContains(t, err, tc.engine+".Act")
		})
	}
}
