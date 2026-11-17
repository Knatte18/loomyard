// compare_test.go covers the comparison of the engines' manifests, the prompt-side items and the unmet requirements.
// It also covers the manifest-only seams, the preparation of each engine and the seat catalog's builder.
// It runs over fake engines and fixture tables.

package shuttleparity

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// manifestEngine is a fake engine whose Prepare runs a given function over the run directory and the spec.
// It meets the fork requirement when forks is set, and no other.
type manifestEngine struct {
	*shuttlefake.Engine
	forks bool
}

// Supports reports whether the engine meets req.
func (e *manifestEngine) Supports(req shuttleengine.Requirement) bool {
	return e.forks && req == shuttleengine.RequirementForks
}

// newManifestEngine returns a fake engine whose Prepare returns prepare's error after running it.
func newManifestEngine(forks bool, prepare func(runDir string, spec shuttleengine.Spec) error) *manifestEngine {
	fake := &shuttlefake.Engine{
		PrepareFn: func(runDir string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
			return shuttleengine.Launch{}, prepare(runDir, spec)
		},
	}
	return &manifestEngine{Engine: fake, forks: forks}
}

// writesManifest returns a Prepare function that writes m into the run directory.
func writesManifest(m shuttleengine.Manifest) func(string, shuttleengine.Spec) error {
	return func(runDir string, _ shuttleengine.Spec) error { return shuttleengine.WriteManifest(runDir, m) }
}

// engineSetOf returns a set of the two compared engines.
func engineSetOf(reference, compared shuttleengine.Engine) shuttleengine.EngineSet {
	return shuttleengine.NewEngineSet(ReferenceEngine, map[string]shuttleengine.Engine{ReferenceEngine: reference, ComparedEngine: compared})
}

// manifestOf returns a manifest holding items.
func manifestOf(items ...shuttleengine.ManifestItem) shuttleengine.Manifest {
	return shuttleengine.Manifest{Items: items}
}

// item returns a delivered item of channel and key, keyed to engine when engine is not empty.
func item(channel shuttleengine.ManifestChannel, key, engine string) shuttleengine.ManifestItem {
	return shuttleengine.ManifestItem{Channel: channel, Key: key, Engine: engine, Delivery: shuttleengine.DeliveryDelivered}
}

