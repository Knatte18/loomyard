// parity_test.go covers the comparison, its shared seams and the preparation of each engine's launch over fake engines.

package shuttleparity

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// manifestEngine is an engine whose Prepare writes a given manifest, fails or writes none, and records each spec.
type manifestEngine struct {
	shuttleengine.Engine
	manifest *shuttleengine.Manifest
	err      error
	// forks makes the engine meet the fork requirement.
	forks    bool
	prepared []shuttleengine.Spec
}

// Prepare records spec and writes the engine's manifest into runDir, unless it is scripted to fail or write none.
func (e *manifestEngine) Prepare(runDir string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
	e.prepared = append(e.prepared, spec)
	if e.err != nil {
		return shuttleengine.Launch{}, e.err
	}
	if e.manifest == nil {
		return shuttleengine.Launch{}, nil
	}
	return shuttleengine.Launch{}, shuttleengine.WriteManifest(runDir, *e.manifest)
}

// Supports reports whether the engine meets requirement.
func (e *manifestEngine) Supports(requirement shuttleengine.Requirement) bool {
	return e.forks && requirement == shuttleengine.RequirementForks
}

// fakeSet returns a set holding the two fakes under the compared engines' names.
func fakeSet(reference, compared *manifestEngine) shuttleengine.EngineSet {
	return shuttleengine.NewEngineSet(ReferenceEngine, map[string]shuttleengine.Engine{ReferenceEngine: reference, ComparedEngine: compared})
}

// fixtureTable returns a one-seat table over a seeded stencil, whose seat pins a model of the reference engine.
func fixtureTable() seatengine.Table {
	return seatengine.Table{
		RolePrefix: "fixture",
		Segment:    "plan",
		Seats: []seatengine.Seat{{
			Stencil:     "drift-template-expect",
			ModelChoice: shuttleengine.ModelChoice{Engine: ReferenceEngine, Model: "opus", Effort: "high", Version: "5"},
			Outputs:     []string{"/out.md"},
		}},
		Values: map[string]string{
			"subject_reading": "s", "plan_reading": "p", "baseline_dir": "/b", "listing_path": "/l",
			"expected_path": "/e", "diff_mode": "lines", "expectation_format": "f",
		},
	}
}

