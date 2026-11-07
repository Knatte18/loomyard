// spec_test.go — tier 0 unit tests for samplerengine.SessionSpec.

package samplerengine

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSessionSpec covers the spec a valid call builds, with and without a PATTERN overview in the worktree;
// the required inputs it refuses by name; and an agent the registry cannot resolve.
func TestSessionSpec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stencilsDir := stencilkit.Seed(t)
	registry := modelspec.Registry{"opus": {Engine: "claude", Model: "opus"}}
	valid := SessionInputs{
		StencilsDir:        stencilsDir,
		ParentName:         "hub:orch",
		Slug:               "try-the-thing",
		WorktreeRoot:       "/work/try-the-thing",
		DecisionRecordPath: "/work/try-the-thing/decision.md",
	}
	cfg := Config{Agent: "opus[high]"}

	t.Run("a valid call builds the interactive session spec", func(t *testing.T) {
		t.Parallel()
		spec, err := SessionSpec(valid, cfg, registry)
		if err != nil {
			t.Fatalf("SessionSpec() = _, %v; want nil error", err)
		}
		if !spec.Interactive || !spec.Unawaited || !spec.DenyPublish {
			t.Errorf("SessionSpec() Interactive, Unawaited, DenyPublish = %v, %v, %v; want all true", spec.Interactive, spec.Unawaited, spec.DenyPublish)
		}
		if spec.Role != Role || spec.Segment != segmentcolor.Darn {
			t.Errorf("SessionSpec() Role, Segment = %q, %q; want %q, %q", spec.Role, spec.Segment, Role, segmentcolor.Darn)
		}
		if want := []string{"scribe:prose", "scribe:conversation", "scribe:code-quality", "scribe:testing"}; !slices.Equal(spec.Skills, want) {
			t.Errorf("SessionSpec() Skills = %v; want %v", spec.Skills, want)
		}
		directive, err := parentdirective.Directive(stencilsDir, "hub:orch", true)
		if err != nil {
			t.Fatalf("parentdirective.Directive() = _, %v; want nil error", err)
		}
		for _, want := range []string{directive, valid.DecisionRecordPath, valid.Slug, valid.WorktreeRoot} {
			if !strings.Contains(spec.Prompt, want) {
				t.Errorf("SessionSpec() prompt lacks %q", want)
			}
		}
	})

	t.Run("a worktree with a PATTERN overview carries the implementer directive", func(t *testing.T) {
		t.Parallel()
		in := valid
		in.WorktreeRoot = t.TempDir()
		const overview = "# PATTERN\n\n- `PATTERN-example` — an entry the sampler agent must follow.\n"
		if err := os.WriteFile(pattern.File(in.WorktreeRoot), []byte(overview), 0o644); err != nil {
			t.Fatal(err)
		}
		spec, err := SessionSpec(in, cfg, registry)
		if err != nil {
			t.Fatalf("SessionSpec() = _, %v; want nil error", err)
		}
		directive, err := pattern.Directive(in.WorktreeRoot, stencilsDir, pattern.RoleImplementer, nil)
		if err != nil {
			t.Fatalf("pattern.Directive() = _, %v; want nil error", err)
		}
		if !strings.Contains(directive, "PATTERN-example") {
			t.Fatalf("pattern.Directive() = %q; want the overview's entry", directive)
		}
		if !strings.Contains(spec.Prompt, directive) {
			t.Errorf("SessionSpec() prompt lacks the implementer PATTERN directive %q", directive)
		}
	})

	refusals := []struct {
		name    string
		mutate  func(*SessionInputs, *Config)
		wantErr string
	}{
		{"empty StencilsDir", func(in *SessionInputs, _ *Config) { in.StencilsDir = "" }, "StencilsDir"},
		{"empty Slug", func(in *SessionInputs, _ *Config) { in.Slug = "" }, "Slug"},
		{"empty WorktreeRoot", func(in *SessionInputs, _ *Config) { in.WorktreeRoot = "" }, "WorktreeRoot"},
		{"empty DecisionRecordPath", func(in *SessionInputs, _ *Config) { in.DecisionRecordPath = "" }, "DecisionRecordPath"},
		{"an agent the registry cannot resolve", func(_ *SessionInputs, c *Config) { c.Agent = "nosuchmodel" }, "agent"},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in, c := valid, cfg
			tt.mutate(&in, &c)
			if _, err := SessionSpec(in, c, registry); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("SessionSpec() = _, %v; want an error containing %q", err, tt.wantErr)
			}
		})
	}
}
