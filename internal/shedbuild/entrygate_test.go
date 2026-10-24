//go:build tier0

// entrygate_test.go covers the entry gate Build makes from a parsed recipe's entry_gate key, evaluated against a fake setting lookup and no worktree.

package shedbuild

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

func TestBuild_EntryGate(t *testing.T) {
	t.Parallel()

	const setting = "loom.plan_auto_accept"
	tests := []struct {
		name string
		// gate is the YAML value of the row's entry_gate key; empty leaves the key out.
		gate string
		// value is what the fake lookup answers for setting, an error value failing the read.
		value any
		// budget is the soft cap of the Code-Review segment.
		budget      int
		wantNilGate bool
		wantHolds   bool
		// wantReason is a substring of the reason a false gate returns.
		wantReason string
		// wantErrSubs are substrings of the evaluation error, which makes the gate undecided.
		wantErrSubs []string
		// rereadAs, when non-nil, replaces the setting's value after the first evaluation, and wantHoldsAfter is the second evaluation's answer.
		rereadAs       any
		wantHoldsAfter bool
	}{
		{name: "a row with no entry_gate carries no gate", wantNilGate: true},
		{name: "a bare setting holding true holds", gate: setting, value: true, wantHolds: true},
		{name: "a bare setting holding false does not hold and the reason shows the value read", gate: setting, value: false, wantReason: setting + " = false"},
		{name: "equality against a number holds on an equal setting", gate: `"` + setting + ` == 3"`, value: 3, wantHolds: true},
		{name: "inequality against a number does not hold on an equal setting", gate: `"` + setting + ` != 3"`, value: 3, wantReason: setting + " != 3 (" + setting + " = 3)"},
		{name: "equality against a bare word compares strings", gate: `"` + setting + ` == fast"`, value: "fast", wantHolds: true},
		{name: "a list holds when every check does", gate: `[` + setting + `, budget.Code-Review]`, value: true, budget: 2, wantHolds: true},
		{name: "a list stops at the first check that does not hold", gate: `[` + setting + `, budget.Code-Review]`, value: true, budget: 0, wantReason: "budget.Code-Review = 0"},
		{name: "a segment budget above zero holds", gate: "budget.Code-Review", budget: 1, wantHolds: true},
		{name: "a segment budget of zero does not hold", gate: "budget.Code-Review", budget: 0, wantReason: "budget.Code-Review = 0"},
		{name: "a segment budget compares against a literal", gate: `"budget.Code-Review == 0"`, budget: 0, wantHolds: true},
		{
			name:        "a bare setting holding a string is an evaluation error naming the config command and the resume command",
			gate:        setting,
			value:       "no",
			wantErrSubs: []string{`row "Gated"`, setting, "holds no, not a bool", "lyx config loom --resolved", "lyx config loom --layer local --set plan_auto_accept=<bool>", "lyx loom resume"},
		},
		{
			name:        "a bare setting holding a number is an evaluation error",
			gate:        setting,
			value:       1,
			wantErrSubs: []string{setting, "holds 1, not a bool", "lyx loom resume"},
		},
		{
			name:        "a failed read is an evaluation error naming the row and the check",
			gate:        setting,
			value:       errors.New("layer unreadable"),
			wantErrSubs: []string{`row "Gated"`, `entry_gate check "` + setting + `"`, "layer unreadable"},
		},
		{name: "the gate reads the setting at each evaluation", gate: setting, value: true, wantHolds: true, rereadAs: false, wantHoldsAfter: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			current := tt.value
			env := shedrecipe.Env{
				Setting: func(module, key string) (any, error) {
					if err, failed := current.(error); failed {
						return nil, err
					}
					return current, nil
				},
				SettingKnown: configreg.CheckSettingKey,
				Segments:     map[string]shedrecipe.SegmentSettings{"Code-Review": {MaxBounces: tt.budget}},
				Resume:       "lyx loom resume",
			}
			gateLine := ""
			if tt.gate != "" {
				gateLine = "    entry_gate: " + tt.gate + "\n"
			}
			recipe, err := Parse([]byte(fmt.Sprintf("version: 1\nentry: Gated\nterminals: [Gated]\nproducers:\n  - name: Gated\n    engine: Stub\n%s", gateLine)))
			if err != nil {
				t.Fatalf("Parse() error = %v; want nil", err)
			}

			defs, err := Build(recipe, env)
			if err != nil {
				t.Fatalf("Build() error = %v; want nil", err)
			}
			gate := defs[0].EntryGate
			if tt.wantNilGate {
				if gate != nil {
					t.Fatal("EntryGate is set; want nil for a row with no entry_gate")
				}
				return
			}
			if gate == nil {
				t.Fatal("EntryGate = nil; want a gate")
			}

			holds, reason, err := gate()
			if len(tt.wantErrSubs) > 0 {
				if err == nil {
					t.Fatalf("gate() = (%v, %q, nil); want an evaluation error", holds, reason)
				}
				for _, want := range tt.wantErrSubs {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("gate() error = %v; want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("gate() error = %v; want nil", err)
			}
			if holds != tt.wantHolds {
				t.Errorf("gate() holds = %v; want %v (reason %q)", holds, tt.wantHolds, reason)
			}
			if !holds && !strings.Contains(reason, tt.wantReason) {
				t.Errorf("gate() reason = %q; want it to contain %q", reason, tt.wantReason)
			}

			if tt.rereadAs != nil {
				current = tt.rereadAs
				if again, _, err := gate(); err != nil || again != tt.wantHoldsAfter {
					t.Errorf("second gate() = (%v, %v); want (%v, nil) after the setting changed", again, err, tt.wantHoldsAfter)
				}
			}
		})
	}
}
