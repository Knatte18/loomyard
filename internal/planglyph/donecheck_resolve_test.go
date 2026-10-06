// donecheck_resolve_test.go covers DoneChecks against fixture repositories, since it resolves
// against the actual on-disk tree rather than a hand-built []quarry.ResolveResult.
// The fixtures are plain files that quarry reads; nothing here spawns git.

package planglyph

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestDoneChecks_Verdicts covers the done-checks of Create, Delete and Rename groups against the
// tree the fork left behind: a landed Create, Delete or Rename passes, and a Create that never
// landed, a Delete that survived or a Rename never performed reports its blocking finding.
// A Rename's new side carries the canonical plan: handle spelling a validated plan's symbol rename
// always has, proving resolveKeyFor strips it.
// A fork that never performed its declared Rename leaves the old side still resolving and the new
// side still unresolvable, and BOTH halves must report rename-not-done — a check inspecting only
// Create and Delete groups recorded that card clean.
func TestDoneChecks_Verdicts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		files map[string]string
		group planparser.TargetGroup
		// wantChecks is the check of each expected finding, all on card 1-one; empty means no finding.
		wantChecks []string
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
		},
		{
			// The canonical plan: handle resolves via its stripped expected-glyph half, not the literal token.
			name:  "Create handle stripped",
			files: map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"},
			group: planparser.TargetGroup{Type: planparser.CardTypeCreate, Refs: []string{"plan:sub#Foo"}},
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

// TestDoneChecks_RootFilenameCreateLanded is R9-1's planglyph-side regression: a Create group
// target naming a repository-root extensionless file must reach DoneChecks already canonicalized
// to its self glyph, which quarry resolves found once the file exists.
//
// Spelled as the bare token "LICENSE" — which is what the parser produced before R9-1's fix, and
// which classifyRef rule 4 explicitly admits as a legal card ref — quarry rejects the target
// BEFORE resolution ("a glyph needs a \"#\""), and the card is blocked forever, on every retry,
// even though it created exactly what it said it would. This test pins both halves: the glyph
// spelling passes, and the bare token is still the blocking refusal it always was, so the
// parser-side canonicalization is the thing keeping this correct. Since crucible round
// fable-high-r10's F1, the refusal is glyph-rejected — the fail-closed arm naming the rejection
// itself — rather than create-not-done misreading the rejection as "did not resolve".
func TestDoneChecks_RootFilenameCreateLanded(t *testing.T) {
	t.Parallel()

	root := writeFixtureRepo(t, map[string]string{
		"sub/a.go": "package sub\n",
		"LICENSE":  "licence text\n",
	})

	landed := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"LICENSE#"}}},
	}}
	got, err := DoneChecks(&planparser.Plan{}, landed, root)
	if err != nil {
		t.Fatalf("DoneChecks(canonicalized root filename) returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DoneChecks(canonicalized root filename) = %+v; want no findings", got)
	}

	bare := []planparser.Card{{
		Number: 1, Slug: "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"LICENSE"}}},
	}}
	stale, err := DoneChecks(&planparser.Plan{}, bare, root)
	if err != nil {
		t.Fatalf("DoneChecks(bare root filename) returned error: %v", err)
	}
	if len(stale) != 1 || stale[0].Check != "glyph-rejected" {
		t.Fatalf("DoneChecks(bare root filename) = %+v; want exactly one glyph-rejected — this is the failure canonicalization now prevents, named as the rejection it is", stale)
	}
}
