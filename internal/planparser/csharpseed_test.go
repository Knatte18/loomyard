//go:build tier0

// csharpseed_test.go covers CardCSharpSeeds: the files and type names a C# plan's card targets seed.

package planparser_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestCardCSharpSeeds(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{Language: "csharp"}
	group := func(cardType planparser.CardType, refs ...string) planparser.TargetGroup {
		return planparser.TargetGroup{Type: cardType, Refs: refs}
	}
	rename := planparser.TargetGroup{
		Type:  planparser.CardTypeRename,
		Refs:  []string{"NORCE.Models#Old.Run", "NORCE.Models#New.Run"},
		Pairs: []planparser.MovePair{{Old: "NORCE.Models#Old.Run", New: "NORCE.Models#New.Run"}},
	}

	tests := []struct {
		name  string
		cards []planparser.Card
		want  []planparser.CSharpSeed
	}{
		{
			name:  "a .cs path seeds that file",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeEdit, "src/Models/Circle.cs")}}},
			want:  []planparser.CSharpSeed{{Ref: "src/Models/Circle.cs", File: "src/Models/Circle.cs"}},
		},
		{
			name:  "a file self glyph seeds that file",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeEdit, "src/Models/Circle.cs#")}}},
			want:  []planparser.CSharpSeed{{Ref: "src/Models/Circle.cs#", File: "src/Models/Circle.cs"}},
		},
		{
			name:  "a member glyph seeds its owner",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeEdit, "NORCE.Models#Foo.Bar")}}},
			want:  []planparser.CSharpSeed{{Ref: "NORCE.Models#Foo.Bar", Types: []string{"Foo"}}},
		},
		{
			name:  "a nested member glyph seeds every owner outermost first",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeEdit, "NORCE.Models#Outer.Inner.M")}}},
			want:  []planparser.CSharpSeed{{Ref: "NORCE.Models#Outer.Inner.M", Types: []string{"Outer", "Inner"}}},
		},
		{
			name:  "a type glyph seeds its own name",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeEdit, "NORCE.Models#Circle")}}},
			want:  []planparser.CSharpSeed{{Ref: "NORCE.Models#Circle", Types: []string{"Circle"}}},
		},
		{
			name: "generic and arity suffixes and a leading @ drop",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{
				group(planparser.CardTypeEdit, "NORCE.Models#Box<T>", "NORCE.Models#Pair<,>.Swap", "NORCE.Models#@event.Raise"),
			}}},
			want: []planparser.CSharpSeed{
				{Ref: "NORCE.Models#Box<T>", Types: []string{"Box"}},
				{Ref: "NORCE.Models#Pair<,>.Swap", Types: []string{"Pair"}},
				{Ref: "NORCE.Models#@event.Raise", Types: []string{"event"}},
			},
		},
		{
			name:  "a plan: handle seeds like its glyph",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeCreate, "plan:NORCE.Models#Foo.Bar")}}},
			want:  []planparser.CSharpSeed{{Ref: "plan:NORCE.Models#Foo.Bar", Types: []string{"Foo"}}},
		},
		{
			name:  "a Rename pair contributes both sides",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{rename}}},
			want: []planparser.CSharpSeed{
				{Ref: "NORCE.Models#Old.Run", Types: []string{"Old"}},
				{Ref: "NORCE.Models#New.Run", Types: []string{"New"}},
			},
		},
		{
			name: "a Uses entry is left out",
			cards: []planparser.Card{{
				Number: 1, Uses: []string{"NORCE.Models#Read.Only"}, HasUses: true,
				TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeEdit, "NORCE.Models#Foo.Bar")},
			}},
			want: []planparser.CSharpSeed{{Ref: "NORCE.Models#Foo.Bar", Types: []string{"Foo"}}},
		},
		{
			name: "a directory path and a namespace self glyph seed nothing",
			cards: []planparser.Card{{Number: 1, TargetGroups: []planparser.TargetGroup{
				group(planparser.CardTypeEdit, "src/Models", "NORCE.Models#"),
			}}},
			want: []planparser.CSharpSeed{{Ref: "src/Models"}, {Ref: "NORCE.Models#"}},
		},
		{
			name: "seeds follow card and group order",
			cards: []planparser.Card{
				{Number: 1, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeEdit, "NORCE.Models#A.M"), group(planparser.CardTypeDelete, "NORCE.Models#B.M")}},
				{Number: 2, TargetGroups: []planparser.TargetGroup{group(planparser.CardTypeCreate, "NORCE.Models#C.M")}},
			},
			want: []planparser.CSharpSeed{
				{Ref: "NORCE.Models#A.M", Types: []string{"A"}},
				{Ref: "NORCE.Models#B.M", Types: []string{"B"}},
				{Ref: "NORCE.Models#C.M", Types: []string{"C"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := planparser.CardCSharpSeeds(plan, tt.cards); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CardCSharpSeeds() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
