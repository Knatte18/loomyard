//go:build tier0

// start_test.go drives the start sequence over a fake bring-up: which target is prepared and how one beam's failure shows beside the others.

package beamcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/orchcli"
)

func TestStartBeams(t *testing.T) {
	t.Parallel()

	prime := beamTarget{Label: "prime"}
	alpha := beamTarget{Slug: "alpha", Label: "alpha"}
	beta := beamTarget{Slug: "beta", Label: "beta"}

	// entryView is what a startEntry shows of one beam: its label, dobby outcome, orch action, error and whether it carries the hub's reconcile.
	type entryView struct {
		Worktree  string
		Outcome   string
		Action    string
		Error     string
		Reconcile bool
	}

	tests := []struct {
		name         string
		targets      []beamTarget
		failing      string
		wantPrepared []string
		wantErr      string
		wantEntries  []entryView
	}{
		{
			name:         "OnlyTheFirstTargetIsPrepared",
			targets:      []beamTarget{prime, alpha, beta},
			wantPrepared: []string{"prime"},
			wantEntries: []entryView{
				{"prime", "started", "relaunched", "", true},
				{"alpha", "started", "relaunched", "", false},
				{"beta", "started", "relaunched", "", false},
			},
		},
		{
			name:         "AFailingMiddleTargetLeavesTheOthersStarted",
			targets:      []beamTarget{prime, alpha, beta},
			failing:      "alpha",
			wantPrepared: []string{"prime"},
			wantErr:      "alpha: boom",
			wantEntries: []entryView{
				{"prime", "started", "relaunched", "", true},
				{"alpha", "started", "relaunched", "boom", false},
				{"beta", "started", "relaunched", "", false},
			},
		},
		{
			name:        "AnEmptyListStartsNothing",
			wantEntries: []entryView{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var prepared, started []string
			result, err := startBeams(tt.targets, func(target beamTarget, prepare bool) (orchcli.BeamUpReport, error) {
				started = append(started, target.Label)
				if prepare {
					prepared = append(prepared, target.Label)
				}
				report := orchcli.BeamUpReport{Worktree: target.Label, Dobby: "started", Action: "relaunched"}
				if target.Label == tt.failing {
					return report, errors.New("boom")
				}
				return report, nil
			})

			if !slices.Equal(prepared, tt.wantPrepared) {
				t.Errorf("startBeams prepared %v; want %v", prepared, tt.wantPrepared)
			}
			if len(started) != len(tt.targets) {
				t.Errorf("startBeams started %v; want one start per target of %d", started, len(tt.targets))
			}
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("startBeams error = %v; want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("startBeams error = %v; want it to contain %q", err, tt.wantErr)
			}
			got := []entryView{}
			for _, entry := range result.Beams {
				got = append(got, entryView{entry.Worktree, entry.Outcome, entry.Action, entry.Error, entry.Reconcile != nil})
			}
			if diff := cmp.Diff(tt.wantEntries, got); diff != "" {
				t.Errorf("startBeams entries mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
