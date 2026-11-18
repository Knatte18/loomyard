// beamdown_test.go pins the order of `lym beam stop`.
// It removes the orch strand first, ends the dobby only when it is live, and ends the orch's process last,
// so a second call stops nothing.

package orchcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBeamDown_StopsOrchThenTheDobbyThenTheOrchProcess(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// dobbyLive is whether the dobby holds its lock when beamDown reads it.
		dobbyLive bool
		// noStrand leaves reed tracking no orch strand.
		noStrand bool
		// orchProcessLive is whether a registered orch process is left for the last step to end.
		orchProcessLive bool
		// endDobbyFails and endOrchFails make that end return an error.
		endDobbyFails bool
		endOrchFails  bool
		// wantErrStep is the step the returned error names; empty when beamDown succeeds.
		wantErrStep          string
		wantCalls            []string
		wantOrchGone         bool
		wantDobbyEnded       bool
		wantOrchProcessEnded bool
	}{
		{
			name: "the tracked orch is removed, then the live dobby and the orch process ended", dobbyLive: true, orchProcessLive: true,
			wantCalls: []string{"remove", "end-dobby", "end-orch"}, wantOrchGone: true, wantDobbyEnded: true, wantOrchProcessEnded: true,
		},
		{name: "a dead dobby is not ended", orchProcessLive: true, wantCalls: []string{"remove", "end-orch"}, wantOrchGone: true, wantOrchProcessEnded: true},
		{name: "a second call stops nothing", noStrand: true, wantCalls: []string{"end-orch"}},
		{
			name: "a failed dobby end still ends the orch process", dobbyLive: true, orchProcessLive: true, endDobbyFails: true,
			wantErrStep: "end the dobby", wantCalls: []string{"remove", "end-dobby", "end-orch"}, wantOrchGone: true, wantOrchProcessEnded: true,
		},
		{
			name: "a failed orch process end keeps what the earlier steps did", dobbyLive: true, endOrchFails: true,
			wantErrStep: "end the orch process", wantCalls: []string{"remove", "end-dobby", "end-orch"}, wantOrchGone: true, wantDobbyEnded: true,
		},
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
				if tt.endDobbyFails {
					return errors.New("refused")
				}
				return nil
			}
			h.cli.endOrch = func() ([]runproc.Ended, error) {
				h.calls = append(h.calls, "end-orch")
				if tt.endOrchFails {
					return nil, errors.New("refused")
				}
				if !tt.orchProcessLive {
					return nil, nil
				}
				return []runproc.Ended{{PID: 77, Role: runproc.RoleOrch}}, nil
			}

			report, err := h.cli.beamDown()

			wantErr := "orch: " + tt.wantErrStep + " of " + h.cli.location.WorktreeName
			if tt.wantErrStep == "" && err != nil {
				t.Fatalf("beamDown() error = %v; want none", err)
			}
			if tt.wantErrStep != "" && (err == nil || !strings.Contains(err.Error(), wantErr)) {
				t.Errorf("beamDown() error = %v; want one containing %q", err, wantErr)
			}
			if !slices.Equal(h.calls, tt.wantCalls) {
				t.Errorf("calls = %v; want %v", h.calls, tt.wantCalls)
			}
			if report.OrchRemoved != tt.wantOrchGone || report.DobbyEnded != tt.wantDobbyEnded || report.OrchProcessEnded != tt.wantOrchProcessEnded {
				t.Errorf("beamDown() = %+v; want orch removed %v, dobby ended %v and orch process ended %v",
					report, tt.wantOrchGone, tt.wantDobbyEnded, tt.wantOrchProcessEnded)
			}
			if report.Worktree != h.cli.location.WorktreeName {
				t.Errorf("beamDown() worktree = %q; want %q", report.Worktree, h.cli.location.WorktreeName)
			}
		})
	}
}
