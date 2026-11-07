// startup_test.go table-tests how Startup classifies pane-capture fixtures:
// the trust screen, readiness by the input marker or the shortcuts footer, and a still-booting capture.
// It also checks the fixed shape of InterruptSequence and ComposeSend.

package claudeengine

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// realBypassGateCapture is the Bypass Permissions acceptance modal as claude 2.1.263 renders it.
// It is transcribed from a live pane.
// Like the trust gate, its caret sits on the REFUSING option.
const realBypassGateCapture = `  WARNING: Claude Code running in Bypass Permissions mode

  In Bypass Permissions mode, Claude Code will not ask for your approval before running potentially dangerous commands.
  This mode should only be used in a sandboxed container/VM that has restricted internet access and can easily be restored if damaged.

  By proceeding, you accept all responsibility for actions taken while running in Bypass Permissions mode.

  https://code.claude.com/docs/en/security

  ❯ No, exit
    Yes, I accept

  Enter to confirm · Esc to cancel`

func TestStartup_Classification(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		capture string
		want    shuttleengine.StartupState
	}{
		{
			// The real trust dialog, captured live from a tmux pane under claude 2.1.200.
			// The TUI's space-stripping rendering quirk is kept verbatim.
			// The dialog contains the "❯" ready marker as its option-selection caret,
			// so the classifier must not read that marker as a ready pane.
			// A capture classified as ready here would never dismiss the dialog.
			// It guards against that misclassification.
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
			// The "Yes, proceed" option is in gateAcceptNeedles, so startupGateNeedles matches this capture on its own.
			// The capture carries only the option list and the gate footer, the shape a cropped or scrolled viewport produces.
			// Reed's capture-pane carries no -S, so it is viewport-only.
			// The caret sits on the option line, so the ready marker alone would read this capture as ready.
			// Read as ready, the startup deadline would silently stop covering the gate.
			name:    "trust_prompt_older_wording_no_prose_in_capture",
			capture: "❯ 2. No, exit\n  1. Yes, proceed\n\nEnter to confirm · Esc to cancel",
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			// A gate phrase with its accepting option and no caret or ready marker is still a gate.
			// The option line alone is the positive evidence Startup requires.
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
			// Startup reads its "shortcuts" needle directly, not through ReadyFooterFixture.
			// This row ties the exported fixture to Startup's classification.
			// Without it, a rewording of the needle leaves the fixture stale.
			// Only a live-substrate caller built from the fixture would notice.
			name:    "ready_shortcuts_footer",
			capture: ReadyFooterFixture,
			want:    shuttleengine.StartupReady,
		},
		{
			name:    "ready_idle_input_box_fixture",
			capture: IdleInputBoxFixture,
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
			// A ready pane whose agent text mentions trusting a folder must still classify Ready.
			// The trust needles match whole phrases ("trust this folder", "files in this folder").
			// They do not match loose word co-occurrence, which keeps the trust-first ordering from masking an already-ready pane.
			name:    "ready_wins_over_coincidental_trust_words",
			capture: "❯ please trust that the folder layout is correct before proceeding",
			want:    shuttleengine.StartupReady,
		},
		{
			// The bypass-permissions ready footer carries no "shortcuts" text, so "❯" alone must be a sufficient ready marker.
			// The bypass gate below is keyed on its own accepting-option label, not on its banner text.
			// A needle matching "bypass permissions" would classify this running pane as a gate and never let a run start.
			name:    "ready_bypass_permissions_footer",
			capture: "❯\n⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
			want:    shuttleengine.StartupReady,
		},
		{
			// The bypass-permissions acceptance modal, captured live from a tmux pane (claude 2.1.263).
			// It is the second one-time gate, raised after the trust gate clears.
			// It is raised on every --dangerously-skip-permissions launch, which is every launch lym makes.
			// It carries the "❯" ready caret, as the trust gate does.
			// Classifying it as Ready would set the run started and park it on a dialog until the master timeout.
			name:    "bypass_permissions_gate_is_a_gate_not_ready",
			capture: realBypassGateCapture,
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			// A gate phrase inside the agent's own transcript is not a gate.
			// Classifying it as one would play TrustDismissSequence's keys into a live agent's pane.
			// The run would then die at the startup deadline.
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
			// The bypass gate's accepting label is the needle the gate is keyed on.
			// An agent quoting it is the sharpest case, since the phrase is a gate option elsewhere.
			name:    "ready_agent_prose_quoting_the_bypass_accept_label",
			capture: "● The modal's accepting option reads yes, i accept — noted.\n\n❯\n? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			// The gate footer is the second piece of positive evidence.
			// A gate whose accepting option is not in gateAcceptNeedles must still fail fast at the startup deadline.
			// It must not park until the run timeout.
			name:    "unrecognized_gate_wording_with_the_gate_footer_is_still_a_gate",
			capture: "Do you trust the files in this folder?\n\n ❯ Decline\n   Affirm\n\n Enter to confirm · Esc to cancel",
			want:    shuttleengine.StartupTrustPrompt,
		},
		{
			name:    "ready_agent_prose_asking_whether_to_trust_the_files_in_this_folder",
			capture: "● Do you trust the files in this folder? I do.\n\n❯\n? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			// A markdown list item that begins with an accept phrase supplies the gate needle and the option-line evidence.
			// The only caret is the healthy pane's input-box marker at the bottom, far from the prose.
			// A rendered gate's caret sits on one of two adjacent option lines, so this must classify Ready.
			name:    "ready_agent_prose_list_item_starting_with_the_accept_label",
			capture: "● Here is my assessment:\n\n- Yes, I accept the risk of merging now\n- The suite is green\n\n❯\n? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			// Prose that carries the gate footer phrase ("press Enter to confirm") and a needle mention.
			// The footer counts as evidence only strictly below the caret, where a rendered gate draws it.
			// Transcript prose sits above the input-box caret, so this must classify Ready.
			name:    "ready_agent_prose_with_footer_phrase_and_needle_mention",
			capture: "● The modal's accepting option reads yes, i accept.\n● Then press Enter to confirm the selection.\n\n❯\n? for shortcuts",
			want:    shuttleengine.StartupReady,
		},
		{
			// The picker's caret marks it ready, so the guards that close the picker run after the ready-agent-pane check.
			name:    "rewind picker reads ready",
			capture: readPaneFixture(t, "pane-rewind-picker.txt"),
			want:    shuttleengine.StartupReady,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New()
			got := c.Startup(tt.capture)
			if got != tt.want {
				t.Errorf("Startup(%q) = %v; want %v", tt.capture, got, tt.want)
			}
		})
	}
}

