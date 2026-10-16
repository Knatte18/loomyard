package claudeengine

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

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

// styledBox draws an input box around the styled caret line, as a pane capture with escapes shows it.
func styledBox(caretLine string) string {
	rule := "────────────────────────────────"
	return rule + "\n" + caretLine + "\n" + rule + "\n  ? for shortcuts\n"
}

// plainBox is the plain capture Claude answers for a styled one.
func plainBox(styled string) string {
	return (&Claude{}).PlainBoxCapture(styled)
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
		{"exported idle input box fixture", IdleInputBoxFixture, true},
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
		{"lone percent mark in the box", readPaneFixture(t, "pane-idle-eol-mark.txt"), true},
		{"boxed side bars around a lone percent mark", "╭" + rule + "╮\n│ ❯     %   │\n╰" + rule + "╯\n", true},
		{"percent beside other text is a draft", rule + "\n❯ 100% done\n" + rule + "\n  ? for shortcuts\n", false},
		{"any other lone character is a draft", rule + "\n❯ #\n" + rule + "\n  ? for shortcuts\n", false},
		{"styled placeholder fixture", plainBox(readPaneFixture(t, "pane-placeholder-styled.txt")), true},
		{"styled restored prompt fixture", plainBox(readPaneFixture(t, "pane-restored-prompt-styled.txt")), false},
		{"styled unstyled typed draft", plainBox(styledBox("❯ hello world")), false},
		{"styled typed text followed by faint text", plainBox(styledBox("❯ hello \x1b[2mworld\x1b[0m")), false},
		{"styled faint paste placeholder", plainBox(styledBox("❯ \x1b[2m[Pasted text #1 +4 lines]\x1b[0m")), false},
		{"styled faint placeholder beside a lone percent mark", plainBox(styledBox("❯ \x1b[2mTry this\x1b[0m %")), true},
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

func TestInterruptGuardReadings(t *testing.T) {
	t.Parallel()

	rule := "────────────────────────────────"
	tests := []struct {
		name        string
		capture     string
		wantRunning bool
		wantPicker  bool
	}{
		{"rewind picker", readPaneFixture(t, "pane-rewind-picker.txt"), false, true},
		{"turn running", readPaneFixture(t, "pane-turn-running.txt"), true, false},
		{"idle empty box", readPaneFixture(t, "pane-idle-empty.txt"), false, false},
		{
			name:       "transcript quoting the picker subtitle above an empty box",
			capture:    "● Restore the code and/or conversation to the point before…\n\n" + rule + "\n❯ \n" + rule + "\n  ? for shortcuts\n",
			wantPicker: false,
		},
		{
			name:        "transcript quoting the running hint above an empty box",
			capture:     "● the hint reads: esc to interrupt\n\n" + rule + "\n❯ \n" + rule + "\n  ? for shortcuts\n",
			wantRunning: true,
		},
		{"permission prompt is not the picker", readPaneFixture(t, "pane-permission-prompt.txt"), false, false},
	}
	c := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := c.RunningTurn(tt.capture); got != tt.wantRunning {
				t.Errorf("RunningTurn = %v; want %v", got, tt.wantRunning)
			}
			if got := c.RewindPicker(tt.capture); got != tt.wantPicker {
				t.Errorf("RewindPicker = %v; want %v", got, tt.wantPicker)
			}
		})
	}
	if got := c.InterruptSettle(); got != 2*time.Second {
		t.Errorf("InterruptSettle = %v; want %v", got, 2*time.Second)
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
		{"rewind picker has no box", readPaneFixture(t, "pane-rewind-picker.txt"), "", false},
		{"no input box", "● some transcript\n", "", false},
		{"lone percent mark in the box", readPaneFixture(t, "pane-idle-eol-mark.txt"), "", true},
		{"boxed side bars around a lone percent mark", "╭" + rule + "╮\n│ ❯     %   │\n╰" + rule + "╯\n", "", true},
		{"percent beside other text", rule + "\n❯ 100% done\n" + rule + "\n  ? for shortcuts\n", "100% done", true},
		{"any other lone character", rule + "\n❯ #\n" + rule + "\n  ? for shortcuts\n", "#", true},
		{"styled placeholder fixture", plainBox(readPaneFixture(t, "pane-placeholder-styled.txt")), "", true},
		{
			"styled restored prompt fixture",
			plainBox(readPaneFixture(t, "pane-restored-prompt-styled.txt")),
			"Please write a very long detailed essay about the history of the river Thames in London, covering the Roman period, the medieval bridges, the Victorian embankments, and the modern flood barrier, with at least twenty paragraphs",
			true,
		},
		{"styled unstyled typed draft", plainBox(styledBox("❯ hello world")), "hello world", true},
		{"styled typed text followed by faint text", plainBox(styledBox("❯ hello \x1b[2mworld\x1b[0m")), "hello world", true},
		{"styled faint paste placeholder", plainBox(styledBox("❯ \x1b[2m[Pasted text #1 +4 lines]\x1b[0m")), "[Pasted text #1 +4 lines]", true},
		{"styled faint placeholder beside a lone percent mark", plainBox(styledBox("❯ \x1b[2mTry this\x1b[0m %")), "", true},
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

// TestPlainBoxCapture pins the plain capture a styled one answers: escapes removed, trailing ASCII spaces trimmed, and only a placeholder-only box blanked.
func TestPlainBoxCapture(t *testing.T) {
	t.Parallel()

	sgr := regexp.MustCompile("\x1b\\[[0-9;]*m")
	restored := readPaneFixture(t, "pane-restored-prompt-styled.txt")
	tests := []struct {
		name   string
		styled string
		want   string
	}{
		{"restored prompt fixture is the fixture without escapes", restored, sgr.ReplaceAllString(restored, "")},
		{"capture with no box is stripped and right-trimmed", "\x1b[1mhello\x1b[0m   \nworld  \n", "hello\nworld\n"},
		{
			"faint line outside the box stays",
			"\x1b[2m● old transcript\x1b[0m\n" + styledBox("❯ \x1b[2mTry this\x1b[0m"),
			"● old transcript\n" + styledBox("❯ "),
		},
		{
			"OSC 8 hyperlink is dropped and its text kept",
			styledBox("❯ \x1b]8;;https://example.com\x1b\\link text\x1b]8;;\x1b\\"),
			styledBox("❯ link text"),
		},
		{
			"trailing spaces under a background colour are trimmed",
			"\x1b[44mtext   \x1b[49m\n",
			"text\n",
		},
		{"caret and NBSP alone keep the NBSP", styledBox("❯ "), styledBox("❯ ")},
		{
			"colour parameters set no faint",
			styledBox("❯ \x1b[38;2;10;2;20mone\x1b[38;5;2m two\x1b[0m"),
			styledBox("❯ one two"),
		},
		{
			"a zero inside a colour clears no faint",
			styledBox("❯ \x1b[2mTry\x1b[38;5;0m this\x1b[0m"),
			styledBox("❯ "),
		},
		{
			"faint cleared by 22 leaves typed text unblanked",
			styledBox("❯ \x1b[2mTry\x1b[22m typed\x1b[0m"),
			styledBox("❯ Try typed"),
		},
		{
			"faint cleared by a bare m leaves typed text unblanked",
			styledBox("❯ \x1b[2mTry\x1b[m typed"),
			styledBox("❯ Try typed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := plainBox(tt.styled); got != tt.want {
				t.Errorf("PlainBoxCapture = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestStrayInput(t *testing.T) {
	t.Parallel()

	rule := "────────────────────────────────"
	box := func(text string) string { return rule + "\n❯ " + text + "\n" + rule + "\n  ? for shortcuts\n" }
	tests := []struct {
		name     string
		capture  string
		wantText string
		wantOK   bool
	}{
		{"plus sign", box("+"), "+", true},
		{"hyphen", box("-"), "-", true},
		{"greater-than sign", box(">"), ">", true},
		{"a letter", box("a"), "", false},
		{"a digit", box("7"), "", false},
		{"the lone percent mark", readPaneFixture(t, "pane-idle-eol-mark.txt"), "", false},
		{"two characters", box("++"), "", false},
		{"a character beside a word", box("+ ok"), "", false},
		{"an empty box", box(""), "", false},
		{"a running turn over a lone character", "● working\n  esc to interrupt\n" + box("+"), "", false},
		{"no input box", "● some transcript\n", "", false},
	}
	c := &Claude{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			text, ok := c.StrayInput(tt.capture)
			if text != tt.wantText || ok != tt.wantOK {
				t.Errorf("StrayInput = (%q, %v); want (%q, %v)", text, ok, tt.wantText, tt.wantOK)
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
	t.Parallel()

	cases := []struct {
		name  string
		focus string
		want  []shuttleengine.PaneInput
	}{
		{"focus", "the open plan", []shuttleengine.PaneInput{{Text: "/compact the open plan"}, {Key: "Enter"}}},
		{"empty", "", []shuttleengine.PaneInput{{Text: "/compact"}, {Key: "Enter"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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
	want := []shuttleengine.PaneInput{{Text: "/reload-plugins"}, {Key: "Enter"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReloadPluginsSequence = %#v; want %#v", got, want)
	}
}

func TestClearInputSequence(t *testing.T) {
	t.Parallel()

	got := New().ClearInputSequence()
	want := []shuttleengine.PaneInput{{Key: "C-u"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClearInputSequence = %#v; want %#v", got, want)
	}

	rule := "────────────────────────────────"
	box := func(rows ...string) string {
		return rule + "\n" + strings.Join(rows, "\n") + "\n" + rule + "\n  ? for shortcuts\n"
	}
	clear, join := shuttleengine.PaneInput{Key: "C-u"}, shuttleengine.PaneInput{Key: "BSpace"}
	restoredTests := []struct {
		name    string
		capture string
		want    []shuttleengine.PaneInput
	}{
		{"restored prompt fixture, three rows", plainBox(readPaneFixture(t, "pane-restored-prompt-styled.txt")), []shuttleengine.PaneInput{clear, join, clear, join, clear}},
		{"two rows", box("❯ first", "  second"), []shuttleengine.PaneInput{clear, join, clear}},
		{"blank middle row", box("❯ first", "", "  third"), []shuttleengine.PaneInput{clear, join, clear, join, clear}},
		{"one row", box("❯ only"), []shuttleengine.PaneInput{clear}},
		{"empty box", box("❯ "), nil},
		{"no box", "● some transcript\n", nil},
	}
	for _, tt := range restoredTests {
		t.Run("restored: "+tt.name, func(t *testing.T) {
			t.Parallel()
			got := New().RestoredPromptClearSequence(tt.capture)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RestoredPromptClearSequence = %#v; want %#v", got, tt.want)
			}
			for _, input := range got {
				if input.Key == "Escape" || input.Key == "C-c" || input.Key == "Enter" {
					t.Errorf("RestoredPromptClearSequence holds %q, which interrupts or submits", input.Key)
				}
			}
		})
	}
}

func TestPastePlaceholder(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", PasteCollapseMinChars)
	longWithLines := strings.Repeat("a line of the request\n", 40) + long
	tests := []struct {
		name string
		box  string
		text string
		want bool
	}{
		{"bare placeholder", "[Pasted text #1]", long, true},
		{"placeholder with line count", "[Pasted text #12 +40 lines]", longWithLines, true},
		{"placeholder with surrounding space", "  [Pasted text #3 +0 lines] ", long, true},
		{"a short text is never collapsed", "[Pasted text #1]", long[:PasteCollapseMinChars-1], false},
		{"line count differing from the text's newlines", "[Pasted text #12 +39 lines]", longWithLines, false},
		{"draft quoting the placeholder", "see [Pasted text #1] above", long, false},
		{"typed text", "hello", long, false},
		{"empty reading", "", long, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := New().OwnPastePlaceholder(tt.box, tt.text); got != tt.want {
				t.Errorf("OwnPastePlaceholder(%q, %d chars) = %v; want %v", tt.box, len(tt.text), got, tt.want)
			}
		})
	}

	if !New().PasteSubmits() {
		t.Error("PasteSubmits() = false, want true")
	}
}

func TestClearSessionSequence(t *testing.T) {
	t.Parallel()

	got := New().ClearSessionSequence()
	want := []shuttleengine.PaneInput{{Text: "/clear"}, {Key: "Enter"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClearSessionSequence = %#v; want %#v", got, want)
	}
}
