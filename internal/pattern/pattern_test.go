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
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// writePatternFile creates root/PATTERN.md with the given content, failing the test on any error.
func writePatternFile(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "PATTERN.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v", err)
	}
}

// newTestStencilsDir returns a seeded stencils directory.
// Seeding runs `stencilstore.Reconcile`, so the files carry the real leading banner, a raw fixture would make the banner-strip test (see TestDirective_StripsBanner) vacuous and let a missing strip pass green.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// TestDirective_ActiveWithFile covers the common active case — PATTERN.md present as a regular file
// — for all three roles.
func TestDirective_ActiveWithFile(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "# PATTERN\n\nsome constraints\n")
	stencilsDir := newTestStencilsDir(t)

	tests := []struct {
		name string
		role Role
	}{
		{"Implementer", RoleImplementer},
		{"ReviewFix", RoleReviewFix},
		{"Orchestrator", RoleOrchestrator},
		{"Designer", RoleDesigner},
		{"Judge", RoleJudge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(root, stencilsDir, tt.role)
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
			got, err := Directive(root, newTestStencilsDir(t), RoleImplementer)
			if err != nil {
				t.Fatalf("Directive(%s, RoleImplementer) = _, %v; want nil error", tt.name, err)
			}
			if got != "" {
				t.Errorf("Directive(%s, RoleImplementer) = %q; want \"\"", tt.name, got)
			}
		})
	}
}

// TestDirective_WhitespaceOnlyPatternFileIsInactive pins the inactive rule for content: an empty
// file and a whitespace-only one both render nothing, since neither carries an overview to inline.
func TestDirective_WhitespaceOnlyPatternFileIsInactive(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"Empty", ""},
		{"Whitespace", " \n\t\r\n  \n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writePatternFile(t, root, tt.content)

			got, err := Directive(root, newTestStencilsDir(t), RoleImplementer)
			if err != nil {
				t.Fatalf("Directive(%s PATTERN.md) = _, %v; want nil error", tt.name, err)
			}
			if got != "" {
				t.Errorf("Directive(%s PATTERN.md) = %q; want \"\"", tt.name, got)
			}
		})
	}
}

// TestDirective_OverviewInlinedVerbatim pins that the file's content reaches every role's directive
// unchanged, malformed content included: format violations are the checker's job, not Directive's.
func TestDirective_OverviewInlinedVerbatim(t *testing.T) {
	overview := "# PATTERN\n\n- PATTERN-one: a rule\nnot a bullet {{.x}} <!-- stray -->\n"
	root := t.TempDir()
	writePatternFile(t, root, overview)
	stencilsDir := newTestStencilsDir(t)

	for name, role := range map[string]Role{"Implementer": RoleImplementer, "ReviewFix": RoleReviewFix, "Orchestrator": RoleOrchestrator, "Designer": RoleDesigner, "Judge": RoleJudge} {
		t.Run(name, func(t *testing.T) {
			got, err := Directive(root, stencilsDir, role)
			if err != nil {
				t.Fatalf("Directive(%v) = _, %v; want nil error", role, err)
			}
			if !strings.Contains(got, overview) {
				t.Errorf("Directive(%v) = %q; want it to contain the overview verbatim %q", role, got, overview)
			}
		})
	}
}

// TestDirective_UnreadablePatternFileIsError pins that a read failure on an existing file is an
// error, simulated through the readFile seam because a real permission failure is not portable.
func TestDirective_UnreadablePatternFileIsError(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")

	original := readFile
	readFile = func(name string) ([]byte, error) {
		return nil, &os.PathError{Op: "read", Path: name, Err: errors.New("permission denied")}
	}
	t.Cleanup(func() { readFile = original })

	got, err := Directive(root, newTestStencilsDir(t), RoleImplementer)
	if err == nil {
		t.Fatalf("Directive(unreadable PATTERN.md) = %q, nil; want a non-nil error", got)
	}
}

// TestDirective_PatternFileAsDirectoryIsInactive pins the "PATTERN.md as a directory counts as
// inactive" edge rule: a directory in that place is not a readable index.
func TestDirective_PatternFileAsDirectoryIsInactive(t *testing.T) {
	root := t.TempDir()
	patternFileAsDir := filepath.Join(root, "PATTERN.md")
	if err := os.MkdirAll(patternFileAsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", patternFileAsDir, err)
	}

	got, err := Directive(root, newTestStencilsDir(t), RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(PATTERN.md as directory) = _, %v; want nil error", err)
	}
	if got != "" {
		t.Errorf("Directive(PATTERN.md as directory) = %q; want \"\"", got)
	}
}

