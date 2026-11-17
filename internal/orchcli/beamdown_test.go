// beamdown_test.go pins the order of `lym beam stop`.
// It removes the orch strand first and ends the dobby only when it is live.
// It ends the prime's coverage-map upkeep pass whether or not the dobby is live, so a second call stops nothing.

package orchcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBeamDown_StopsOrchThenTheDobbyByRole(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// dobbyLive is whether the dobby holds its lock when beamDown reads it.
		dobbyLive bool
		// noStrand leaves reed tracking no orch strand.
		noStrand bool
		// testMap wires the upkeep pass end, as for the prime; testMapFails makes it fail.
		testMap        bool
		testMapFails   bool
		wantCalls      []string
		wantOrchGone   bool
		wantDobbyEnded bool
		// wantErr is the text the returned error holds, empty for none.
		wantErr string
	}{
		{name: "the tracked orch is removed and then the live dobby ended", dobbyLive: true, wantCalls: []string{"remove", "end-dobby"}, wantOrchGone: true, wantDobbyEnded: true},
		{name: "a dead dobby is not ended", wantCalls: []string{"remove"}, wantOrchGone: true},
		{name: "a second call stops nothing", noStrand: true, wantCalls: nil},
		{name: "the prime's upkeep pass is ended after its live dobby", dobbyLive: true, testMap: true, wantCalls: []string{"remove", "end-dobby", "end-test-map"}, wantOrchGone: true, wantDobbyEnded: true},
		{name: "the prime's upkeep pass is ended though its dobby is dead", testMap: true, wantCalls: []string{"remove", "end-test-map"}, wantOrchGone: true},
		{name: "an upkeep pass end that fails is returned after the dobby ended", dobbyLive: true, testMap: true, testMapFails: true, wantCalls: []string{"remove", "end-dobby", "end-test-map"}, wantOrchGone: true, wantDobbyEnded: true, wantErr: "end the coverage-map upkeep pass"},
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
			if tt.testMap {
				h.cli.endTestMap = func() error {
					h.calls = append(h.calls, "end-test-map")
					if tt.testMapFails {
						return errors.New("survivors")
					}
					return nil
				}
			}

			report, err := h.cli.beamDown()

			if tt.wantErr == "" && err != nil {
				t.Fatalf("beamDown() error = %v; want none", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "survivors")) {
				t.Fatalf("beamDown() error = %v; want one holding %q and the end's cause", err, tt.wantErr)
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
