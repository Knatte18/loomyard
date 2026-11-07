// index_test.go covers the checks the plan index appends beside planparser's entry points.

package planglyph

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// noFabricReference is a FabricReferenceMatcher that never matches.
type noFabricReference struct{}

func (noFabricReference) MatchPath(string) (string, bool)     { return "", false }
func (noFabricReference) MatchSpelling(string) (string, bool) { return "", false }

// TestIndex_AppendsPlanChecks asserts the index reports two checks in its format, rework and dispatch validations.
// `fabric-vocabulary-name` flags a declared host-geometry name, and `edit-test-file` flags a test file under Edit.
// A completed card is dropped at dispatch for the first check;
// for the second, it is dropped at format validation, which dispatch never runs.
// A test file the repository holds with no symbol is admitted;
// a file with a member, a file the repository lacks, and a file quarry cannot parse are rejected.
func TestIndex_AppendsPlanChecks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	declaring := func(handle, decl string) planparser.Card {
		return planparser.Card{
			Number:       1,
			Slug:         "one",
			Declarations: []planparser.CardDeclaration{{Handle: handle, Decl: decl}},
		}
	}
	editingTestFile := planparser.Card{
		Number:       1,
		Slug:         "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/x/x_test.go#"}}},
	}
	countFindings := func(t *testing.T, findings []Finding, check string) int {
		t.Helper()
		count := 0
		for _, finding := range findings {
			if finding.Check == check {
				count++
			}
		}
		return count
	}

	offending := declaring("plan:internal/x#hostBranch", "func hostBranch()")
	plain := declaring("plan:internal/x#plainName", "func plainName()")
	idx := NewIndex(noFabricReference{})

	tests := []struct {
		name string
		// files, when set, makes the worktree root a repository holding them; it is an empty directory otherwise.
		files    map[string]string
		run      func(plan *planparser.Plan, root string) ([]Finding, error)
		check    string
		language string
		card     planparser.Card
		want     int
	}{
		{
			name: "format with no done card",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, nil)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     1,
		},
		{
			name: "rework told the plan's first card",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateRework(plan, root, plan.FirstCard)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     1,
		},
		{
			name: "dispatch with the card forthcoming",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateDispatch(plan, root, nil, plan.Cards)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     1,
		},
		{
			name: "dispatch with the card completed",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateDispatch(plan, root, plan.Cards, nil)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     0,
		},
		{
			name: "format over a plain name",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, nil)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     plain,
			want:     0,
		},
		{
			name: "format over a test file under Edit",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, nil)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     1,
		},
		{
			name: "rework over a test file under Edit",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateRework(plan, root, plan.FirstCard)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     1,
		},
		{
			name:  "format over a test file with no symbol",
			files: map[string]string{"internal/x/x_test.go": "package x\n\nvar _ = 1\n"},
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, nil)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     0,
		},
		{
			name:  "rework over a test file with no symbol",
			files: map[string]string{"internal/x/x_test.go": "package x\n\nvar _ = 1\n"},
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateRework(plan, root, plan.FirstCard)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     0,
		},
		{
			name:  "format over a test file with a member",
			files: map[string]string{"internal/x/x_test.go": "package x\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n"},
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, nil)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     1,
		},
		{
			name:  "format over a test file the repository lacks",
			files: map[string]string{"internal/x/other.go": "package x\n"},
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, nil)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     1,
		},
		{
			name:  "format over a test file with a syntax error",
			files: map[string]string{"internal/x/x_test.go": "package x\n\nfunc (\n"},
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, nil)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     1,
		},
		{
			name: "dispatch over a test file under Edit",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateDispatch(plan, root, nil, plan.Cards)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     0,
		},
		{
			name: "format with the test-file card done",
			run: func(plan *planparser.Plan, root string) ([]Finding, error) {
				return idx.ValidateFormat(plan, root, plan.Cards)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan := minimalPlan(t, t.TempDir())
			plan.Language = tc.language
			plan.FirstCard = 1
			plan.Cards = []planparser.Card{tc.card}
			root := t.TempDir()
			if tc.files != nil {
				root = writeFixtureRepo(t, tc.files)
			}

			findings, _ := tc.run(plan, root)

			if got := countFindings(t, findings, tc.check); got != tc.want {
				t.Errorf("%s findings = %d, want %d (all: %v)", tc.check, got, tc.want, findings)
			}
		})
	}
}

// TestResolveGlyphs asserts the index answers each glyph by the same status policy a card ref gets.
// The answers keep the glyphs' order, and a quarry failure comes back as an error.
func TestResolveGlyphs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := writeFixtureRepo(t, map[string]string{
		"go.mod":         "module example.com/m\n\ngo 1.22\n",
		"p/t.go":         "package p\n\ntype T struct{}\n",
		"p/f_linux.go":   "package p\n\nfunc F() {}\n",
		"p/f_windows.go": "package p\n\nfunc F() {}\n",
		"p/g1.go":        "package p\n\nfunc G() {}\n",
		"p/g2.go":        "package p\n\nfunc G() {}\n",
	})
	idx := NewIndex(noFabricReference{})

	glyphs := []string{"p#T", "p#F", "p#G", "p#Missing", "nosuch#X", "p#bad#x"}
	answers, err := idx.ResolveGlyphs(root, glyphs)
	if err != nil {
		t.Fatalf("ResolveGlyphs() error = %v; want nil", err)
	}
	if len(answers) != len(glyphs) {
		t.Fatalf("ResolveGlyphs() returned %d answers; want %d", len(answers), len(glyphs))
	}

	tests := []struct {
		verdict        planindex.GlyphVerdict
		files          []string
		candidateFiles []string
	}{
		{verdict: planindex.GlyphFound, files: []string{"p/t.go"}},
		{verdict: planindex.GlyphFound, files: []string{"p/f_linux.go", "p/f_windows.go"}},
		{verdict: planindex.GlyphAmbiguous, candidateFiles: []string{"p/g1.go", "p/g2.go"}},
		{verdict: planindex.GlyphMissing},
		{verdict: planindex.GlyphMissing},
		{verdict: planindex.GlyphMissing},
	}
	for i, want := range tests {
		got := answers[i]
		if got.Glyph != glyphs[i] || got.Verdict != want.verdict {
			t.Errorf("answer %d = %+v; want glyph %q and verdict %q", i, got, glyphs[i], want.verdict)
		}
		gotFiles := slices.Clone(got.Files)
		slices.Sort(gotFiles)
		if !slices.Equal(gotFiles, want.files) {
			t.Errorf("answer %d Files = %v; want %v", i, got.Files, want.files)
		}
		if len(got.Candidates) != len(want.candidateFiles) {
			t.Errorf("answer %d Candidates = %v; want one per file of %v", i, got.Candidates, want.candidateFiles)
			continue
		}
		for _, file := range want.candidateFiles {
			if !slices.ContainsFunc(got.Candidates, func(candidate string) bool { return strings.Contains(candidate, file) }) {
				t.Errorf("answer %d Candidates = %v; want one naming %q", i, got.Candidates, file)
			}
		}
	}

	answers, err = idx.ResolveGlyphs(filepath.Join(t.TempDir(), "absent"), glyphs)
	if !errors.Is(err, planindex.ErrQuarryUnavailable) || answers != nil {
		t.Errorf("ResolveGlyphs(absent root) = %v, %v; want no answers and an error wrapping ErrQuarryUnavailable", answers, err)
	}
}
