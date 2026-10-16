//go:build tmux

// paste_integration_test.go drives PasteSubmit against a real tmux pane running cat:
// the text and the Enter arrive, a long line pastes whole, no lyx-inject buffer outlives the paste, and the operator's own paste buffer stays on top of the default stack.

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// waitForCapture polls guid's pane until pred accepts its screen, and returns that screen.
func waitForCapture(t *testing.T, e *Engine, guid string, pred func(screen string) bool) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen, err := e.CapturePane(guid)
		if err != nil {
			t.Fatalf("CapturePane: %v", err)
		}
		if pred(screen) {
			return screen
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane screen never satisfied the wait; last screen:\n%s", screen)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestPasteSubmit_DeliversTextAndLeavesTheBufferStackAlone(t *testing.T) {
	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	strand, err := e.AddStrand(AddSpec{Cmd: "cat", Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}
	if err := e.tmux.run("set-buffer", "operator-buffer"); err != nil {
		t.Fatalf("set-buffer: %v", err)
	}

	if err := e.PasteSubmit(strand.GUID, "submitted line", true); err != nil {
		t.Fatalf("PasteSubmit(submit): %v", err)
	}
	// cat echoes the submitted line once typed and once more on its own output, so the Enter shows as two copies.
	waitForCapture(t, e, strand.GUID, func(screen string) bool {
		return strings.Count(screen, "submitted line") >= 2
	})

	if err := e.PasteSubmit(strand.GUID, "draft only", false); err != nil {
		t.Fatalf("PasteSubmit(no submit): %v", err)
	}
	screen := waitForCapture(t, e, strand.GUID, func(screen string) bool {
		return strings.Contains(screen, "draft only")
	})
	if strings.Count(screen, "draft only") != 1 {
		t.Errorf("screen shows %d copies of the unsubmitted text, want 1:\n%s", strings.Count(screen, "draft only"), screen)
	}

	// A long line, dash-leading and semicolon-ending, goes whole through standard input.
	// The pane's terminal is made non-canonical because a canonical tty line holds at most 4095 bytes.
	sinkFile := filepath.Join(t.TempDir(), "long-paste.txt")
	sink, err := e.AddStrand(AddSpec{Cmd: "stty -icanon -echo; cat > " + sinkFile, Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand(sink): %v", err)
	}
	longText := "-" + strings.Repeat("x", 70000) + ";"
	if err := e.PasteSubmit(sink.GUID, longText, false); err != nil {
		t.Fatalf("PasteSubmit(long): %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if info, err := os.Stat(sinkFile); err == nil && info.Size() >= int64(len(longText)) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	got, err := os.ReadFile(sinkFile)
	if err != nil {
		t.Fatalf("read the sink file: %v", err)
	}
	if string(got) != longText {
		t.Errorf("sink file holds %d bytes, want the %d-byte long paste verbatim", len(got), len(longText))
	}

	buffers := mustTmuxOutput(t, e, "list-buffers", "-F", "#{buffer_name}")
	if strings.Contains(buffers, "lyx-inject-") {
		t.Errorf("list-buffers = %q, want no lyx-inject- buffer left behind", buffers)
	}
	top := strings.TrimSpace(mustTmuxOutput(t, e, "show-buffer"))
	if top != "operator-buffer" {
		t.Errorf("top of the default buffer stack = %q, want the operator's own buffer", top)
	}
}
