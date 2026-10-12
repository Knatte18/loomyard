// pack_test.go covers KickstartPack against fixture trees written to a temp directory.

package planglyph

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestKickstartPack covers which refs the pack selects and how each lands, resolved with its declarations or unresolved with its reason, and the empty and degraded packs.
func TestKickstartPack(t *testing.T) {
	t.Parallel()

	const source = "package sub\n\nfunc Foo() {}\n\nfunc Bar() {}\n\nfunc Baz() {}\n\nfunc Qux() {}\n" +
		"\nvar single = map[string]int{\n\t\"a\": 1,\n}\n" +
		"\nvar (\n\tfirst  = 1\n\tsecond = []int{\n\t\t1,\n\t}\n)\n" +
		"\nconst joined = \"a\" +\n\t\"b\"\n" +
		"\nconst (\n\tone = 1\n\ttwo = \"a\" +\n\t\t\"b\"\n)\n" +
		"\nfunc Long(\n\ta int,\n\tb int,\n) {}\n" +
		"\ntype T struct{}\n" +
		"\nfunc (T) LongMethod(\n\ta int,\n\tb int,\n) {}\n"
	declaration := func(start int, signature string) []planindex.PackDeclaration {
		return []planindex.PackDeclaration{{File: "sub/a.go", Start: start, End: start, Signature: signature}}
	}
	spanned := func(start, end int, signature string) []planindex.PackDeclaration {
		return []planindex.PackDeclaration{{File: "sub/a.go", Start: start, End: end, Signature: signature}}
	}
	group := func(cardType planparser.CardType, refs ...string) planparser.TargetGroup {
		return planparser.TargetGroup{Type: cardType, Refs: refs}
	}
	card := func(number int, uses []string, groups ...planparser.TargetGroup) planparser.Card {
		return planparser.Card{Number: number, Slug: "card", TargetGroups: groups, Uses: uses}
	}
	handleReason := "names a member a card creates and does not resolve yet"

	cases := []struct {
		name     string
		language string
		cards    []planparser.Card
		want     planindex.Pack
	}{
		{
			name: "Edit, Delete, Rename old side and Uses resolve with file, span and signature",
			cards: []planparser.Card{card(1, []string{"sub#Baz"},
				group(planparser.CardTypeEdit, "sub#Foo"),
				group(planparser.CardTypeDelete, "sub#Bar"),
				planparser.TargetGroup{
					Type:  planparser.CardTypeRename,
					Refs:  []string{"sub#Qux", "sub#Quux"},
					Pairs: []planparser.MovePair{{Old: "sub#Qux", New: "sub#Quux"}},
				},
			)},
			want: planindex.Pack{Resolved: []planindex.PackEntry{
				{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")},
				{Glyph: "sub#Bar", Declarations: declaration(5, "func Bar()")},
				{Glyph: "sub#Qux", Declarations: declaration(9, "func Qux()")},
				{Glyph: "sub#Baz", Declarations: declaration(7, "func Baz()")},
			}},
		},
		{
			name: "a ref repeated across cards appears once at its first position",
			cards: []planparser.Card{
				card(1, nil, group(planparser.CardTypeEdit, "sub#Foo", "sub#Bar")),
				card(2, []string{"sub#Foo"}, group(planparser.CardTypeEdit, "sub#Baz", "sub#Bar")),
			},
			want: planindex.Pack{Resolved: []planindex.PackEntry{
				{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")},
				{Glyph: "sub#Bar", Declarations: declaration(5, "func Bar()")},
				{Glyph: "sub#Baz", Declarations: declaration(7, "func Baz()")},
			}},
		},
		{
			name:  "self glyphs and plain paths are left out",
			cards: []planparser.Card{card(1, []string{"sub/a.go#"}, group(planparser.CardTypeEdit, "sub/a.go#", "docs/readme.md", "sub#Foo"))},
			want: planindex.Pack{Resolved: []planindex.PackEntry{
				{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")},
			}},
		},
		{
			name: "a Create target and a Rename new side are not resolved, their plan: handles are listed",
			cards: []planparser.Card{card(1, []string{"plan:sub#Later"},
				group(planparser.CardTypeCreate, "plan:sub#Made", "sub#Created"),
				planparser.TargetGroup{
					Type:  planparser.CardTypeRename,
					Refs:  []string{"sub#Qux", "plan:sub#Quux"},
					Pairs: []planparser.MovePair{{Old: "sub#Qux", New: "plan:sub#Quux"}},
				},
			)},
			want: planindex.Pack{
				Resolved: []planindex.PackEntry{{Glyph: "sub#Qux", Declarations: declaration(9, "func Qux()")}},
				Unresolved: []planindex.PackUnresolved{
					{Ref: "plan:sub#Made", Reason: handleReason},
					{Ref: "plan:sub#Quux", Reason: handleReason},
					{Ref: "plan:sub#Later", Reason: handleReason},
				},
			},
		},
		{
			name: "a var or const keeps the first line of its signature, a multi-line function keeps all of it",
			cards: []planparser.Card{card(1, nil, group(planparser.CardTypeEdit,
				"sub#single", "sub#second", "sub#joined", "sub#two", "sub#Long", "sub#T.LongMethod"))},
			want: planindex.Pack{Resolved: []planindex.PackEntry{
				{Glyph: "sub#single", Declarations: spanned(11, 13, "var single = map[string]int{")},
				{Glyph: "sub#second", Declarations: spanned(17, 19, "var second = []int{")},
				{Glyph: "sub#joined", Declarations: spanned(22, 23, `const joined = "a" +`)},
				{Glyph: "sub#two", Declarations: spanned(27, 28, `const two = "a" +`)},
				{Glyph: "sub#Long", Declarations: spanned(31, 34, "func Long(\n\ta int,\n\tb int,\n)")},
				{Glyph: "sub#T.LongMethod", Declarations: spanned(38, 41, "func (T) LongMethod(\n\ta int,\n\tb int,\n)")},
			}},
		},
		{
			name:  "a not_found member glyph is listed unresolved with its status",
			cards: []planparser.Card{card(1, nil, group(planparser.CardTypeEdit, "sub#Missing", "sub#Foo"))},
			want: planindex.Pack{
				Resolved:   []planindex.PackEntry{{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")}},
				Unresolved: []planindex.PackUnresolved{{Ref: "sub#Missing", Reason: "not_found"}},
			},
		},
		{
			name:     "a plan whose language opts out returns an empty pack",
			language: "none",
			cards:    []planparser.Card{card(1, nil, group(planparser.CardTypeEdit, "sub#Foo"))},
			want:     planindex.Pack{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := writeFixtureRepo(t, map[string]string{"sub/a.go": source})
			plan := &planparser.Plan{Language: tc.language}

			got, err := KickstartPack(plan, tc.cards, root)
			if err != nil {
				t.Fatalf("KickstartPack(%s) returned error: %v", tc.name, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("KickstartPack(%s) = %+v; want %+v", tc.name, got, tc.want)
			}
		})
	}

	t.Run("a worktree quarry cannot open returns the degraded pack and the unavailable error", func(t *testing.T) {
		t.Parallel()

		missing := filepath.Join(t.TempDir(), "does-not-exist")
		cards := []planparser.Card{card(1, nil,
			group(planparser.CardTypeEdit, "sub#Foo"),
			group(planparser.CardTypeCreate, "plan:sub#Made"),
		)}

		got, err := KickstartPack(&planparser.Plan{}, cards, missing)
		if !errors.Is(err, ErrQuarryUnavailable) {
			t.Fatalf("KickstartPack(missing root) error = %v; want errors.Is(err, ErrQuarryUnavailable)", err)
		}
		if len(got.Resolved) != 0 || len(got.Unresolved) != 2 {
			t.Fatalf("KickstartPack(missing root) = %+v; want nothing resolved and both refs unresolved", got)
		}
		if got.Unresolved[0].Ref != "sub#Foo" || got.Unresolved[0].Reason != err.Error() || got.Unresolved[1] != (planindex.PackUnresolved{Ref: "plan:sub#Made", Reason: handleReason}) {
			t.Errorf("KickstartPack(missing root).Unresolved = %+v; want sub#Foo with the error text, then the handle", got.Unresolved)
		}
	})
}
