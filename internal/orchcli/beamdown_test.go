//go:build tier0

// beamdown_test.go pins that `lyx beam stop` removes the orch strand first and then ends the dobby only when it is live, so a second call stops nothing.

package orchcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestBeamDown_StopsOrchThenTheDobbyByRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// dobbyLive is whether the dobby holds its lock when beamDown reads it.
		dobbyLive bool
		// noStrand leaves reed tracking no orch strand.
		noStrand       bool
		wantCalls      []string
		wantOrchGone   bool
		wantDobbyEnded bool
	}{
		{name: "the tracked orch is removed and then the live dobby ended", dobbyLive: true, wantCalls: []string{"remove", "end-dobby"}, wantOrchGone: true, wantDobbyEnded: true},
		{name: "a dead dobby is not ended", wantCalls: []string{"remove"}, wantOrchGone: true},
		{name: "a second call stops nothing", noStrand: true, wantCalls: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var strands []reedengine.StrandStatus
			if !tt.noStrand {
				strands = []reedengine.StrandStatus{{GUID: "g1", Name: "orch", Live: true}}
			}
			h := newStartHarness(t, strands...)
			if err := orchengine.SaveState(h.cli.paths, orchengine.State{Strand: "g1", Phase: orchengine.PhaseIdle}); err != nil {
				t.Fatal(err)
			}
			h.cli.dobbyLive = func() (bool, error) { return tt.dobbyLive, nil }
			h.cli.endDobby = func() error {
				h.calls = append(h.calls, "end-dobby")
				return nil
			}

			report, err := h.cli.beamDown()

			if err != nil {
				t.Fatalf("beamDown() error = %v; want none", err)
			}
			if !slices.Equal(h.calls, tt.wantCalls) {
				t.Errorf("calls = %v; want %v", h.calls, tt.wantCalls)
			}
			if report.OrchRemoved != tt.wantOrchGone || report.DobbyEnded != tt.wantDobbyEnded {
				t.Errorf("beamDown() = %+v; want orch removed %v and dobby ended %v", report, tt.wantOrchGone, tt.wantDobbyEnded)
			}
			if report.Worktree != h.cli.location.WorktreeName {
				t.Errorf("beamDown() worktree = %q; want %q", report.Worktree, h.cli.location.WorktreeName)
			}
		})
	}
}
