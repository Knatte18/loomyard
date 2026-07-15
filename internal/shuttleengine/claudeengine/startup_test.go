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
			// The REAL trust dialog as captured live from a tmux pane
			// (claude 2.1.200): the TUI's space-stripping rendering quirk is
			// preserved verbatim, and — critically — the dialog contains the
			// "❯" ready marker as its option-selection caret. This is the
			// regression case for the round-2 ready-first ordering, which
			// classified this capture Ready and never dismissed the dialog.
			name: "trust_prompt_real_dialog_with_ready_caret",
			capture: "Accessingworkspace:\n\nC:\\some\\fresh\\dir\n\n" +
				"Quicksafetycheck:Isthisaprojectyoucreatedoroneyoutrust?\n\n" +
				"ClaudeCode'llbeabletoread,edit,andexecutefileshere.\n\n" +
				"Securityguide\n\n❯1.Yes,Itrustthisfolder\n2.No,exit\n\nEntertoconfirm·Esctocancel",
			want: shuttleengine.StartupTrustPrompt,
		},
		{
			// The same dialog under a normal (space-preserving) rendering.
			name:    "trust_prompt_real_dialog_spaced",
			capture: "Security guide\n\n❯ 1. Yes, I trust this folder\n  2. No, exit\n\nEnter to confirm · Esc to cancel",
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			name:    "trust_prompt_older_wording",
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
		{
			// A ready pane whose agent text coincidentally mentions trusting
			// a folder must still classify Ready: the trust needles match
			// whole phrases ("trust this folder" / "files in this folder"),
			// never loose word co-occurrence, precisely so the trust-first
			// ordering cannot mask an already-ready pane (the round-2 L1
			// concern, preserved across the trust-first reordering).
			name:    "ready_wins_over_coincidental_trust_words",
			capture: "❯ please trust that the folder layout is correct before proceeding",
			want:    shuttleengine.StartupReady,
		},
		{
			// The bypass-permissions ready footer (captured live) carries no
			// "shortcuts" text at all — "❯" must remain a sufficient ready
			// marker on its own.
			name:    "ready_bypass_permissions_footer",
			capture: "❯\n⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
			want:    shuttleengine.StartupReady,
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

func TestTrustDismissSequence(t *testing.T) {
	c := New()
	got := c.TrustDismissSequence()
	want := []shuttleengine.PaneInput{{Key: "Enter"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("TrustDismissSequence() = %+v; want %+v", got, want)
	}
}

func TestComposeSend(t *testing.T) {
	c := New()
	got := c.ComposeSend("hello")
	want := []shuttleengine.PaneInput{
		{Key: "Escape", SettleMS: composeSendSettleMS},
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
