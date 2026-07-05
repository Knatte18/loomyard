// startup_test.go table-tests Startup's classification of pane-capture
// fixtures (trust screen, ready via the input marker, ready via the
// shortcuts footer, and a still-booting capture) and checks the fixed shape
// of InterruptSequence and ComposeSend.

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestStartup_Classification(t *testing.T) {
	tests := []struct {
		name    string
		capture string
		want    shuttleengine.StartupState
	}{
		{
			name:    "trust_prompt",
			capture: "Do you trust the files in this folder?\n> 1. Yes, proceed\n  2. No, exit",
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			name:    "trust_prompt_case_insensitive",
			capture: "DO YOU TRUST THIS FOLDER?",
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			name:    "ready_input_marker",
			capture: "❯ ",
			want:    shuttleengine.StartupReady,
		},
		{
			name:    "ready_shortcuts_footer",
			capture: "? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			name:    "pending_cold_boot",
			capture: "Loading...",
			want:    shuttleengine.StartupPending,
		},
		{
			name:    "pending_empty",
			capture: "",
			want:    shuttleengine.StartupPending,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			got := c.Startup(tt.capture)
			if got != tt.want {
				t.Errorf("Startup(%q) = %v; want %v", tt.capture, got, tt.want)
			}
		})
	}
}

func TestInterruptSequence(t *testing.T) {
	c := New()
	got := c.InterruptSequence()
	want := []shuttleengine.PaneInput{{Key: "Escape"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("InterruptSequence() = %+v; want %+v", got, want)
	}
}

func TestComposeSend(t *testing.T) {
	c := New()
	got := c.ComposeSend("hello")
	want := []shuttleengine.PaneInput{
		{Key: "Escape"},
		{Text: "hello", Submit: true},
	}
	if len(got) != len(want) {
		t.Fatalf("ComposeSend(%q) = %+v; want %+v", "hello", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ComposeSend(%q)[%d] = %+v; want %+v", "hello", i, got[i], want[i])
		}
	}
}
