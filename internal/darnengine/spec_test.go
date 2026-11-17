// spec_test.go — tier 0 unit tests for DarnTable.
// Pure Go over an in-memory Config, a temp-dir modelspec registry and seeded stencils;
// the lead's prompt is composed through seatengine.Prompts, which starts nothing.

package darnengine

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newSpecInputs returns inputs over a seeded stencils directory.
// Every required field is set and every optional one is empty.
func newSpecInputs(t *testing.T) DarnInputs {
	t.Helper()
	return DarnInputs{
		StencilsDir:     stencilkit.Seed(t),
		SpecsDir:        filepath.Join(t.TempDir(), "specs"),
		WorktreeRoot:    t.TempDir(),
		Slug:            "fix-the-thing",
		DescriptionPath: filepath.Join(t.TempDir(), "description.md"),

		DecisionRecordPath: filepath.Join(t.TempDir(), "decision-record.md"),
		StopMarkerPath:     filepath.Join(t.TempDir(), "stop-marker"),

		ConditionStopMarkerPath: filepath.Join(t.TempDir(), "condition-stop-marker"),
		SubagentModel:           "haiku",
	}
}

// newTestRegistry loads an empty-directory model registry, which still resolves the built-in aliases.
func newTestRegistry(t *testing.T) modelspec.Registry {
	t.Helper()
	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}
	return reg
}

// leadPrompt returns the prompt of table's lead.
// It composes the prompt under a geometry over in's stencils directory, naming parentName.
func leadPrompt(t *testing.T, in DarnInputs, parentName string, table seatengine.Table) string {
	t.Helper()
	prompts, err := seatengine.Prompts(seatengine.Geometry{
		WorktreeRoot: in.WorktreeRoot,
		AnchorPath:   in.WorktreeRoot,
		StencilsDir:  in.StencilsDir,
		ParentName:   parentName,
		Shortname:    "ly",
		Slug:         in.Slug,
	}, table)
	if err != nil {
		t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
	}
	return prompts[""]
}

// The Step 4 lines that tell the gate rule apart.
// The writer's and its subagents' gate runs have the verify gate's fixer off,
// and the no-gate step has it on.
const (
	ungatedTestStep     = "3. Test with `lym gate test` after the last edit of each commit"
	ungatedSubagentGate = "the gate rule from your system prompt with its Bash tool timeout, for the subagent to run on its own package"
	gatedNoGateStep     = "3. Run no `lym gate test` yourself."
)

// TestDarnSpec covers the table's lead and settings.
// It checks the rendered tokens for a first spawn, where the optional prior-work token renders to none.
// It also checks a spawn that carries prior work, a parent and a friction directory.
func TestDarnSpec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cfg := Config{Writer: "opus[effort=high]", WriterTimeoutMin: 45}
	tests := []struct {
		name            string
		configure       func(in *DarnInputs)
		parentName      string
		wantInPrompt    func(in DarnInputs) []string
		wantNotInPrompt []string
		wantNone        int
		wantNoParent    bool
		wantSkills      []string
	}{
		{
			name:      "first spawn renders the prior-work token as none",
			configure: func(in *DarnInputs) {},
			wantInPrompt: func(in DarnInputs) []string {
				return []string{"fix-the-thing", in.DescriptionPath, filepath.ToSlash(in.SpecsDir), in.DecisionRecordPath, in.StopMarkerPath, in.ConditionStopMarkerPath, `model: "haiku"`, ungatedTestStep, ungatedSubagentGate}
			},
			wantNotInPrompt: []string{gatedNoGateStep},
			wantNone:        1,
			wantNoParent:    true,
		},
		{
			name:      "a cold fixer on the verify gate has the writer and its subagents run no gate",
			configure: func(in *DarnInputs) { in.GateFixed = true },
			wantInPrompt: func(in DarnInputs) []string {
				return []string{`model: "haiku"`, gatedNoGateStep}
			},
			wantNotInPrompt: []string{ungatedTestStep, ungatedSubagentGate},
			wantNone:        1,
			wantNoParent:    true,
		},
		{
			name: "resumed spawn renders the prior work, the parent and the friction note path",
			configure: func(in *DarnInputs) {
				in.PriorWork = "the verify gate failed on TestCache"
				in.FrictionDir = t.TempDir()
			},
			parentName: "the-parent",
			wantInPrompt: func(in DarnInputs) []string {
				return []string{"the verify gate failed on TestCache", "the-parent", filepath.Join(in.FrictionDir, "Darn-Write.md")}
			},
		},
		{
			name: "the seat's skills are the hub's language skills alone, the role's coming from agents.yaml",
			configure: func(in *DarnInputs) {
				in.LanguageSkills = []string{"scribe:golang-testing"}
			},
			wantInPrompt: func(in DarnInputs) []string { return nil },
			wantNone:     1,
			wantNoParent: true,
			wantSkills:   []string{"scribe:golang-testing"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := newSpecInputs(t)
			tt.configure(&in)
			reg := newTestRegistry(t)
			table, err := DarnTable(in, cfg, reg)
			if err != nil {
				t.Fatalf("DarnTable() = _, %v; want nil error", err)
			}
			if err := table.Validate(in.StencilsDir, ""); err != nil {
				t.Fatalf("table.Validate() = %v; want nil", err)
			}
			// The loom wiring fills this marker blank for a pair under no beam, which only an Optional marker accepts.
			table.Values["project_directive"] = ""
			if err := table.Validate(in.StencilsDir, ""); err != nil {
				t.Errorf("table.Validate() with a blank project_directive = %v; want nil, the marker is optional", err)
			}
			if len(table.Seats) != 1 {
				t.Fatalf("table has %d seats; want the lead alone", len(table.Seats))
			}

			lead := table.Chair()
			if role := seatengine.SeatRole(table.RolePrefix, lead.Name); lead.Name != "" || role != "darn" {
				t.Errorf("lead name %q and role %q; want no name and the role %q", lead.Name, role, "darn")
			}
			if lead.Engine != "claude" || lead.Model != "opus" || lead.Effort != "high" || lead.Version != "" {
				t.Errorf("lead engine/model/effort/version = %q/%q/%q/%q; want claude/opus/high/none", lead.Engine, lead.Model, lead.Effort, lead.Version)
			}
			if len(lead.Requires) != 0 {
				t.Errorf("lead Requires = %v; want none, a lone seat declares nothing", lead.Requires)
			}
			if want := []string{in.DescriptionPath}; !slices.Equal(lead.Outputs, want) {
				t.Errorf("lead outputs = %v; want the description alone %v", lead.Outputs, want)
			}
			if lead.ConfigRole != agentconfig.RoleDarn {
				t.Errorf("lead config role = %q; want %q", lead.ConfigRole, agentconfig.RoleDarn)
			}
			if !slices.Equal(lead.Skills, tt.wantSkills) {
				t.Errorf("lead skills = %v; want %v", lead.Skills, tt.wantSkills)
			}
			if table.Segment != segmentcolor.Darn {
				t.Errorf("table segment = %q; want %q", table.Segment, segmentcolor.Darn)
			}
			if table.Timeout != 45*time.Minute {
				t.Errorf("table timeout = %s; want 45m0s", table.Timeout)
			}
			if table.Interactive || lead.ForkSubagents {
				t.Errorf("table interactive %v and lead fork %v; want both false", table.Interactive, lead.ForkSubagents)
			}

			prompt := leadPrompt(t, in, tt.parentName, table)
			for _, want := range tt.wantInPrompt(in) {
				if !strings.Contains(prompt, want) {
					t.Errorf("lead prompt does not contain %q", want)
				}
			}
			for _, unwanted := range tt.wantNotInPrompt {
				if strings.Contains(prompt, unwanted) {
					t.Errorf("lead prompt contains %q; want it absent", unwanted)
				}
			}
			if got := strings.Count(prompt, "\n(none)\n"); got != tt.wantNone {
				t.Errorf("lead prompt renders (none) on its own line %d times; want %d", got, tt.wantNone)
			}
			if got := strings.Contains(prompt, "the-parent"); got == tt.wantNoParent {
				t.Errorf("lead prompt names the parent = %v; want %v", got, !tt.wantNoParent)
			}
		})
	}
}

