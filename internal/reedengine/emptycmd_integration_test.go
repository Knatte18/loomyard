//go:build integration

// emptycmd_integration_test.go proves AddStrand with Cmd: "" succeeds against a real tmux session
// and leaves a live pane, following contract_integration_test.go's own conventions for fixture
// setup, tmux-binary resolution, the skip when the multiplexer is absent, and teardown (via this
// package's shared newColdScratchEngine helper) rather than inventing a second rig.
//
// This is the one assumption in this task's _mill/discussion.md prelude-is-session-scoped-in-both-dialects
// decision that no existing code already pins: with the pane-binary prelude in place (panebin.go),
// an empty-Cmd strand's launchStrandLocked send-keys literal is the composed prelude alone, with no
// trailing separator and no empty command fragment, rather than the empty string send-keys -l ""
// this test pinned before the prelude landed. This test confirms a real tmux accepts that payload
// and leaves the pane live -- the reason the empty-Cmd operator pane now receives a session-scoped
// statement in both dialects rather than nothing on POSIX.

package reedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestAddStrand_EmptyCmdLeavesALivePane proves AddStrand accepts Cmd: "" and leaves the resulting
// pane live: an empty command still leaves a live pane running the pane's own shell, now with the
// pane-binary prelude applied to it, rather than that an empty send-keys literal is accepted.
func TestAddStrand_EmptyCmdLeavesALivePane(t *testing.T) {
	e := newColdScratchEngine(t)

	strand, err := e.AddStrand(AddSpec{
		Cmd:          "",
		NameOverride: "empty-cmd-strand",
		Display:      render.Display{Anchor: render.AnchorBelowParent},
	})
	if err != nil {
		t.Fatalf("AddStrand(Cmd: \"\") = %v, want a nil error", err)
	}

	status, err := e.Status()
	if err != nil {
		t.Fatalf("Status() = %v, want a nil error", err)
	}

	var found *StrandStatus
	for i := range status.Strands {
		if status.Strands[i].GUID == strand.GUID {
			found = &status.Strands[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("Status() strands = %+v, want an entry for the added strand %q", status.Strands, strand.GUID)
	}
	if !found.Live {
		t.Errorf("added strand's Status() entry = %+v, want Live = true", found)
	}
}
