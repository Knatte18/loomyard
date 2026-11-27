// reach_test.go covers the caveats the reach report step writes for a function no mapped test reaches.

package gatecli

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

func TestUnseenTestCaveats(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	unresolved := testcover.SpawnVerdict{Unresolved: true}
	tests := []struct {
		name   string
		unseen []testreach.UnseenTest
		want   []string
	}{
		{name: "NoUnseenTest"},
		{
			name: "OneCaveatPerCauseSortedByCause",
			unseen: []testreach.UnseenTest{
				{Test: "TestA", Spawn: testcover.SpawnVerdict{OutOfProcess: true, Unresolved: true}},
				{Test: "TestB", Spawn: unresolved},
				{Test: "TestC", Spawn: unresolved},
				{Test: "TestD", Spawn: unresolved, Outcome: testcover.OutcomeFailed},
				{Test: "TestE", Outcome: testcover.OutcomeSkipped},
			},
			want: []string{
				"TestD of p may run it unseen: the map run recorded their runs alone as failed, with no coverage",
				"TestE of p may run it unseen: the map run recorded their runs alone as skipped, with no coverage",
				"TestB, TestC of p may run it unseen: they call what the spawn scan cannot resolve, which may run code in another process",
				"TestA of p may run it unseen: they run code in another process, so the map run never ran them",
			},
		},
		{
			name:   "AnUnresolvedTestThatCoveredNothingCountsAsUnresolved",
			unseen: []testreach.UnseenTest{{Test: "TestA", Spawn: unresolved, Outcome: testcover.OutcomeNoCoverage}},
			want: []string{
				"TestA of p may run it unseen: they call what the spawn scan cannot resolve, which may run code in another process",
			},
		},
		{
			name: "TestsUpToTheCapAreAllNamed",
			unseen: []testreach.UnseenTest{
				{Test: "T1", Spawn: unresolved}, {Test: "T2", Spawn: unresolved}, {Test: "T3", Spawn: unresolved},
				{Test: "T4", Spawn: unresolved}, {Test: "T5", Spawn: unresolved},
			},
			want: []string{
				"T1, T2, T3, T4, T5 of p may run it unseen: they call what the spawn scan cannot resolve, which may run code in another process",
			},
		},
		{
			name: "TestsPastTheCapAreCounted",
			unseen: []testreach.UnseenTest{
				{Test: "T1", Spawn: unresolved}, {Test: "T2", Spawn: unresolved}, {Test: "T3", Spawn: unresolved},
				{Test: "T4", Spawn: unresolved}, {Test: "T5", Spawn: unresolved}, {Test: "T6", Spawn: unresolved},
				{Test: "T7", Spawn: unresolved},
			},
			want: []string{
				"T1, T2, T3, T4, T5 and 2 more of p may run it unseen: they call what the spawn scan cannot resolve, which may run code in another process",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, unseenTestCaveats("p", tt.unseen)); diff != "" {
				t.Errorf("unseenTestCaveats() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnmappedTierNames(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []testcover.TestFunc{
		{Name: "TestPlain"},
		{Name: "TestSlow", Tier: 2, Tiered: true},
		{Name: "TestUndeclared", Tier: 1, Tiered: true},
		{Name: "TestMapped", Tier: 0, Tiered: true},
	}
	want := []string{"tier1", "tier2", "no tier"}
	if diff := cmp.Diff(want, unmappedTierNames(tests, map[int]gateslot.Tier{0: {}})); diff != "" {
		t.Errorf("unmappedTierNames() mismatch (-want +got):\n%s", diff)
	}
}
