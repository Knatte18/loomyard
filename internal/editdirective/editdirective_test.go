// Every test here is in tier 0: it reads a seeded temporary stencils directory and spawns nothing.

package editdirective

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDirective_RendersTheRuleWithoutBannerOrMarkers covers the seeded stencil:
// the rule's anchor phrase is present, with no marker and no leading comment.
func TestDirective_RendersTheRuleWithoutBannerOrMarkers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := stencilkit.Seed(t)

	got, err := Directive(dir)
	if err != nil {
		t.Fatalf("Directive = _, %v; want nil error", err)
	}
	if !strings.Contains(got, "Edit or Write") {
		t.Errorf("Directive = %q; want it to contain %q", got, "Edit or Write")
	}
	if strings.Contains(got, "{{") {
		t.Errorf("Directive = %q; want no marker", got)
	}
	if strings.HasPrefix(got, "<!--") {
		t.Errorf("Directive = %q; want no leading comment", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("Directive = %q; want no trailing newline", got)
	}
}

// TestEngineDirective_ReadsTheEnginesCopyWithoutBanner covers each shipped engine copy:
// its own tool wording is present, with no marker, leading comment or trailing newline.
func TestEngineDirective_ReadsTheEnginesCopyWithoutBanner(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		engine string
		want   string
	}{
		{"claude", "Edit or Write"},
		{"codex", "apply_patch"},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)

			got, err := EngineDirective(dir, tt.engine)
			if err != nil {
				t.Fatalf("EngineDirective(dir, %q) = _, %v; want nil error", tt.engine, err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("EngineDirective(dir, %q) = %q; want it to contain %q", tt.engine, got, tt.want)
			}
			if strings.Contains(got, "{{") {
				t.Errorf("EngineDirective(dir, %q) = %q; want no marker", tt.engine, got)
			}
			if strings.HasPrefix(got, "<!--") {
				t.Errorf("EngineDirective(dir, %q) = %q; want no leading comment", tt.engine, got)
			}
			if strings.HasSuffix(got, "\n") {
				t.Errorf("EngineDirective(dir, %q) = %q; want no trailing newline", tt.engine, got)
			}
		})
	}
}

// TestEngineDirective_EngineWithoutACopyNamesTheStencil covers an engine that ships no copy:
// the error names the engine's stencil.
func TestEngineDirective_EngineWithoutACopyNamesTheStencil(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := stencilkit.Seed(t)

	got, err := EngineDirective(dir, "noengine")
	if err == nil {
		t.Fatalf("EngineDirective(dir, %q) = %q, nil; want an error", "noengine", got)
	}
	if want := stencilstore.EngineBlockName("noengine", EngineBlock); !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q; want it to name %q", err, want)
	}
}

// TestDirective_MissingStencilNamesIt covers a stencil removed from the directory: the error names it.
func TestDirective_MissingStencilNamesIt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := stencilkit.Seed(t)
	stencilkit.Remove(t, dir, directiveStencil)

	got, err := Directive(dir)
	if err == nil {
		t.Fatalf("Directive = %q, nil; want an error", got)
	}
	if !strings.Contains(err.Error(), directiveStencil) {
		t.Errorf("error = %q; want it to name %q", err, directiveStencil)
	}
}
