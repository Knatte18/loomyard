// pattern_test.go exercises Directive's active check and its three directive variants.
// Every test here is untagged Tier 1: it uses only os.Stat (via the package's statFile seam),
// os.MkdirAll/os.WriteFile inside a t.TempDir() (via newTestStencilsDir, seeding a hermetic stencils
// directory), and t.TempDir itself, and spawns nothing.

package pattern

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// layoutAt builds a minimal *lyxcwd.Location rooted at worktreeRoot, with
// the given RelPath, sufficient for PatternFileHere() to resolve — the only
// accessor this package calls.
func layoutAt(worktreeRoot, relPath string) *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot), AnchorRel: relPath}
}

// writePatternFile creates root/_lyx/PATTERN.md (and the _lyx
// directory) with the given content, failing the test on any error.
func writePatternFile(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, lyxdirs.LyxDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v", err)
	}
}

// newTestStencilsDir builds a t.TempDir() seeded with the three pattern-directive stencils, copied
// from the stencils package's embedded defaults, and returns the directory to pass as stencilsDir.
//
// Unlike the three existing consumer test helpers (loomengine, burlerengine, websterengine), which
// write each embedded default's bytes raw, this helper writes each file's bytes through
// stencilstore.ApplyStamp(content, stencilstore.BodyHash(content)) first, so the fixture matches what
// stencilstore.Reconcile really puts on disk in a hub — a real leading banner, `lyx-stencil:` stamp
// line included. Do not "fix" this into consistency with the other three: the other helpers get away
// with raw bytes because everything they feed passes through stencil.Fill, which strips the banner
// either way, whereas a raw fixture here would make the banner-strip test (see
// TestDirective_StripsBanner) vacuous and let a missing strip pass green.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	patternDir := filepath.Join(dir, "pattern")
	if err := os.MkdirAll(patternDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
	}
	files := map[string][]byte{
		"pattern-directive-implementer.md":  stencils.PatternDirectiveImplementer,
		"pattern-directive-review-fix.md":   stencils.PatternDirectiveReviewFix,
		"pattern-directive-orchestrator.md": stencils.PatternDirectiveOrchestrator,
	}
	for name, content := range files {
		stamped := stencilstore.ApplyStamp(content, stencilstore.BodyHash(content))
		if err := os.WriteFile(filepath.Join(patternDir, name), stamped, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
	return dir
}

// TestDirective_ActiveWithFile covers the common active case — PATTERN.md present as a regular file
// — for all three roles.
func TestDirective_ActiveWithFile(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "# PATTERN\n\nsome constraints\n")
	l := layoutAt(root, ".")
	stencilsDir := newTestStencilsDir(t)

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
			got, err := Directive(l, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(active, %v) = _, %v; want nil error", tt.role, err)
			}
			if got == "" {
				t.Errorf("Directive(active, %v) = \"\"; want non-empty", tt.role)
			}
		})
	}
}

// TestDirective_InactiveWithoutFile covers the two ordinary inactive cases: an unrelated stray
// directory present without PATTERN.md,
// and neither present at all.
func TestDirective_InactiveWithoutFile(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "DirPresentFileAbsent",
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, "stray_dir"), 0o755); err != nil {
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
			got, err := Directive(l, newTestStencilsDir(t), RoleImplementer)
			if err != nil {
				t.Fatalf("Directive(%s, RoleImplementer) = _, %v; want nil error", tt.name, err)
			}
			if got != "" {
				t.Errorf("Directive(%s, RoleImplementer) = %q; want \"\"", tt.name, got)
			}
		})
	}
}

// TestDirective_EmptyPatternFileIsActive pins the "empty file still counts as active" edge rule: a
// degenerate but harmless state, preferable to a content-inspecting check that would turn a benign
// empty file into a runtime error.
func TestDirective_EmptyPatternFileIsActive(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "")
	l := layoutAt(root, ".")

	got, err := Directive(l, newTestStencilsDir(t), RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(empty PATTERN.md) = _, %v; want nil error", err)
	}
	if got == "" {
		t.Errorf("Directive(empty PATTERN.md) = \"\"; want non-empty")
	}
}

