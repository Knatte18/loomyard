// resign_test.go covers RebindResigns: the carrying of a recorded C# re-sign arrow into the plan once its edit is in the tree.

package planglyph

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRebindResigns runs RebindResigns over a fixture copy and a plan whose card 1 re-signs a member and whose card 2 names the member by its old glyph.
func TestRebindResigns(t *testing.T) {
	t.Parallel()

	const (
		namespace = "Glyphchain.Sharp"
		draw      = namespace + "#Renderer.Draw(int)"
		drawLong  = namespace + "#Renderer.Draw(long)"
	)
	drawArrow := resignLeg{draw, "public void Draw(long count)"}
	landed := [][2]string{{"public void Draw(int count)", "public void Draw(long count)"}}
	tests := []struct {
		name     string
		language string
		// edits are the source replacements made to csharp/Renderer.cs before the plan is checked.
		edits [][2]string
		cards []string
		// want is the one finding RebindResigns must report, none when empty.
		want []findingKey
		// card2 is the substring card 2's file must hold afterwards.
		card2 string
		// rewritten is whether the plan's card files change: card 1's arrow target and card 2's reference both move to the minted id.
		rewritten bool
	}{
		{
			name:      "a landed arrow rewrites the later card to the minted id",
			edits:     landed,
			cards:     []string{resignCard(drawArrow), editCard(draw)},
			card2:     drawLong,
			rewritten: true,
		},
		{
			name:  "an arrow whose edit did not land is not done and rewrites nothing",
			cards: []string{resignCard(drawArrow), editCard(draw)},
			want:  []findingKey{{"resign-not-done", "1-card1", SeverityBlocking}},
			card2: draw,
		},
		{
			name:  "one landed and one unlanded arrow on a card are one not-done finding and rewrite nothing",
			edits: landed,
			cards: []string{resignCard(drawArrow, resignLeg{namespace + "#Renderer.Fill(int)", "public void Fill(long count)"}), editCard(draw)},
			want:  []findingKey{{"resign-not-done", "1-card1", SeverityBlocking}},
			card2: draw,
		},
		{
			name:  "an arrow minting its own target's id does nothing",
			cards: []string{resignCard(resignLeg{namespace + "#Renderer.Title", "public string Title { get; set; }"}), editCard(namespace + "#Renderer.Title")},
			card2: namespace + "#Renderer.Title",
		},
		{
			name:     "a go plan's arrow does nothing",
			language: "go",
			cards:    []string{resignCard(resignLeg{"shapes#Func", "func Func(count int) int"}), editCard("shapes#Func")},
			card2:    "shapes#Func",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := copyGlyphChainFixture(t)
			rendererPath := filepath.Join(root, "csharp", "Renderer.cs")
			source, err := os.ReadFile(rendererPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) failed: %v", rendererPath, err)
			}
			for _, edit := range tt.edits {
				source = []byte(strings.Replace(string(source), edit[0], edit[1], 1))
			}
			if err := os.WriteFile(rendererPath, source, 0o644); err != nil {
				t.Fatalf("WriteFile(%q) failed: %v", rendererPath, err)
			}

			dir, plan := writeLanguagePlan(t, cmp.Or(tt.language, "csharp"), tt.cards)
			card1Before := readCardFile(t, dir, 1, "card1")
			findings, err := RebindResigns(plan, plan.Cards[:1], root)
			if err != nil {
				t.Fatalf("RebindResigns(...) returned error: %v", err)
			}

			want := tt.want
			if want == nil {
				want = []findingKey{}
			}
			if got := sortedKeys(findings); !slices.Equal(got, want) {
				t.Errorf("RebindResigns findings = %+v; want %+v", got, want)
			}
			if len(findings) == 1 && !strings.Contains(findings[0].Detail, "rebaseline --card") {
				t.Errorf("resign-not-done detail %q lacks %q", findings[0].Detail, "rebaseline --card")
			}
			if got := readCardFile(t, dir, 2, "card2"); !strings.Contains(got, tt.card2) {
				t.Errorf("card 2 = %q; want it to hold %q", got, tt.card2)
			}
			if changed := readCardFile(t, dir, 1, "card1") != card1Before; changed != tt.rewritten {
				t.Errorf("card 1 rewritten = %v; want %v", changed, tt.rewritten)
			}
		})
	}
}