// TestDirective_EmptyRoot pins the empty-root guard: several Deps structs are assembled
// field-by-field by CLI callers that could leave the root unset,
// and an unguarded resolution here would take down all five agent paths for a slip unrelated to
// PATTERN.
// The return-value assertion alone would be insufficient: an inactive PATTERN returns the same
// ("", nil) pair, so only the absence of the stat distinguishes the guard from a cwd-dependent
// lookalike.
func TestDirective_EmptyRoot(t *testing.T) {
	statAttempted := false
	original := statFile
	statFile = func(name string) (os.FileInfo, error) {
		statAttempted = true
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { statFile = original })

	got, err := Directive("", newTestStencilsDir(t), RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(\"\", RoleImplementer) = _, %v; want nil error", err)
	}
	if got != "" {
		t.Errorf("Directive(\"\", RoleImplementer) = %q; want \"\"", got)
	}
	if statAttempted {
		t.Error("Directive(\"\", RoleImplementer) attempted a stat; want no read attempted at all")
	}
}

// TestDirective_UnknownRole pins the documented unknown/zero Role behaviour: no directive text,
// even when PATTERN is active.
func TestDirective_UnknownRole(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
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
			got, err := Directive(root, stencilsDir, tt.role)
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
// and that each carries the literal relative background pointer "pattern/" — never an interpolated
// absolute path, which would make the value vary per worktree.
func TestDirective_VariantsArePairwiseDistinct(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	stencilsDir := newTestStencilsDir(t)

	implementerText, err := Directive(root, stencilsDir, RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(active, RoleImplementer) = _, %v; want nil error", err)
	}
	reviewFixText, err := Directive(root, stencilsDir, RoleReviewFix)
	if err != nil {
		t.Fatalf("Directive(active, RoleReviewFix) = _, %v; want nil error", err)
	}
	orchestratorText, err := Directive(root, stencilsDir, RoleOrchestrator)
	if err != nil {
		t.Fatalf("Directive(active, RoleOrchestrator) = _, %v; want nil error", err)
	}

	designerText, err := Directive(root, stencilsDir, RoleDesigner)
	if err != nil {
		t.Fatalf("Directive(active, RoleDesigner) = _, %v; want nil error", err)
	}

	judgeText, err := Directive(root, stencilsDir, RoleJudge)
	if err != nil {
		t.Fatalf("Directive(active, RoleJudge) = _, %v; want nil error", err)
	}

	variants := map[Role]string{
		RoleImplementer:  implementerText,
		RoleReviewFix:    reviewFixText,
		RoleOrchestrator: orchestratorText,
		RoleDesigner:     designerText,
		RoleJudge:        judgeText,
	}
	for role, text := range variants {
		if !strings.Contains(text, "under pattern/ ") {
			t.Errorf("Directive(%v) does not contain the literal background pointer pattern/: %q", role, text)
		}
		if strings.Contains(text, "_lyx/") {
			t.Errorf("Directive(%v) still names _lyx/: %q", role, text)
		}
		if !strings.Contains(text, patternDirName+"/") {
			t.Errorf("Directive(%v) does not carry patternDirName %q as its pointer: %q", role, patternDirName, text)
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
	if variants[RoleDesigner] == variants[RoleImplementer] {
		t.Error("RoleDesigner and RoleImplementer render identical directive text")
	}
	if variants[RoleDesigner] == variants[RoleReviewFix] {
		t.Error("RoleDesigner and RoleReviewFix render identical directive text")
	}
	if variants[RoleDesigner] == variants[RoleOrchestrator] {
		t.Error("RoleDesigner and RoleOrchestrator render identical directive text")
	}
	for _, other := range []Role{RoleImplementer, RoleReviewFix, RoleOrchestrator, RoleDesigner} {
		if variants[RoleJudge] == variants[other] {
			t.Errorf("RoleJudge and role %v render identical directive text", other)
		}
	}
}

// TestDirective_VariantsBeginWithOwnHeading pins that each variant carries its own "##" heading
// inline, so an inactive render leaves no orphan heading behind in the surrounding prompt template.
func TestDirective_VariantsBeginWithOwnHeading(t *testing.T) {
	root := t.TempDir()
	writePatternFile(t, root, "content")
	stencilsDir := newTestStencilsDir(t)

	tests := []struct {
		name string
		role Role
	}{
		{"Implementer", RoleImplementer},
		{"ReviewFix", RoleReviewFix},
		{"Orchestrator", RoleOrchestrator},
		{"Designer", RoleDesigner},
		{"Judge", RoleJudge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(root, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(%v) = _, %v; want nil error", tt.role, err)
			}
			if !strings.HasPrefix(got, "## ") {
				t.Errorf("Directive(%v) does not begin with its own \"##\" heading: %q", tt.role, got)
			}
		})
	}
}

// TestDirective_ReadsOnlyTheGivenRoot is the regression guard for the root the overview is read
// from: Directive reads <root>/PATTERN.md and nothing else, so a subdirectory of the root, such as a
// subpath anchor, never finds the overview planted at the root.
// Callers therefore pass the repository's worktree root, never the anchor path.
func TestDirective_ReadsOnlyTheGivenRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v", sub, err)
	}
	stencilsDir := newTestStencilsDir(t)
	writePatternFile(t, root, "content")

	got, err := Directive(sub, stencilsDir, RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(sub) = _, %v; want nil error", err)
	}
	if got != "" {
		t.Errorf("Directive(sub) found the root-planted PATTERN.md; got %q, want \"\"", got)
	}

	got, err = Directive(root, stencilsDir, RoleImplementer)
	if err != nil {
		t.Fatalf("Directive(root) = _, %v; want nil error", err)
	}
	if got == "" {
		t.Error("Directive(root) did not find PATTERN.md planted at the root")
	}
}

