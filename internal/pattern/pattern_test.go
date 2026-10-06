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
// Seeding runs `stencilstore.Reconcile`, so the files carry the real leading banner, a raw fixture would make the banner-strip check in TestDirective_PerRole vacuous and let a missing strip pass green.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
}

// TestDirective_PerRole covers all five roles against an active PATTERN.md whose overview is deliberately malformed, since format violations are the checker's job, not Directive's.
// Each role's directive is non-empty, carries the overview verbatim, begins with its own "##" heading (so an inactive render leaves no orphan heading behind), and holds no leftover banner:
// stencilstore.Read does not strip a leading banner and the value never passes through stencil.Fill's banner strip, so Directive itself must strip it, and the seeded fixture's real banner makes that check bite.
// The directive points at the literal relative background directory, never an interpolated absolute path or `_lyx/`, and equals the embedded default with its banner stripped and its overview marker filled.
// That last check is a cheap tripwire against a stencil file drifting from the embedded default, not proof a relocation was faithful.
// The five directives are pairwise distinct.
// Against a stencils directory that carries none of the pattern stencils, each role fails loud with a non-nil error naming its missing stencil.
// The role subtests run serially so the parent can compare their texts.
func TestDirective_PerRole(t *testing.T) {
	t.Parallel()
	overview := "# PATTERN\n\n- PATTERN-one: a rule\nnot a bullet {{.x}} <!-- stray -->\n"
	root := t.TempDir()
	writePatternFile(t, root, overview)
	stencilsDir := newTestStencilsDir(t)
	emptyStencilsDir := t.TempDir()

	tests := []struct {
		name            string
		role            Role
		embeddedDefault []byte
		stencil         string
	}{
		{"Implementer", RoleImplementer, stencils.PatternDirectiveImplementer, implementerDirectiveStencil},
		{"ReviewFix", RoleReviewFix, stencils.PatternDirectiveReviewFix, reviewFixDirectiveStencil},
		{"Orchestrator", RoleOrchestrator, stencils.PatternDirectiveOrchestrator, orchestratorDirectiveStencil},
		{"Designer", RoleDesigner, stencils.PatternDirectiveDesigner, designerDirectiveStencil},
		{"Judge", RoleJudge, stencils.PatternDirectiveJudge, judgeDirectiveStencil},
	}
	texts := make(map[string]string, len(tests))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Directive(root, stencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(active, %v) = _, %v; want nil error", tt.role, err)
			}
			texts[tt.name] = got
			if !strings.Contains(got, overview) {
				t.Errorf("Directive(%v) = %q; want it to contain the overview verbatim %q", tt.role, got, overview)
			}
			if !strings.HasPrefix(got, "## ") {
				t.Errorf("Directive(%v) = %q; want it to begin with its own \"##\" heading, not a leading banner", tt.role, got)
			}
			if strings.Contains(strings.Replace(got, overview, "", 1), "<!--") {
				t.Errorf("Directive(%v) = %q; want no leftover \"<!--\" banner content", tt.role, got)
			}
			if !strings.Contains(got, "under pattern/ ") || !strings.Contains(got, patternDirName+"/") {
				t.Errorf("Directive(%v) = %q; want the literal background pointer %q", tt.role, got, patternDirName+"/")
			}
			if strings.Contains(got, "_lyx/") {
				t.Errorf("Directive(%v) = %q; want no _lyx/ pointer", tt.role, got)
			}
			filled, err := stencil.Fill([]byte(stencil.StripLeadingComment(string(tt.embeddedDefault))), map[string]string{"pattern_overview": overview})
			if err != nil {
				t.Fatalf("Fill(embedded default, %v) = _, %v; want nil error", tt.role, err)
			}
			if want := string(filled); got != want {
				t.Errorf("Directive(%v) = %q; want %q (the embedded default, banner stripped and overview filled)", tt.role, got, want)
			}

			got, err = Directive(root, emptyStencilsDir, tt.role)
			if err == nil {
				t.Fatalf("Directive(active, %v, empty stencilsDir) = %q, nil; want a non-nil error", tt.role, got)
			}
			if !strings.Contains(err.Error(), tt.stencil) {
				t.Errorf("Directive(active, %v, empty stencilsDir) error = %q; want it to name the missing stencil %q", tt.role, err.Error(), tt.stencil)
			}
		})
	}

	for i, a := range tests {
		for _, b := range tests[i+1:] {
			if texts[a.name] != "" && texts[a.name] == texts[b.name] {
				t.Errorf("%s and %s render identical directive text", a.name, b.name)
			}
		}
	}
}

