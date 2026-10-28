//go:build tier1

// asker_integration_test.go proves Asker over a real hub: the walk starts at a beam or the prime itself and at a pair's recorded parent.

package orchroute

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

func TestAsker(t *testing.T) {
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
	beamLive := func(live bool) func(*lymcwd.Location) (bool, error) {
		return func(worktree *lymcwd.Location) (bool, error) { return live && worktree.WorktreeName == "b", nil }
	}
	beamOrch := hubforge.TestShortname + ":b:orch"
	primeOrch := hubforge.TestShortname + ":orch"

	tests := []struct {
		name         string
		asker        *lymcwd.Location
		live         bool
		unrecordBeam bool
		wantName     string
		wantRecorded bool
	}{
		{"A beam with its orch live answers its own orch", beam, true, false, beamOrch, true},
		{"A beam with its orch not live answers the prime's", beam, false, false, primeOrch, true},
		{"The prime answers the prime's", h.Location, true, false, primeOrch, true},
		{"A member pair answers its beam's live orch", member, true, false, beamOrch, true},
		{"A walk ending at an orch with no strand recorded answers not recorded", beam, true, true, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