// TestDirective_NonNotExistStatErrorIsError pins the stat edge rule: a stat error that is not
// os.IsNotExist (a permission or I/O failure) is an error, not an inactive PATTERN.
// This is simulated through the package-level statFile seam rather than a real unreadable-directory
// trick, because an actual permission-denied stat error is not portable — it depends on the OS and
// on whether the test process runs elevated (e.g.
// as root in a container, where POSIX permission bits are not enforced), and Windows has no
// equivalent lever at all.
func TestDirective_NonNotExistStatErrorIsError(t *testing.T) {
	root := t.TempDir()

	original := statFile
	statFile = func(name string) (os.FileInfo, error) {
		return nil, &os.PathError{Op: "stat", Path: name, Err: errors.New("permission denied")}
	}
	t.Cleanup(func() { statFile = original })

	got, err := Directive(root, newTestStencilsDir(t), RoleImplementer)
	if err == nil {
		t.Fatalf("Directive() with a non-IsNotExist stat error = %q, nil; want a non-nil error", got)
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
		got, err := Directive(root, missingStencilsDir, RoleImplementer)
		if err != nil {
			t.Fatalf("Directive(inactive, missing stencilsDir) = _, %v; want nil error", err)
		}
		if got != "" {
			t.Errorf("Directive(inactive, missing stencilsDir) = %q; want \"\"", got)
		}
	})

	t.Run("empty root, stencilsDir does not exist", func(t *testing.T) {
		got, err := Directive("", missingStencilsDir, RoleImplementer)
		if err != nil {
			t.Fatalf("Directive(\"\", missing stencilsDir) = _, %v; want nil error", err)
		}
		if got != "" {
			t.Errorf("Directive(\"\", missing stencilsDir) = %q; want \"\"", got)
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
	emptyStencilsDir := t.TempDir()

	tests := []struct {
		name    string
		role    Role
		stencil string
	}{
		{"Implementer", RoleImplementer, implementerDirectiveStencil},
		{"ReviewFix", RoleReviewFix, reviewFixDirectiveStencil},
		{"Orchestrator", RoleOrchestrator, orchestratorDirectiveStencil},
		{"Designer", RoleDesigner, designerDirectiveStencil},
		{"Judge", RoleJudge, judgeDirectiveStencil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(root, emptyStencilsDir, tt.role)
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
	stencilsDir := newTestStencilsDir(t)

	tests := []struct {
		name string
		role Role
	}{
		{"Implementer", RoleImplementer},
		{"ReviewFix", RoleReviewFix},
		{"Orchestrator", RoleOrchestrator},
		{"Designer", RoleDesigner},
		{"Judge", RoleJudge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(root, stencilsDir, tt.role)
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
// that default with its banner stripped and its overview marker filled — never
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
	stencilsDir := newTestStencilsDir(t)

	tests := []struct {
		name            string
		role            Role
		embeddedDefault []byte
	}{
		{"Implementer", RoleImplementer, stencils.PatternDirectiveImplementer},
		{"ReviewFix", RoleReviewFix, stencils.PatternDirectiveReviewFix},
		{"Orchestrator", RoleOrchestrator, stencils.PatternDirectiveOrchestrator},
		{"Designer", RoleDesigner, stencils.PatternDirectiveDesigner},
		{"Judge", RoleJudge, stencils.PatternDirectiveJudge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(root, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(active, %v) = _, %v; want nil error", tt.role, err)
			}
			filled, err := stencil.Fill([]byte(stencil.StripLeadingComment(string(tt.embeddedDefault))), map[string]string{"pattern_overview": "content"})
			if err != nil {
				t.Fatalf("Fill(embedded default, %v) = _, %v; want nil error", tt.role, err)
			}
			if want := string(filled); got != want {
				t.Errorf("Directive(active, %v) = %q; want %q (the embedded default, banner stripped and overview filled)", tt.role, got, want)
			}
		})
	}
}