func TestInterruptSequence(t *testing.T) {
	tiergate.Require(t, 0)
	c := New()
	got := c.InterruptSequence()
	want := []shuttleengine.PaneInput{{Key: "Escape"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("InterruptSequence() = %+v; want %+v", got, want)
	}
}

// realTrustGateCapture is the trust gate as claude 2.1.263 renders it, transcribed from a live pane.
// The caret sits on the refusing option, so a single Enter quits claude instead of trusting the folder.
const realTrustGateCapture = ` Accessing workspace:
 /home/hanf/Code/r5sandbox/lym-test-HUB/r5-crash

 Quick safety check: Is this a project you created or one you trust? (Like your own code, a
 well-known open source project, or work from your team). If not, take a moment to review what's
 in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 Security guide

 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel`

func TestTrustDismissSequence(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		capture string
		want    []shuttleengine.PaneInput
	}{
		{
			// A bare Enter here confirms "No, exit" and claude quits.
			// An agent spawned in a not-yet-trusted directory therefore dies at startup.
			name:    "real gate with the caret on the refusing option moves down first",
			capture: realTrustGateCapture,
			want: []shuttleengine.PaneInput{
				{Key: "Down", SettleMS: gateSelectSettleMS},
				{Key: "Enter"},
			},
		},
		{
			// The same mechanism dismisses the second gate, with no second code path.
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
			// Scrollback carrying an older gate must not decide the answer:
			// only the gate drawn at the bottom of the pane is on screen.
			name:    "only the last caret and last accepting option count",
			capture: "   Yes, I trust this folder\n ❯ No, exit\n" + realTrustGateCapture,
			want: []shuttleengine.PaneInput{
				{Key: "Down", SettleMS: gateSelectSettleMS},
				{Key: "Enter"},
			},
		},
		{
			// TestStartup_Classification treats the older "Yes, proceed" trust-gate wording as a recognized gate.
			// The dismissal must act on every recognized gate.
			// A gate it cannot act on presses nothing and dies at the startup deadline.
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
			// A caret and an accepting option that are not adjacent are not a gate, so press nothing.
			// The accept phrase sits far above the input-box caret.
			// A walk from there sends Up presses (history recall in claude's input box) and an Enter into a live pane.
			name:    "prose accept line far from the input-box caret presses nothing at all",
			capture: "● Here is my assessment:\n\n- Yes, I accept the risk of merging now\n- The suite is green\n\n❯\n? for shortcuts",
			want:    nil,
		},
		{
			// A healthy, ready pane whose agent transcript renders a gate phrase must get no pane input at all.
			// Choosing the wrong key was not the defect; sending any key was.
			// TestStartup_Classification pins the same captures as Ready.
			name:    "live pane whose agent prose names the files in this folder presses nothing",
			capture: "● I'll start by reading the files in this folder.\n\n❯\n⏵⏵ bypass permissions on (shift+tab to cycle)",
			want:    nil,
		},
		{
			name:    "live pane whose agent prose names trust this folder presses nothing",
			capture: "● You asked whether to trust this folder; I'd say yes.\n\n❯\n? for shortcuts",
			want:    nil,
		},
		{
			name:    "live pane whose agent prose quotes the accept label presses nothing",
			capture: "● The modal's accepting option reads yes, i accept — noted.\n\n❯\n? for shortcuts",
			want:    nil,
		},
		{
			name:    "live pane whose agent prose asks about the files in this folder presses nothing",
			capture: "● Do you trust the files in this folder? I do.\n\n❯\n? for shortcuts",
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

func TestComposeSend(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		engine       *Claude
		redrawSettle time.Duration
	}{
		{"default settle", New(), 300 * time.Millisecond},
		{"configured settle", NewFromConfig(shuttleengine.Config{SubmitSettleMS: 120, SubmitRedrawSettleMS: 80}, configengine.Dirs{}), 80 * time.Millisecond},
		{"zero redraw settle reads at once", NewFromConfig(shuttleengine.Config{SubmitSettleMS: 120}, configengine.Dirs{}), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.engine.SubmitSettle(); got != tt.redrawSettle {
				t.Errorf("SubmitSettle() = %v, want %v", got, tt.redrawSettle)
			}

			got := tt.engine.ComposeSend("hello")
			want := []shuttleengine.PaneInput{
				{Text: "hello"},
				{Key: "Enter"},
			}
			if len(got) != len(want) {
				t.Fatalf("ComposeSend(%q) = %+v; want %+v", "hello", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("ComposeSend(%q)[%d] = %+v; want %+v", "hello", i, got[i], want[i])
				}
			}

			// TypeSequence is the text step alone.
			wantTyped := []shuttleengine.PaneInput{{Text: "hello"}}
			typed := tt.engine.TypeSequence("hello")
			if len(typed) != len(wantTyped) {
				t.Fatalf("TypeSequence(%q) = %+v; want %+v", "hello", typed, wantTyped)
			}
			for i := range wantTyped {
				if typed[i] != wantTyped[i] {
					t.Errorf("TypeSequence(%q)[%d] = %+v; want %+v", "hello", i, typed[i], wantTyped[i])
				}
			}
		})
	}
}
