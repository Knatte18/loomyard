// asker_integration_test.go proves Asker over a real hub: the walk starts at a beam or the prime itself and at a pair's recorded parent.
// The worktree it starts at keeps the notice while reed holds its orch strand, live or not.

package orchroute

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAsker(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "m", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	hubforge.AddPairWith(t, h, "p", fabricengine.AddOptions{SkipPush: true})
	locate := func(slug string) *lymcwd.Location {
		t.Helper()
		location, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree(%q): %v", slug, err)
		}
		return location
	}
	beam, member := locate("b"), locate("m")
	seed := func(location *lymcwd.Location, guid string) {
		t.Helper()
		if err := orchengine.SaveState(Paths(location), orchengine.State{Phase: orchengine.PhaseIdle, Strand: guid}); err != nil {
			t.Fatalf("SaveState(%s): %v", location.WorktreeName, err)
		}
	}
	seed(h.Location, "prime-guid")
	seed(beam, "beam-guid")
	beamOrch := hubforge.TestShortname + ":b:orch"
	holdBeamStrand := func(held bool) {
		t.Helper()
		reedState := &reedengine.ReedState{}
		if held {
			reedState.Strands = []reedengine.Strand{{GUID: "beam-guid", Name: beamOrch}}
		}
		if err := reedengine.SaveState(filepath.Join(beam.AnchorPath(), lymdirs.DotLymDirName), reedState); err != nil {
			t.Fatalf("reedengine.SaveState: %v", err)
		}
	}
	holdBeamStrand(true)
	beamLive := func(live bool) func(*lymcwd.Location) (bool, error) {
		return func(worktree *lymcwd.Location) (bool, error) { return live && worktree.WorktreeName == "b", nil }
	}
	primeOrch := hubforge.TestShortname + ":orch"

	tests := []struct {
		name         string
		asker        *lymcwd.Location
		live         bool
		unrecordBeam bool
		stopBeam     bool
		wantName     string
		wantRecorded bool
	}{
		{"A beam with its orch live answers its own orch", beam, true, false, false, beamOrch, true},
		{"A beam whose orch reads as down still answers its own orch", beam, false, false, false, beamOrch, true},
		{"A beam with no orch strand and none live answers the prime's", beam, false, true, false, primeOrch, true},
		{"A beam whose orch strand reed no longer holds, as after a stop, answers the prime's", beam, false, false, true, primeOrch, true},
		{"The prime answers the prime's", h.Location, true, false, false, primeOrch, true},
		{"A member pair answers its beam's live orch", member, true, false, false, beamOrch, true},
		{"A walk ending at an orch with no strand recorded answers not recorded", beam, true, true, false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stopBeam {
				holdBeamStrand(false)
				defer holdBeamStrand(true)
			}
			if tt.unrecordBeam {
				seed(beam, "")
				defer seed(beam, "beam-guid")
			}
			name, recorded, err := Asker(tt.asker, beamLive(tt.live))
			if err != nil || recorded != tt.wantRecorded || name != tt.wantName {
				t.Errorf("Asker(%s) = (%q, %v, %v); want (%q, %v, nil)", tt.asker.WorktreeName, name, recorded, err, tt.wantName, tt.wantRecorded)
			}
		})
	}
}
