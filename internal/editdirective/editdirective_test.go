// Every test here is in tier 0: it reads a seeded temporary stencils directory and spawns nothing.

package editdirective_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDirective_ReadsTheEnginesCopyWithoutBanner covers each shipped engine copy:
// its own tool wording is present, with no marker, leading comment or trailing newline.
func TestDirective_ReadsTheEnginesCopyWithoutBanner(t *testing.T) {
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

			got, err := editdirective.Directive(dir, tt.engine)
			if err != nil {
				t.Fatalf("Directive(dir, %q) = _, %v; want nil error", tt.engine, err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("Directive(dir, %q) = %q; want it to contain %q", tt.engine, got, tt.want)
			}
			if strings.Contains(got, "{{") {
				t.Errorf("Directive(dir, %q) = %q; want no marker", tt.engine, got)
			}
			if strings.HasPrefix(got, "<!--") {
				t.Errorf("Directive(dir, %q) = %q; want no leading comment", tt.engine, got)
			}
			if strings.HasSuffix(got, "\n") {
				t.Errorf("Directive(dir, %q) = %q; want no trailing newline", tt.engine, got)
			}
		})
	}
}

// TestDirective_EngineWithoutACopyNamesTheStencil covers an engine that ships no copy.
// It also covers a copy removed from the directory.
// The error names the engine's stencil.
func TestDirective_EngineWithoutACopyNamesTheStencil(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		engine  string
		removed bool
	}{
		{"NoCopyShipped", "noengine", false},
		{"CopyRemoved", "claude", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)
			want := stencilstore.EngineBlockName(tt.engine, editdirective.EngineBlock)
			if tt.removed {
				stencilkit.Remove(t, dir, want)
			}

			got, err := editdirective.Directive(dir, tt.engine)
			if err == nil {
				t.Fatalf("Directive(dir, %q) = %q, nil; want an error", tt.engine, got)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q; want it to name %q", err, want)
			}
		})
	}
}
