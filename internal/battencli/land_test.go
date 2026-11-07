// land_test.go covers the Land row's wiring:
// how a pull's error splits into a records-side stop and a failure.
// It also covers what the landed-child seam reads from the task worktree's status.

package battencli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLandDeps_ChildLandedReadsTheTaskWorktreeStatus asserts the wired seam reports a child landed only when:
// its status history holds a Finalize with outcome Done.
// A run with no Finalize row, a motif run, reads as landing nothing.
func TestLandDeps_ChildLandedReadsTheTaskWorktreeStatus(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	locateErr := errors.New("no task worktree")
	finalize := func(outcome shedengine.Outcome) *shedengine.Status {
		return &shedengine.Status{State: shedengine.StateDone, History: []shedengine.HistoryEntry{{Producer: "Discussion-Bouncer", Outcome: shedengine.Done}, {Producer: loomshed.NameFinalize, Outcome: outcome}}}
	}
	tests := []struct {
		name      string
		status    *shedengine.Status
		locateErr error
		want      bool
		wantErr   error
	}{
		{name: "AbsentStatusLandedNothing"},
		{name: "HistoryWithoutFinalizeLandedNothing", status: &shedengine.Status{State: shedengine.StateDone, History: []shedengine.HistoryEntry{{Producer: "Discussion-Bouncer", Outcome: shedengine.Done}}}},
		{name: "FinalizeNotDoneLandedNothing", status: finalize(shedengine.Stuck)},
		{name: "FinalizeDoneLanded", status: finalize(shedengine.Done), want: true},
		{name: "AbsentTaskWorktreeIsAnError", locateErr: locateErr, wantErr: locateErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prime := &lymcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "hub-repo", AnchorRel: "."}
			task := &lymcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "hub-repo", AnchorRel: "."}
			if tt.status != nil {
				statusPath, lockPath := shedrun.StatusFile(task, shedrun.SelfRunID), shedrun.StatusLock(task, shedrun.SelfRunID)
				for _, dir := range []string{filepath.Dir(statusPath), filepath.Dir(lockPath)} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if err := state.WriteJSON(statusPath, lockPath, *tt.status); err != nil {
					t.Fatal(err)
				}
			}

			deps := landDeps(prime, "a-slug", func() (*lymcwd.Location, error) { return task, tt.locateErr })
			got, err := deps.ChildLanded()
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("ChildLanded() = %v, %v; want %v, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

// TestPullStopOutcome asserts a lone records-side stop comes back as its message with a nil error.
// It also asserts that a stop joined with a code-side error or any other error comes back as that error with no stop.
func TestPullStopOutcome(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stop := fabricengine.NewRecordsStopError("main", errors.New("replay conflict"), "settle it")
	codeSideErr := errors.New("code side fetch failed")
	joined := errors.Join(stop, codeSideErr)
	unrelated := errors.New("open fabric failed")

	tests := []struct {
		name        string
		err         error
		wantStop    string
		wantPullErr error
	}{
		{"LoneRecordsStop", stop, stop.Error(), nil},
		{"RecordsStopJoinedWithCodeSideError", joined, "", joined},
		{"UnrelatedError", unrelated, "", unrelated},
		{"NoError", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStop, gotErr := pullStopOutcome(tt.err)
			if gotStop != tt.wantStop || gotErr != tt.wantPullErr {
				t.Errorf("pullStopOutcome(%v) = %q, %v; want %q, %v", tt.err, gotStop, gotErr, tt.wantStop, tt.wantPullErr)
			}
		})
	}
}
