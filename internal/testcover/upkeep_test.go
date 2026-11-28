package testcover_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPlanUpkeep(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	at := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	files := map[string]string{"b.go": "b1", "b_test.go": "t1"}
	planned := testcover.Map{
		Commit: "old",
		Files:  files,
		Blocks: []testcover.BlockKey{{File: "b.go", Func: "Own", Hash: "own"}},
		Tests: []testcover.MapTest{
			{Name: "TestFailed", Outcome: testcover.OutcomeFailed},
			{Name: "TestHit", Blocks: []int{0}},
			{Name: "TestKeep", Blocks: []int{0}},
			{Name: "TestSpawned", Spawn: testcover.SpawnVerdict{OutOfProcess: true}},
		},
	}
	declared := []testcover.TestFunc{{Name: "TestFailed"}, {Name: "TestHit"}, {Name: "TestKeep"}, {Name: "TestSpawned"}}
	noScan := func() (map[string]testcover.SpawnVerdict, error) {
		return nil, errors.New("the spawn scan ran with no declared test lacking an entry")
	}
	tests := []struct {
		name string
		// edit changes the row's input from a Current selection of planned at the commit "head", its files unchanged.
		edit        func(in *testcover.UpkeepInput)
		wantVerdict testcover.UpkeepVerdict
		wantChanged bool
		// wantTests names the planned map's entries, and wantQueue the queued tests.
		wantTests []string
		wantQueue []string
		// check, when set, asserts more of the plan.
		check func(t *testing.T, plan testcover.UpkeepPlan)
		// scans lets the row's default spawn scan run, as a dropped test lacks an entry; otherwise it fails the row.
		scans   bool
		wantErr bool
	}{
		{
			name: "CurrentMovesTheMapToTheCommitWithEveryEntry",
			edit: func(*testcover.UpkeepInput) {}, wantVerdict: testcover.UpkeepCurrent, wantChanged: true,
			wantTests: []string{"TestFailed", "TestHit", "TestKeep", "TestSpawned"},
			check: func(t *testing.T, plan testcover.UpkeepPlan) {
				if plan.Map.Commit != "head" || !reflect.DeepEqual(plan.Map.Blocks, planned.Blocks) {
					t.Errorf("plan map = commit %s, blocks %v; want commit head and the planned blocks", plan.Map.Commit, plan.Map.Blocks)
				}
			},
		},
		{
			name: "CurrentAtTheMapsOwnCommitChangesNothing",
			edit: func(in *testcover.UpkeepInput) { in.Map.Commit = "head" }, wantVerdict: testcover.UpkeepCurrent,
			wantTests: []string{"TestFailed", "TestHit", "TestKeep", "TestSpawned"},
		},
		{
			name: "AffectedDropsAndQueuesTheAffectedTests",
			edit: func(in *testcover.UpkeepInput) { in.Selection.Tests = []string{"TestHit"} }, wantVerdict: testcover.UpkeepAffected, wantChanged: true,
			wantTests: []string{"TestFailed", "TestKeep", "TestSpawned"}, wantQueue: []string{"TestHit"}, scans: true,
		},
		{
			name: "AnOwnFileChangeDropsAndQueuesTheOutcomeEntries",
			edit: func(in *testcover.UpkeepInput) {
				in.Tree.Files = map[string]string{"b.go": "b2", "b_test.go": "t1"}
			},
			wantVerdict: testcover.UpkeepCurrent, wantChanged: true,
			wantTests: []string{"TestHit", "TestKeep", "TestSpawned"}, wantQueue: []string{"TestFailed"}, scans: true,
		},
		{
			name: "ARemovedTestDropsItsEntry",
			edit: func(in *testcover.UpkeepInput) {
				in.Declared = []testcover.TestFunc{{Name: "TestFailed"}, {Name: "TestHit"}, {Name: "TestSpawned"}}
			},
			wantVerdict: testcover.UpkeepCurrent, wantChanged: true,
			wantTests: []string{"TestFailed", "TestHit", "TestSpawned"},
		},
		{
			name: "AFailureAtAnEarlierCommitIsCleared",
			edit: func(in *testcover.UpkeepInput) {
				in.Map.Commit = "head"
				in.Map.Failure = &testcover.MapFailure{Commit: "older", Error: "the build failed"}
			},
			wantVerdict: testcover.UpkeepCurrent, wantChanged: true,
			wantTests: []string{"TestFailed", "TestHit", "TestKeep", "TestSpawned"},
			check: func(t *testing.T, plan testcover.UpkeepPlan) {
				if plan.Map.Failure != nil {
					t.Errorf("plan map failure = %+v; want it cleared", plan.Map.Failure)
				}
			},
		},
		{
			name: "ALinkedChangeKeepsTheOutcomeEntries",
			edit: func(in *testcover.UpkeepInput) {
				in.Tree.Linked = map[string]map[string]string{"m/a": {"a.go": "a2"}}
			},
			wantVerdict: testcover.UpkeepCurrent, wantChanged: true,
			wantTests: []string{"TestFailed", "TestHit", "TestKeep", "TestSpawned"},
		},
		{
			name: "ADeclaredTestWithNoEntryIsQueuedUnlessTheSpawnScanMarksIt",
			edit: func(in *testcover.UpkeepInput) {
				in.Declared = append(in.Declared, testcover.TestFunc{Name: "TestNew"}, testcover.TestFunc{Name: "TestNewSpawns", Tier: 1, Hash: "fn"})
				in.Spawns = func() (map[string]testcover.SpawnVerdict, error) {
					return map[string]testcover.SpawnVerdict{"TestNewSpawns": {Unresolved: true}}, nil
				}
			},
			wantVerdict: testcover.UpkeepCurrent, wantChanged: true,
			wantTests: []string{"TestFailed", "TestHit", "TestKeep", "TestNewSpawns", "TestSpawned"}, wantQueue: []string{"TestNew"},
			check: func(t *testing.T, plan testcover.UpkeepPlan) {
				want := testcover.MapTest{Name: "TestNewSpawns", Tier: 1, FuncHash: "fn", Spawn: testcover.SpawnVerdict{Unresolved: true}, Commit: "head", MeasuredAt: at}
				if got := plan.Map.Tests[3]; !reflect.DeepEqual(got, want) {
					t.Errorf("spawn-marked entry = %+v; want %+v", got, want)
				}
			},
		},
		{
			name: "ASpawnScanFailureIsTheError",
			edit: func(in *testcover.UpkeepInput) {
				in.Declared = append(in.Declared, testcover.TestFunc{Name: "TestNew"})
				in.Spawns = func() (map[string]testcover.SpawnVerdict, error) { return nil, errors.New("spawn scan failed") }
			},
			wantErr: true,
		},
		{
			name: "AMissLeavesTheMapAndQueuesNoTest",
			edit: func(in *testcover.UpkeepInput) {
				in.Selection = testcover.Selection{Miss: &testcover.Miss{Reason: testcover.MissNonBody, Detail: "a.go"}}
				in.Declared = append(in.Declared, testcover.TestFunc{Name: "TestNew"})
			},
			wantVerdict: testcover.UpkeepMiss,
			check: func(t *testing.T, plan testcover.UpkeepPlan) {
				if plan.Miss == nil || plan.Miss.Reason != testcover.MissNonBody {
					t.Errorf("plan miss = %v; want the selection's miss", plan.Miss)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := testcover.UpkeepInput{
				Map: planned, Commit: "head", At: at,
				Tree:     testcover.UpkeepTree{Files: files},
				Declared: append([]testcover.TestFunc(nil), declared...), Spawns: noScan,
			}
			if tt.scans {
				in.Spawns = func() (map[string]testcover.SpawnVerdict, error) { return nil, nil }
			}
			tt.edit(&in)
			plan, err := testcover.PlanUpkeep(in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PlanUpkeep() error = %v; want error %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			var names []string
			for _, test := range plan.Map.Tests {
				names = append(names, test.Name)
			}
			if plan.Verdict != tt.wantVerdict || plan.Changed != tt.wantChanged || !reflect.DeepEqual(names, tt.wantTests) || !reflect.DeepEqual(plan.Queue, tt.wantQueue) {
				t.Errorf("PlanUpkeep() = verdict %s, changed %v, entries %v, queue %v; want %s, %v, %v, %v",
					plan.Verdict, plan.Changed, names, plan.Queue, tt.wantVerdict, tt.wantChanged, tt.wantTests, tt.wantQueue)
			}
			if tt.check != nil {
				tt.check(t, plan)
			}
		})
	}
}