// TestCompare covers Compare, PromptItems, Unmet, UnrunHooks, PrepareEach and seatsOf.
func TestCompare(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("Compare", func(t *testing.T) {
		t.Parallel()
		passage := item(shuttleengine.ChannelStandingPassage, "agents-prompt", "")
		fileEntry := item(shuttleengine.ChannelInstructionSource, "heddle-file/agents-file", "")
		claudeOnly := item(shuttleengine.ChannelInstructionSource, "heddle-file/claude-file", ReferenceEngine)
		forkDeny := item(shuttleengine.ChannelGuard, shuttleengine.WebsterForkDenyKey, "")
		unrun := shuttleengine.ManifestItem{Channel: shuttleengine.ChannelPluginContext, Key: "p@m/emit", Delivery: shuttleengine.DeliveryOmitted, Reason: "unrun"}
		held := item(shuttleengine.ChannelPluginContext, "p@m/emit", "")
		extra := item(shuttleengine.ChannelNotice, "extra", "")
		tests := []struct {
			name      string
			claude    shuttleengine.Manifest
			codex     shuttleengine.Manifest
			codexFork bool
			want      []shuttleengine.ManifestItem
		}{
			{name: "SharedHeddleItemsAbsentFromCodexAreMissing", claude: manifestOf(passage, fileEntry), codex: manifestOf(), want: []shuttleengine.ManifestItem{passage, fileEntry}},
			{name: "ItemKeyedToClaudeIsNotMissing", claude: manifestOf(claudeOnly), codex: manifestOf()},
			{name: "OmittedPluginContextCountsPresent", claude: manifestOf(held), codex: manifestOf(unrun)},
			{name: "ExtraCodexItemIsNoFinding", claude: manifestOf(passage), codex: manifestOf(passage, extra)},
			{name: "ForkDenyIsNotAskedOfAnEngineWithoutForks", claude: manifestOf(forkDeny), codex: manifestOf()},
			{name: "ForkDenyIsAskedOfAnEngineWithForks", claude: manifestOf(forkDeny, passage), codex: manifestOf(passage), codexFork: true, want: []shuttleengine.ManifestItem{forkDeny}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				engines := engineSetOf(newManifestEngine(true, nil), newManifestEngine(tt.codexFork, nil))
				got := Compare(engines, map[string]shuttleengine.Manifest{ReferenceEngine: tt.claude, ComparedEngine: tt.codex})
				if len(got) != len(tt.want) || (len(got) > 0 && !reflect.DeepEqual(got, tt.want)) {
					t.Errorf("Compare() = %+v; want %+v", got, tt.want)
				}
			})
		}
	})

	t.Run("PromptItems", func(t *testing.T) {
		t.Parallel()
		stencils := stencilkit.Seed(t)
		directive, err := editdirective.Directive(stencils, ReferenceEngine)
		if err != nil {
			t.Fatalf("editdirective.Directive() error: %v", err)
		}
		tests := []struct {
			name   string
			prompt string
			want   []shuttleengine.ManifestItem
		}{
			{name: "PromptHoldingTheEnginesCopy", prompt: "Do it.\n" + directive, want: []shuttleengine.ManifestItem{item(shuttleengine.ChannelEditDirective, "edit-directive", "")}},
			{name: "PromptWithoutIt", prompt: "Do it."},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got := PromptItems(stencils, ReferenceEngine, tt.prompt); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("PromptItems() = %+v; want %+v", got, tt.want)
				}
			})
		}
	})

	t.Run("Unmet", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			engine *manifestEngine
			spec   shuttleengine.Spec
			want   []shuttleengine.Requirement
		}{
			{name: "ForkSubagentsOnAnEngineMeetingNone", engine: newManifestEngine(false, nil), spec: shuttleengine.Spec{ForkSubagents: true}, want: []shuttleengine.Requirement{shuttleengine.RequirementForks}},
			{name: "PlainSpec", engine: newManifestEngine(false, nil), spec: shuttleengine.Spec{}},
			{name: "ListedRequirementThenForks", engine: newManifestEngine(false, nil), spec: shuttleengine.Spec{ForkSubagents: true, Requires: []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake}}, want: []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake, shuttleengine.RequirementForks}},
			{name: "ForkSubagentsOnAnEngineWithForks", engine: newManifestEngine(true, nil), spec: shuttleengine.Spec{ForkSubagents: true}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got := Unmet(tt.engine, tt.spec); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("Unmet() = %v; want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("UnrunHooks", func(t *testing.T) {
		t.Parallel()
		if _, err := UnrunHooks(shuttleengine.HookRun{}); err != shuttleengine.ErrHookUnrun {
			t.Errorf("UnrunHooks() error = %v; want shuttleengine.ErrHookUnrun itself, unwrapped", err)
		}
		if err := ConfiguredSandbox("codex", "home", "dir"); err != nil {
			t.Errorf("ConfiguredSandbox() error = %v; want a sandbox that starts", err)
		}
	})

	t.Run("PrepareEach", func(t *testing.T) {
		t.Parallel()
		stencils := stencilkit.Seed(t)
		cfg := shuttleengine.Config{Agents: shuttlefake.Agents(t)}
		roleSkills, err := cfg.Agents.Skills(agentconfig.RoleDarn, nil)
		if err != nil {
			t.Fatalf("Skills() error: %v", err)
		}
		held := manifestOf(item(shuttleengine.ChannelNotice, "n", ""))
		specOf := func(mutate func(*shuttleengine.Spec)) map[string]shuttleengine.Spec {
			spec := shuttleengine.Spec{Prompt: "p", Role: "darn"}
			mutate(&spec)
			return map[string]shuttleengine.Spec{ReferenceEngine: spec, ComparedEngine: spec}
		}
		var received []shuttleengine.Spec
		recording := func(m shuttleengine.Manifest) func(string, shuttleengine.Spec) error {
			return func(runDir string, spec shuttleengine.Spec) error {
				received = append(received, spec)
				return writesManifest(m)(runDir, spec)
			}
		}
		failing := errors.New("prepare failed")
		tests := []struct {
			name     string
			compared func() *manifestEngine
			specs    map[string]shuttleengine.Spec
			wantErr  []string
			// wantSkills is the skills each engine's Prepare is to receive for a spec with a role.
			wantSkills []string
		}{
			{name: "ReadsEachManifest", compared: func() *manifestEngine { return newManifestEngine(false, recording(held)) }, specs: specOf(func(*shuttleengine.Spec) {})},
			{name: "FailingPrepareNamesItsEngine", compared: func() *manifestEngine {
				return newManifestEngine(false, func(string, shuttleengine.Spec) error { return failing })
			}, specs: specOf(func(*shuttleengine.Spec) {}), wantErr: []string{ComparedEngine, "prepare failed"}},
			{name: "NoManifestNamesTheEngineAndTheWriter", compared: func() *manifestEngine {
				return newManifestEngine(false, func(string, shuttleengine.Spec) error { return nil })
			}, specs: specOf(func(*shuttleengine.Spec) {}), wantErr: []string{ComparedEngine, "shuttleengine.WriteManifest"}},
			{
				name: "RoleSkillsThenTheSpecsOwn", compared: func() *manifestEngine { return newManifestEngine(false, recording(held)) },
				specs: specOf(func(spec *shuttleengine.Spec) {
					spec.ConfigRole, spec.Skills = agentconfig.RoleDarn, []string{"own:skill"}
				}),
				wantSkills: append(append([]string{}, roleSkills...), "own:skill"),
			},
			{
				name: "RoleWithNoSkillListIsNamed", compared: func() *manifestEngine { return newManifestEngine(false, recording(held)) },
				specs:   specOf(func(spec *shuttleengine.Spec) { spec.ConfigRole = "no-such-role" }),
				wantErr: []string{"no-such-role"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				received = nil
				engines := engineSetOf(newManifestEngine(false, recording(held)), tt.compared())
				manifests, err := PrepareEach(engines, tt.specs, stencils, t.TempDir(), cfg)
				if len(tt.wantErr) > 0 {
					if err == nil {
						t.Fatal("PrepareEach() error = nil; want a refusal")
					}
					for _, want := range tt.wantErr {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("PrepareEach() error %q does not contain %q", err, want)
						}
					}
					return
				}
				if err != nil {
					t.Fatalf("PrepareEach() error: %v", err)
				}
				for _, engine := range []string{ReferenceEngine, ComparedEngine} {
					if got := manifests[engine].Items; len(got) != 1 || got[0].Key != "n" {
						t.Errorf("manifest of %s = %+v; want the one item its Prepare wrote", engine, got)
					}
				}
				for _, spec := range received {
					if tt.wantSkills != nil && !reflect.DeepEqual(spec.Skills, tt.wantSkills) {
						t.Errorf("Prepare received skills %v; want %v", spec.Skills, tt.wantSkills)
					}
				}
			})
		}
	})

	t.Run("seatsOf", func(t *testing.T) {
		t.Parallel()
		stencils := stencilkit.Seed(t)
		writeFixtureFile(t, stencilstore.Path(stencils, "parity-test-bare"), "Lead {{.seat_name}}.\n")
		env := SeatEnv{
			StencilsDir: stencils, Dir: t.TempDir(),
			Location:  &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "wt", AnchorRel: "."},
			Shortname: "ly", Slug: "task", ParentName: "ly:orch",
		}
		table := seatengine.Table{
			RolePrefix: "parity",
			Seats: []seatengine.Seat{{
				Stencil:     "parity-test-bare",
				ModelChoice: shuttleengine.ModelChoice{Engine: "x", Model: "opus", Effort: "high", Version: "5"},
				Outputs:     []string{filepath.Join(env.Dir, "out.md")},
			}},
		}
		cause := errors.New("no stand-in")
		tables := []seatTable{
			{Builder: "internal/x#First", Build: func(SeatEnv) ([]seatengine.Table, error) { return []seatengine.Table{table}, nil }},
			{Builder: "internal/x#Second", Build: func(SeatEnv) ([]seatengine.Table, error) { return nil, cause }},
		}

		seats, failures := seatsOf(env, tables)
		if len(seats) != 1 || seats[0].Name != "parity" || seats[0].Builder != "internal/x#First" {
			t.Fatalf("seatsOf() seats = %+v; want the first entry's one seat", seats)
		}
		if len(failures) != 1 || !errors.Is(failures[0], cause) || !strings.Contains(failures[0].Error(), "internal/x#Second") || !strings.Contains(failures[0].Error(), "way forward") {
			t.Errorf("seatsOf() failures = %v; want one naming the second builder, wrapping its error, with a way forward", failures)
		}
		for _, engine := range []string{ReferenceEngine, ComparedEngine} {
			spec, err := seats[0].Specs(engine)
			if err != nil {
				t.Fatalf("Specs(%s) error: %v", engine, err)
			}
			if want := (shuttleengine.ModelChoice{Engine: engine}); spec.ModelChoice != want {
				t.Errorf("Specs(%s) model choice = %+v; want the engine alone, %+v", engine, spec.ModelChoice, want)
			}
		}
	})
}