// TestDirective_PatternFileAsDirectoryIsInactive pins the "PATTERN.md as a directory counts as
// inactive" edge rule: a directory in that place is not a readable index.
func TestDirective_PatternFileAsDirectoryIsInactive(t *testing.T) {
	root := t.TempDir()
	patternFileAsDir := filepath.Join(root, lyxdirs.LyxDirName, "PATTERN.md")
	if err := os.MkdirAll(patternFileAsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", patternFileAsDir, err)
	}
	l := layoutAt(root, ".")

	got, err := Directive(l, newTestStencilsDir(t), RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(PATTERN.md as directory) = _, %v; want nil error", err)
	}
	if got != "" {
		t.Errorf("Directive(PATTERN.md as directory) = %q; want \"\"", got)
	}
}

// TestDirective_NilLayout pins the nil-Layout guard: several Deps structs are assembled
// field-by-field by CLI callers that could leave Layout unset,
// and a nil dereference here would take down all five agent paths for a slip unrelated to PATTERN.
func TestDirective_NilLayout(t *testing.T) {
	got, err := Directive(nil, newTestStencilsDir(t), RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(nil, RoleImplementer) = _, %v; want nil error", err)
	}
	if got != "" {
		t.Errorf("Directive(nil, RoleImplementer) = %q; want \"\"", got)
	}
}

// TestDirective_UnknownRole pins the documented unknown/zero Role behaviour: no directive text,
// even when PATTERN is active.
func TestDirective_UnknownRole(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")
	stencilsDir := newTestStencilsDir(t)

	tests := []struct {
		name string
		role Role
	}{
		{"ZeroRole", Role(0)},
		{"OutOfRangeRole", Role(99)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(l, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(active, %v) = _, %v; want nil error", tt.role, err)
			}
			if got != "" {
				t.Errorf("Directive(active, %v) = %q; want \"\"", tt.role, got)
			}
		})
	}
}

// TestDirective_VariantsArePairwiseDistinct pins that the three role variants never collapse into
// the same text,
// and that each carries the literal relative pointer "_lyx/PATTERN.md" — never an interpolated
// absolute path, which would make the value vary per worktree.
func TestDirective_VariantsArePairwiseDistinct(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")
	stencilsDir := newTestStencilsDir(t)

	implementerText, err := Directive(l, stencilsDir, RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(active, RoleImplementer) = _, %v; want nil error", err)
	}
	reviewFixText, err := Directive(l, stencilsDir, RoleReviewFix)
	if err != nil {
		t.Fatalf("Directive(active, RoleReviewFix) = _, %v; want nil error", err)
	}
	orchestratorText, err := Directive(l, stencilsDir, RoleOrchestrator)
	if err != nil {
		t.Fatalf("Directive(active, RoleOrchestrator) = _, %v; want nil error", err)
	}

	variants := map[Role]string{
		RoleImplementer:  implementerText,
		RoleReviewFix:    reviewFixText,
		RoleOrchestrator: orchestratorText,
	}
	for role, text := range variants {
		if !strings.Contains(text, "_lyx/PATTERN.md") {
			t.Errorf("Directive(%v) does not contain the literal pointer _lyx/PATTERN.md: %q", role, text)
		}
		if !strings.Contains(text, "_lyx/pattern/") {
			t.Errorf("Directive(%v) does not contain the literal detail-doc pointer _lyx/pattern/: %q", role, text)
		}
		if strings.Contains(text, "_pattern/") {
			t.Errorf("Directive(%v) still contains the old _pattern/ substring: %q", role, text)
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

// TestDirective_VariantsBeginWithOwnHeading pins that each variant carries its own "##" heading
// inline, so an inactive render leaves no orphan heading behind in the surrounding prompt template.
func TestDirective_VariantsBeginWithOwnHeading(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")
	stencilsDir := newTestStencilsDir(t)

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
			got, err := Directive(l, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(%v) = _, %v; want nil error", tt.role, err)
			}
			if !strings.HasPrefix(got, "## ") {
				t.Errorf("Directive(%v) does not begin with its own \"##\" heading: %q", tt.role, got)
			}
		})
	}
}

