// deleteorder_test.go covers LaterDeleteReferences against a fixture repository: which delete/edit pairs it refuses and which reference shapes it matches.

package planglyph

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
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
		// earlier are passed beside later, numbered before the deleting card.
		earlier []planparser.Card
		// extraFiles are written beside deleteOrderFixture.
		extraFiles map[string]string
		// wantRefs lists "<severity> <location>" for each expected finding, all attributed to the deleting card: "<file>:<line>" for a blocking one and "<file> at line <n>" for an informational one.
		wantRefs []string
	}{
		{
			name:     "a later card's Edit that still calls the deleted function is refused",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub#Bar"),
			wantRefs: []string{"blocking sub/b.go:3"},
		},
		{
			name:     "the delete numbered after the editing card passes",
			deleting: groupCard(2, "del", planparser.CardTypeDelete, "sub#Foo"),
			later:    groupCard(1, "edit", planparser.CardTypeEdit, "sub#Bar"),
		},
		{
			name: "a reference the deleting card's own Edit also covers is not reported",
			deleting: planparser.Card{Number: 1, Slug: "del", TargetGroups: []planparser.TargetGroup{
				{Type: planparser.CardTypeDelete, Refs: []string{"sub#Foo"}},
				{Type: planparser.CardTypeEdit, Refs: []string{"sub#Bar"}},
			}},
			later: groupCard(2, "edit", planparser.CardTypeEdit, "sub/b.go#"),
		},
		{
			name:     "a reference an earlier card's Edit also covers is not reported",
			deleting: groupCard(2, "del", planparser.CardTypeDelete, "sub#Foo"),
			later:    groupCard(3, "edit", planparser.CardTypeEdit, "sub#Bar"),
			earlier:  []planparser.Card{groupCard(1, "rewrite", planparser.CardTypeEdit, "sub#Bar")},
		},
		{
			name: "a reference only a later card's Edit covers is reported beside the deleting card's own Edit",
			deleting: planparser.Card{Number: 1, Slug: "del", TargetGroups: []planparser.TargetGroup{
				{Type: planparser.CardTypeDelete, Refs: []string{"sub#Foo"}},
				{Type: planparser.CardTypeEdit, Refs: []string{"sub#Bar"}},
			}},
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "other#Use", "sub/b.go#"),
			wantRefs: []string{"blocking other/c.go:5"},
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
			wantRefs: []string{"blocking other/c.go:5"},
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
			wantRefs: []string{"blocking sub/b.go:3"},
		},
		{
			name:     "a method target matches as a selector",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#Thing.Run"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub#Call"),
			wantRefs: []string{"informational sub/d.go at line 3"},
		},
		{
			name:     "a package self glyph target matches an import of the package",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "other/c.go#"),
			wantRefs: []string{"blocking other/c.go:3"},
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
			wantRefs: []string{"blocking part/c.go:3"},
		},
		{
			name:     "a later card's Edit of a member partitioned by build constraints searches each declaration's span",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub#Each"),
			extraFiles: map[string]string{
				"sub/e.go": "//go:build linux\n\npackage sub\n\nfunc Each() { Foo() }\n",
				"sub/f.go": "//go:build !linux\n\npackage sub\n\nfunc Each() {}\n",
			},
			wantRefs: []string{"blocking sub/e.go:5"},
		},
		{
			name:     "a Go-less Edit target that quotes the deleted package's path is not searched",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "docs/notes.md#"),
			extraFiles: map[string]string{
				"docs/notes.md": "The package \"x/sub\" is going away.\n",
			},
		},
		{
			name:     "a comment and a string literal naming the deleted function are not references",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub/g.go#"),
			extraFiles: map[string]string{
				"sub/g.go": "package sub\n\n// Foo is only named in this comment and in the string below.\nvar note = \"Foo\"\n",
			},
		},
		{
			name:     "a reference under a testdata directory is not searched for a deleted function",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "other/testdata/x.go#"),
			extraFiles: map[string]string{
				"other/testdata/x.go": "package x\n\nimport \"x/sub\"\n\nfunc use() { sub.Foo() }\n",
			},
		},
		{
			name:     "an import under a testdata directory is not searched for a deleted package",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "other/testdata/x.go#"),
			extraFiles: map[string]string{
				"other/testdata/x.go": "package x\n\nimport \"x/sub\"\n\nfunc use() { sub.Foo() }\n",
			},
		},
		{
			name:     "a later Edit of the file declaring the deleted function reports its own declaration nowhere",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub/a.go#"),
		},
		{
			name:     "a later Edit of a Go file absent from disk finds nothing for a deleted function",
			deleting: deleteFoo,
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub/missing.go#"),
		},
		{
			name:     "a later Edit of a Go file absent from disk finds nothing for a deleted package",
			deleting: groupCard(1, "del", planparser.CardTypeDelete, "sub#"),
			later:    groupCard(2, "edit", planparser.CardTypeEdit, "sub/missing.go#"),
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

			got, err := LaterDeleteReferences(plan, []planparser.Card{tc.deleting}, append([]planparser.Card{tc.later}, tc.earlier...), root)
			if err != nil {
				t.Fatalf("LaterDeleteReferences(...) returned error: %v", err)
			}
			if sites := deleteOrderSites(got); !slices.Equal(sites, tc.wantRefs) {
				t.Errorf("LaterDeleteReferences(%s) sites = %q; want %q", tc.name, sites, tc.wantRefs)
			}
			for _, f := range got {
				if f.Check != "delete-before-reference" || f.Card != tc.deleting.ID() {
					t.Errorf("finding %+v; want a delete-before-reference on card %s", f, tc.deleting.ID())
				}
				for _, want := range []string{tc.later.ID(), tc.deleting.TargetGroups[0].Refs[0]} {
					if !strings.Contains(f.Detail, want) {
						t.Errorf("Detail = %q; want it to name %q", f.Detail, want)
					}
				}
			}
		})
	}
}

// blockingDeleteOrderSite and informationalDeleteOrderSite pick the location out of a delete-before-reference detail.
var (
	blockingDeleteOrderSite      = regexp.MustCompile(`still references it at (\S+:\d+) in its Edit code`)
	informationalDeleteOrderSite = regexp.MustCompile(`follows a dot in (.+), in the Edit code of card`)
)

// deleteOrderSites returns, sorted, "<severity> <location>" for each delete-before-reference finding of findings, the location read from its detail: "<file>:<line>" when blocking and "<file> at line <n>" when informational.
// A finding whose detail holds no location yields its whole detail, so a changed wording fails the comparison instead of passing unseen.
func deleteOrderSites(findings []Finding) []string {
	var sites []string
	for _, f := range findings {
		if f.Check != "delete-before-reference" {
			continue
		}
		pattern := blockingDeleteOrderSite
		if f.Severity == SeverityInformational {
			pattern = informationalDeleteOrderSite
		}
		location := f.Detail
		if match := pattern.FindStringSubmatch(f.Detail); match != nil {
			location = match[1]
		}
		sites = append(sites, fmt.Sprintf("%s %s", f.Severity, location))
	}
	slices.Sort(sites)
	return sites
}
