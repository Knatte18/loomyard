package shedrecipe

import (
	"path/filepath"
	"testing"
)

// gatesCfg builds a row Config carrying a one-entry "gates" list, the shape every shipped gated row has.
func gatesCfg(name string, attempts int) Config {
	return Config{"gates": []any{map[string]any{"name": name, "attempts": attempts}}}
}

// gateCapableEntries lists every gate-capable Constructor with a Config that is otherwise valid, for the leftover-key rejection.
func gateCapableEntries() map[string]Constructor {
	return map[string]Constructor{
		"DiscussionWrite": discussionWriteEntry,
		"PlanWrite":       planWriteEntry,
		"Describe":        describeEntry,
		"PRRework":        prReworkEntry,
		"Webster":         websterEntry,
	}
}

func TestResolveGateSpec_AbsentKeyIsUngated(t *testing.T) {
	spec, err := resolveGateSpec("Row", Config{}, newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 0 {
		t.Errorf("resolveGateSpec() = %+v; want the empty GateSpec", spec)
	}
}

func TestResolveGateSpec_ValidOneEntryPerVocabularyName(t *testing.T) {
	for _, name := range []string{"discussion", "plan", "rework-plan", "description"} {
		t.Run(name, func(t *testing.T) {
			env := reworkTestEnv(t)
			env.DescriptionPath = filepath.Join(t.TempDir(), "summary.md")
			spec, err := resolveGateSpec("Row", gatesCfg(name, 4), env)
			if err != nil {
				t.Fatalf("resolveGateSpec() error = %v; want nil", err)
			}
			if len(spec) != 1 {
				t.Fatalf("resolveGateSpec() = %+v; want one entry", spec)
			}
			if spec[0].Gate == nil || spec[0].Name != name || spec[0].Attempts != 4 || spec[0].PassOnCap {
				t.Errorf("resolveGateSpec()[0] = %+v; want closure set, Name %q, Attempts 4, PassOnCap false", spec[0], name)
			}
		})
	}
}

func TestResolveGateSpec_ListOrderAndPassOnCap(t *testing.T) {
	cfg := Config{"gates": []any{
		map[string]any{"name": "plan", "attempts": 2},
		map[string]any{"name": "discussion", "attempts": 1, "pass_on_cap": true},
	}}
	spec, err := resolveGateSpec("Row", cfg, newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 2 || spec[0].Name != "plan" || spec[1].Name != "discussion" {
		t.Fatalf("resolveGateSpec() = %+v; want plan then discussion", spec)
	}
	if spec[0].PassOnCap || !spec[1].PassOnCap {
		t.Errorf("PassOnCap = %v, %v; want false, true", spec[0].PassOnCap, spec[1].PassOnCap)
	}
}

func TestResolveGateSpec_ZeroAttemptsEntryStillResolves(t *testing.T) {
	spec, err := resolveGateSpec("Row", gatesCfg("plan", 0), newTestEnv(t))
	if err != nil {
		t.Fatalf("resolveGateSpec() error = %v; want nil", err)
	}
	if len(spec) != 1 || spec[0].Attempts != 0 || spec[0].Gate == nil {
		t.Errorf("resolveGateSpec() = %+v; want one off entry with its closure resolved", spec)
	}

	// An off gate still checks its Env requirements, so a typo fails loud.
	env := newTestEnv(t)
	env.AnchorPath = ""
	_, err = resolveGateSpec("Row", gatesCfg("plan", 0), env)
	assertErrContains(t, err, "AnchorPath")
}

func TestResolveGateSpec_Rejections(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"EmptyList", Config{"gates": []any{}}, "gates"},
		{"NotAList", Config{"gates": "plan"}, "gates"},
		{"ElementNotAMap", Config{"gates": []any{"plan"}}, "element 0"},
		{"DuplicateName", Config{"gates": []any{
			map[string]any{"name": "plan", "attempts": 1},
			map[string]any{"name": "plan", "attempts": 2},
		}}, "repeats gate name"},
		{"NegativeAttempts", gatesCfg("plan", -1), "negative"},
		{"MissingName", Config{"gates": []any{map[string]any{"attempts": 1}}}, "name"},
		{"MissingAttempts", Config{"gates": []any{map[string]any{"name": "plan"}}}, "attempts"},
		{"UnknownElementKey", Config{"gates": []any{map[string]any{"name": "plan", "attempts": 1, "bogus": true}}}, "bogus"},
		{"UnknownName", gatesCfg("bogus", 1), "bogus"},
		{"UnknownNameNamesElement", Config{"gates": []any{
			map[string]any{"name": "plan", "attempts": 1},
			map[string]any{"name": "bogus", "attempts": 1},
		}}, `"gates" element 1`},
		{"BadPassOnCap", Config{"gates": []any{map[string]any{"name": "plan", "attempts": 1, "pass_on_cap": "yes"}}}, "pass_on_cap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveGateSpec("TheRow", tt.cfg, newTestEnv(t))
			assertErrContains(t, err, "TheRow")
			assertErrContains(t, err, tt.want)
		})
	}
}

func TestGateCapableEntries_RejectLeftoverOldKeys(t *testing.T) {
	for name, entry := range gateCapableEntries() {
		for _, key := range []string{"gate", "gate_attempts"} {
			t.Run(name+"/"+key, func(t *testing.T) {
				_, err := entry("Row", Config{key: "plan"}, reworkTestEnv(t))
				assertErrContains(t, err, "unrecognized config key")
				assertErrContains(t, err, key)
			})
		}
	}

	for _, key := range []string{"gate", "gate_attempts"} {
		t.Run("BurlerRound/"+key, func(t *testing.T) {
			cfg := minimalBurlerConfig()
			cfg[key] = "plan"
			_, err := burlerRoundEntry("Row", cfg, newTestEnv(t))
			assertErrContains(t, err, "unrecognized config key")
			assertErrContains(t, err, key)
		})
	}
}
