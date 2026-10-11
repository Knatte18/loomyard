//go:build llm

// smoke_paste_test.go pins, against a REAL claude in a REAL tmux pane, the facts the paste path rests on:
// an Enter right after reed's bracketed paste submits it, a slash command pasted that way starts no turn, and a long paste collapses into a placeholder from claudeengine.PasteCollapseMinChars characters.
// It is the operator's check when Claude Code changes, and the plan's verify compiles it and never runs it.

package shuttlecli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
)

// pasteObservationWindow is how long the test waits for a pasted text's effect before reading it as absent.
const pasteObservationWindow = 45 * time.Second

// turnStartsAndEnds returns how many turn starts and turn ends the events file holds.
func turnStartsAndEnds(t *testing.T, claude *claudeengine.Claude, eventsPath string) (starts, ends int) {
	t.Helper()
	data, err := os.ReadFile(eventsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read events: %v", err)
	}
	signals, _ := claude.ParseSessionSignals(data)
	for _, signal := range signals {
		switch signal.Kind {
		case shuttleengine.SessionSignalTurnStart:
			starts++
		case shuttleengine.SessionSignalTurnEnd, shuttleengine.SessionSignalAPIErrorTurnEnd:
			ends++
		}
	}
	return starts, ends
}

// readBoxText returns the input box's text as the engine reads it from the strand's pane right now, with ok false for a pane showing no readable box.
func readBoxText(t *testing.T, reedEngine *reedengine.Engine, claude *claudeengine.Claude, guid string) (string, bool) {
	t.Helper()
	capture, err := reedEngine.CapturePane(guid)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	return claude.InputBoxText(capture)
}

// waitFor polls condition once a second until it holds or the observation window passes, and reports whether it held.
func waitFor(condition func() bool) bool {
	deadline := time.Now().Add(pasteObservationWindow)
	for {
		if condition() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Second)
	}
}

// TestSmokePasteSubmitAndCollapse drives reed's PasteSubmit into a ready claude run and reads what the pane did.
// A plain one-line text pasted with submit true starts a turn and leaves the box empty.
// `/color green` pasted with submit true leaves the box empty and starts no turn.
// Both observations must equal Claude.PasteSubmits(), so a Claude Code release that turns the post-paste Enter into a newline fails here and names the constant to flip.
// A text of claudeengine.PasteCollapseMinChars characters pasted with submit false shows a placeholder that OwnPastePlaceholder accepts for it, and one character shorter shows the text itself.
func TestSmokePasteSubmitAndCollapse(t *testing.T) {
	llmkit.Claude(t, "LYX_REED_CLAUDE")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.Path)
	registerReedServer(t, h)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	runner, _, reedEngine, _ := newSmokeRunner(t)
	claude := claudeengine.New()
	run, err := runner.Start(shuttleengine.Spec{
		Prompt:      "Reply with the single word ready and then wait for my next message. Use no tools.",
		OutputFiles: []string{filepath.Join(h.PrimeWorktree(), "smoke-paste-output.txt")},
		Model:       smokeClaudeModel,
		Timeout:     10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runner.Start: %v", err)
	}
	guid := run.StrandGUID()
	eventsPath := filepath.Join(run.RunDir(), "events.jsonl")

	boxEmpty := func() bool {
		text, ok := readBoxText(t, reedEngine, claude, guid)
		return ok && strings.TrimSpace(text) == ""
	}
	turnsSettled := func() bool {
		starts, ends := turnStartsAndEnds(t, claude, eventsPath)
		return starts >= 1 && starts == ends && boxEmpty()
	}
	if !waitFor(turnsSettled) {
		t.Fatal("the run's first turn did not settle with an empty box")
	}

	startsBefore, _ := turnStartsAndEnds(t, claude, eventsPath)
	if err := reedEngine.PasteSubmit(guid, "Reply with the single word pong.", true); err != nil {
		t.Fatalf("PasteSubmit of a plain text: %v", err)
	}
	plainSubmitted := waitFor(func() bool {
		starts, _ := turnStartsAndEnds(t, claude, eventsPath)
		return starts > startsBefore
	}) && waitFor(turnsSettled)
	if plainSubmitted != claude.PasteSubmits() {
		t.Errorf("a plain text pasted with submit true was submitted = %v, but Claude.PasteSubmits() = %v; flip pasteSubmitsOnEnter in claudeengine", plainSubmitted, claude.PasteSubmits())
	}

	startsBefore, _ = turnStartsAndEnds(t, claude, eventsPath)
	if err := reedEngine.PasteSubmit(guid, "/color green", true); err != nil {
		t.Fatalf("PasteSubmit of /color green: %v", err)
	}
	slashSubmitted := waitFor(boxEmpty)
	if starts, _ := turnStartsAndEnds(t, claude, eventsPath); starts != startsBefore {
		t.Errorf("/color green started a turn: turn starts %d, want %d", starts, startsBefore)
	}
	if slashSubmitted != claude.PasteSubmits() {
		t.Errorf("/color green pasted with submit true left an empty box = %v, but Claude.PasteSubmits() = %v; flip pasteSubmitsOnEnter in claudeengine", slashSubmitted, claude.PasteSubmits())
	}

	clearBox := func() {
		t.Helper()
		if err := reedEngine.SendKey(guid, "C-u"); err != nil {
			t.Fatalf("SendKey C-u: %v", err)
		}
		if !waitFor(boxEmpty) {
			t.Fatal("the input box did not empty after C-u")
		}
	}

	collapsing := strings.Repeat("x", claudeengine.PasteCollapseMinChars)
	if err := reedEngine.PasteSubmit(guid, collapsing, false); err != nil {
		t.Fatalf("PasteSubmit of a collapsing text: %v", err)
	}
	var collapsedBox string
	if !waitFor(func() bool {
		collapsedBox, _ = readBoxText(t, reedEngine, claude, guid)
		return claude.OwnPastePlaceholder(collapsedBox, collapsing)
	}) {
		t.Errorf("a paste of %d characters shows the box %q, want a placeholder OwnPastePlaceholder accepts", len(collapsing), collapsedBox)
	}
	clearBox()

	shorter := collapsing[:len(collapsing)-1]
	if err := reedEngine.PasteSubmit(guid, shorter, false); err != nil {
		t.Fatalf("PasteSubmit of a text below the collapse length: %v", err)
	}
	var shorterBox string
	if !waitFor(func() bool {
		shorterBox, _ = readBoxText(t, reedEngine, claude, guid)
		return strings.TrimSpace(shorterBox) != ""
	}) {
		t.Fatal("a paste below the collapse length left the box empty")
	}
	if got := strings.Join(strings.Fields(shorterBox), ""); got != shorter {
		t.Errorf("a paste of %d characters shows the box %q, want the text itself", len(shorter), shorterBox)
	}
	clearBox()
}