// TestDirective_RelPathNestedSubdirectory is the regression guard for the worst failure mode in
// this task: a Layout whose RelPath is a nested subdirectory must resolve PATTERN.md at
// <WorktreeRoot>/<RelPath>/_lyx/PATTERN.md and must NOT be satisfied by one planted at the
// worktree root instead.
// Without this guard, a root-anchored resolution would render PATTERN silently inactive in every
// agent invoked from a subdirectory, with no error anywhere.
func TestDirective_RelPathNestedSubdirectory(t *testing.T) {
	root := t.TempDir()
	relPath := filepath.Join("sub", "dir")
	stencilsDir := newTestStencilsDir(t)

	// Plant PATTERN.md only at the (wrong) worktree root; the RelPath-aware
	// resolution must still see this worktree as inactive.
	writePatternFile(t, root, "content")
	l := layoutAt(root, relPath)
	got, err := Directive(l, stencilsDir, RoleImplementer)
	if err != nil {
		t.Fatalf("Directive() = _, %v; want nil error", err)
	}
	if got != "" {
		t.Errorf("Directive() found the root-planted PATTERN.md via a nested RelPath; got %q, want \"\"", got)
	}

	// Now plant PATTERN.md at the correct nested location; the resolution
	// must find it there.
	nestedRoot := filepath.Join(root, relPath)
	writePatternFile(t, nestedRoot, "content")
	got, err = Directive(l, stencilsDir, RoleImplementer)
	if err != nil {
		t.Fatalf("Directive() = _, %v; want nil error", err)
	}
	if got == "" {
		t.Error("Directive() did not find PATTERN.md planted at <WorktreeRoot>/<RelPath>/_lyx/PATTERN.md")
	}
}

// TestDirective_NonNotExistStatErrorIsActive pins the third edge rule: a stat error that is not
// os.IsNotExist (a permission or I/O failure) is treated as active, not inactive.
// This is simulated through the package-level statFile seam rather than a real unreadable-directory
// trick, because an actual permission-denied stat error is not portable — it depends on the OS and
// on whether the test process runs elevated (e.g.
// as root in a container, where POSIX permission bits are not enforced), and Windows has no
// equivalent lever at all.
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

	got, err := Directive(l, newTestStencilsDir(t), RoleImplementer)
	if err != nil {
		t.Fatalf("Directive() = _, %v; want nil error", err)
	}
	if got == "" {
		t.Error("Directive() with a non-IsNotExist stat error = \"\"; want the directive text (active)")
	}
}

// TestDirective_LazyRead pins the read as lazy: on every inactive path Directive returns ("", nil)
// without ever touching stencilsDir, so an eager-read refactor that would break burlerengine's,
// websterengine's, and loomengine's inactive-PATTERN fixtures fails here first, locally, before it
// ever reaches those packages' own tests.
func TestDirective_LazyRead(t *testing.T) {
	missingStencilsDir := filepath.Join(t.TempDir(), "does-not-exist")

	t.Run("PATTERN inactive, stencilsDir does not exist", func(t *testing.T) {
		root := t.TempDir()
		l := layoutAt(root, ".")
		got, err := Directive(l, missingStencilsDir, RoleImplementer)
		if err != nil {
			t.Fatalf("Directive(inactive, missing stencilsDir) = _, %v; want nil error", err)
		}
		if got != "" {
			t.Errorf("Directive(inactive, missing stencilsDir) = %q; want \"\"", got)
		}
	})

	t.Run("nil layout, stencilsDir does not exist", func(t *testing.T) {
		got, err := Directive(nil, missingStencilsDir, RoleImplementer)
		if err != nil {
			t.Fatalf("Directive(nil, missing stencilsDir) = _, %v; want nil error", err)
		}
		if got != "" {
			t.Errorf("Directive(nil, missing stencilsDir) = %q; want \"\"", got)
		}
	})
}

