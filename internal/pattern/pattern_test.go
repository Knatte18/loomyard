// pattern_test.go exercises Directive's active check and its three
// directive variants. Every test here is untagged Tier 1: it uses only
// os.Stat (via the package's statFile seam) and t.TempDir, and spawns
// nothing.

package pattern

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// layoutAt builds a minimal *hubgeometry.Layout rooted at worktreeRoot, with
// the given RelPath, sufficient for PatternFileHere() to resolve — the only
// accessor this package calls.
func layoutAt(worktreeRoot, relPath string) *hubgeometry.Layout {
	return &hubgeometry.Layout{
		WorktreeRoot: worktreeRoot,
		RelPath:      relPath,
	}
}

// writePatternFile creates root/_pattern/PATTERN.md (and the _pattern
// directory) with the given content, failing the test on any error.
func writePatternFile(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, "_pattern")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v", err)
	}
}

// TestDirective_ActiveWithFile covers the common active case — PATTERN.md
// present as a regular file — for all three roles.
func TestDirective_ActiveWithFile(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "# PATTERN\n\nsome constraints\n")
	l := layoutAt(root, ".")

	tests := []struct {
		name string
		role Role
	}{
		{"Implementer", RoleImplementer},
		{"ReviewFix", RoleReviewFix},
		{"Orchestrator", RoleOrchestrator},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Directive(l, tt.role)
			if got == "" {
				t.Errorf("Directive(active, %v) = \"\"; want non-empty", tt.role)
			}
		})
	}
}

// TestDirective_InactiveWithoutFile covers the two ordinary inactive cases:
// the _pattern directory present without PATTERN.md (the normal state
// lyx init leaves behind), and neither present at all.
func TestDirective_InactiveWithoutFile(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "DirPresentFileAbsent",
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, "_pattern"), 0o755); err != nil {
					t.Fatalf("MkdirAll = %v", err)
				}
			},
		},
		{
			name:  "NeitherPresent",
			setup: func(t *testing.T, root string) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)
			l := layoutAt(root, ".")
			if got := Directive(l, RoleImplementer); got != "" {
				t.Errorf("Directive(%s, RoleImplementer) = %q; want \"\"", tt.name, got)
			}
		})
	}
}

// TestDirective_EmptyPatternFileIsActive pins the "empty file still counts
// as active" edge rule: a degenerate but harmless state, preferable to a
// content-inspecting check that would turn a benign empty file into a
// runtime error.
func TestDirective_EmptyPatternFileIsActive(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "")
	l := layoutAt(root, ".")

	if got := Directive(l, RoleImplementer); got == "" {
		t.Errorf("Directive(empty PATTERN.md) = \"\"; want non-empty")
	}
}

// TestDirective_PatternFileAsDirectoryIsInactive pins the "PATTERN.md as a
// directory counts as inactive" edge rule: a directory in that place is not
// a readable index.
func TestDirective_PatternFileAsDirectoryIsInactive(t *testing.T) {
	root := t.TempDir()
	patternFileAsDir := filepath.Join(root, "_pattern", "PATTERN.md")
	if err := os.MkdirAll(patternFileAsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", patternFileAsDir, err)
	}
	l := layoutAt(root, ".")

	if got := Directive(l, RoleImplementer); got != "" {
		t.Errorf("Directive(PATTERN.md as directory) = %q; want \"\"", got)
	}
}

// TestDirective_NilLayout pins the nil-Layout guard: several Deps structs
// are assembled field-by-field by CLI callers that could leave Layout
// unset, and a nil dereference here would take down all five agent paths
// for a slip unrelated to PATTERN.
func TestDirective_NilLayout(t *testing.T) {
	if got := Directive(nil, RoleImplementer); got != "" {
		t.Errorf("Directive(nil, RoleImplementer) = %q; want \"\"", got)
	}
}

// TestDirective_UnknownRole pins the documented unknown/zero Role
// behaviour: no directive text, even when PATTERN is active.
func TestDirective_UnknownRole(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")

	tests := []struct {
		name string
		role Role
	}{
		{"ZeroRole", Role(0)},
		{"OutOfRangeRole", Role(99)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Directive(l, tt.role); got != "" {
				t.Errorf("Directive(active, %v) = %q; want \"\"", tt.role, got)
			}
		})
	}
}

