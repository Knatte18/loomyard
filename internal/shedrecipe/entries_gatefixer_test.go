// entries_gatefixer_test.go covers the "fixer" gate element key: splitGateFixer's reading and refusals, and applyGateFixer's call of the Env seam.

package shedrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordGateFixer sets env.GateFixer to a seam that records every request and returns its spec unchanged, and returns the recorded requests.
func recordGateFixer(env *Env) *[]GateFixerRequest {
	var calls []GateFixerRequest
	env.GateFixer = func(req GateFixerRequest) (shuttleengine.GateSpec, error) {
		calls = append(calls, req)
		return req.Spec, nil
	}
	return &calls
}

func TestGateFixerElement(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	withFixer := func(value any) Config {
		return Config{"gates": []any{
			map[string]any{"name": "clean", "attempts": 1},
			map[string]any{"name": "verify", "attempts": 2, "fixer": value},
		}}
	}
	tests := []struct {
		name       string
		cfg        Config
		settings   map[string]any
		nilSetting bool
		nilSeam    bool
		wantCalls  int
		wantErr    []string
	}{
		{name: "NoFixerKey", cfg: gatesCfg("verify", 2)},
		{name: "EmptySettingValue", cfg: withFixer("darn.gate_fixer"), settings: map[string]any{"darn.gate_fixer": ""}},
		{name: "NilSeam", cfg: withFixer("darn.gate_fixer"), settings: map[string]any{"darn.gate_fixer": "opus"}, nilSeam: true},
		{name: "ModelSettingCallsTheSeam", cfg: withFixer("darn.gate_fixer"), settings: map[string]any{"darn.gate_fixer": "opus"}, wantCalls: 1},
		{name: "NilSetting", cfg: withFixer("darn.gate_fixer"), nilSetting: true, wantErr: []string{"Setting"}},
		{name: "MissingSetting", cfg: withFixer("darn.gate_fixer"), wantErr: []string{"element 1", "darn.gate_fixer", "way forward:"}},
		{name: "NonStringSetting", cfg: withFixer("darn.gate_fixer"), settings: map[string]any{"darn.gate_fixer": 3}, wantErr: []string{"element 1", "darn.gate_fixer", "way forward:"}},
		{name: "UnparsableValue", cfg: withFixer("darn.gate_fixer"), settings: map[string]any{"darn.gate_fixer": "not a spec"}, wantErr: []string{"element 1", "not a spec", "way forward:"}},
		{name: "MalformedReference", cfg: withFixer("nodot"), wantErr: []string{"element 1", "nodot", "way forward:"}},
		{name: "ReferenceNotAString", cfg: withFixer(3), wantErr: []string{"element 1", "way forward:"}},
		{name: "FixerOnTwoElements", cfg: Config{"gates": []any{
			map[string]any{"name": "clean", "attempts": 1, "fixer": "darn.gate_fixer"},
			map[string]any{"name": "verify", "attempts": 2, "fixer": "darn.gate_fixer"},
		}}, settings: map[string]any{"darn.gate_fixer": ""}, wantErr: []string{"element 1", "way forward:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			if !tt.nilSetting {
				env.Setting = fakeSetting(tt.settings)
			} else {
				env.Setting = nil
			}
			calls := recordGateFixer(&env)
			if tt.nilSeam {
				env.GateFixer = nil
			}

			stripped, req, err := splitGateFixer("TheRow", tt.cfg, env)
			if len(tt.wantErr) > 0 {
				assertErrContains(t, err, "TheRow")
				for _, want := range tt.wantErr {
					assertErrContains(t, err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitGateFixer() error = %v; want nil", err)
			}
			elems, _, _ := configMapList(stripped, "gates")
			for i, elem := range elems {
				if _, has := elem[gateFixerKey]; has {
					t.Errorf("stripped element %d still carries %q", i, gateFixerKey)
				}
			}
			spec := shuttleengine.GateSpec{{Name: "verify", Attempts: 2}}
			got, err := applyGateFixer("Row", spec, req, env)
			if err != nil {
				t.Fatalf("applyGateFixer() error = %v; want nil", err)
			}
			if len(got) != len(spec) {
				t.Errorf("applyGateFixer() = %+v; want the spec unchanged", got)
			}
			if len(*calls) != tt.wantCalls {
				t.Errorf("GateFixer called %d times; want %d", len(*calls), tt.wantCalls)
			}
		})
	}
}
