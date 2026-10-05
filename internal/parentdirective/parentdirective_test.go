// parentdirective_test.go covers Directive's three renderings and its missing-stencil error.
// Every test here is untagged Tier 1: it reads a seeded temporary stencils directory and spawns nothing.

package parentdirective

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// banLine is a phrase unique to the operator-ban stencil.
const banLine = "never the way forward"

// TestDirective_ParentNameRendersParentAndBan covers the non-interactive parent variant: the parent name and the operator-ban line are both present.
func TestDirective_ParentNameRendersParentAndBan(t *testing.T) {
	t.Parallel()
	dir := stencilkit.Seed(t)

	got, err := Directive(dir, "ab:cd:webster", false)
	if err != nil {
		t.Fatalf("Directive = _, %v; want nil error", err)
	}
	if !strings.Contains(got, "ab:cd:webster") {
		t.Errorf("Directive = %q; want it to name the parent", got)
	}
	if !strings.Contains(got, banLine) {
		t.Errorf("Directive = %q; want the operator-ban line", got)
	}
	if strings.Contains(got, "<!--") || strings.Contains(got, "{{") {
		t.Errorf("Directive = %q; want no banner or unfilled marker", got)
	}
}

// TestDirective_InteractiveOmitsOnlyBan covers the interactive variant: the text equals the non-interactive one minus the ban line.
func TestDirective_InteractiveOmitsOnlyBan(t *testing.T) {
	t.Parallel()
	dir := stencilkit.Seed(t)

	plain, err := Directive(dir, "ab:cd:webster", false)
	if err != nil {
		t.Fatalf("Directive(non-interactive) = _, %v; want nil error", err)
	}
	interactive, err := Directive(dir, "ab:cd:webster", true)
	if err != nil {
		t.Fatalf("Directive(interactive) = _, %v; want nil error", err)
	}
	if strings.Contains(interactive, banLine) {
		t.Errorf("Directive(interactive) = %q; want no operator-ban line", interactive)
	}
	if !strings.Contains(interactive, "ab:cd:webster") {
		t.Errorf("Directive(interactive) = %q; want it to name the parent", interactive)
	}
	if len(interactive) >= len(plain) {
		t.Errorf("Directive(interactive) is %d bytes, non-interactive %d; want the ban line omitted only", len(interactive), len(plain))
	}
}

// TestDirective_EmptyNameRendersNoParentVariant covers the no-parent variant: no parent sentence, and the same text whether or not the role is interactive.
func TestDirective_EmptyNameRendersNoParentVariant(t *testing.T) {
	t.Parallel()
	dir := stencilkit.Seed(t)

	got, err := Directive(dir, "", false)
	if err != nil {
		t.Fatalf("Directive = _, %v; want nil error", err)
	}
	if !strings.Contains(got, "No parent is recorded") {
		t.Errorf("Directive = %q; want the no-parent variant", got)
	}
	if strings.Contains(got, "SendMessage") {
		t.Errorf("Directive = %q; want no parent sentence", got)
	}
	again, err := Directive(dir, "", true)
	if err != nil || again != got {
		t.Errorf("Directive(interactive) = %q, %v; want the same text %q", again, err, got)
	}
}

// TestDirective_MissingStencilNamesIt covers a missing stencil: each variant's error names the stencil it could not read.
func TestDirective_MissingStencilNamesIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		parentName  string
		interactive bool
		missing     string
	}{
		{"None", "", false, noneStencil},
		{"Parent", "ab:cd:webster", true, parentStencil},
		{"OperatorBan", "ab:cd:webster", false, operatorBanStencil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)
			if err := os.Remove(stencilstore.Path(dir, tt.missing)); err != nil {
				t.Fatalf("os.Remove = %v; want nil error", err)
			}

			got, err := Directive(dir, tt.parentName, tt.interactive)
			if err == nil {
				t.Fatalf("Directive = %q, nil; want an error", got)
			}
			if !strings.Contains(err.Error(), tt.missing) {
				t.Errorf("Directive error = %v; want it to name %q", err, tt.missing)
			}
		})
	}
}
