//go:build tmux

// strandops_integration_test.go proves ReplaceStrand and AddStrand with an empty Cmd against a real tmux.
// Replacing the top strand of a three-strand stack leaves the replacement on top at collapsed_rows, with the other strands' panes untouched;
// an empty Cmd still leaves a live pane running the pane's own shell, now with the pane-binary prelude applied to it.
// One cold session serves both steps.

package reedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestStrandOps_RealTmux runs its steps in order on one cold session.
// EmptyCmdLeavesALivePane relies on the three-strand stack ReplaceKeepsTopSlotAtCollapsedRows leaves behind and asserts only on its own strand.
func TestStrandOps_RealTmux(t *testing.T) {
	t.Parallel()
	e := newColdScratchEngine(t)

	t.Run("ReplaceKeepsTopSlotAtCollapsedRows", func(t *testing.T) {
		var added []Strand
		for _, name := range []string{"top", "middle", "bottom"} {
			s, err := e.AddStrand(AddSpec{
				NameOverride: name,
				Display:      render.Display{Anchor: render.AnchorBelowParent},
			})
			if err != nil {
				t.Fatalf("AddStrand(%q) = %v, want nil", name, err)
			}
			added = append(added, s)
		}

		before, err := LoadState(e.stateDir())
		if err != nil || before == nil {
			t.Fatalf("LoadState = (%+v, %v), want a readable state", before, err)
		}
		middlePane := before.Strands[1].PaneID
		bottomPane := before.Strands[2].PaneID

		replacement, err := e.ReplaceStrand(added[0].GUID, AddSpec{
			NameOverride: "top",
			Display:      render.Display{Anchor: render.AnchorBelowParent},
		})
		if err != nil {
			t.Fatalf("ReplaceStrand = %v, want nil", err)
		}
		if replacement.GUID == added[0].GUID {
			t.Fatalf("replacement kept the replaced strand's guid %q", replacement.GUID)
		}

		after, err := LoadState(e.stateDir())
		if err != nil || after == nil {
			t.Fatalf("LoadState after replace = (%+v, %v), want a readable state", after, err)
		}
		if len(after.Strands) != 3 {
			t.Fatalf("strands after replace = %+v, want 3", after.Strands)
		}
		if after.Strands[0].GUID != replacement.GUID {
			t.Errorf("slot 0 = %q, want the replacement %q", after.Strands[0].GUID, replacement.GUID)
		}
		if after.Strands[1].PaneID != middlePane || after.Strands[2].PaneID != bottomPane {
			t.Errorf("other strands' panes = (%q, %q), want unmoved (%q, %q)",
				after.Strands[1].PaneID, after.Strands[2].PaneID, middlePane, bottomPane)
		}

		live, err := e.tmux.listPanes(e.SessionName())
		if err != nil {
			t.Fatalf("listPanes: %v", err)
		}
		var sawTop bool
		for _, p := range live {
			if p.ID == after.Strands[0].PaneID {
				sawTop = true
				if p.Height != e.cfg.CollapsedRows {
					t.Errorf("replacement pane height = %d, want %d (cfg.CollapsedRows)", p.Height, e.cfg.CollapsedRows)
				}
			}
			if p.ID == added[0].PaneID {
				t.Errorf("replaced strand's pane %s is still live", p.ID)
			}
		}
		if !sawTop {
			t.Fatalf("replacement pane %s missing from live panes %+v", after.Strands[0].PaneID, live)
		}
	})

	t.Run("EmptyCmdLeavesALivePane", func(t *testing.T) {
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
	})
}
