// spec_test.go — untagged Tier-1 unit tests for DarnSpec.
// Pure Go over an in-memory Config, a temp-dir modelspec registry and seeded stencils;
// no live hub, reed, or network involved.

package darnengine

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
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
	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}
	return reg
}

// TestDarnSpec covers the spec's fields and the rendered tokens for a first spawn, where both optional tokens render (none), and for a spawn that carries rejection findings, prior work, a parent and a friction directory.
func TestDarnSpec(t *testing.T) {
	t.Parallel()

	cfg := Config{Writer: "opus[effort=high]", WriterTimeoutMin: 45}
	tests := []struct {
		name         string
		configure    func(in *DarnInputs)
		wantInPrompt func(in DarnInputs) []string
		wantNone     int
		wantNoParent bool
	}{
		{
			name:      "first spawn renders both optional tokens as none",
			configure: func(in *DarnInputs) {},
			wantInPrompt: func(in DarnInputs) []string {
				return []string{"`lyx board get fix-the-thing`", in.DescriptionPath, filepath.ToSlash(in.SpecsDir) + "/final-summary-spec.md"}
			},
			wantNone:     2,
			wantNoParent: true,
		},
		{
			name: "rework spawn renders the findings, the prior work, the parent and the friction note path",
			configure: func(in *DarnInputs) {
				in.RejectionFindings = "the cache is never invalidated"
				in.PriorWork = "the verify gate failed on TestCache"
				in.ParentName = "the-parent"
				in.FrictionDir = t.TempDir()
			},
			wantInPrompt: func(in DarnInputs) []string {
				return []string{"the cache is never invalidated", "the verify gate failed on TestCache", "the-parent", filepath.Join(in.FrictionDir, "Darn.md")}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := newSpecInputs(t)
			tt.configure(&in)
			spec, err := DarnSpec(in, cfg, newTestRegistry(t))
			if err != nil {
				t.Fatalf("DarnSpec() = _, %v; want nil error", err)
			}

			if spec.Role != "darn" || spec.Segment != segmentcolor.Darn || spec.Interactive {
				t.Errorf("DarnSpec() role, segment, interactive = %q, %q, %v; want darn, %q, false", spec.Role, spec.Segment, spec.Interactive, segmentcolor.Darn)
			}
			if !slices.Equal(spec.Skills, []string{"scribe:prose", "scribe:code-quality"}) {
				t.Errorf("DarnSpec().Skills = %v; want the prose and code-quality skills", spec.Skills)
			}
			if !slices.Equal(spec.OutputFiles, []string{in.DescriptionPath}) {
				t.Errorf("DarnSpec().OutputFiles = %v; want only the description", spec.OutputFiles)
			}
			if spec.Effort != "high" || spec.Model == "" {
				t.Errorf("DarnSpec() model, effort = %q, %q; want a resolved model at effort high", spec.Model, spec.Effort)
			}
			if want := 45 * time.Minute; spec.Timeout != want {
				t.Errorf("DarnSpec().Timeout = %s; want %s", spec.Timeout, want)
			}
			for _, want := range tt.wantInPrompt(in) {
				if !strings.Contains(spec.Prompt, want) {
					t.Errorf("DarnSpec().Prompt does not contain %q", want)
				}
			}
			if got := strings.Count(spec.Prompt, "\n(none)\n"); got != tt.wantNone {
				t.Errorf("DarnSpec().Prompt renders (none) on its own line %d times; want %d", got, tt.wantNone)
			}
			if got := strings.Contains(spec.Prompt, "the-parent"); got == tt.wantNoParent {
				t.Errorf("DarnSpec().Prompt names the parent = %v; want %v", got, !tt.wantNoParent)
			}
		})
	}
}

// TestDarnSpec_Refusals covers the refusal of each empty required field by name, an unusable writer spec, and a stencil missing from the stencils directory.
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
			spec, err := DarnSpec(in, cfg, newTestRegistry(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DarnSpec() = _, %v; want an error containing %q", err, tt.want)
			}
			if spec.Prompt != "" {
				t.Errorf("DarnSpec().Prompt = %q alongside the error; want empty", spec.Prompt)
			}
		})
	}
}
