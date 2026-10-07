//go:build tmux && linux

// waitmark_integration_test.go pins, against a real tmux server, that the default status line expands a marked pane into its title, label and elapsed minutes:
// the elapsed arithmetic inside the segment format can only be proven on the tmux in use.

package reedengine

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestWaitMarkSegmentExpandsOnARealTmux sets a mark through SetWaitMark and expands the pinned status-left against the marked pane.
// An unmarked session expands to the identity text alone.
func TestWaitMarkSegmentExpandsOnARealTmux(t *testing.T) {
	e := newColdScratchEngine(t)

	strand, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}
	target := exactSessionWindowTarget(e.SessionName())
	expandStatusLeft := func() string {
		out, err := e.tmux.output("display-message", "-p", "-t", target, "#{T:status-left}")
		if err != nil {
			t.Fatalf("expanding status-left: %v", err)
		}
		return strings.TrimSpace(out)
	}

	if got := expandStatusLeft(); strings.Contains(got, "⏳") {
		t.Fatalf("status-left before any mark = %q; want no wait segment", got)
	}

	if err := e.SetWaitMark(strand.GUID, "verify loom", time.Now().Add(-3*time.Minute)); err != nil {
		t.Fatalf("SetWaitMark: %v", err)
	}
	got := expandStatusLeft()
	if !regexp.MustCompile(`⏳verify loom [23]m`).MatchString(got) {
		t.Errorf("status-left with a mark = %q; want it to contain the label and a two-to-three minute elapsed count", got)
	}

	if err := e.SetWaitMark(strand.GUID, "", time.Time{}); err != nil {
		t.Fatalf("SetWaitMark (clear): %v", err)
	}
	if got := expandStatusLeft(); strings.Contains(got, "⏳") {
		t.Errorf("status-left after a clear = %q; want no wait segment", got)
	}
}
