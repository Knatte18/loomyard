// spec_test.go — untagged Tier-1 unit tests for DarnTable.
// Pure Go over an in-memory Config, a temp-dir modelspec registry and seeded stencils;
// the lead's prompt is composed through seatengine.Prompts, which starts nothing.

package darnengine

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// newSpecInputs returns inputs over a seeded stencils directory, with every required field set and every optional one empty.
func newSpecInputs(t *testing.T) DarnInputs {
	t.Helper()
	return DarnInputs{
		StencilsDir:     stencilkit.Seed(t),
		SpecsDir:        filepath.Join(t.TempDir(), "specs"),
		WorktreeRoot:    t.TempDir(),
		Slug:            "fix-the-thing",
		DescriptionPath: filepath.Join(t.TempDir(), "description.md"),
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

// leadPrompt returns the prompt of table's lead composed under a geometry over in's stencils directory naming parentName.
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

// TestDarnSpec covers the table's lead and settings, and the rendered tokens for a first spawn, where both optional tokens render (none), and for a spawn that carries rejection findings, prior work, a parent and a friction directory.
func TestDarnSpec(t *testing.T) {
	t.Parallel()

	cfg := Config{Writer: "opus[effort=high]", WriterTimeoutMin: 45}
	tests := []struct {
		name         string
		configure    func(in *DarnInputs)
		parentName   string
		wantInPrompt func(in DarnInputs) []string
		wantNone     int
		wantNoParent bool
	}{
		{
			name:      "first spawn renders both optional tokens as none",
			configure: func(in *DarnInputs) {},
			wantInPrompt: func(in DarnInputs) []string {
				return []string{"`lyx board get '{\"slug\":\"fix-the-thing\"}'`", in.DescriptionPath, filepath.ToSlash(in.SpecsDir) + "/loom/loom-final-summary-spec.md"}
			},
			wantNone:     2,
			wantNoParent: true,
		},
		{
			name: "rework spawn renders the findings, the prior work, the parent and the friction note path",
			configure: func(in *DarnInputs) {
				in.RejectionFindings = "the cache is never invalidated"
				in.PriorWork = "the verify gate failed on TestCache"
				in.FrictionDir = t.TempDir()
			},
			parentName: "the-parent",
			wantInPrompt: func(in DarnInputs) []string {
				return []string{"the cache is never invalidated", "the verify gate failed on TestCache", "the-parent", filepath.Join(in.FrictionDir, "Darn-Write.md")}
			},
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
			if err := table.Validate(in.StencilsDir); err != nil {
				t.Fatalf("table.Validate() = %v; want nil", err)
			}
			if len(table.Seats) != 1 {
				t.Fatalf("table has %d seats; want the lead alone", len(table.Seats))
			}

			lead := table.Chair()
			if role := seatengine.SeatRole(table.RolePrefix, lead.Name); lead.Name != "" || role != "darn" {
				t.Errorf("lead name %q and role %q; want no name and the role %q", lead.Name, role, "darn")
			}
			if lead.Model != "opus" || lead.Effort != "high" || lead.Version != "" {
				t.Errorf("lead model/effort/version = %q/%q/%q; want opus/high/none", lead.Model, lead.Effort, lead.Version)
			}
			if want := []string{in.DescriptionPath}; !slices.Equal(lead.Outputs, want) {
				t.Errorf("lead outputs = %v; want the description alone %v", lead.Outputs, want)
			}
			if want := []string{"scribe:prose", "scribe:code-quality"}; !slices.Equal(lead.Skills, want) {
				t.Errorf("lead skills = %v; want %v", lead.Skills, want)
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
			if got := strings.Count(prompt, "\n(none)\n"); got != tt.wantNone {
				t.Errorf("lead prompt renders (none) on its own line %d times; want %d", got, tt.wantNone)
			}
			if got := strings.Contains(prompt, "the-parent"); got == tt.wantNoParent {
				t.Errorf("lead prompt names the parent = %v; want %v", got, !tt.wantNoParent)
			}
		})
	}
}

// TestDarnSpec_Refusals covers the refusal of each empty required field by name, an unusable writer spec, and a stencil missing from the stencils directory.
// A missing stencil is the seat engine's refusal: the table builder reads none, and Validate reads every seat's.
func TestDarnSpec_Refusals(t *testing.T) {
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
		{"malformed writer spec", func(t *testing.T, in *DarnInputs, cfg *Config) { cfg.Writer = "opus[effort=high" }, "writer model-spec"},
		{"missing stencil", func(t *testing.T, in *DarnInputs, cfg *Config) { stencilkit.Remove(t, in.StencilsDir, darnStencilName) }, darnStencilName},
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
				err = table.Validate(in.StencilsDir)
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DarnTable() and its Validate = %v; want an error containing %q", err, tt.want)
			}
		})
	}
}
