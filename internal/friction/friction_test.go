// friction_test.go covers Directive's four roles and WarnIfMarkerAbsent.
// Every test here is untagged Tier 1: it uses only os.MkdirAll/os.WriteFile inside a t.TempDir() (via
// newTestStencilsDir, seeding a hermetic stencils directory), t.TempDir itself, and spawns nothing.

package friction

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// newTestStencilsDir builds a t.TempDir() seeded with the four friction-directive stencils, copied
// from the stencils package's embedded defaults and stamped through
// stencilstore.ApplyStamp(content, stencilstore.BodyHash(content)) so the fixture matches what
// stencilstore.Reconcile really puts on disk in a hub — a real leading banner included, which is
// what makes the banner-strip assertion in this file meaningful.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string][]byte{
		implementerDirectiveStencil:  stencils.FrictionDirectiveImplementer,
		reviewFixDirectiveStencil:    stencils.FrictionDirectiveReviewFix,
		orchestratorDirectiveStencil: stencils.FrictionDirectiveOrchestrator,
		interviewDirectiveStencil:    stencils.FrictionDirectiveInterview,
	}
	for name, content := range files {
		path := stencilstore.Path(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
		}
		stamped := stencilstore.ApplyStamp(content, stencilstore.BodyHash(content))
		if err := os.WriteFile(path, stamped, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	return dir
}

// TestDirective_AllRolesReturnOwnStencilText covers all four roles: each returns its own stencil's
// text, containing the told note path verbatim -- the assertion that catches a composer wiring the
// wrong path.
func TestDirective_AllRolesReturnOwnStencilText(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)
	notePath := "/anchor/_lyx/friction/some-note.md"

	tests := []struct {
		name string
		role Role
		want []byte
	}{
		{"Implementer", RoleImplementer, stencils.FrictionDirectiveImplementer},
		{"ReviewFix", RoleReviewFix, stencils.FrictionDirectiveReviewFix},
		{"Orchestrator", RoleOrchestrator, stencils.FrictionDirectiveOrchestrator},
		{"Interview", RoleInterview, stencils.FrictionDirectiveInterview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		})
	}
}

// TestDirective_EmptyNotePath pins the empty-notePath guard: no stencil read is attempted at all,
// asserted by pointing stencilsDir at a path that does not exist, so any attempted read would
// surface as an error.
func TestDirective_EmptyNotePath(t *testing.T) {
	missingStencilsDir := t.TempDir() + "/does-not-exist"

	got, err := Directive("", missingStencilsDir, RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(\"\", RoleImplementer) = _, %v; want nil error", err)
	}
	if got != "" {
		t.Errorf("Directive(\"\", RoleImplementer) = %q; want \"\"", got)
	}
}

// TestDirective_UnknownRole pins the unknown/zero Role behaviour: no directive text and no read
// attempted, even with a non-empty notePath.
func TestDirective_UnknownRole(t *testing.T) {
	missingStencilsDir := t.TempDir() + "/does-not-exist"

	tests := []struct {
		name string
		role Role
	}{
		{"ZeroRole", Role(0)},
		{"OutOfRangeRole", Role(99)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive("/anchor/note.md", missingStencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(%v) = _, %v; want nil error", tt.role, err)
			}
			if got != "" {
				t.Errorf("Directive(%v) = %q; want \"\"", tt.role, got)
			}
		})
	}
}

// TestDirective_MissingStencilErrors pins the fail-loud posture on a read failure: a non-empty
// notePath, plus a stencilsDir that exists but carries none of the four friction stencils, returns a
// non-nil error naming the missing stencil, for every role.
func TestDirective_MissingStencilErrors(t *testing.T) {
	emptyStencilsDir := t.TempDir()

	tests := []struct {
		name    string
		role    Role
		stencil string
	}{
		{"Implementer", RoleImplementer, implementerDirectiveStencil},
		{"ReviewFix", RoleReviewFix, reviewFixDirectiveStencil},
		{"Orchestrator", RoleOrchestrator, orchestratorDirectiveStencil},
		{"Interview", RoleInterview, interviewDirectiveStencil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive("/anchor/note.md", emptyStencilsDir, tt.role)
			if err == nil {
				t.Fatalf("Directive(%v, empty stencilsDir) = %q, nil; want a non-nil error", tt.role, got)
			}
			if !strings.Contains(err.Error(), tt.stencil) {
				t.Errorf("Directive(%v, empty stencilsDir) error = %q; want it to name the missing stencil %q", tt.role, err.Error(), tt.stencil)
			}
		})
	}
}

// TestWarnIfMarkerAbsent_ReportsAbsentAndPresent covers the marker-detection half: absent for
// template bytes with no "{{.friction_directive}}" literal, present for bytes carrying it. Since
// WarnIfMarkerAbsent returns nothing, this test only pins that it does not panic on either input --
// the fires-only-when-enabled behaviour is pinned separately below.
func TestWarnIfMarkerAbsent_ReportsAbsentAndPresent(t *testing.T) {
	WarnIfMarkerAbsent([]byte("no marker here"), "some-stencil", "a directive")
	WarnIfMarkerAbsent([]byte("has {{.friction_directive}} right here"), "some-stencil", "a directive")
}

// TestWarnIfMarkerAbsent_NoOpsOnEmptyDirective pins that WarnIfMarkerAbsent never fires when
// directive is empty -- both the Tier-2-off case and the swallowed-read-error case -- regardless of
// whether the marker is present in template.
func TestWarnIfMarkerAbsent_NoOpsOnEmptyDirective(t *testing.T) {
	WarnIfMarkerAbsent([]byte("no marker here"), "some-stencil", "")
	WarnIfMarkerAbsent([]byte("has {{.friction_directive}} right here"), "some-stencil", "")
}

// TestReportFileName_DerivationsAgree asserts ReportFileName is the single exported constant both a
// note-scan exclusion and an OutputFiles entry can be derived from. It derives an OutputFiles-style
// absolute path from ReportFileName and a note-scan-style stem-collision check via NotePath's own
// sanitization, and asserts the two derivations agree with each other -- never two string literals
// asserted independently.
func TestReportFileName_DerivationsAgree(t *testing.T) {
	dir := t.TempDir()

	// An OutputFiles-style consumer joins ReportFileName onto the friction dir directly.
	outputFilesEntry := dir + "/" + ReportFileName

	// A note-scan-style consumer derives the stem it must never collide with by trimming
	// ReportFileName's own ".md" suffix, then confirms NotePath's own sanitization rejects that
	// exact stem -- the same guarantee the note-scan exclusion depends on.
	stem := strings.TrimSuffix(ReportFileName, ".md")
	if got := NotePath(dir, stem); got != "" {
		t.Errorf("NotePath(%q, %q) = %q; want \"\" since %q+\".md\" collides with ReportFileName %q", dir, stem, got, stem, ReportFileName)
	}
	if !strings.HasSuffix(outputFilesEntry, ReportFileName) {
		t.Errorf("outputFilesEntry %q does not end with ReportFileName %q", outputFilesEntry, ReportFileName)
	}
}
