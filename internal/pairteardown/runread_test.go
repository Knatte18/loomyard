// runread_test.go drives ReadRun's reader through fake substrates, so it spawns nothing.

package pairteardown

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// runFakes is the state of the substrates a runReader reads, with a record of the seams it called.
type runFakes struct {
	boardStatus *string
	boardFound  bool

	batten      shedengine.Status
	battenFound bool

	taskBranchOnOrigin bool
	pairHere           bool

	childHere, childOrigin           shedengine.Status
	childHereFound, childOriginFound bool

	owner, adoptSubject string
	elsewhere           bool
	pauseCovers         bool

	skipPush          bool
	originParentHolds bool
	localParentHolds  bool
	restepped         bool

	boardErr error

	// calls lists the seams that tell a branch of the reader, in call order.
	calls []string
}

func (f *runFakes) reader() runReader {
	return runReader{
		board: func(string) (*string, bool, error) { return f.boardStatus, f.boardFound, f.boardErr },
		batten: func(string) (shedengine.Status, bool, error) {
			return f.batten, f.battenFound, nil
		},
		taskBranchOnOrigin: func(string) (bool, error) {
			f.calls = append(f.calls, "task-branch")
			return f.taskBranchOnOrigin, nil
		},
		pairHere: func(string) (bool, error) { return f.pairHere, nil },
		childHere: func(string) (shedengine.Status, bool, error) {
			f.calls = append(f.calls, "child-here")
			return f.childHere, f.childHereFound, nil
		},
		childOnOrigin: func(string) (shedengine.Status, bool, error) {
			f.calls = append(f.calls, "child-origin")
			return f.childOrigin, f.childOriginFound, nil
		},
		owner:       func(string) (string, string, bool, error) { return f.owner, f.adoptSubject, f.elsewhere, nil },
		pauseCovers: func(string) (bool, error) { return f.pauseCovers, nil },
		skipPush:    f.skipPush,
		originParentHolds: func(string) (bool, error) {
			f.calls = append(f.calls, "parent-origin")
			return f.originParentHolds, nil
		},
		localParentHolds: func(string) (bool, error) {
			f.calls = append(f.calls, "parent-local")
			return f.localParentHolds, nil
		},
		restepped: func(string, shedengine.Status) (bool, error) {
			f.calls = append(f.calls, "restepped")
			return f.restepped, nil
		},
	}
}

func statusAt(state shedengine.State, row string) shedengine.Status {
	return shedengine.Status{State: state, CurrentProducer: row}
}

func TestReadRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	empty, done := "", boardengine.StatusDone
	tests := []struct {
		name      string
		fakes     runFakes
		wantErr   string
		want      runfsm.Records
		wantIn    runfsm.Inputs
		wantCalls []string
	}{
		{
			name:      "ReleasedEntryWithNoRecordsReadsTheChildFromOrigin",
			fakes:     runFakes{boardStatus: &empty, boardFound: true},
			want:      runfsm.Records{Released: true},
			wantCalls: []string{"child-origin"},
		},
		{
			name:      "DoneEntry",
			fakes:     runFakes{boardStatus: &done, boardFound: true},
			want:      runfsm.Records{BoardDone: true},
			wantCalls: []string{"child-origin"},
		},
		{
			name: "BattenBeforeRunShedIsNotStarted",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, "Worktree-Create"), battenFound: true,
			},
			want:      runfsm.Records{Batten: statusAt(shedengine.StateRunning, "Worktree-Create"), BattenFound: true},
			wantCalls: []string{"child-origin"},
		},
		{
			name: "BattenAtRunShedIsStartedAndTheChildIsReadFromThePair",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, runShedRow), battenFound: true,
				pairHere: true, childHere: statusAt(shedengine.StateRunning, "Code-Burler"), childHereFound: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, runShedRow), BattenFound: true, BattenStarted: true,
				Child: statusAt(shedengine.StateRunning, "Code-Burler"), ChildFound: true,
			},
			wantCalls: []string{"child-here"},
		},
		{
			name: "CancelledRecordReadsTheTaskBranch",
			fakes: runFakes{
				batten: statusAt(shedengine.StateCancelled, "Code-Burler"), battenFound: true, taskBranchOnOrigin: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateCancelled, "Code-Burler"), BattenFound: true, TaskBranchOnOrigin: true,
			},
			wantCalls: []string{"task-branch", "child-origin"},
		},
		{
			name: "NoPairHereReadsTheChildFromOriginAndNoParentSeam",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, runShedRow), battenFound: true,
				childOrigin: statusAt(shedengine.StateRunning, loomshed.NameFinalize), childOriginFound: true,
				originParentHolds: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, runShedRow), BattenFound: true, BattenStarted: true,
				Child: statusAt(shedengine.StateRunning, loomshed.NameFinalize), ChildFound: true, ChildLanding: true,
			},
			wantCalls: []string{"child-origin"},
		},
		{
			name: "FinalizeRunningReadsTheParentFromOrigin",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, runShedRow), battenFound: true,
				pairHere: true, childHere: statusAt(shedengine.StateRunning, loomshed.NameFinalize), childHereFound: true,
				originParentHolds: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, runShedRow), BattenFound: true, BattenStarted: true,
				Child: statusAt(shedengine.StateRunning, loomshed.NameFinalize), ChildFound: true, ChildLanding: true,
			},
			wantIn:    runfsm.Inputs{PastParentMerge: true},
			wantCalls: []string{"child-here", "parent-origin"},
		},
		{
			name: "FinalizeRunningUnderThePushSkipReadsTheParentLocally",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, runShedRow), battenFound: true,
				pairHere: true, childHere: statusAt(shedengine.StateRunning, loomshed.NameFinalize), childHereFound: true,
				skipPush: true, localParentHolds: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, runShedRow), BattenFound: true, BattenStarted: true,
				Child: statusAt(shedengine.StateRunning, loomshed.NameFinalize), ChildFound: true, ChildLanding: true,
			},
			wantIn:    runfsm.Inputs{PastParentMerge: true},
			wantCalls: []string{"child-here", "parent-local"},
		},
		{
			name: "FinalizeFailedTransientReadsTheRestepRecordBeforeTheParent",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, runShedRow), battenFound: true,
				pairHere: true, childHereFound: true, originParentHolds: true,
				childHere: shedengine.Status{State: shedengine.StateFailed, CurrentProducer: loomshed.NameFinalize, Transient: "push"},
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, runShedRow), BattenFound: true, BattenStarted: true,
				Child:      shedengine.Status{State: shedengine.StateFailed, CurrentProducer: loomshed.NameFinalize, Transient: "push"},
				ChildFound: true, ChildLanding: true,
			},
			wantIn:    runfsm.Inputs{PastParentMerge: true},
			wantCalls: []string{"child-here", "restepped", "parent-origin"},
		},
		{
			name: "BattenAtLandIsPastTheParentMergeWithNoParentRead",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, landRow), battenFound: true,
				pairHere: true, childHere: statusAt(shedengine.StateRunning, loomshed.NameFinalize), childHereFound: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, landRow), BattenFound: true, BattenStarted: true,
				Child: statusAt(shedengine.StateRunning, loomshed.NameFinalize), ChildFound: true, ChildLanding: true,
			},
			wantIn:    runfsm.Inputs{PastParentMerge: true},
			wantCalls: []string{"child-here"},
		},
		{
			name: "BattenAtWorktreeTeardownIsPastTheParentMergeWithNoChild",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, worktreeTeardownRow), battenFound: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, worktreeTeardownRow), BattenFound: true, BattenStarted: true, ChildLanding: true,
			},
			wantIn:    runfsm.Inputs{PastParentMerge: true},
			wantCalls: []string{"child-origin"},
		},
		{
			name: "DoneChildIsLanding",
			fakes: runFakes{
				batten: statusAt(shedengine.StateRunning, runShedRow), battenFound: true,
				pairHere: true, childHere: statusAt(shedengine.StateDone, "Summary"), childHereFound: true,
			},
			want: runfsm.Records{
				Batten: statusAt(shedengine.StateRunning, runShedRow), BattenFound: true, BattenStarted: true,
				Child: statusAt(shedengine.StateDone, "Summary"), ChildFound: true, ChildLanding: true,
			},
			wantCalls: []string{"child-here"},
		},
		{
			name:      "OwnerAndPauseCoverageAreCarriedThrough",
			fakes:     runFakes{owner: "other", adoptSubject: "beam", elsewhere: true, pauseCovers: true},
			wantIn:    runfsm.Inputs{Owner: "other", AdoptSubject: "beam", OwnedElsewhere: true, PauseCovers: true},
			wantCalls: []string{"child-origin"},
		},
		{
			name:    "AReadErrorNamesTheReadAndTheSlug",
			fakes:   runFakes{boardErr: errors.New("boom")},
			wantErr: "read the board entry of slug: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fakes := tt.fakes

			got, gotIn, err := fakes.reader().read("slug")

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("read(slug) error = %v; want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("read(slug) error = %v; want none", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("read(slug) records = %+v; want %+v", got, tt.want)
			}
			if gotIn != tt.wantIn {
				t.Errorf("read(slug) inputs = %+v; want %+v", gotIn, tt.wantIn)
			}
			if !slices.Equal(fakes.calls, tt.wantCalls) {
				t.Errorf("read(slug) called %v; want %v", fakes.calls, tt.wantCalls)
			}
		})
	}
}

// TestReadRun_RowNamesMatchTheBattenRecipe pins the row names ReadRun holds locally against the batten recipe.
func TestReadRun_RowNamesMatchTheBattenRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	if runShedRow != battenrecipe.NameRunShed {
		t.Errorf("runShedRow = %q; want %q", runShedRow, battenrecipe.NameRunShed)
	}
	recipe, err := shedbuild.Parse(recipes.BattenRecipe)
	if err != nil {
		t.Fatalf("Parse(BattenRecipe) error = %v; want none", err)
	}
	var started []string
	before := battenrecipe.RowsBeforeRunShed()
	for _, row := range recipe.Producers {
		if !slices.Contains(before, row.Name) {
			started = append(started, row.Name)
		}
	}
	want := []string{runShedRow, landRow, worktreeTeardownRow}
	slices.Sort(started)
	slices.Sort(want)
	if !slices.Equal(started, want) {
		t.Errorf("the batten recipe's rows from Run-Shed on = %v; want ReadRun's started rows %v", started, want)
	}
}
