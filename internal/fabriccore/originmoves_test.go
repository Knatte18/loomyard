// originmoves_test.go tests the rules of moving judged branches: the order, the verdicts and the records branch's state.

package fabriccore_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestMoveVerdict(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("MoveOrderIsBoltThenThePrimeThenEachPairRecordsBeforeCode", func(t *testing.T) {
		t.Parallel()
		targets := []fabriccore.MoveTarget{
			{Repo: fabriclayout.RepoCode, Prime: true},
			{Repo: fabriclayout.RepoRecords, Prime: true},
			{Repo: fabriclayout.RepoCode, Slug: "ask"},
			{Repo: fabriclayout.RepoRecords, Slug: "ask"},
			{Repo: fabriclayout.RepoCode, Slug: "neve"},
			{Repo: fabriclayout.RepoRecords, Slug: "neve"},
			{Repo: fabriclayout.RepoBolt},
		}

		bolt, prime, pair := fabriccore.MoveOrder(targets, []string{"neve", "ask"})

		if !slices.Equal(bolt, []int{6}) || !slices.Equal(prime, []int{1, 0}) || !slices.Equal(pair, []int{5, 4, 3, 2}) {
			t.Errorf("MoveOrder() = %v, %v, %v; want [6], [1 0], [5 4 3 2]", bolt, prime, pair)
		}
	})

	t.Run("TheVerdictTakesTheFirstReasonThatHolds", func(t *testing.T) {
		t.Parallel()
		behind := fabriclayout.BranchFastForward
		replay := fabriclayout.BranchNeedsReplay
		records := fabriclayout.RepoRecords
		tests := []struct {
			name  string
			facts fabriccore.MoveFacts
			want  fabriccore.MoveStep
		}{
			{"DirtyBranchNeedsNoMove", fabriccore.MoveFacts{State: fabriclayout.BranchDirty, Repo: records, BusyLock: "prime"}, fabriccore.MoveNone},
			{"ParentOfARunScope", fabriccore.MoveFacts{State: behind, Run: "task", Repo: fabriclayout.RepoCode, BusyLock: "prime"}, fabriccore.MoveNone},
			{"BoltOfARunScope", fabriccore.MoveFacts{State: behind, Run: "task", Repo: fabriclayout.RepoBolt}, fabriccore.MoveRecords},
			{"OwnedByTheEndedStep", fabriccore.MoveFacts{State: behind, Repo: records, Slug: "task", OwnedByEnded: true, BusyLock: "prime"}, fabriccore.MoveNone},
			{"ReplayOfAnEntryOwnedElsewhere", fabriccore.MoveFacts{State: replay, Repo: records, Slug: "task", OwnedElsewhere: true}, fabriccore.MoveNone},
			{"FastForwardOfAnEntryOwnedElsewhere", fabriccore.MoveFacts{State: behind, Repo: fabriclayout.RepoCode, Slug: "task", OwnedElsewhere: true, LiveProbed: true}, fabriccore.MoveFastForward},
			{"BusyBeforeTheProbe", fabriccore.MoveFacts{State: behind, Repo: records, Slug: "task", BusyLock: "prime"}, fabriccore.MoveBusy},
			{"OtherPairIsProbed", fabriccore.MoveFacts{State: behind, Repo: records, Slug: "task"}, fabriccore.MoveProbeLive},
			{"LiveBeforeTheAnchor", fabriccore.MoveFacts{State: behind, Repo: records, Slug: "task", LiveProbed: true, Live: true, AnchorStopped: true}, fabriccore.MoveLive},
			{"TheScopesOwnPairIsNotProbed", fabriccore.MoveFacts{State: behind, Run: "task", Repo: fabriclayout.RepoCode, Slug: "task"}, fabriccore.MoveFastForward},
			{"MoveRecordStands", fabriccore.MoveFacts{State: behind, Repo: records, MoveStanding: true}, fabriccore.MoveAnchorStopped},
			{"AnchorStops", fabriccore.MoveFacts{State: replay, Repo: records, AnchorStopped: true}, fabriccore.MoveAnchorStopped},
			{"CodeBranch", fabriccore.MoveFacts{State: behind, Repo: fabriclayout.RepoCode}, fabriccore.MoveFastForward},
			{"RecordsBranch", fabriccore.MoveFacts{State: replay, Repo: records}, fabriccore.MoveRecords},
		}
		for _, tt := range tests {
			if got := fabriccore.MoveVerdict(tt.facts); got != tt.want {
				t.Errorf("%s: MoveVerdict() = %q; want %q", tt.name, got, tt.want)
			}
		}
	})

	t.Run("ARecordsBranchIsJudgedByItsLocksAndThenItsReplay", func(t *testing.T) {
		t.Parallel()
		paths := []string{"notes/c.md"}
		tests := []struct {
			name  string
			facts fabriccore.RecordsMoveFacts
			want  fabriclayout.BranchOutcome
		}{
			{"HeldLock", fabriccore.RecordsMoveFacts{BusyLock: "weft push", PushLockHeld: true}, fabriclayout.BranchOutcome{State: fabriclayout.BranchBusy, BusyLock: "weft push", PushLockHeld: true}},
			{"AlreadyLevel", fabriccore.RecordsMoveFacts{Level: true}, fabriclayout.BranchOutcome{State: fabriclayout.BranchLevel}},
			{"Behind", fabriccore.RecordsMoveFacts{Behind: true}, fabriclayout.BranchOutcome{State: fabriclayout.BranchFastForward}},
			{"Ahead", fabriccore.RecordsMoveFacts{Ahead: true}, fabriclayout.BranchOutcome{State: fabriclayout.BranchAhead}},
			{"BothSidesMoved", fabriccore.RecordsMoveFacts{}, fabriclayout.BranchOutcome{State: fabriclayout.BranchNeedsReplay}},
			{"ReplayedToOriginsHead", fabriccore.RecordsMoveFacts{Replayed: true, Level: true}, fabriclayout.BranchOutcome{State: fabriclayout.BranchLevel, Moved: true}},
			{"ReplayedOntoOriginsHead", fabriccore.RecordsMoveFacts{Replayed: true}, fabriclayout.BranchOutcome{State: fabriclayout.BranchAhead, Moved: true}},
			{"ReplayBlockedByAChange", fabriccore.RecordsMoveFacts{ReplayDirty: true, Paths: paths}, fabriclayout.BranchOutcome{State: fabriclayout.BranchDirty, Paths: paths}},
			{"ReplayConflict", fabriccore.RecordsMoveFacts{ReplayConflict: true, Paths: paths, Commits: []string{"abc"}}, fabriclayout.BranchOutcome{State: fabriclayout.BranchDiverged, Paths: paths, Commits: []string{"abc"}}},
		}
		for _, tt := range tests {
			if got := fabriccore.RecordsMoveState(tt.facts); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: RecordsMoveState() = %+v; want %+v", tt.name, got, tt.want)
			}
		}
	})
}
