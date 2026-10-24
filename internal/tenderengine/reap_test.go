//go:build tier0

// reap_test.go drives Reap over fake end seams and a scripted owner record, worktree list and process listing.

package tenderengine

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// reapResult is what a fake end seam answers for one owner or pid.
type reapResult struct {
	ended []runproc.Ended
	err   error
}

func TestReap(t *testing.T) {
	t.Parallel()
	record := func(pid int, role string) runproc.Record {
		return runproc.Record{PID: pid, Role: role, StartTime: fmt.Sprintf("s%d", pid)}
	}
	survived := &runproc.SurvivorsError{Survivors: []runproc.Ended{{PID: 10, Role: runproc.RoleLoop}}}
	stray := proc.Process{PID: 50, StartTime: "s50", Exe: "/bin/lyx", Cwd: "/hub/gone (deleted)"}

	tests := []struct {
		name         string
		runProcs     map[string][]runproc.Record
		worktrees    []string
		processes    []proc.Process
		ownerResults map[string]reapResult
		pidResults   map[int]reapResult
		wantOwners   []string
		wantPIDs     []string
		wantEnded    []ReapedProcess
		wantSurvived []ReapedProcess
		wantErr      []string
	}{
		{
			name:         "OrphanOwnerIsEndedAndPairBackedOwnerIsNot",
			runProcs:     map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop), record(11, runproc.RoleDriver)}, "kept": {record(12, runproc.RoleLoop)}},
			worktrees:    []string{"/hub/kept"},
			ownerResults: map[string]reapResult{"gone": {ended: []runproc.Ended{{PID: 10, Role: runproc.RoleLoop}, {PID: 11, Role: runproc.RoleDriver}}}},
			wantOwners:   []string{"gone"},
			wantEnded:    []ReapedProcess{{PID: 10, Role: runproc.RoleLoop}, {PID: 11, Role: runproc.RoleDriver}},
		},
		{
			name:       "OwnerWithARecordKeptForItsWorkingDirectoryHasOnlyTheOthersEnded",
			runProcs:   map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop), record(11, runproc.RoleDriver)}},
			worktrees:  []string{"/hub/other"},
			processes:  []proc.Process{{PID: 10, Cwd: "/hub/other/sub"}, {PID: 11, Cwd: "/elsewhere"}},
			pidResults: map[int]reapResult{11: {ended: []runproc.Ended{{PID: 11, Role: runproc.RoleDriver}}}},
			wantPIDs:   []string{"11@s11"},
			wantEnded:  []ReapedProcess{{PID: 11, Role: runproc.RoleDriver}},
		},
		{
			name:      "HubOwnerIsNeverEnded",
			runProcs:  map[string][]runproc.Record{runproc.HubOwner: {record(10, runproc.RoleSync)}},
			worktrees: []string{"/hub/other"},
		},
		{
			name:       "UnregisteredOrphanIsEndedWithTheReadingsStartTime",
			worktrees:  []string{"/hub/other"},
			processes:  []proc.Process{stray},
			pidResults: map[int]reapResult{50: {ended: []runproc.Ended{{PID: 50, Role: "unregistered"}}}},
			wantPIDs:   []string{"50@s50"},
			wantEnded:  []ReapedProcess{{PID: 50, Role: "unregistered"}},
		},
		{
			name:         "SurvivorYieldsAnErrorNamingItAndTheOtherOrphansAreStillEnded",
			runProcs:     map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop)}, "later": {record(20, runproc.RoleDriver)}},
			worktrees:    []string{"/hub/other"},
			ownerResults: map[string]reapResult{"gone": {err: survived}, "later": {ended: []runproc.Ended{{PID: 20, Role: runproc.RoleDriver}}}},
			wantOwners:   []string{"gone", "later"},
			wantEnded:    []ReapedProcess{{PID: 20, Role: runproc.RoleDriver}},
			wantSurvived: []ReapedProcess{{PID: 10, Role: runproc.RoleLoop}},
			wantErr:      []string{"10 (loop)", "run `lyx tender reap` again once they are gone"},
		},
		{
			name:         "NonSurvivorErrorIsReturnedAfterTheRemainingOrphansAreEnded",
			runProcs:     map[string][]runproc.Record{"gone": {record(10, runproc.RoleLoop)}, "later": {record(20, runproc.RoleDriver)}},
			worktrees:    []string{"/hub/other"},
			ownerResults: map[string]reapResult{"gone": {err: errors.New("signal refused")}, "later": {ended: []runproc.Ended{{PID: 20, Role: runproc.RoleDriver}}}},
			wantOwners:   []string{"gone", "later"},
			wantEnded:    []ReapedProcess{{PID: 20, Role: runproc.RoleDriver}},
			wantErr:      []string{"signal refused"},
		},
		{
			name: "NothingToReapReturnsAnEmptyReport",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			e.runProcs, e.worktrees, e.processes, e.hubPath, e.executable = tt.runProcs, tt.worktrees, tt.processes, "/hub", "/bin/lyx"
			deps := e.deps()
			var ownerCalls, pidCalls []string
			deps.EndOwner = func(owner string) ([]runproc.Ended, error) {
				ownerCalls = append(ownerCalls, owner)
				result := tt.ownerResults[owner]
				return result.ended, result.err
			}
			deps.EndProcess = func(pid int, startTime string) ([]runproc.Ended, error) {
				pidCalls = append(pidCalls, fmt.Sprintf("%d@%s", pid, startTime))
				result := tt.pidResults[pid]
				return result.ended, result.err
			}

			report, err := Reap(deps)

			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("Reap() error = %v; want it to contain %q", err, want)
				}
			}
			if len(tt.wantErr) == 0 && err != nil {
				t.Errorf("Reap() error = %v; want none", err)
			}
			if !reflect.DeepEqual(ownerCalls, tt.wantOwners) || !reflect.DeepEqual(pidCalls, tt.wantPIDs) {
				t.Errorf("end calls: owners %v, pids %v; want owners %v, pids %v", ownerCalls, pidCalls, tt.wantOwners, tt.wantPIDs)
			}
			wantEnded, wantSurvived := tt.wantEnded, tt.wantSurvived
			if wantEnded == nil {
				wantEnded = []ReapedProcess{}
			}
			if wantSurvived == nil {
				wantSurvived = []ReapedProcess{}
			}
			if !reflect.DeepEqual(report.Ended, wantEnded) || !reflect.DeepEqual(report.Survivors, wantSurvived) {
				t.Errorf("report ended %v, survivors %v; want %v, %v", report.Ended, report.Survivors, wantEnded, wantSurvived)
			}
		})
	}
}
