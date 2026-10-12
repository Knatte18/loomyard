// vocabname_test.go covers CheckFabricVocabularyName over in-memory plans.

package planparser_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricvocab"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestCheckFabricVocabularyName covers the names a plan declares that the fabric-vocabulary scan refuses and the ones it admits.
// Side-token names are built from fabricvocab.SideTokens, since this file sits outside the owner set.
func TestCheckFabricVocabularyName(t *testing.T) {
	t.Parallel()

	for _, side := range fabricvocab.SideTokens() {
		side := side
		capitalized := strings.ToUpper(side[:1]) + side[1:]
		declare := func(handle string) planparser.Card {
			card := validCard(1, "one")
			card.Declarations = []planparser.CardDeclaration{{Handle: handle, Decl: "func Declared()"}}
			return card
		}
		rename := func(newHandle string) planparser.Card {
			card := validCard(1, "one")
			card.Pairs = []planparser.MovePair{{Old: "internal/foo#Old", New: newHandle}}
			return card
		}

		tests := []struct {
			name string
			card planparser.Card
			ref  string
			want int
		}{
			{name: "test name outside internal", card: declare("plan:tools/x#Test" + capitalized + "Thing"), ref: "plan:tools/x#Test" + capitalized + "Thing", want: 1},
			{name: "token under internal", card: declare("plan:internal/foo#" + side + "Thing"), ref: "plan:internal/foo#" + side + "Thing", want: 1},
			{name: "token under cmd", card: declare("plan:cmd/lyx#" + capitalized + "Thing"), ref: "plan:cmd/lyx#" + capitalized + "Thing", want: 1},
			{name: "token in configsync", card: declare("plan:internal/configsync#" + capitalized + "Thing"), ref: "plan:internal/configsync#" + capitalized + "Thing", want: 1},
			{name: "test name in configsync external test package", card: declare("plan:internal/configsync_test#Test" + capitalized + "Thing"), ref: "plan:internal/configsync_test#Test" + capitalized + "Thing", want: 1},
			{name: "rename new handle", card: rename("plan:internal/foo#" + capitalized + "New"), ref: "plan:internal/foo#" + capitalized + "New", want: 1},
			{name: "token in a method's type segment alone", card: declare("plan:internal/foo#Type" + capitalized + ".Method"), ref: "plan:internal/foo#Type" + capitalized + ".Method", want: 1},
			{name: "host geometry identifier under internal", card: declare("plan:internal/foo#hostBranch"), ref: "plan:internal/foo#hostBranch", want: 1},
			{name: "host geometry identifier as a method's last segment", card: declare("plan:internal/foo#Type.hostBranch"), ref: "plan:internal/foo#Type.hostBranch", want: 1},
			{name: "host geometry identifier under cmd", card: declare("plan:cmd/lyx#hostBranch"), ref: "plan:cmd/lyx#hostBranch", want: 1},
			{name: "name failing both rules gets one finding", card: declare("plan:internal/foo#Type" + capitalized + ".hostBranch"), ref: "plan:internal/foo#Type" + capitalized + ".hostBranch", want: 1},
			{name: "token in a non-configsync owner package", card: declare("plan:internal/fabricengine#" + capitalized + "Thing"), want: 0},
			{name: "test name in an owner package's external test package", card: declare("plan:internal/fabricengine_test#Test" + capitalized + "Thing"), want: 0},
			{name: "token without test prefix outside internal and the doc roots", card: declare("plan:tools/x#" + capitalized + "Thing"), want: 0},
			{name: "host geometry identifier under tools", card: declare("plan:tools/x#hostBranch"), want: 0},
			{name: "name with no token", card: declare("plan:internal/foo#Thing"), want: 0},
			{name: "handle without a member", card: declare("plan:internal/foo"), want: 0},
		}
		for _, tc := range tests {
			t.Run(side+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				plan := &planparser.Plan{Cards: []planparser.Card{tc.card}}

				findings := planparser.CheckFabricVocabularyName(plan)

				if len(findings) != tc.want {
					t.Fatalf("got %d findings %v, want %d", len(findings), findings, tc.want)
				}
				if tc.want == 0 {
					return
				}
				got := findings[0]
				if got.Check != "fabric-vocabulary-name" || got.Card != "1-one" || got.Ref != tc.ref {
					t.Errorf("finding = %+v, want check fabric-vocabulary-name, card 1-one, ref %q", got, tc.ref)
				}
			})
		}
	}
}