// TestCompare pins which of the reference manifest's items the compared one must hold.
// It also pins the prompt items, the unmet requirements and the unrun-hook error.
// It also pins the preparation of each engine and the seat listing.
func TestCompare(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	item := func(channel shuttleengine.ManifestChannel, key, engine string, delivery shuttleengine.ManifestDelivery) shuttleengine.ManifestItem {
		return shuttleengine.ManifestItem{Channel: channel, Key: key, Engine: engine, Delivery: delivery}
	}
	passage := item(shuttleengine.ManifestChannelStandingPassage, "agents-prompt", "", shuttleengine.ManifestDelivered)
	filed := item(shuttleengine.ManifestChannelInstructionSource, "heddle-file/agents-file", "", shuttleengine.ManifestNative)
	claudeFiled := item(shuttleengine.ManifestChannelInstructionSource, "heddle-file/claude-file", ReferenceEngine, shuttleengine.ManifestNative)
	context := item(shuttleengine.ManifestChannelPluginContext, "p@m/emit", "", shuttleengine.ManifestNative)
	omitted := item(shuttleengine.ManifestChannelPluginContext, "p@m/emit", "", shuttleengine.ManifestOmitted)
	extra := item(shuttleengine.ManifestChannelNotice, "extra", "", shuttleengine.ManifestDelivered)
	forkDeny := item(shuttleengine.ManifestChannelGuard, shuttleengine.WebsterForkDenyKey, "", shuttleengine.ManifestDelivered)

	t.Run("Compare", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			reference []shuttleengine.ManifestItem
			compared  []shuttleengine.ManifestItem
			forks     bool
			want      []shuttleengine.ManifestItem
		}{
			{name: "shared heddle targets absent", reference: []shuttleengine.ManifestItem{passage, filed}, want: []shuttleengine.ManifestItem{passage, filed}},
			{name: "an item keyed to the reference engine", reference: []shuttleengine.ManifestItem{claudeFiled}},
			{name: "an omitted context counts present", reference: []shuttleengine.ManifestItem{context}, compared: []shuttleengine.ManifestItem{omitted}},
			{name: "an extra item", compared: []shuttleengine.ManifestItem{extra}},
			{name: "the fork deny on an engine without forks", reference: []shuttleengine.ManifestItem{forkDeny}},
			{name: "the fork deny on an engine with forks", reference: []shuttleengine.ManifestItem{forkDeny}, forks: true, want: []shuttleengine.ManifestItem{forkDeny}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				set := fakeSet(&manifestEngine{}, &manifestEngine{forks: tt.forks})
				got := Compare(set, map[string]shuttleengine.Manifest{
					ReferenceEngine: {Items: tt.reference},
					ComparedEngine:  {Items: tt.compared},
				})
				if len(got) != len(tt.want) || len(got) > 0 && !reflect.DeepEqual(got, tt.want) {
					t.Errorf("Compare() = %+v; want %+v", got, tt.want)
				}
			})
		}
	})

	t.Run("PromptItems", func(t *testing.T) {
		t.Parallel()
		stencils := stencilkit.Seed(t)
		directive, err := editdirective.Directive(stencils, ComparedEngine)
		if err != nil {
			t.Fatal(err)
		}
		got, err := PromptItems(stencils, ComparedEngine, "before\n"+directive+"\nafter")
		want := []shuttleengine.ManifestItem{item(shuttleengine.ManifestChannelEditDirective, "edit-directive", "", shuttleengine.ManifestDelivered)}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("PromptItems(prompt with the directive) = %+v, %v; want %+v", got, err, want)
		}
		got, err = PromptItems(stencils, ComparedEngine, "a prompt without it")
		if err != nil || got != nil {
			t.Errorf("PromptItems(prompt without the directive) = %+v, %v; want none", got, err)
		}
	})

	t.Run("Unmet", func(t *testing.T) {
		t.Parallel()
		want := []shuttleengine.Requirement{shuttleengine.RequirementForks}
		if got := Unmet(&manifestEngine{}, shuttleengine.Spec{ForkSubagents: true}); !reflect.DeepEqual(got, want) {
			t.Errorf("Unmet(a forking spec) = %v; want %v", got, want)
		}
		if got := Unmet(&manifestEngine{}, shuttleengine.Spec{}); got != nil {
			t.Errorf("Unmet(a plain spec) = %v; want none", got)
		}
	})

	t.Run("UnrunHooks", func(t *testing.T) {
		t.Parallel()
		if _, err := UnrunHooks(shuttleengine.HookRun{}); err != shuttleengine.ErrHookUnrun {
			t.Errorf("UnrunHooks() error = %v; want shuttleengine.ErrHookUnrun itself", err)
		}
	})

	t.Run("PrepareEach", func(t *testing.T) {
		t.Parallel()
		stencils := stencilkit.Seed(t)
		cfg := shuttleengine.Config{Agents: shuttlefake.Agents(t)}
		roleSkills, err := cfg.Agents.Skills("darn", []string{"own:skill"})
		if err != nil {
			t.Fatal(err)
		}
		written := &shuttleengine.Manifest{}
		tests := []struct {
			name     string
			compared manifestEngine
			spec     shuttleengine.Spec
			wantErr  []string
		}{
			{name: "a failing Prepare", compared: manifestEngine{err: errors.New("boom")}, wantErr: []string{ComparedEngine, "boom"}},
			{name: "no record", wantErr: []string{ComparedEngine, "shuttleengine.WriteManifest"}},
			{name: "a config role's skills", compared: manifestEngine{manifest: written}, spec: shuttleengine.Spec{ConfigRole: "darn", Skills: []string{"own:skill"}}},
			{name: "a role with no skill list", compared: manifestEngine{manifest: written}, spec: shuttleengine.Spec{ConfigRole: "nobody"}, wantErr: []string{"nobody"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				reference := &manifestEngine{manifest: written}
				compared := tt.compared
				specs := map[string]shuttleengine.Spec{ReferenceEngine: tt.spec, ComparedEngine: tt.spec}

				_, err := PrepareEach(fakeSet(reference, &compared), cfg, stencils, t.TempDir(), specs)

				if tt.wantErr != nil {
					for _, want := range tt.wantErr {
						if err == nil || !strings.Contains(err.Error(), want) {
							t.Errorf("PrepareEach() error = %v; want it to contain %q", err, want)
						}
					}
					return
				}
				if err != nil {
					t.Fatalf("PrepareEach() error = %v", err)
				}
				for name, engine := range map[string]*manifestEngine{ReferenceEngine: reference, ComparedEngine: &compared} {
					if len(engine.prepared) != 1 || !reflect.DeepEqual(engine.prepared[0].Skills, roleSkills) {
						t.Errorf("engine %q was prepared with %+v; want the skills %v", name, engine.prepared, roleSkills)
					}
				}
			})
		}
	})

	t.Run("seatsOf", func(t *testing.T) {
		t.Parallel()
		broken := errors.New("no stand-in")
		stencils := stencilkit.Seed(t)
		env := seatsEnv(t, stencils)
		table := fixtureTable()
		seats, failures := seatsOf(env, []seatTable{
			{Builder: "first#Table", Build: func(SeatEnv) ([]seatengine.Table, error) { return []seatengine.Table{table}, nil }},
			{Builder: "second#Table", Build: func(SeatEnv) ([]seatengine.Table, error) { return nil, broken }},
		})

		if len(failures) != 1 || !errors.Is(failures[0], broken) || !strings.Contains(failures[0].Error(), "second#Table") {
			t.Errorf("seatsOf() failures = %v; want one naming second#Table and wrapping its error", failures)
		}
		if len(seats) != 1 || seats[0].Name != "fixture" || seats[0].Builder != "first#Table" {
			t.Fatalf("seatsOf() seats = %+v; want the first entry's one seat", seats)
		}
		for _, engine := range []string{ReferenceEngine, ComparedEngine} {
			spec, err := seats[0].Specs(engine)
			if err != nil {
				t.Fatalf("Specs(%q) error = %v", engine, err)
			}
			if want := (shuttleengine.ModelChoice{Engine: engine}); spec.ModelChoice != want {
				t.Errorf("Specs(%q) model choice = %+v; want %+v", engine, spec.ModelChoice, want)
			}
		}
	})
}
