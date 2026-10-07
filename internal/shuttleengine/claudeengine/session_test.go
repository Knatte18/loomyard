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

func TestInputBoxText(t *testing.T) {
	t.Parallel()

	rule := "────────────────────────────────"
	tests := []struct {
		name     string
		capture  string
		wantText string
		wantOK   bool
	}{
		{"empty box", readPaneFixture(t, "pane-idle-empty.txt"), "", true},
		{"draft in box", readPaneFixture(t, "pane-idle-draft.txt"), "please also run the linter", true},
		{"wrapped two-line draft", rule + "\n❯ first half of the draft\n  second half of the draft\n" + rule + "\n  ? for shortcuts\n", "first half of the draft second half of the draft", true},
		{"collapsed paste placeholder", rule + "\n❯ [Pasted text #1 +42 lines]\n" + rule + "\n  ? for shortcuts\n", "[Pasted text #1 +42 lines]", true},
		{"turn running over an empty box", readPaneFixture(t, "pane-turn-running.txt"), "", true},
		{"boxed side bars", "╭" + rule + "╮\n│ ❯ hello     │\n╰" + rule + "╯\n", "hello", true},
		{"permission prompt has no box", readPaneFixture(t, "pane-permission-prompt.txt"), "", false},
		{"no input box", "● some transcript\n", "", false},
	}
	c := &Claude{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			text, ok := c.InputBoxText(tt.capture)
			if text != tt.wantText || ok != tt.wantOK {
				t.Errorf("InputBoxText = (%q, %v); want (%q, %v)", text, ok, tt.wantText, tt.wantOK)
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
		{"focus", "the open plan", []shuttleengine.PaneInput{{Text: "/compact the open plan", SettleMS: defaultSubmitSettleMS}, {Key: "Enter"}}},
		{"empty", "", []shuttleengine.PaneInput{{Text: "/compact", SettleMS: defaultSubmitSettleMS}, {Key: "Enter"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := New().CompactSessionSequence(tc.focus)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("CompactSessionSequence(%q) = %#v; want %#v", tc.focus, got, tc.want)
			}
		})
	}
}

func TestReloadPluginsSequence(t *testing.T) {
	t.Parallel()

	got := New().ReloadPluginsSequence()
	want := []shuttleengine.PaneInput{{Text: "/reload-plugins", SettleMS: defaultSubmitSettleMS}, {Key: "Enter"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReloadPluginsSequence = %#v; want %#v", got, want)
	}
}

func TestClearSessionSequence(t *testing.T) {
	got := New().ClearSessionSequence()
	want := []shuttleengine.PaneInput{{Text: "/clear", SettleMS: defaultSubmitSettleMS}, {Key: "Enter"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClearSessionSequence = %#v; want %#v", got, want)
	}
}