// TestDirective_RendersNothing pins every case that renders no directive and reads no stencil, asserted by pointing stencilsDir at a path that does not exist:
// a stray directory without PATTERN.md, nothing at all, an empty or whitespace-only file (neither carries an overview to inline), PATTERN.md as a directory (not a readable index), a subdirectory of a root that holds the overview (Directive reads <root>/PATTERN.md and nothing else, so callers pass the worktree root, never an anchor path), an empty root, and an unknown or zero Role even when PATTERN is active.
// An eager-read refactor that would break the engines' inactive-PATTERN fixtures fails here first, locally.
func TestDirective_RendersNothing(t *testing.T) {
	t.Parallel()
	missingStencilsDir := filepath.Join(t.TempDir(), "does-not-exist")

	tests := []struct {
		name string
		// setup prepares a fresh root and returns the root to pass to Directive.
		setup func(t *testing.T, root string) string
		role  Role
	}{
		{
			name: "DirPresentFileAbsent",
			setup: func(t *testing.T, root string) string {
				if err := os.MkdirAll(filepath.Join(root, "stray_dir"), 0o755); err != nil {
					t.Fatalf("MkdirAll = %v", err)
				}
				return root
			},
			role: RoleImplementer,
		},
		{
			name:  "NeitherPresent",
			setup: func(t *testing.T, root string) string { return root },
			role:  RoleImplementer,
		},
		{
			name: "EmptyFile",
			setup: func(t *testing.T, root string) string {
				writePatternFile(t, root, "")
				return root
			},
			role: RoleImplementer,
		},
		{
			name: "WhitespaceFile",
			setup: func(t *testing.T, root string) string {
				writePatternFile(t, root, " \n\t\r\n  \n")
				return root
			},
			role: RoleImplementer,
		},
		{
			name: "PatternFileAsDirectory",
			setup: func(t *testing.T, root string) string {
				if err := os.MkdirAll(filepath.Join(root, "PATTERN.md"), 0o755); err != nil {
					t.Fatalf("MkdirAll(PATTERN.md) = %v", err)
				}
				return root
			},
			role: RoleImplementer,
		},
		{
			name: "SubdirectoryOfRoot",
			setup: func(t *testing.T, root string) string {
				sub := filepath.Join(root, "sub", "dir")
				if err := os.MkdirAll(sub, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v", sub, err)
				}
				writePatternFile(t, root, "content")
				return sub
			},
			role: RoleImplementer,
		},
		{
			name:  "EmptyRoot",
			setup: func(t *testing.T, root string) string { return "" },
			role:  RoleImplementer,
		},
		{
			name: "ZeroRole",
			setup: func(t *testing.T, root string) string {
				writePatternFile(t, root, "content")
				return root
			},
			role: Role(0),
		},
		{
			name: "OutOfRangeRole",
			setup: func(t *testing.T, root string) string {
				writePatternFile(t, root, "content")
				return root
			},
			role: Role(99),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := tt.setup(t, t.TempDir())

			got, err := Directive(root, missingStencilsDir, tt.role)
			if err != nil {
				t.Fatalf("Directive(%s, %v) = _, %v; want nil error", tt.name, tt.role, err)
			}
			if got != "" {
				t.Errorf("Directive(%s, %v) = %q; want \"\"", tt.name, tt.role, got)
			}
		})
	}
}

// TestDirective_FilesystemFailuresAreErrors pins that a stat error that is not os.IsNotExist, and a read failure on an existing file, are errors, not an inactive PATTERN.
// Both are simulated through the package-level statFile and readFile seams, because a real permission failure is not portable:
// it depends on the OS and on whether the test process runs elevated (as root in a container POSIX permission bits are not enforced), and Windows has no equivalent lever at all.
// The seams are process-global, so this test stays serial.
func TestDirective_FilesystemFailuresAreErrors(t *testing.T) {
	permissionDenied := func(op, name string) error {
		return &os.PathError{Op: op, Path: name, Err: errors.New("permission denied")}
	}
	tests := []struct {
		name string
		swap func(t *testing.T)
	}{
		{
			name: "NonNotExistStatError",
			swap: func(t *testing.T) {
				original := statFile
				statFile = func(name string) (os.FileInfo, error) { return nil, permissionDenied("stat", name) }
				t.Cleanup(func() { statFile = original })
			},
		},
		{
			name: "UnreadablePatternFile",
			swap: func(t *testing.T) {
				original := readFile
				readFile = func(name string) ([]byte, error) { return nil, permissionDenied("read", name) }
				t.Cleanup(func() { readFile = original })
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writePatternFile(t, root, "content")
			tt.swap(t)

			got, err := Directive(root, newTestStencilsDir(t), RoleImplementer)
			if err == nil {
				t.Fatalf("Directive(%s) = %q, nil; want a non-nil error", tt.name, got)
			}
		})
	}
}

// TestDirective_EmptyRoot pins the empty-root guard: several Deps structs are assembled field-by-field by CLI callers that could leave the root unset, and an unguarded resolution here would take down all five agent paths for a slip unrelated to PATTERN.
// The return-value assertion alone would be insufficient: an inactive PATTERN returns the same ("", nil) pair, so only the absence of the stat distinguishes the guard from a cwd-dependent lookalike.
// The stat seam is process-global, so this test stays serial.
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
