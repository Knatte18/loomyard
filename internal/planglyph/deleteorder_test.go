// deleteorder_test.go covers LaterDeleteReferences against a fixture repository: which delete/edit pairs it refuses and which reference shapes it matches.

package planglyph

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// deleteOrderFixture declares Foo and Thing.Run in package sub, and uses them from sub, from another package through the import and from a same-named local.
// Line numbers matter: the table names the reference lines.
var deleteOrderFixture = map[string]string{
	"sub/a.go":   "package sub\n\nfunc Foo() {}\n\ntype Thing struct{}\n\nfunc (t Thing) Run() {}\n",
	"sub/b.go":   "package sub\n\nfunc Bar() { Foo() }\n\nfunc Baz() {}\n",
	"sub/d.go":   "package sub\n\nfunc Call(t Thing) { t.Run() }\n",
	"other/c.go": "package other\n\nimport \"x/sub\"\n\nfunc Use() { sub.Foo() }\n\nfunc Local() { Foo() }\n",
}

// groupCard returns a card whose single target group has the given type and refs.
func groupCard(number int, slug string, groupType planparser.CardType, refs ...string) planparser.Card {
	return planparser.Card{
		Number:       number,
		Slug:         slug,
		TargetGroups: []planparser.TargetGroup{{Type: groupType, Refs: refs}},
	}
}

func TestLaterDeleteReferences(t *testing.T) {
	t.Parallel()

	deleteFoo := groupCard(1, "del", planparser.CardTypeDelete, "sub#Foo")

	cases := []struct {
		name     string
		language string
		deleting planparser.Card
		later    planparser.Card
		// extraFiles are written beside deleteOrderFixture.
		extraFiles map[string]string
		// wantRefs is the file:line of each expected finding, all attributed to the deleting card.
		wantRefs []string
	}{
		{
			name:     "a later card's Edit that still calls the deleted function is refused",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub#Bar"),
			wantRefs: []string{"sub/b.go:3"},
		},
		{
			name:     "the delete numbered after the editing card passes",
			deleting: groupCard(2, "del", planparser.CardTypeDelete, "sub#Foo"),
			later:    groupCard(1, "edit", planparser.CardTypeEdit, "sub#Bar"),
		},
		{
			name:     "a reference outside the later card's member span is not searched",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub#Baz"),
		},
		{
			name:     "a package-qualified reference from another package matches",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "other#Use"),
			wantRefs: []string{"other/c.go:5"},
		},
		{
			name:     "an unqualified same name in another package does not match",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "other#Local"),
		},
		{
			name:     "a file self glyph Edit searches the whole file",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub/b.go#"),
			wantRefs: []string{"sub/b.go:3"},
		},
		{
			name:     "a method target matches as a selector",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#Thing.Run"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub#Call"),
			wantRefs: []string{"sub/d.go:3"},
		},
		{
			name:     "a package self glyph target matches an import of the package",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "other/c.go#"),
			wantRefs: []string{"other/c.go:3"},
		},
		{
			name:     "a file-path Delete target is not checked",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub/a.go"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub/b.go#"),
		},
		{
			name:     "a member target that no longer resolves is skipped",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#Gone"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub/b.go#"),
		},
		{
			name:     "a member partitioned by build constraints is checked through its declarations",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "part#Foo"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "part/c.go#"),
			extraFiles: map[string]string{
				"part/a.go": "//go:build linux\n\npackage part\n\nfunc Foo() {}\n",
				"part/b.go": "//go:build !linux\n\npackage part\n\nfunc Foo() {}\n",
				"part/c.go": "package part\n\nfunc Use() { Foo() }\n",
			},
			wantRefs: []string{"part/c.go:3"},
		},
		{
			name:     "a non-glyph language finds nothing and opens no repository",
			language: "none",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub#Bar"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := maps.Clone(deleteOrderFixture)
			maps.Copy(files, tc.extraFiles)
			root := writeFixtureRepo(t, files)
			if tc.language == "none" {
				root = filepath.Join(root, "does-not-exist")
			}
			plan := &planparser.Plan{Language: tc.language}

			got, err := LaterDeleteReferences(plan, []planparser.Card{tc.deleting}, []planparser.Card{tc.later}, root)
			if err != nil {
				t.Fatalf("LaterDeleteReferences(...) returned error: %v", err)
			}
			if len(got) != len(tc.wantRefs) {
				t.Fatalf("LaterDeleteReferences(%s) = %+v; want %d finding(s) at %v", tc.name, got, len(tc.wantRefs), tc.wantRefs)
			}
			for i, f := range got {
				if f.Check != "delete-before-reference" || f.Severity != SeverityBlocking || f.Card != tc.deleting.ID() {
					t.Errorf("finding %+v; want a blocking delete-before-reference on card %s", f, tc.deleting.ID())
				}
				for _, want := range []string{tc.wantRefs[i], tc.later.ID(), tc.deleting.TargetGroups[0].Refs[0]} {
					if !strings.Contains(f.Detail, want) {
						t.Errorf("Detail = %q; want it to name %q", f.Detail, want)
					}
				}
			}
		})
	}
}
