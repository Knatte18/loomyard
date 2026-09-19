//go:build integration

// emptycmd_integration_test.go proves AddStrand with Cmd: "" succeeds against a real tmux session
// and leaves a live pane, following contract_integration_test.go's own conventions for fixture
// setup, tmux-binary resolution, the skip when the multiplexer is absent, and teardown (via this
// package's shared newColdScratchEngine helper) rather than inventing a second rig.
//
// This is the one assumption in _mill/discussion.md's empty-cmd-leaves-the-panes-own-shell
// decision that no existing code already pins: launchStrandLocked issues
// `send-keys -t <pane> -l ""` followed by Enter for an empty command, and
// sendKeysLiteralArg("") returns the empty string, so this test confirms tmux accepts that
// argument rather than assuming it.

package reedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestAddStrand_EmptyCmdLeavesALivePane proves AddStrand accepts Cmd: "" and leaves the resulting
// pane live: an empty command still launches the pane's own shell, rather than send-keys -l ""
// being refused or the pane failing to come up.
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
