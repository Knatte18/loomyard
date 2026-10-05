package claudeengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func readPaneFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func TestIdleSession(t *testing.T) {
	rule := "────────────────────────────────"
	tests := []struct {
		name    string
		capture string
		want    bool
	}{
		{"empty box", readPaneFixture(t, "pane-idle-empty.txt"), true},
		{"draft in box", readPaneFixture(t, "pane-idle-draft.txt"), false},
		{"permission prompt", readPaneFixture(t, "pane-permission-prompt.txt"), false},
		{"turn running", readPaneFixture(t, "pane-turn-running.txt"), false},
		{"no input box", "● some transcript\n\nnothing else here\n", false},
		{"empty capture", "", false},
		{
			name:    "transcript quoting the running hint above an empty box",
			capture: "● the hint reads: esc to interrupt\n\n" + rule + "\n❯ \n" + rule + "\n  ? for shortcuts\n",
			want:    false,
		},
		{
			name:    "named session labels the top rule",
			capture: rule + " tst:orch ─\n❯ \n" + rule + "\n  ? for shortcuts\n",
			want:    true,
		},
		{
			name:    "draft line shaped like a labelled rule below the caret",
			capture: rule + "\n❯ \n  ─── note ─\n  more text\n" + rule + "\n  ? for shortcuts\n",
			want:    false,
		},
		{
			name:    "label with no trailing rule glyph is not a top rule",
			capture: rule + " tst:orch\n❯ \n" + rule + "\n  ? for shortcuts\n",
			want:    false,
		},
		{
			name:    "boxed side bars around an empty box",
			capture: "╭" + rule + "╮\n│ ❯          │\n╰" + rule + "╯\n",
			want:    true,
		},
	}
	c := &Claude{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.IdleSession(tt.capture); got != tt.want {
				t.Errorf("IdleSession = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestPaneTooShort(t *testing.T) {
	tests := []struct {
		name    string
		capture string
		want    bool
	}{
		{"short capture with no box", "● working\n  esc to interrupt\n", true},
		{"one line", "● working", true},
		{"busy capture of normal height", "● a\n\n● b\n● c\n  esc to interrupt\n", false},
		{"idle box", readPaneFixture(t, "pane-idle-empty.txt"), false},
		{"short capture with a caret", "────\n❯ \n", false},
	}
	c := &Claude{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.PaneTooShort(tt.capture); got != tt.want {
				t.Errorf("PaneTooShort = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestCompactSessionSequence(t *testing.T) {
	cases := []struct {
		name  string
		focus string
		want  []shuttleengine.PaneInput
	}{
		{"focus", "the open plan", []shuttleengine.PaneInput{{Text: "/compact the open plan", Submit: true}}},
		{"empty", "", []shuttleengine.PaneInput{{Text: "/compact", Submit: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (&Claude{}).CompactSessionSequence(tc.focus)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("CompactSessionSequence(%q) = %#v; want %#v", tc.focus, got, tc.want)
			}
		})
	}
}

func TestClearSessionSequence(t *testing.T) {
	got := (&Claude{}).ClearSessionSequence()
	want := []shuttleengine.PaneInput{{Text: "/clear", Submit: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClearSessionSequence = %#v; want %#v", got, want)
	}
}

func TestSkillLoadSequence(t *testing.T) {
	got := (&Claude{}).SkillLoadSequence("scribe:prose")
	want := []shuttleengine.PaneInput{{Text: "/scribe:prose", Submit: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SkillLoadSequence = %#v; want %#v", got, want)
	}
}

func TestSkillUnknown(t *testing.T) {
	rule := "────────────────────────────────"
	box := rule + "\n> \n" + rule + "\n"
	tests := []struct {
		name    string
		capture string
		want    bool
	}{
		{"unknown skill notice", "> /scribe:prose\n  Unknown skill: scribe:prose\n" + box, true},
		{"unknown slash command notice", "> /scribe:prose\n  Unknown slash command: scribe:prose\n" + box, true},
		{"notice naming another skill", "  Unknown skill: scribe:testing\n" + box, false},
		{"idle box", box, false},
		{"running turn", "  Unknown skill: scribe:prose\n  esc to interrupt\n" + box, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (&Claude{}).SkillUnknown(tc.capture, "scribe:prose"); got != tc.want {
				t.Errorf("SkillUnknown = %v; want %v", got, tc.want)
			}
		})
	}
}
