// pane_test.go classifies the pane captures the spike recorded and pins the key choreography sent into the pane.

package codexengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// Pane captures as the spike recorded them (docs/research/codex/codex-spike-2026-10-24/report.md).
const (
	readyCapture = `  >_ OpenAI Codex (v0.164.0)
     /w

  Tip: Use the desktop app.

› Ask Codex to do anything

  GPT-6-Luna low · /w · Workspace · Context 0% used · 5h 97% left · weekly 100%`

	busyCapture = `› Reply with the single word ok.

Working (0s • esc to interrupt)

› Ask Codex to do anything

  GPT-6-Luna low · /w · Workspace · Context 0% used · 5h 97% left · weekly 100…`

	idleAfterTurnCapture = `› Reply with the single word ok.

• ok

  Worked for 1s • 11:56 AM

› Ask Codex to do anything

  GPT-6-Luna low · /w · Reply with ok · Workspace · Context 1% used · 5h 97% l…`

	draftCapture = `› Ask Codex to do anything

• done

› first line of a draft
  second line

  GPT-6-Luna low · /w · Workspace · Context 1% used`

	trustCapture = `  Folder access
  /w

  Trust this folder? Codex can read, edit, and run files here, subject to your permission settings. Folder settings can run code
  automatically, even without a model request. Continue only if you trust these files. Your trust decision will be saved.

› 1. Trust and continue
  2. Quit

  enter continue · esc quit`

	hookReviewCapture = `  Hooks need review
  4 hooks are new or changed.
  Hooks can run outside the sandbox after you trust them.

› 1. Review hooks
  2. Trust all and continue
  3. Continue without trusting (hooks won't run)

  enter confirm · esc skip`

	quotedTrustCapture = `› Does Codex ask "Trust this folder?" here?

• It does, once per folder.

› Ask Codex to do anything

  GPT-6-Luna low · /w · Workspace`
)

// TestStartup pins the startup classification of each capture.
func TestStartup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		capture string
		want    shuttleengine.StartupState
	}{
		{"Ready", readyCapture, shuttleengine.StartupReady},
		{"Busy", busyCapture, shuttleengine.StartupReady},
		{"TrustPrompt", trustCapture, shuttleengine.StartupTrustPrompt},
		{"TrustPromptWithoutItsFooter", strings.TrimSuffix(trustCapture, "  enter continue · esc quit"), shuttleengine.StartupTrustPrompt},
		{"QuotedTrustQuestion", quotedTrustCapture, shuttleengine.StartupReady},
		{"HookReviewPrompt", hookReviewCapture, shuttleengine.StartupPending},
		{"Booting", "", shuttleengine.StartupPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := New().Startup(tt.capture); got != tt.want {
				t.Errorf("Startup(%s) = %v; want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestTrustDismissSequence pins the accept: Enter alone with the caret on the option, a move first when the caret sits on the other option, and nothing when the option is missing or far from the caret.
func TestTrustDismissSequence(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	caretOnQuit := strings.Replace(strings.Replace(trustCapture, "› 1. Trust", "  1. Trust", 1), "  2. Quit", "› 2. Quit", 1)
	tests := []struct {
		name    string
		capture string
		want    []shuttleengine.PaneInput
	}{
		{"CaretOnAccept", trustCapture, []shuttleengine.PaneInput{{Key: "Enter"}}},
		{"CaretOnQuit", caretOnQuit, []shuttleengine.PaneInput{{Key: "Up", SettleMS: optionMoveSettleMS}, {Key: "Enter"}}},
		{"NoAcceptOption", strings.Replace(trustCapture, "Trust and continue", "Continue", 1), nil},
		{"NoCaret", strings.ReplaceAll(trustCapture, "›", " "), nil},
		{"QuotedOptionFarFromCaret", "  1. Trust and continue, it said\n\n\n\n› Ask Codex to do anything", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := New().TrustDismissSequence(tt.capture); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TrustDismissSequence(%s) = %+v; want %+v", tt.name, got, tt.want)
			}
		})
	}
}

// TestComposerReadings pins the composer text, the running turn and the idle reading of each capture.
func TestComposerReadings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		capture     string
		wantText    string
		wantBox     bool
		wantRunning bool
		wantIdle    bool
	}{
		{name: "Ready", capture: readyCapture, wantBox: true, wantIdle: true},
		{name: "IdleAfterTurn", capture: idleAfterTurnCapture, wantBox: true, wantIdle: true},
		{name: "Busy", capture: busyCapture, wantBox: true, wantRunning: true},
		{name: "Draft", capture: draftCapture, wantText: "first line of a draft second line", wantBox: true},
		{name: "PastePlaceholder", capture: "› [Pasted Content 1686 chars]\n\n  GPT-6-Luna low", wantText: "[Pasted Content 1686 chars]", wantBox: true},
		{name: "NoComposer", capture: "loading"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine := New()
			text, box := engine.InputBoxText(tt.capture)
			if text != tt.wantText || box != tt.wantBox {
				t.Errorf("InputBoxText() = %q, %v; want %q, %v", text, box, tt.wantText, tt.wantBox)
			}
			if got := engine.RunningTurn(tt.capture); got != tt.wantRunning {
				t.Errorf("RunningTurn() = %v; want %v", got, tt.wantRunning)
			}
			if got := engine.IdleSession(tt.capture); got != tt.wantIdle {
				t.Errorf("IdleSession() = %v; want %v", got, tt.wantIdle)
			}
		})
	}
}

