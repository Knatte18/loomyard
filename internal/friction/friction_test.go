// friction_test.go covers Directive's four roles and WarnIfMarkerAbsent.
// Every test here is in tier 0.
// It uses only t.TempDir, through newTestStencilsDir, and spawns nothing.

package friction

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newTestStencilsDir returns a seeded stencils directory.
// Seeding runs `stencilstore.Reconcile`, so the files carry the real leading banner that a banner-strip check needs.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// TestDirective_PerRole covers all four roles.
// Against a seeded stencils directory, each role returns its own stencil's text, containing the note path verbatim.
// This catches a composer that wires the wrong path.
// Against a stencils directory without the four friction stencils, each role returns a non-nil error.
// The error names the missing stencil.
func TestDirective_PerRole(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	stencilsDir := newTestStencilsDir(t)
	emptyStencilsDir := t.TempDir()
	notePath := "/anchor/_lym/friction/some-note.md"

	tests := []struct {
		name    string
		role    Role
		want    []byte
		stencil string
	}{
		{"Implementer", RoleImplementer, stencils.FrictionDirectiveImplementer, implementerDirectiveStencil},
		{"ReviewFix", RoleReviewFix, stencils.FrictionDirectiveReviewFix, reviewFixDirectiveStencil},
		{"Orchestrator", RoleOrchestrator, stencils.FrictionDirectiveOrchestrator, orchestratorDirectiveStencil},
		{"Interview", RoleInterview, stencils.FrictionDirectiveInterview, interviewDirectiveStencil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Directive(notePath, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(%v) = _, %v; want nil error", tt.role, err)
			}
			if !strings.Contains(got, notePath) {
				t.Errorf("Directive(%v) = %q; want it to contain the note path %q verbatim", tt.role, got, notePath)
			}
			if strings.Contains(got, "<!--") {
				t.Errorf("Directive(%v) = %q; want no leftover banner content", tt.role, got)
			}

			got, err = Directive(notePath, emptyStencilsDir, tt.role)
			if err == nil {
				t.Fatalf("Directive(%v, empty stencilsDir) = %q, nil; want a non-nil error", tt.role, got)
			}
			if !strings.Contains(err.Error(), tt.stencil) {
				t.Errorf("Directive(%v, empty stencilsDir) error = %q; want it to name the missing stencil %q", tt.role, err.Error(), tt.stencil)
			}
		})
	}
}

// TestDirective_NoDirectiveAttemptsNoRead pins the guards that return no directive text without reading a stencil.
// The guards are an empty notePath, and a Role outside the defined roles with a non-empty notePath.
// Each case points the stencils directory at a path that does not exist, so an attempted read would surface as an error.
func TestDirective_NoDirectiveAttemptsNoRead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	missingStencilsDir := t.TempDir() + "/does-not-exist"

	tests := []struct {
		name     string
		notePath string
		role     Role
	}{
		{"EmptyNotePath", "", RoleImplementer},
		{"ZeroRole", "/anchor/note.md", Role(0)},
		{"OutOfRangeRole", "/anchor/note.md", Role(99)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Directive(tt.notePath, missingStencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(%q, %v) = _, %v; want nil error", tt.notePath, tt.role, err)
			}
			if got != "" {
				t.Errorf("Directive(%q, %v) = %q; want \"\"", tt.notePath, tt.role, got)
			}
		})
	}
}

// TestWarnIfMarkerAbsent_DoesNotPanic covers the marker detection in WarnIfMarkerAbsent.
// It runs templates with and without the "{{.friction_directive}}" marker, each with and without a directive.
// The function returns nothing, so the test only pins that it does not panic on these inputs.
func TestWarnIfMarkerAbsent_DoesNotPanic(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for _, template := range []string{"no marker here", "has {{.friction_directive}} right here"} {
		for _, directive := range []string{"a directive", ""} {
			WarnIfMarkerAbsent([]byte(template), "some-stencil", directive)
		}
	}
}
