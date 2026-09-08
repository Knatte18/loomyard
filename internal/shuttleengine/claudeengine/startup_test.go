// startup_test.go table-tests Startup's classification of pane-capture fixtures (trust screen,
// ready via the input marker, ready via the shortcuts footer, and a still-booting capture) and
// checks the fixed shape of InterruptSequence and ComposeSend.

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// realBypassGateCapture is the Bypass Permissions acceptance modal exactly as claude 2.1.263
// renders it, transcribed from a live pane during crucible round opus-medium-r5. Like the trust
// gate, its caret sits on the REFUSING option.
const realBypassGateCapture = `  WARNING: Claude Code running in Bypass Permissions mode

  In Bypass Permissions mode, Claude Code will not ask for your approval before running potentially dangerous commands.
  This mode should only be used in a sandboxed container/VM that has restricted internet access and can easily be restored if damaged.

  By proceeding, you accept all responsibility for actions taken while running in Bypass Permissions mode.

  https://code.claude.com/docs/en/security

  ❯ No, exit
    Yes, I accept

  Enter to confirm · Esc to cancel`

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
			// SF-1 (crucible round sonnet-xhigh-r8): the "Yes, proceed" wording's own leading prose
			// paragraph ("Do you trust the files in this folder?") is what always supplied
			// startupGateNeedles' hit in every fixture above -- this capture carries ONLY the option
			// list and the gate footer, the exact shape a cropped or scrolled viewport produces
			// (reed's capture-pane carries no -S, so it is viewport-only). Before the fix,
			// startupGateNeedles had no needle for "yes,proceed" at all (unlike gateAcceptNeedles,
			// which has recognized it as dismissable since R6-2), so this classified StartupReady --
			// the caret sits right there on the option line -- silently disabling the startup
			// deadline for a genuinely rendered gate.
			name:    "trust_prompt_older_wording_no_prose_in_capture",
			capture: "❯ 2. No, exit\n  1. Yes, proceed\n\nEnter to confirm · Esc to cancel",
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			// A gate phrase with a gate's accepting option present but no caret and no ready marker
			// is still a gate: the option line alone is the positive evidence Startup requires.
			name:    "trust_prompt_case_insensitive",
			capture: "DO YOU TRUST THIS FOLDER?\n  YES, I TRUST THIS FOLDER\n  NO, EXIT",
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
			// It is also the reason the bypass GATE below is keyed on its own
			// accepting-option label rather than on its banner text: a needle
			// matching "bypass permissions" would classify this healthy,
			// running pane as a gate and never let any run start.
			name:    "ready_bypass_permissions_footer",
			capture: "❯\n⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
			want:    shuttleengine.StartupReady,
		},
		{
			// The bypass-permissions acceptance modal, captured live from a
			// tmux pane (claude 2.1.263). It is the SECOND one-time gate, raised
			// after the trust gate clears, on every --dangerously-skip-permissions
			// launch — which is every launch lyx makes. It carries the "❯" ready
			// caret exactly as the trust gate does, so classifying it Ready set
			// *started and let a run park on a dialog for the whole master
			// timeout (crucible round opus-medium-r5, R5-7).
			name:    "bypass_permissions_gate_is_a_gate_not_ready",
			capture: realBypassGateCapture,
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			// A gate phrase inside the agent's OWN transcript is not a gate. Before this,
			// classifying it one played TrustDismissSequence's keys into a live agent's pane and
			// then killed the run at the startup deadline (crucible round opus-medium-r6, R6-1).
			name:    "ready_agent_prose_naming_the_files_in_this_folder",
			capture: "● I'll start by reading the files in this folder.\n\n❯\n⏵⏵ bypass permissions on (shift+tab to cycle)",
			want:    shuttleengine.StartupReady,
		},
		{
			name:    "ready_agent_prose_naming_trust_this_folder",
			capture: "● You asked whether to trust this folder; I'd say yes.\n\n❯\n? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			// The bypass gate's own accepting label is the needle R5-7 keyed on, so an agent
			// quoting it is the sharpest case: the phrase IS a gate option elsewhere.
			name:    "ready_agent_prose_quoting_the_bypass_accept_label",
			capture: "● The modal's accepting option reads yes, i accept — noted.\n\n❯\n? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			// The footer is the second piece of positive evidence: a gate whose accepting option
			// has been reworded out of gateAcceptNeedles must still fail FAST at the startup
			// deadline rather than parking until the run timeout.
			name:    "unrecognized_gate_wording_with_the_gate_footer_is_still_a_gate",
			capture: "Do you trust the files in this folder?\n\n ❯ Decline\n   Affirm\n\n Enter to confirm · Esc to cancel",
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			// R7-F1's first shape: ONE prose line that is a markdown list item beginning with an
			// accept phrase supplies both the gate needle and (before the adjacency rule) the
			// accepting-option-line evidence by itself. The only caret is the healthy pane's own
			// input-box marker at the bottom, far from the prose — a rendered gate's caret sits ON
			// one of two adjacent option lines, so this must classify Ready.
			name:    "ready_agent_prose_list_item_starting_with_the_accept_label",
			capture: "● Here is my assessment:\n\n- Yes, I accept the risk of merging now\n- The suite is green\n\n❯\n? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			// R7-F1's second shape: prose carrying the gate FOOTER phrase ("press Enter to
			// confirm") plus a needle mention. The footer evidence only counts strictly BELOW the
			// caret, where a rendered gate draws it — transcript prose sits above the input-box
			// caret, so this must classify Ready rather than killing the run at the startup
			// deadline.
			name:    "ready_agent_prose_with_footer_phrase_and_needle_mention",
			capture: "● The modal's accepting option reads yes, i accept.\n● Then press Enter to confirm the selection.\n\n❯\n? for shortcuts",
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

// realTrustGateCapture is the trust gate exactly as claude 2.1.263 renders it, transcribed from a
// live pane during crucible round opus-medium-r5: the caret sits on the REFUSING option, which is
// what made the previous fixed single-Enter dismissal quit claude instead of trusting the folder.
const realTrustGateCapture = ` Accessing workspace:
 /home/hanf/Code/r5sandbox/lyx-test-HUB/r5-crash

 Quick safety check: Is this a project you created or one you trust? (Like your own code, a
 well-known open source project, or work from your team). If not, take a moment to review what's
 in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide

 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel`

func TestTrustDismissSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		capture string
		want    []shuttleengine.PaneInput
	}{
		{
			// The regression case: a bare Enter here confirms "No, exit" and claude quits, which
			// is how every agent spawned in a not-yet-trusted directory died at startup.
			name:    "real gate with the caret on the refusing option moves down first",
			capture: realTrustGateCapture,
			want: []shuttleengine.PaneInput{
				{Key: "Down", SettleMS: gateSelectSettleMS},
				{Key: "Enter"},
			},
		},
		{
			// The same mechanism must carry the SECOND gate with no second code path.
			name:    "bypass-permissions gate walks the caret onto Yes, I accept",
			capture: realBypassGateCapture,
			want: []shuttleengine.PaneInput{
				{Key: "Down", SettleMS: gateSelectSettleMS},
				{Key: "Enter"},
			},
		},
		{
			name:    "caret already on the accepting option confirms without moving",
			capture: "   No, exit\n ❯ Yes, I trust this folder\n Enter to confirm",
			want:    []shuttleengine.PaneInput{{Key: "Enter"}},
		},
		{
			name:    "accepting option above the caret moves up",
			capture: "   Yes, I trust this folder\n ❯ No, exit",
			want: []shuttleengine.PaneInput{
				{Key: "Up", SettleMS: gateSelectSettleMS},
				{Key: "Enter"},
			},
		},
		{
			// Scrollback carrying an older gate must not decide the answer: only the gate drawn at
			// the bottom of the pane is the one on screen.
			name:    "only the last caret and last accepting option count",
			capture: "   Yes, I trust this folder\n ❯ No, exit\n" + realTrustGateCapture,
			want: []shuttleengine.PaneInput{
				{Key: "Down", SettleMS: gateSelectSettleMS},
				{Key: "Enter"},
			},
		},
		{
			// The older "Yes, proceed" trust-gate wording TestStartup_Classification has always
			// treated as a recognized gate must be DISMISSABLE too: a gate the classifier
			// recognizes and the dismissal cannot act on presses nothing and dies at the startup
			// deadline (crucible round opus-medium-r6, R6-2).
			name:    "older Yes, proceed trust-gate wording is dismissable",
			capture: "Do you trust the files in this folder?\n ❯ 2. No, exit\n   1. Yes, proceed\n Enter to confirm · Esc to cancel",
			want: []shuttleengine.PaneInput{
				{Key: "Down", SettleMS: gateSelectSettleMS},
				{Key: "Enter"},
			},
		},
		{
			name:    "no accepting option in the capture presses nothing at all",
			capture: " ❯ Some unrecognized option\n   Another one",
			want:    nil,
		},
		{
			name:    "no caret in the capture presses nothing at all",
			capture: "   Yes, I trust this folder",
			want:    nil,
		},
		{
			// R7-F1's key-press half: with the accept-phrase prose line far above the healthy
			// pane's own input-box caret, the pre-fix walk sent a burst of Up presses (history
			// recall in claude's input box) and an Enter into a live pane. A caret and accepting
			// option that are not adjacent are not a gate — press nothing.
			name:    "prose accept line far from the input-box caret presses nothing at all",
			capture: "● Here is my assessment:\n\n- Yes, I accept the risk of merging now\n- The suite is green\n\n❯\n? for shortcuts",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := New().TrustDismissSequence(tt.capture)
			if len(got) != len(tt.want) {
				t.Fatalf("TrustDismissSequence() = %+v; want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("TrustDismissSequence()[%d] = %+v; want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestTrustDismissSequence_NeverConfirmsWithoutSelectingAccept is the sabotage-proof for the
// defect's actual shape: whatever the sequence is, an Enter must never be reachable while the caret
// is still on an option the accepting-option needles do not match.
func TestTrustDismissSequence_NeverConfirmsWithoutSelectingAccept(t *testing.T) {
	t.Parallel()

	got := New().TrustDismissSequence(realTrustGateCapture)
	if len(got) == 0 {
		t.Fatal("TrustDismissSequence() returned nothing for the real, recognized gate; want a caret move plus Enter")
	}
	if got[0].Key == "Enter" {
		t.Fatalf("TrustDismissSequence() confirms as its FIRST step (%+v) while the caret is on %q; that confirms the refusing option and quits claude", got[0], "No, exit")
	}
}

// TestTrustDismissSequence_PressesNothingIntoALiveAgentsPane is the sabotage-proof for R6-1's actual
// shape: a healthy, ready pane whose agent transcript happens to render a gate phrase must produce NO
// pane input at all. The defect was not that the wrong key was chosen — it was that any key was sent.
func TestTrustDismissSequence_PressesNothingIntoALiveAgentsPane(t *testing.T) {
	t.Parallel()

	livePaneCaptures := []string{
		"● I'll start by reading the files in this folder.\n\n❯\n⏵⏵ bypass permissions on (shift+tab to cycle)",
		"● You asked whether to trust this folder; I'd say yes.\n\n❯\n? for shortcuts",
		"● The modal's accepting option reads yes, i accept — noted.\n\n❯\n? for shortcuts",
		"● Do you trust the files in this folder? I do.\n\n❯\n? for shortcuts",
	}
	for _, capture := range livePaneCaptures {
		c := New()
		if got := c.Startup(capture); got != shuttleengine.StartupReady {
			t.Errorf("Startup(%q) = %v; want StartupReady — a live agent's own transcript is not a gate", capture, got)
		}
		if got := c.TrustDismissSequence(capture); len(got) != 0 {
			t.Errorf("TrustDismissSequence(%q) = %+v; want no inputs — those keys land in a working agent's pane", capture, got)
		}
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
