// Every test here is untagged Tier 1: it reads a seeded temporary stencils directory and spawns nothing.

package editdirective

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// TestDirective_RendersTheRuleWithoutBannerOrMarkers covers the seeded stencil: the rule's key phrases are present, with no marker and no leading comment.
func TestDirective_RendersTheRuleWithoutBannerOrMarkers(t *testing.T) {
	t.Parallel()
	dir := stencilkit.Seed(t)

	got, err := Directive(dir)
	if err != nil {
		t.Fatalf("Directive = _, %v; want nil error", err)
	}
	for _, want := range []string{"Edit or Write", "sed", "replace_all", "grep"} {
		if !strings.Contains(got, want) {
			t.Errorf("Directive = %q; want it to contain %q", got, want)
		}
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

// TestDirective_MissingStencilNamesIt covers a stencil removed from the directory: the error names it.
func TestDirective_MissingStencilNamesIt(t *testing.T) {
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
