//go:build tier0

// patternentry_test.go covers the readers of a card's PATTERN entries: its fenced entry lines and the entry IDs its text names.

package planparser

import (
	"reflect"
	"testing"
)

func TestCardPatternEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want []PatternEntryLine
	}{
		{
			name: "a fenced entry line is read with its ID",
			text: "# Card 1 — x\n\n```\n- `PATTERN-a` — rule a.\n```\n",
			want: []PatternEntryLine{{ID: "PATTERN-a", Line: "- `PATTERN-a` — rule a."}},
		},
		{
			name: "two fenced lines for one ID are both returned in order",
			text: "```markdown\n- `PATTERN-a` — first.\n- `PATTERN-b` — b.\n- `PATTERN-a` — second.\n```\n",
			want: []PatternEntryLine{
				{ID: "PATTERN-a", Line: "- `PATTERN-a` — first."},
				{ID: "PATTERN-b", Line: "- `PATTERN-b` — b."},
				{ID: "PATTERN-a", Line: "- `PATTERN-a` — second."},
			},
		},
		{
			name: "an inline-span entry line and a prose line are ignored",
			text: "Write `- `PATTERN-a` — inline.` into the overview.\n- `PATTERN-b` — prose bullet.\n",
			want: nil,
		},
		{
			name: "a line under a tilde fence is read",
			text: "~~~\n- `PATTERN-a` — tilde.\n~~~\n",
			want: []PatternEntryLine{{ID: "PATTERN-a", Line: "- `PATTERN-a` — tilde."}},
		},
		{
			name: "a line in an unclosed fence is read",
			text: "```\n- `PATTERN-a` — unclosed.\n",
			want: []PatternEntryLine{{ID: "PATTERN-a", Line: "- `PATTERN-a` — unclosed."}},
		},
		{
			name: "a CRLF line is returned without its carriage return",
			text: "```\r\n- `PATTERN-a` — crlf.\r\n```\r\n",
			want: []PatternEntryLine{{ID: "PATTERN-a", Line: "- `PATTERN-a` — crlf."}},
		},
		{
			name: "a card with no fence returns nil",
			text: "# Card 1 — x\n\nNo fenced block here.\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CardPatternEntries(Card{Text: tt.text})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CardPatternEntries() = %#v; want %#v", got, tt.want)
			}
		})
	}
}

func TestCardNamesPatternEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"BacktickedIDInProse", "Delete the `PATTERN-a` bullet from PATTERN.md.\n", true},
		{"IDInAFencedLine", "```\n- `PATTERN-a` — rule a.\n```\n", true},
		{"BareIDWithoutBackticks", "Delete PATTERN-a from PATTERN.md.\n", false},
		{"LongerIDSharingThePrefix", "Delete the `PATTERN-ab` bullet.\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := CardNamesPatternEntry(Card{Text: tt.text}, "PATTERN-a"); got != tt.want {
				t.Errorf("CardNamesPatternEntry(%q, PATTERN-a) = %v; want %v", tt.text, got, tt.want)
			}
		})
	}
}
