// unwire_exclude_test.go covers, with no git, which `.git/info/exclude` lines an unwire strips:
// the lines chosen from the sibling census and their removal from the file content.

package fabricengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestExcludeLinesToStrip covers the exclude lines that survive an unwire given which names other worktrees still link.
// A legacy bare line matches at every depth, so it stays while any worktree wires the name at any anchor;
// an anchored line stays while another worktree links the same anchored path.
func TestExcludeLinesToStrip(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
		// sameAnchor and anyAnchor are the names a sibling still links at the same anchored path and at any anchor.
		sameAnchor []string
		anyAnchor  []string
		content    string
		want       string
	}{
		{
			name:       "SiblingAtTheSameAnchorKeepsTheAnchoredLine",
			anchorRel:  ".",
			sameAnchor: []string{"_lym"},
			anyAnchor:  []string{"_lym"},
			content:    "/_lym\n",
			want:       "/_lym\n",
		},
		{
			name:      "SiblingAtAnotherAnchorKeepsBareAndItsOwnLineAndStripsTheAnchoredOne",
			anchorRel: "sub",
			anyAnchor: []string{"_lym"},
			content:   "/_lym\n/sub/_lym\n_lym\n",
			want:      "/_lym\n_lym\n",
		},
		{
			name:       "SiblingAtTheSubAnchorKeepsTheSubLine",
			anchorRel:  "sub",
			sameAnchor: []string{"_lym"},
			anyAnchor:  []string{"_lym"},
			content:    "/sub/_lym\n",
			want:       "/sub/_lym\n",
		},
		{
			name:      "NoSiblingStripsBothSpellingsAndKeepsOtherLines",
			anchorRel: "sub",
			content:   "# header\n/sub/_lym\n_lym\nother\n",
			want:      "# header\nother\n",
		},
		{
			name:      "LineOfAnotherNameStays",
			anchorRel: ".",
			content:   "/_lym\n/_extra\n",
			want:      "/_extra\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wired := siblingWiring{sameAnchor: map[string]bool{}, anyAnchor: map[string]bool{}}
			for _, name := range tt.sameAnchor {
				wired.sameAnchor[name] = true
			}
			for _, name := range tt.anyAnchor {
				wired.anyAnchor[name] = true
			}

			stripSet := excludeLinesToStrip(tt.anchorRel, []string{"_lym"}, wired)
			if got := stripExcludeLines(tt.content, stripSet); got != tt.want {
				t.Errorf("stripExcludeLines(%q) = %q; want %q", tt.content, got, tt.want)
			}
		})
	}
}
