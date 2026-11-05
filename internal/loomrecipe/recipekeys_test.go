// recipekeys_test.go covers the recipe-keyed accessors: contract, engines, record engine, verify source and retired rows.

package loomrecipe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGateFixerSetting covers the setting each writer row's gate element names for its cold fixer, an element that names none, and the refusals of a recipe or engine that does not exist.
func TestGateFixerSetting(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		recipe  string
		engine  string
		gate    string
		want    string
		wantErr bool
	}{
		{name: "tapestry plan", recipe: shedrun.RecipeTapestry, engine: "PlanWrite", gate: "plan", want: "tapestry.plan_gate_fixer"},
		{name: "tapestry discussion", recipe: shedrun.RecipeTapestry, engine: "DiscussionWrite", gate: "discussion", want: "loom.discussion_gate_fixer"},
		{name: "darn verify", recipe: shedrun.RecipeDarn, engine: "DarnWrite", gate: "verify", want: "darn.gate_fixer"},
		{name: "an element without a fixer", recipe: shedrun.RecipeDarn, engine: "DarnWrite", gate: "clean", want: ""},
		{name: "an empty recipe name reads as tapestry", recipe: "", engine: "PlanWrite", gate: "plan", want: "tapestry.plan_gate_fixer"},
		{name: "an unknown recipe", recipe: "no-such-recipe", engine: "PlanWrite", gate: "plan", wantErr: true},
		{name: "a recipe with no row of the engine", recipe: shedrun.RecipeDarn, engine: "PlanWrite", gate: "plan", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := GateFixerSetting(tt.recipe, tt.engine, tt.gate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GateFixerSetting(%q, %q, %q) error = %v; want error %v", tt.recipe, tt.engine, tt.gate, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("GateFixerSetting(%q, %q, %q) = %q; want %q", tt.recipe, tt.engine, tt.gate, got, tt.want)
			}
		})
	}
}

func TestRecipeKeys(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name          string
		recipe        string
		wantLoom      bool
		wantRecord    string
		wantVerify    string
		wantEngines   map[string]bool
		wantEngineRow map[string]string
		wantRetired   map[string]string
		wantRefusals  bool
	}{
		{
			name:        "MotifRunsAlone",
			recipe:      shedrun.RecipeMotif,
			wantLoom:    true,
			wantRecord:  "DiscussionWrite",
			wantEngines: map[string]bool{"DiscussionWrite": true, "Block": false, "Implementer": false, "Finalize": false},
		},
		{
			name:       "Tapestry",
			recipe:     shedrun.RecipeTapestry,
			wantLoom:   true,
			wantRecord: "DiscussionWrite",
			wantVerify: "plan",
			wantEngines: map[string]bool{
				"Block":       true,
				"Implementer": true,
				"PlanWrite":   true,
				"RecordWrite": false,
			},
			wantEngineRow: map[string]string{"DiscussionWrite": "Discussion-Write", "Block": "Motif"},
			wantRetired: map[string]string{
				"Merge-In":           "Parent-Align",
				"Loom-Preflight":     "Preflight",
				"Tapestry-Preflight": "Preflight",
			},
		},
		{
			name:       "EmptyReadsAsTapestry",
			recipe:     "",
			wantLoom:   true,
			wantRecord: "DiscussionWrite",
			wantVerify: "plan",
			wantEngines: map[string]bool{
				"Implementer": true,
			},
			wantRetired: map[string]string{
				"Merge-In":           "Parent-Align",
				"Loom-Preflight":     "Preflight",
				"Tapestry-Preflight": "Preflight",
			},
		},
		{
			name:        "Darn",
			recipe:      shedrun.RecipeDarn,
			wantLoom:    true,
			wantRecord:  "RecordWrite",
			wantVerify:  "tree_scope",
			wantEngines: map[string]bool{"DarnWrite": true, "RecordWrite": true, "PlanWrite": false},
			wantRetired: map[string]string{"Darn": "Darn-Write"},
		},
		{
			name:        "Pawl",
			recipe:      shedrun.RecipePawl,
			wantLoom:    true,
			wantRecord:  "RecordWrite",
			wantVerify:  "plan",
			wantEngines: map[string]bool{"BeamAwait": true, "RecordWrite": true, "Bouncer": false, "Implementer": false},
		},
		{
			name:        "Sampler",
			recipe:      shedrun.RecipeSampler,
			wantLoom:    true,
			wantRecord:  "RecordWrite",
			wantVerify:  "tree_scope",
			wantEngines: map[string]bool{"SamplerSession": true, "RecordWrite": true, "DarnWrite": false},
		},
		{
			name:        "BattenIsNeither",
			recipe:      shedrun.RecipeBatten,
			wantEngines: map[string]bool{"Implementer": false},
		},
		{name: "Beam", recipe: "beam", wantRefusals: true},
		{name: "Unknown", recipe: "nope", wantRefusals: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.wantRefusals {
				assertRefusedByEveryAccessor(t, tt.recipe)
				return
			}

			gotLoom, err := IsLoom(tt.recipe)
			if err != nil || gotLoom != tt.wantLoom {
				t.Errorf("IsLoom(%q) = %v, %v; want %v, nil", tt.recipe, gotLoom, err, tt.wantLoom)
			}
			gotRecord, err := RecordEngine(tt.recipe)
			if err != nil || gotRecord != tt.wantRecord {
				t.Errorf("RecordEngine(%q) = %q, %v; want %q, nil", tt.recipe, gotRecord, err, tt.wantRecord)
			}
			gotVerify, err := VerifySource(tt.recipe)
			if err != nil || gotVerify != tt.wantVerify {
				t.Errorf("VerifySource(%q) = %q, %v; want %q, nil", tt.recipe, gotVerify, err, tt.wantVerify)
			}
			for engine, want := range tt.wantEngines {
				got, err := HasEngine(tt.recipe, engine)
				if err != nil || got != want {
					t.Errorf("HasEngine(%q, %q) = %v, %v; want %v, nil", tt.recipe, engine, got, err, want)
				}
			}
			for engine, want := range tt.wantEngineRow {
				got, found, err := EngineRow(tt.recipe, engine)
				if err != nil || !found || got != want {
					t.Errorf("EngineRow(%q, %q) = %q, %v, %v; want %q, true, nil", tt.recipe, engine, got, found, err, want)
				}
			}
			if got := RetiredRows(tt.recipe); !reflect.DeepEqual(got, tt.wantRetired) {
				t.Errorf("RetiredRows(%q) = %v; want %v", tt.recipe, got, tt.wantRetired)
			}
		})
	}
}

// assertRefusedByEveryAccessor fails unless every error-returning accessor refuses the recipe naming it.
func assertRefusedByEveryAccessor(t *testing.T, recipe string) {
	t.Helper()
	_, errLoom := IsLoom(recipe)
	_, errHas := HasEngine(recipe, "Implementer")
	_, errRecord := RecordEngine(recipe)
	_, errVerify := VerifySource(recipe)
	for accessor, err := range map[string]error{
		"IsLoom": errLoom, "HasEngine": errHas, "RecordEngine": errRecord, "VerifySource": errVerify,
	} {
		if err == nil || !strings.Contains(err.Error(), recipe) {
			t.Errorf("%s(%q) error = %v; want a refusal naming the recipe", accessor, recipe, err)
		}
	}
	if got := RetiredRows(recipe); got != nil {
		t.Errorf("RetiredRows(%q) = %v; want nil", recipe, got)
	}
}
