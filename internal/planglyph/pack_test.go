// pack_test.go covers KickstartPack against fixture trees written to a temp directory.

package planglyph

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
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
		want     []planindex.Pack
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
			want: []planindex.Pack{{Resolved: []planindex.PackEntry{
				{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")},
				{Glyph: "sub#Bar", Declarations: declaration(5, "func Bar()")},
				{Glyph: "sub#Qux", Declarations: declaration(9, "func Qux()")},
				{Glyph: "sub#Baz", Declarations: declaration(7, "func Baz()")},
			}}},
		},
		{
			name: "a ref two cards name appears under both, once per card",
			cards: []planparser.Card{
				card(1, []string{"sub#Foo"}, group(planparser.CardTypeEdit, "sub#Foo", "sub#Bar")),
				card(2, []string{"sub#Foo"}, group(planparser.CardTypeEdit, "sub#Baz", "sub#Bar")),
			},
			want: []planindex.Pack{
				{Resolved: []planindex.PackEntry{
					{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")},
					{Glyph: "sub#Bar", Declarations: declaration(5, "func Bar()")},
				}},
				{Resolved: []planindex.PackEntry{
					{Glyph: "sub#Baz", Declarations: declaration(7, "func Baz()")},
					{Glyph: "sub#Bar", Declarations: declaration(5, "func Bar()")},
					{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")},
				}},
			},
		},
		{
			name:  "self glyphs and plain paths are left out",
			cards: []planparser.Card{card(1, []string{"sub/a.go#"}, group(planparser.CardTypeEdit, "sub/a.go#", "docs/readme.md", "sub#Foo"))},
			want: []planindex.Pack{{Resolved: []planindex.PackEntry{
				{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")},
			}}},
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
			want: []planindex.Pack{{
				Resolved: []planindex.PackEntry{{Glyph: "sub#Qux", Declarations: declaration(9, "func Qux()")}},
				Unresolved: []planindex.PackUnresolved{
					{Ref: "plan:sub#Made", Reason: handleReason},
					{Ref: "plan:sub#Quux", Reason: handleReason},
					{Ref: "plan:sub#Later", Reason: handleReason},
				},
			}},
		},
		{
			name: "a var or const keeps the first line of its signature, a multi-line function keeps all of it",
			cards: []planparser.Card{card(1, nil, group(planparser.CardTypeEdit,
				"sub#single", "sub#second", "sub#joined", "sub#two", "sub#Long", "sub#T.LongMethod"))},
			want: []planindex.Pack{{Resolved: []planindex.PackEntry{
				{Glyph: "sub#single", Declarations: spanned(11, 13, "var single = map[string]int{")},
				{Glyph: "sub#second", Declarations: spanned(17, 19, "var second = []int{")},
				{Glyph: "sub#joined", Declarations: spanned(22, 23, `const joined = "a" +`)},
				{Glyph: "sub#two", Declarations: spanned(27, 28, `const two = "a" +`)},
				{Glyph: "sub#Long", Declarations: spanned(31, 34, "func Long(\n\ta int,\n\tb int,\n)")},
				{Glyph: "sub#T.LongMethod", Declarations: spanned(38, 41, "func (T) LongMethod(\n\ta int,\n\tb int,\n)")},
			}}},
		},
		{
			name:  "a not_found member glyph is listed unresolved with its status",
			cards: []planparser.Card{card(1, nil, group(planparser.CardTypeEdit, "sub#Missing", "sub#Foo"))},
			want: []planindex.Pack{{
				Resolved:   []planindex.PackEntry{{Glyph: "sub#Foo", Declarations: declaration(3, "func Foo()")}},
				Unresolved: []planindex.PackUnresolved{{Ref: "sub#Missing", Reason: "not_found"}},
			}},
		},
		{
			name:     "a plan whose language opts out returns no packs",
			language: "none",
			cards:    []planparser.Card{card(1, nil, group(planparser.CardTypeEdit, "sub#Foo"))},
			want:     nil,
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
			if tc.want == nil {
				if got != nil {
					t.Errorf("KickstartPack(%s) = %+v; want nil", tc.name, got)
				}
				return
			}
			if len(got) != len(tc.cards) {
				t.Fatalf("KickstartPack(%s) returned %d packs; want one per card, %d", tc.name, len(got), len(tc.cards))
			}
			for i, cardPack := range got {
				if cardPack.Card.Number != tc.cards[i].Number {
					t.Errorf("KickstartPack(%s)[%d] is card %d; want card %d", tc.name, i, cardPack.Card.Number, tc.cards[i].Number)
				}
				if !reflect.DeepEqual(cardPack.Pack, tc.want[i]) {
					t.Errorf("KickstartPack(%s)[%d] = %+v; want %+v", tc.name, i, cardPack.Pack, tc.want[i])
				}
			}
		})
	}

	t.Run("a worktree quarry cannot open returns the degraded pack and the unavailable error", func(t *testing.T) {
		t.Parallel()

		missing := filepath.Join(t.TempDir(), "does-not-exist")
		cards := []planparser.Card{
			card(1, nil,
				group(planparser.CardTypeEdit, "sub#Foo"),
				group(planparser.CardTypeCreate, "plan:sub#Made"),
			),
			card(2, nil, group(planparser.CardTypeEdit, "sub#Foo")),
		}

		got, err := KickstartPack(&planparser.Plan{}, cards, missing)
		if !errors.Is(err, ErrQuarryUnavailable) {
			t.Fatalf("KickstartPack(missing root) error = %v; want errors.Is(err, ErrQuarryUnavailable)", err)
		}
		want := []planindex.Pack{
			{Unresolved: []planindex.PackUnresolved{
				{Ref: "sub#Foo", Reason: err.Error()},
				{Ref: "plan:sub#Made", Reason: handleReason},
			}},
			{Unresolved: []planindex.PackUnresolved{{Ref: "sub#Foo", Reason: err.Error()}}},
		}
		if len(got) != len(want) {
			t.Fatalf("KickstartPack(missing root) = %+v; want one pack per card", got)
		}
		for i := range want {
			if !reflect.DeepEqual(got[i].Pack, want[i]) {
				t.Errorf("KickstartPack(missing root)[%d] = %+v; want %+v", i, got[i].Pack, want[i])
			}
		}
	})

	t.Run("the batch's distinct member refs are resolved in one call", func(t *testing.T) {
		t.Parallel()

		cards := []planparser.Card{
			card(1, []string{"sub#Foo"}, group(planparser.CardTypeEdit, "sub#Foo", "sub#Bar")),
			card(2, []string{"sub#Foo"}, group(planparser.CardTypeEdit, "sub#Baz", "sub#Bar")),
		}
		var calls [][]string
		resolve := func(targets []string) ([]quarry.ResolveResult, error) {
			calls = append(calls, targets)
			results := make([]quarry.ResolveResult, len(targets))
			for i := range targets {
				results[i] = quarry.ResolveResult{Status: quarry.StatusFound, Symbols: []quarry.Symbol{{File: "sub/a.go", Start: i + 1, End: i + 1}}}
			}
			return results, nil
		}

		got, err := kickstartPack(&planparser.Plan{}, cards, resolve)
		if err != nil {
			t.Fatalf("kickstartPack returned error: %v", err)
		}
		if want := [][]string{{"sub#Foo", "sub#Bar", "sub#Baz"}}; !reflect.DeepEqual(calls, want) {
			t.Fatalf("kickstartPack resolve calls = %q; want exactly %q", calls, want)
		}
		glyphs := func(pack planindex.Pack) []string {
			var names []string
			for _, entry := range pack.Resolved {
				names = append(names, entry.Glyph)
			}
			return names
		}
		if want := []string{"sub#Foo", "sub#Bar"}; !reflect.DeepEqual(glyphs(got[0].Pack), want) {
			t.Errorf("card 1 resolved %q; want %q", glyphs(got[0].Pack), want)
		}
		if want := []string{"sub#Baz", "sub#Bar", "sub#Foo"}; !reflect.DeepEqual(glyphs(got[1].Pack), want) {
			t.Errorf("card 2 resolved %q; want %q", glyphs(got[1].Pack), want)
		}
	})
}