// TestDirective_MissingStencilErrors pins the fail-loud posture on a read failure: PATTERN active,
// plus a stencilsDir that exists but carries none of the three pattern stencils, returns a non-nil
// error naming the missing stencil, for every role. See
// TestDiscussionSpec_MissingStencilsDirIsHardError in internal/loomengine/discussion_test.go for the
// same shape's precedent.
func TestDirective_MissingStencilErrors(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")
	emptyStencilsDir := t.TempDir()

	tests := []struct {
		name    string
		role    Role
		stencil string
	}{
		{"Implementer", RoleImplementer, implementerDirectiveStencil},
		{"ReviewFix", RoleReviewFix, reviewFixDirectiveStencil},
		{"Orchestrator", RoleOrchestrator, orchestratorDirectiveStencil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(l, emptyStencilsDir, tt.role)
			if err == nil {
				t.Fatalf("Directive(active, %v, empty stencilsDir) = %q, nil; want a non-nil error", tt.role, got)
			}
			if !strings.Contains(err.Error(), tt.stencil) {
				t.Errorf("Directive(active, %v, empty stencilsDir) error = %q; want it to name the missing stencil %q", tt.role, err.Error(), tt.stencil)
			}
		})
	}
}

// TestDirective_StripsBanner is the regression guard for the correctness bug that would otherwise
// ship: stencilstore.Read does not strip a leading banner, and Directive's returned value never
// passes through stencil.Fill (which is the only other place a banner strip could happen), so
// Directive itself must be the one that strips it. Directive against newTestStencilsDir(t) — whose
// files carry a realistic leading banner including a `lyx-stencil:` stamp line, because that helper
// writes them through stencilstore.ApplyStamp — must return text beginning at the `## ` heading and
// containing no `<!--` anywhere, for every role.
func TestDirective_StripsBanner(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")
	stencilsDir := newTestStencilsDir(t)

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
			got, err := Directive(l, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(active, %v) = _, %v; want nil error", tt.role, err)
			}
			if !strings.HasPrefix(got, "## ") {
				t.Errorf("Directive(active, %v) = %q; want it to begin with its own \"##\" heading, not a leading banner", tt.role, got)
			}
			if strings.Contains(got, "<!--") {
				t.Errorf("Directive(active, %v) = %q; want no leftover \"<!--\" banner content", tt.role, got)
			}
		})
	}
}

// TestDirective_StrippedBodyMatchesEmbeddedDefault asserts each role's returned text equals
// stencil.StripLeadingComment(string(<the matching stencils package embedded default>)) — never
// whole-file byte equality against the on-disk fixture (which carries a banner and a stamp the
// return value never does) and never equality against the raw embedded default (which still carries
// its own banner).
//
// This is a cheap tripwire against a future edit to a stencil file drifting from the embedded
// default, not proof the relocation was faithful — with the constants deleted it effectively
// compares the stencil against itself. What actually pins the relocation is
// TestDirective_VariantsArePairwiseDistinct, TestDirective_VariantsBeginWithOwnHeading, and loom's
// own end-to-end PATTERN-active ordering assertion.
func TestDirective_StrippedBodyMatchesEmbeddedDefault(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	l := layoutAt(root, ".")
	stencilsDir := newTestStencilsDir(t)

	tests := []struct {
		name            string
		role            Role
		embeddedDefault []byte
	}{
		{"Implementer", RoleImplementer, stencils.PatternDirectiveImplementer},
		{"ReviewFix", RoleReviewFix, stencils.PatternDirectiveReviewFix},
		{"Orchestrator", RoleOrchestrator, stencils.PatternDirectiveOrchestrator},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(l, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(active, %v) = _, %v; want nil error", tt.role, err)
			}
			want := stencil.StripLeadingComment(string(tt.embeddedDefault))
			if got != want {
				t.Errorf("Directive(active, %v) = %q; want %q (StripLeadingComment of the embedded default)", tt.role, got, want)
			}
		})
	}
}