// TestDirective_VariantsArePairwiseDistinct pins that the three role
// variants never collapse into the same text, and that each carries the
// literal relative pointer "_pattern/PATTERN.md" — never an interpolated
// absolute path, which would make the value vary per worktree.
func TestDirective_VariantsArePairwiseDistinct(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")

	variants := map[Role]string{
		RoleImplementer:  Directive(l, RoleImplementer),
		RoleReviewFix:    Directive(l, RoleReviewFix),
		RoleOrchestrator: Directive(l, RoleOrchestrator),
	}
	for role, text := range variants {
		if !strings.Contains(text, "_pattern/PATTERN.md") {
			t.Errorf("Directive(%v) does not contain the literal pointer _pattern/PATTERN.md: %q", role, text)
		}
	}

	if variants[RoleImplementer] == variants[RoleReviewFix] {
		t.Error("RoleImplementer and RoleReviewFix render identical directive text")
	}
	if variants[RoleImplementer] == variants[RoleOrchestrator] {
		t.Error("RoleImplementer and RoleOrchestrator render identical directive text")
	}
	if variants[RoleReviewFix] == variants[RoleOrchestrator] {
		t.Error("RoleReviewFix and RoleOrchestrator render identical directive text")
	}
}

// TestDirective_VariantsBeginWithOwnHeading pins that each variant carries
// its own "##" heading inline, so an inactive render leaves no orphan
// heading behind in the surrounding prompt template.
func TestDirective_VariantsBeginWithOwnHeading(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")

	tests := []struct {
		name string
		role Role
	}{
		{"Implementer", RoleImplementer},
		{"ReviewFix", RoleReviewFix},
		{"Orchestrator", RoleOrchestrator},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Directive(l, tt.role)
			if !strings.HasPrefix(got, "## ") {
				t.Errorf("Directive(%v) does not begin with its own \"##\" heading: %q", tt.role, got)
			}
		})
	}
}

// TestDirective_RelPathNestedSubdirectory is the regression guard for the
// worst failure mode in this task: a Layout whose RelPath is a nested
// subdirectory must resolve PATTERN.md at
// <WorktreeRoot>/<RelPath>/_pattern/PATTERN.md and must NOT be satisfied by
// one planted at the worktree root instead. Without this guard, a
// root-anchored resolution would render PATTERN silently inactive in every
// agent invoked from a subdirectory, with no error anywhere.
func TestDirective_RelPathNestedSubdirectory(t *testing.T) {
	root := t.TempDir()
	relPath := filepath.Join("sub", "dir")

	// Plant PATTERN.md only at the (wrong) worktree root; the RelPath-aware
	// resolution must still see this worktree as inactive.
	writePatternFile(t, root, "content")
	l := layoutAt(root, relPath)
	if got := Directive(l, RoleImplementer); got != "" {
		t.Errorf("Directive() found the root-planted PATTERN.md via a nested RelPath; got %q, want \"\"", got)
	}

	// Now plant PATTERN.md at the correct nested location; the resolution
	// must find it there.
	nestedRoot := filepath.Join(root, relPath)
	writePatternFile(t, nestedRoot, "content")
	if got := Directive(l, RoleImplementer); got == "" {
		t.Error("Directive() did not find PATTERN.md planted at <WorktreeRoot>/<RelPath>/_pattern/PATTERN.md")
	}
}

// TestDirective_NonNotExistStatErrorIsActive pins the third edge rule: a
// stat error that is not os.IsNotExist (a permission or I/O failure) is
// treated as active, not inactive. This is simulated through the
// package-level statFile seam rather than a real unreadable-directory
// trick, because an actual permission-denied stat error is not portable —
// it depends on the OS and on whether the test process runs elevated (e.g.
// as root in a container, where POSIX permission bits are not enforced),
// and Windows has no equivalent lever at all.
func TestDirective_NonNotExistStatErrorIsActive(t *testing.T) {
	root := t.TempDir()
	l := layoutAt(root, ".")

	// PATTERN.md is absent on disk; without the seam this would resolve
	// inactive via os.IsNotExist. Force a distinct, non-not-exist error to
	// confirm the "any other stat error is active" rule.
	original := statFile
	statFile = func(name string) (os.FileInfo, error) {
		return nil, &os.PathError{Op: "stat", Path: name, Err: errors.New("permission denied")}
	}
	t.Cleanup(func() { statFile = original })

	if got := Directive(l, RoleImplementer); got == "" {
		t.Error("Directive() with a non-IsNotExist stat error = \"\"; want the directive text (active)")
	}
}