// TestPaneTooShort pins that only a capture with no composer and fewer rows than the composer and status line take reads too short.
func TestPaneTooShort(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		capture string
		want    bool
	}{
		{"› Ask Codex to do anything", false},
		{"Working (3s • esc to interrupt)\n", true},
		{"a\nb\nc", false},
	}
	for _, tt := range tests {
		if got := New().PaneTooShort(tt.capture); got != tt.want {
			t.Errorf("PaneTooShort(%q) = %v; want %v", tt.capture, got, tt.want)
		}
	}
}

// TestOwnPastePlaceholder pins that the placeholder is a paste's own only when its count is the text's characters and it is the whole reading.
func TestOwnPastePlaceholder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	text := strings.Repeat("é", 1686)
	tests := []struct {
		boxText string
		want    bool
	}{
		{"[Pasted Content 1686 chars]", true},
		{" [Pasted Content 1686 chars] ", true},
		{"[Pasted Content 1687 chars]", false},
		{"see [Pasted Content 1686 chars]", false},
	}
	for _, tt := range tests {
		if got := New().OwnPastePlaceholder(tt.boxText, text); got != tt.want {
			t.Errorf("OwnPastePlaceholder(%q) = %v; want %v", tt.boxText, got, tt.want)
		}
	}
	if New().PasteSubmits() {
		t.Error("PasteSubmits() = true; want false, so shuttle sends its own guarded Enter")
	}
}

// TestKeySequences pins the interrupt, the typed send with its Enter after the paste-burst settle, the typing alone, and the engine's empty color and session commands.
func TestKeySequences(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	engine := New()
	checks := []struct {
		name string
		got  []shuttleengine.PaneInput
		want []shuttleengine.PaneInput
	}{
		{"InterruptSequence", engine.InterruptSequence(), []shuttleengine.PaneInput{{Key: "Escape"}}},
		{"ComposeSend", engine.ComposeSend("hi"), []shuttleengine.PaneInput{{Text: "hi", SettleMS: composeSubmitSettleMS}, {Key: "Enter"}}},
		{"TypeSequence", engine.TypeSequence("hi"), []shuttleengine.PaneInput{{Text: "hi"}}},
		{"ColorSequence", engine.ColorSequence("red"), nil},
		{"ClearSessionSequence", engine.ClearSessionSequence(), nil},
		{"CompactSessionSequence", engine.CompactSessionSequence("focus"), nil},
		{"ReloadPluginsSequence", engine.ReloadPluginsSequence(), nil},
	}
	for _, check := range checks {
		if !reflect.DeepEqual(check.got, check.want) {
			t.Errorf("%s = %+v; want %+v", check.name, check.got, check.want)
		}
	}
}
