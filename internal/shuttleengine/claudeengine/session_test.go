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

func TestClearSessionSequence(t *testing.T) {
	got := (&Claude{}).ClearSessionSequence()
	want := []shuttleengine.PaneInput{{Text: "/clear", Submit: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClearSessionSequence = %#v; want %#v", got, want)
	}
}
