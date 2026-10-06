// friction_test.go covers Directive's four roles and WarnIfMarkerAbsent.
// Every test here is untagged Tier 1: it uses only t.TempDir (via newTestStencilsDir, seeding a hermetic stencils directory) and spawns nothing.

package friction

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// newTestStencilsDir returns a seeded stencils directory.
// Seeding runs `stencilstore.Reconcile`, so the files carry the real leading banner, which is what makes the banner-strip assertion in this file meaningful.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// TestDirective_PerRole covers all four roles.
// Against a seeded stencils directory each role returns its own stencil's text, containing the told note path verbatim -- the assertion that catches a composer wiring the wrong path.
// Against a stencils directory that carries none of the four friction stencils each role fails loud with a non-nil error naming its missing stencil.
func TestDirective_PerRole(t *testing.T) {
	t.Parallel()
	stencilsDir := newTestStencilsDir(t)
	emptyStencilsDir := t.TempDir()
	notePath := "/anchor/_lyx/friction/some-note.md"

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

// TestDirective_NoDirectiveAttemptsNoRead pins the two guards that return no directive text and attempt no stencil read: an empty notePath, and an unknown or zero Role with a non-empty notePath.
// Each is asserted by pointing stencilsDir at a path that does not exist, so any attempted read would surface as an error.
func TestDirective_NoDirectiveAttemptsNoRead(t *testing.T) {
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

// TestWarnIfMarkerAbsent_DoesNotPanic covers the marker-detection half: absent for template bytes with no "{{.friction_directive}}" literal, present for bytes carrying it, each with a directive and with an empty one.
// Since WarnIfMarkerAbsent returns nothing, this test only pins that it does not panic on any input.
func TestWarnIfMarkerAbsent_DoesNotPanic(t *testing.T) {
	t.Parallel()
	for _, template := range []string{"no marker here", "has {{.friction_directive}} right here"} {
		for _, directive := range []string{"a directive", ""} {
			WarnIfMarkerAbsent([]byte(template), "some-stencil", directive)
		}
	}
}
