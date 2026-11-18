// strandops_integration_test.go proves ReplaceStrand and AddStrand with an empty Cmd against a real tmux.
// Replacing the first of three strands leaves the replacement in its slot of the strand table.
// The other strands' panes stay untouched.
// An empty Cmd still leaves a live pane running the pane's own shell, with the pane-binary prelude applied to it.
// Resume drops a non-live strand whose done-when paths all exist, and relaunches one whose list is partly missing.
// One cold session serves every step.
// Ending a strand's process holds its record under its GUID, and Resume leaves it down.
// RelaunchStrand brings it back with new commands, and a removal in between makes the relaunch refuse.
// Releasing the hold lets Resume relaunch it.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStrandOps_RealTmux runs its steps in order on one cold session.
// The empty-Cmd step relies on the three strands the replace step leaves behind, and asserts only on its own strand.
func TestStrandOps_RealTmux(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	e := newColdScratchEngine(t)

	t.Run("ReplaceKeepsTheTopSlot", func(t *testing.T) {
		var added []Strand
		for _, name := range []string{"top", "middle", "bottom"} {
			s, err := e.AddStrand(AddSpec{
				NameOverride: name,
				Provider:     "claude",
				Display:      render.Display{Anchor: render.AnchorOwnWindow},
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
			Provider:     "codex",
			Display:      render.Display{Anchor: render.AnchorOwnWindow},
		})
		if err != nil {
			t.Fatalf("ReplaceStrand = %v, want nil", err)
		}
		if replacement.Provider != "codex" {
			t.Errorf("replacement Provider = %q, want codex", replacement.Provider)
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

		live, err := e.tmux.listWindowPanes(e.SessionName())
		if err != nil {
			t.Fatalf("listWindowPanes: %v", err)
		}
		var sawTop bool
		for _, p := range live {
			if p.PaneID == after.Strands[0].PaneID {
				sawTop = true
			}
			if p.PaneID == added[0].PaneID {
				t.Errorf("replaced strand's pane %s is still live", p.PaneID)
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
			Display:      render.Display{Anchor: render.AnchorOwnWindow},
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

	t.Run("ResumeDropsAFinishedStrandAndRelaunchesAnUnfinishedOne", func(t *testing.T) {
		finishedPath := filepath.Join(t.TempDir(), "finished")
		if err := os.WriteFile(finishedPath, nil, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		display := render.Display{Anchor: render.AnchorOwnWindow}
		finished, err := e.AddStrand(AddSpec{NameOverride: "finished", Display: display, DoneWhen: []string{finishedPath}})
		if err != nil {
			t.Fatalf("AddStrand(finished) = %v, want nil", err)
		}
		unfinished, err := e.AddStrand(AddSpec{NameOverride: "unfinished", Provider: "claude", CreelHooks: true, Display: display, DoneWhen: []string{filepath.Join(t.TempDir(), "never-written")}})
		if err != nil {
			t.Fatalf("AddStrand(unfinished) = %v, want nil", err)
		}
		for _, strand := range []Strand{finished, unfinished} {
			if err := e.tmux.run("kill-pane", "-t", strand.PaneID); err != nil {
				t.Fatalf("kill-pane %s: %v", strand.PaneID, err)
			}
		}

		result, err := e.Resume()
		if err != nil {
			t.Fatalf("Resume = %v, want nil", err)
		}
		if result.Dropped != 1 || result.Resumed != 1 {
			t.Errorf("Resume result = %+v, want Dropped 1 and Resumed 1", result)
		}
		after, err := LoadState(e.stateDir())
		if err != nil || after == nil {
			t.Fatalf("LoadState after resume = (%+v, %v), want a readable state", after, err)
		}
		if _, ok := strandByGUID(after.Strands, finished.GUID); ok {
			t.Errorf("finished strand %q is still in state after Resume", finished.GUID)
		}
		relaunched, ok := strandByGUID(after.Strands, unfinished.GUID)
		if !ok || relaunched.PaneID == "" {
			t.Errorf("unfinished strand after Resume = (%+v, %v), want it present with a live pane", relaunched, ok)
		}
		if relaunched.Provider != "claude" {
			t.Errorf("relaunched strand Provider = %q, want claude kept", relaunched.Provider)
		}
		if !relaunched.CreelHooks {
			t.Errorf("relaunched strand CreelHooks = false, want true kept")
		}
	})
}

// strandStatusOf returns the Status entry of guid, failing the test when it has none.
func strandStatusOf(t *testing.T, e *Engine, guid string) StrandStatus {
	t.Helper()
	status, err := e.Status()
	if err != nil {
		t.Fatalf("Status() = %v, want nil", err)
	}
	for _, s := range status.Strands {
		if s.GUID == guid {
			return s
		}
	}
	t.Fatalf("Status() strands = %+v, want an entry for %q", status.Strands, guid)
	return StrandStatus{}
}

// TestEndAndRelaunchStrand_KeepTheGUID runs its steps in order on one cold session.
// Each step adds its own strand and leaves it live or removed.
// The Resume counts of the later steps therefore see no other non-live strand.
func TestEndAndRelaunchStrand_KeepTheGUID(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	e := newColdScratchEngine(t)
	display := render.Display{Anchor: render.AnchorOwnWindow}

	t.Run("EndedStrandStaysHeldAndDownThroughResumeThenRelaunchesUnderTheSameGUID", func(t *testing.T) {
		strand, err := e.AddStrand(AddSpec{NameOverride: "held", Cmd: "sleep 600", Provider: "claude", Display: display})
		if err != nil {
			t.Fatalf("AddStrand = %v, want nil", err)
		}

		if err := e.EndStrandProcess(strand.GUID); err != nil {
			t.Fatalf("EndStrandProcess = %v, want nil", err)
		}
		ended, ok := stateStrand(t, e, strand.GUID)
		if !ok || !ended.Held || ended.PaneID != "" || ended.Cmd != "sleep 600" {
			t.Errorf("record after EndStrandProcess = (%+v, %v), want it kept, held, unbound and with its command", ended, ok)
		}
		if status := strandStatusOf(t, e, strand.GUID); status.Live || !status.Held {
			t.Errorf("Status after EndStrandProcess = %+v, want not live and held", status)
		}

		result, err := e.Resume()
		if err != nil {
			t.Fatalf("Resume = %v, want nil", err)
		}
		if result.Resumed != 0 || len(result.Held) != 1 {
			t.Errorf("Resume result = %+v, want nothing resumed and the held strand reported", result)
		}
		if after, ok := stateStrand(t, e, strand.GUID); !ok || !after.Held || after.PaneID != "" {
			t.Errorf("record after Resume = (%+v, %v), want it still held and down", after, ok)
		}

		spec := RelaunchSpec{Cmd: "sleep 601", ResumeCmd: "sleep 602", SessionID: "session-2"}
		if err := e.RelaunchStrand(strand.GUID, spec); err != nil {
			t.Fatalf("RelaunchStrand = %v, want nil", err)
		}
		relaunched, ok := stateStrand(t, e, strand.GUID)
		if !ok || relaunched.Held || relaunched.PaneID == "" {
			t.Fatalf("record after RelaunchStrand = (%+v, %v), want it present, released and bound to a pane", relaunched, ok)
		}
		if relaunched.Cmd != spec.Cmd || relaunched.ResumeCmd != spec.ResumeCmd || relaunched.SessionID != spec.SessionID {
			t.Errorf("record commands after RelaunchStrand = (%q, %q, %q), want %+v",
				relaunched.Cmd, relaunched.ResumeCmd, relaunched.SessionID, spec)
		}
		if status := strandStatusOf(t, e, strand.GUID); !status.Live || status.Held {
			t.Errorf("Status after RelaunchStrand = %+v, want live and not held", status)
		}

		if err := e.RelaunchStrand(strand.GUID, spec); err == nil {
			t.Errorf("RelaunchStrand on a strand that is not held = nil, want a refusal")
		}
	})

	t.Run("RemovalBetweenEndAndRelaunchMakesTheRelaunchRefuse", func(t *testing.T) {
		strand, err := e.AddStrand(AddSpec{NameOverride: "removed", Cmd: "sleep 600", Display: display})
		if err != nil {
			t.Fatalf("AddStrand = %v, want nil", err)
		}
		if err := e.EndStrandProcess(strand.GUID); err != nil {
			t.Fatalf("EndStrandProcess = %v, want nil", err)
		}
		if _, err := e.RemoveStrand(strand.GUID, false); err != nil {
			t.Fatalf("RemoveStrand of a held strand = %v, want nil", err)
		}

		err = e.RelaunchStrand(strand.GUID, RelaunchSpec{Cmd: "sleep 601"})
		if !errors.Is(err, ErrUnknownStrand) {
			t.Errorf("RelaunchStrand after a removal = %v, want an error wrapping ErrUnknownStrand", err)
		}
		if _, ok := stateStrand(t, e, strand.GUID); ok {
			t.Errorf("strand %q is in state after its removal and a refused relaunch, want none", strand.GUID)
		}
	})

	t.Run("ReleasedStrandIsRelaunchedByResume", func(t *testing.T) {
		strand, err := e.AddStrand(AddSpec{NameOverride: "released", Cmd: "sleep 600", Display: display})
		if err != nil {
			t.Fatalf("AddStrand = %v, want nil", err)
		}
		if err := e.EndStrandProcess(strand.GUID); err != nil {
			t.Fatalf("EndStrandProcess = %v, want nil", err)
		}
		if err := e.ReleaseStrandHold(strand.GUID); err != nil {
			t.Fatalf("ReleaseStrandHold = %v, want nil", err)
		}
		if released, ok := stateStrand(t, e, strand.GUID); !ok || released.Held {
			t.Errorf("record after ReleaseStrandHold = (%+v, %v), want it present and not held", released, ok)
		}

		result, err := e.Resume()
		if err != nil {
			t.Fatalf("Resume = %v, want nil", err)
		}
		if result.Resumed != 1 {
			t.Errorf("Resume result = %+v, want Resumed 1", result)
		}
		if after, ok := stateStrand(t, e, strand.GUID); !ok || after.PaneID == "" {
			t.Errorf("record after Resume = (%+v, %v), want it bound to a relaunched pane", after, ok)
		}
	})
}

// stateStrand reads guid's record from the persisted state.
func stateStrand(t *testing.T, e *Engine, guid string) (Strand, bool) {
	t.Helper()
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
	}
	return strandByGUID(st.Strands, guid)
}
