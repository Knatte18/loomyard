package testcover_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func blockSet(names ...string) testcover.BlockSet {
	set := testcover.BlockSet{}
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

func passRun(name string, elapsed float64, blocks ...string) testcover.Run {
	return testcover.Run{Name: name, Elapsed: elapsed, Action: testcover.ActionPass, Blocks: blockSet(blocks...)}
}

func TestJudge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	skipped := passRun("Skipped", 0, "b1")
	skipped.Action = testcover.ActionSkip
	failed := passRun("Failed", 1, "b1")
	failed.Action = testcover.ActionFail
	spawning := passRun("OutOfProcess", 1, "b1")
	spawning.Spawn = testcover.SpawnVerdict{OutOfProcess: true}
	unresolved := passRun("Unresolved", 1, "b1")
	unresolved.Spawn = testcover.SpawnVerdict{Unresolved: true}

	tests := []struct {
		name string
		runs []testcover.Run
		want []testcover.Verdict
	}{
		{
			name: "subset of another test is removable, the superset is not contained",
			runs: []testcover.Run{passRun("A", 1, "b1", "b2"), passRun("B", 1, "b1", "b2", "b3")},
			want: []testcover.Verdict{
				{Name: "A", Contained: true, Removable: true, Covering: []string{"B"}},
				{Name: "B"},
			},
		},
		{
			name: "mutual pair: the slowest is removed and the other kept",
			runs: []testcover.Run{passRun("C", 2, "b1"), passRun("D", 1, "b1"), passRun("E", 1, "b9")},
			want: []testcover.Verdict{
				{Name: "C", Contained: true, Removable: true, Covering: []string{"D"}},
				{Name: "D", Contained: true, Covering: []string{"C"}},
				{Name: "E"},
			},
		},
		{
			name: "the removable set keeps the superset, whose covering tests come by overlap and skip one that adds nothing",
			runs: []testcover.Run{
				passRun("P", 1, "b1", "b2", "b3"), passRun("Q", 1, "b4"),
				passRun("R", 1, "b1"), passRun("T", 1, "b1", "b2", "b3", "b4"),
			},
			want: []testcover.Verdict{
				{Name: "P", Contained: true, Removable: true, Covering: []string{"T"}},
				{Name: "Q", Contained: true, Removable: true, Covering: []string{"T"}},
				{Name: "R", Contained: true, Removable: true, Covering: []string{"T"}},
				{Name: "T", Contained: true, Covering: []string{"P", "Q"}},
			},
		},
		{
			name: "no blocks, skipped, failed, out-of-process and unresolved tests are never judged and cover nothing",
			runs: []testcover.Run{
				passRun("Empty", 1), skipped, failed, spawning, unresolved, passRun("Base", 1, "b1", "b2"),
			},
			want: []testcover.Verdict{
				{Name: "Empty", Reason: testcover.ReasonNoBlocks},
				{Name: "Skipped", Reason: testcover.ReasonSkipped},
				{Name: "Failed", Reason: testcover.ReasonFailed},
				{Name: "OutOfProcess", Reason: testcover.ReasonOutOfProcess},
				{Name: "Unresolved", Reason: testcover.ReasonUnresolved},
				{Name: "Base"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := testcover.Judge(tt.runs); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Judge = %+v\nwant    %+v", got, tt.want)
			}
		})
	}
}
