// donecheck_resolve_test.go covers DoneChecks against fixture repositories, since it resolves against the actual on-disk tree rather than a hand-built []quarry.ResolveResult.
// The fixtures are plain files that quarry reads; nothing here spawns git.

package planglyph

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestDoneChecks_Verdicts covers the done-checks of Create, Delete and Rename groups against the tree the fork left behind: a landed Create, Delete or Rename passes, and a Create that never landed, a Delete that survived or a Rename never performed reports its blocking finding.
// A Rename's new side carries the canonical plan: handle spelling a validated plan's symbol rename always has, proving resolveKeyFor strips it.
// A fork that never performed its declared Rename leaves the old side still resolving and the new side still unresolvable, and BOTH halves must report rename-not-done — a check inspecting only Create and Delete groups recorded that card clean.
// A target no glyph spells, a bare root filename or a path whose directory holds a space, is judged by whether its path exists, and a missing Create names the unit's files and where else its name is declared.
func TestDoneChecks_Verdicts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		files map[string]string
		group planparser.TargetGroup
		// wantChecks is the check of each expected finding, all on card 1-one; empty means no finding.
		wantChecks []string
		// wantDetail holds substrings of the first finding's detail.
		wantDetail []string
	}{
		{
			name:  "Create symbol landed",
			files: map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"sub#Foo"}},
		},
		{
			name:       "Create symbol missing",
			files:      map[string]string{"sub/a.go": "package sub\n"},
			group:      planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"sub#Foo"}},
			wantChecks: []string{"create-not-done"},
			wantDetail: []string{"no declaration named Foo exists anywhere in the worktree"},
		},
		{
			// The finding names the unit's files and where the name was declared instead, so a misplaced declaration reads apart from a missing one.
			name: "Create symbol declared in another unit",
			files: map[string]string{
				"sub/a.go":   "package sub\n\nfunc Bar() {}\n",
				"other/b.go": "package other\n\nfunc Foo() {}\n",
			},
			group:      planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"sub#Foo"}},
			wantChecks: []string{"create-not-done"},
			wantDetail: []string{"quarry searched the unit's files sub/a.go; Foo is declared as other#Foo in other/b.go"},
		},
		{
			name: "Create test in external test package landed",
			files: map[string]string{
				"sub/a.go":      "package sub\n",
				"sub/a_test.go": "package sub_test\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\n",
			},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"plan:sub_test#TestFoo"}},
		},
		{
			name:  "Create root filename self glyph landed",
			files: map[string]string{"sub/a.go": "package sub\n", "LICENSE": "licence text\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"LICENSE#"}},
		},
		{
			// A path-shaped key that reached the done-check uncanonicalized is judged on disk.
			name:  "Create bare root filename landed",
			files: map[string]string{"sub/a.go": "package sub\n", "LICENSE": "licence text\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"LICENSE"}},
		},
		{
			// No glyph spells a path whose directory holds a space, so it is checked on disk.
			name:  "Create spaced path landed",
			files: map[string]string{"sub/a.go": "package sub\n", "tree/with space/Spaced.cs": "namespace Tree;\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"tree/with space/Spaced.cs"}},
		},
		{
			name:       "Create spaced path missing",
			files:      map[string]string{"sub/a.go": "package sub\n"},
			group:      planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"tree/with space/Spaced.cs"}},
			wantChecks: []string{"create-not-done"},
			wantDetail: []string{`checked "tree/with space/Spaced.cs" on disk, since no glyph spells it, and nothing exists there`},
		},
		{
			name:  "Delete spaced path gone",
			files: map[string]string{"sub/a.go": "package sub\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: []string{"tree/with space/Spaced.cs"}},
		},
		{
			// A parent component that is now a regular file stats as not-a-directory, which reads as nothing existing there.
			name:  "Delete spaced path under a file parent gone",
			files: map[string]string{"sub/a.go": "package sub\n", "tree/with space": "now a file\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: []string{"tree/with space/Spaced.cs"}},
		},
		{
			name:       "Delete spaced path survives",
			files:      map[string]string{"sub/a.go": "package sub\n", "tree/with space/Spaced.cs": "namespace Tree;\n"},
			group:      planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: []string{"tree/with space/Spaced.cs"}},
			wantChecks: []string{"delete-not-done"},
			wantDetail: []string{`checked "tree/with space/Spaced.cs" on disk, since no glyph spells it, and it exists`},
		},
		{
			name:  "Rename spaced path landed",
			files: map[string]string{"sub/a.go": "package sub\n", "tree/with space/New.cs": "namespace Tree;\n"},
			group: planparser.TargetGroup{
				Type:  planparser.CardTypeRename,
				Refs:  []string{"tree/with space/Old.cs", "tree/with space/New.cs"},
				Pairs: []planparser.MovePair{{Old: "tree/with space/Old.cs", New: "tree/with space/New.cs"}},
			},
		},
		{
			name:  "Rename spaced path skipped",
			files: map[string]string{"sub/a.go": "package sub\n", "tree/with space/Old.cs": "namespace Tree;\n"},
			group: planparser.TargetGroup{
				Type:  planparser.CardTypeRename,
				Refs:  []string{"tree/with space/Old.cs", "tree/with space/New.cs"},
				Pairs: []planparser.MovePair{{Old: "tree/with space/Old.cs", New: "tree/with space/New.cs"}},
			},
			wantChecks: []string{"rename-not-done", "rename-not-done"},
			wantDetail: []string{`checked "tree/with space/Old.cs" on disk, since no glyph spells it, and it exists`},
		},
		{
			// The canonical plan: handle resolves via its stripped expected-glyph half, not the literal token.
			name:  "Create handle stripped",
			files: map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"plan:sub#Foo"}},
		},
		{
			// A root .scratch/ draft is transient: its card may remove it once read.
			name:  "Create scratch draft removed",
			files: map[string]string{"sub/a.go": "package sub\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{".scratch/draft.md#"}},
		},
		{
			name:       "Create nested scratch file missing",
			files:      map[string]string{"sub/a.go": "package sub\n"},
			group:      planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"sub/.scratch/draft.md#"}},
			wantChecks: []string{"create-not-done"},
		},
		{
			name:  "Delete symbol gone",
			files: map[string]string{"sub/a.go": "package sub\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: []string{"sub#Foo"}},
		},
		{
			name:       "Delete symbol survives",
			files:      map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			group:      planparser.TargetGroup{Type: planparser.CardTypeDelete, Refs: []string{"sub#Foo"}},
			wantChecks: []string{"delete-not-done"},
		},
		{
			name:  "Rename landed",
			files: map[string]string{"sub/a.go": "package sub\n\nfunc Bar() {}\n"},
			group: planparser.TargetGroup{
				Type:  planparser.CardTypeRename,
				Refs:  []string{"sub#Foo", "plan:sub#Bar"},
				Pairs: []planparser.MovePair{{Old: "sub#Foo", New: "plan:sub#Bar"}},
			},
		},
		{
			name:  "Rename skipped",
			files: map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			group: planparser.TargetGroup{
				Type:  planparser.CardTypeRename,
				Refs:  []string{"sub#Foo", "plan:sub#Bar"},
				Pairs: []planparser.MovePair{{Old: "sub#Foo", New: "plan:sub#Bar"}},
			},
			wantChecks: []string{"rename-not-done", "rename-not-done"},
		},
		{
			// A file-rename pair has both sides as self glyphs.
			name:  "file Rename landed",
			files: map[string]string{"sub/b.go": "package sub\n\nfunc Foo() {}\n"},
			group: planparser.TargetGroup{
				Type:  planparser.CardTypeRename,
				Refs:  []string{"sub/a.go#", "sub/b.go#"},
				Pairs: []planparser.MovePair{{Old: "sub/a.go#", New: "sub/b.go#"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := writeFixtureRepo(t, tc.files)
			cards := []planparser.Card{{
				Number: 1, Slug: "one",
				TargetGroups: []planparser.TargetGroup{tc.group},
			}}

			got, err := DoneChecks(&planparser.Plan{}, cards, root)
			if err != nil {
				t.Fatalf("DoneChecks(...) returned error: %v", err)
			}
			if len(got) != len(tc.wantChecks) {
				t.Fatalf("DoneChecks(%s) = %+v; want %d finding(s) %v", tc.name, got, len(tc.wantChecks), tc.wantChecks)
			}
			for i, f := range got {
				if f.Check != tc.wantChecks[i] || f.Card != "1-one" {
					t.Errorf("finding %+v; want Check %s on card 1-one", f, tc.wantChecks[i])
				}
			}
			for _, want := range tc.wantDetail {
				if !strings.Contains(got[0].Detail, want) {
					t.Errorf("finding detail = %q; want it to contain %q", got[0].Detail, want)
				}
			}
		})
	}
}

// TestDoneChecks_QuarryUnavailable covers a quarry-unavailable root: the infrastructure error
// blocks the done-checks rather than passing them.
func TestDoneChecks_QuarryUnavailable(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cards := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"sub#Foo"}}},
	}}

	_, err := DoneChecks(&planparser.Plan{}, cards, missing)
	if !errors.Is(err, ErrQuarryUnavailable) {
		t.Errorf("DoneChecks(...) error = %v; want errors.Is(err, ErrQuarryUnavailable)", err)
	}
}