// TestDarnSpec_Refusals covers the refusal of each empty required field by name.
// It also covers an unusable writer spec and a stencil missing from the stencils directory.
func TestDarnSpec_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		configure func(t *testing.T, in *DarnInputs, cfg *Config)
		want      string
	}{
		{"empty StencilsDir", func(t *testing.T, in *DarnInputs, cfg *Config) { in.StencilsDir = "" }, "StencilsDir"},
		{"empty SpecsDir", func(t *testing.T, in *DarnInputs, cfg *Config) { in.SpecsDir = "" }, "SpecsDir"},
		{"empty WorktreeRoot", func(t *testing.T, in *DarnInputs, cfg *Config) { in.WorktreeRoot = "" }, "WorktreeRoot"},
		{"empty Slug", func(t *testing.T, in *DarnInputs, cfg *Config) { in.Slug = "" }, "Slug"},
		{"empty DescriptionPath", func(t *testing.T, in *DarnInputs, cfg *Config) { in.DescriptionPath = "" }, "DescriptionPath"},
		{"empty DecisionRecordPath", func(t *testing.T, in *DarnInputs, cfg *Config) { in.DecisionRecordPath = "" }, "DecisionRecordPath"},
		{"empty StopMarkerPath", func(t *testing.T, in *DarnInputs, cfg *Config) { in.StopMarkerPath = "" }, "StopMarkerPath"},
		{"empty ConditionStopMarkerPath", func(t *testing.T, in *DarnInputs, cfg *Config) { in.ConditionStopMarkerPath = "" }, "ConditionStopMarkerPath"},
		{"empty SubagentModel", func(t *testing.T, in *DarnInputs, cfg *Config) { in.SubagentModel = "" }, "SubagentModel"},
		{"malformed writer spec", func(t *testing.T, in *DarnInputs, cfg *Config) { cfg.Writer = "opus[effort=high" }, "writer model-spec"},
		{"missing stencil", func(t *testing.T, in *DarnInputs, cfg *Config) { stencilkit.Remove(t, in.StencilsDir, darnStencilName) }, darnStencilName},
		{"missing Step 4 fragment", func(t *testing.T, in *DarnInputs, cfg *Config) {
			stencilkit.Remove(t, in.StencilsDir, makeChangeStencil)
		}, makeChangeStencil},
		{"missing gated Step 4 fragment", func(t *testing.T, in *DarnInputs, cfg *Config) {
			in.GateFixed = true
			stencilkit.Remove(t, in.StencilsDir, makeChangeGatedStencil)
		}, makeChangeGatedStencil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := newSpecInputs(t)
			cfg := Config{Writer: "opus[medium]", WriterTimeoutMin: 45}
			tt.configure(t, &in, &cfg)
			table, err := DarnTable(in, cfg, newTestRegistry(t))
			if err != nil && len(table.Seats) != 0 {
				t.Errorf("DarnTable() seats = %d alongside the error; want none", len(table.Seats))
			}
			if err == nil {
				err = table.Validate(in.StencilsDir, "")
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DarnTable() and its Validate = %v; want an error containing %q", err, tt.want)
			}
		})
	}
}
