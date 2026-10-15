// edittestfile_test.go covers CheckEditTestFile over in-memory plans.

package planparser_test

import (
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestCheckEditTestFile covers the refs under Edit that the check refuses and the ones it admits: groups other than Edit, member glyphs, handles, paths the go tool ignores, other languages and done cards.
func TestCheckEditTestFile(t *testing.T) {
	t.Parallel()

	wantDetail := func(card, ref string) string {
		return fmt.Sprintf("card %s lists the test file %q under Edit; name each existing test function, fake or helper the card changes as a member glyph under Edit, "+
			"declare each new one as a Create plan: handle on the package directory (or on <dir>_test for an external test package), "+
			"keep a new test file as a Create path, and when the change is confined to the file's header, such as its build constraint, name every member of that file",
			card, ref)
	}
	finding := func(card, ref string) planparser.ValidationError {
		return planparser.ValidationError{Check: "edit-test-file", Card: card, Ref: ref, Detail: wantDetail(card, ref)}
	}
	underGroup := func(typ planparser.CardType, ref string) planparser.Card {
		card := validCard(1, "one")
		card.TargetGroups = []planparser.TargetGroup{{Type: typ, Refs: []string{ref}}}
		return card
	}
	underEdit := func(ref string) planparser.Card { return underGroup(planparser.CardTypeEdit, ref) }

	const testFile = "internal/x/foo_test.go#"
	usesTestFile := validCard(1, "one")
	usesTestFile.TargetGroups = []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/x/foo.go#"}}}
	usesTestFile.Uses = []string{testFile}

	tests := []struct {
		name     string
		cards    []planparser.Card
		language string
		done     []planparser.Card
		want     []planparser.ValidationError
	}{
		{name: "test file self glyph under Edit", cards: []planparser.Card{underEdit(testFile)}, want: []planparser.ValidationError{finding("1-one", testFile)}},
		{name: "test file self glyph under Create", cards: []planparser.Card{underGroup(planparser.CardTypeCreate, testFile)}},
		{name: "test file self glyph under Delete", cards: []planparser.Card{underGroup(planparser.CardTypeDelete, testFile)}},
		{name: "test file self glyph in Uses", cards: []planparser.Card{usesTestFile}},
		{name: "test member glyph under Edit", cards: []planparser.Card{underEdit("internal/x#TestFoo")}},
		{name: "path with no glyph marker under Edit", cards: []planparser.Card{underEdit("internal/x/foo_test.go")}},
		{name: "plan handle under Edit", cards: []planparser.Card{underEdit("plan:internal/x#TestNew")}},
		{name: "malformed glyph under Edit", cards: []planparser.Card{underEdit("internal/x/foo_test.go#a#b")}},
		{name: "test file under testdata", cards: []planparser.Card{underEdit("internal/x/testdata/foo_test.go#")}},
		{name: "test file under vendor", cards: []planparser.Card{underEdit("vendor/x/foo_test.go#")}},
		{name: "test file under a dot directory", cards: []planparser.Card{underEdit("internal/.x/foo_test.go#")}},
		{name: "test file under an underscore directory", cards: []planparser.Card{underEdit("internal/_x/foo_test.go#")}},
		{name: "test file whose base name starts with an underscore", cards: []planparser.Card{underEdit("internal/x/_foo_test.go#")}},
		{name: "file name not ending in _test.go", cards: []planparser.Card{underEdit("internal/x/foo_test.go.orig#")}},
		{name: "production go file", cards: []planparser.Card{underEdit("internal/x/foo.go#")}},
		{name: "markdown file", cards: []planparser.Card{underEdit("docs/foo.md#")}},
		{name: "language none", cards: []planparser.Card{underEdit(testFile)}, language: "none"},
		{
			name:  "only the first of two offending cards done",
			cards: []planparser.Card{underEdit(testFile), func() planparser.Card { c := underEdit(testFile); c.Number, c.Slug = 2, "two"; return c }()},
			done:  []planparser.Card{underEdit(testFile)},
			want:  []planparser.ValidationError{finding("2-two", testFile)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan := &planparser.Plan{Language: tt.language, Cards: tt.cards}

			got := planparser.CheckEditTestFile(plan, tt.done)

			if len(got) != len(tt.want) {
				t.Fatalf("CheckEditTestFile() = %v; want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("CheckEditTestFile()[%d] = %+v; want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
