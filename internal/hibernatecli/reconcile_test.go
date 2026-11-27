// reconcile_test.go pins the mapping of an origin reconcile report onto the hibernate engine's own view of it.

package hibernatecli

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWakeReconcileFromReport(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	recorder := fabricengine.NewMutations("")
	recorder.AppendRef(fabriclayout.KindTagCreated, "set-aside/p1/1", "snapshot of uncommitted paths")
	recorder.AppendRef(fabriclayout.KindBranchDeleted, "p1", "")
	withMutations := fabriclayout.OriginReport{MutationRecord: fabriclayout.MutationRecord{Mutations: recorder.Snapshot()}}

	tests := []struct {
		name   string
		report fabriclayout.OriginReport
		want   hibernateengine.WakeReconcile
	}{
		{
			"A stopped branch's way forward reaches its entry's run exactly",
			fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "p1", Repo: "records", Slug: "p1", State: fabriclayout.BranchDiverged, WayForward: "resolve a.txt, then `lym fabric push`"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "p1", State: fabriclayout.EntryStopped, WayForward: "resolve a.txt, then `lym fabric push`"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "resolve a.txt, then `lym fabric push`"}}},
		},
		{
			"A busy records branch's push lock becomes the run's parked-push flag",
			fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "p1", Repo: "records", Slug: "p1", State: fabriclayout.BranchBusy, PushLockHeld: true, WayForward: "`lym tender resume p1`"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "p1", State: fabriclayout.EntryStopped, WayForward: "`lym tender resume p1`"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "`lym tender resume p1`", Busy: true, ParkedPush: true}}},
		},
		{
			"A busy branch holding another lock is busy without a parked push",
			fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "p1", Repo: "records", Slug: "p1", State: fabriclayout.BranchBusy, BusyLock: "records write", WayForward: "wait for the holder of the records write lock to end"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "p1", State: fabriclayout.EntryStopped, WayForward: "wait for the holder of the records write lock to end"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "wait for the holder of the records write lock to end", Busy: true}}},
		},
		{
			"A branch stopped as gone sets the run's gone flag",
			fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "p1", Repo: "code", Slug: "p1", State: fabriclayout.BranchGone, WayForward: "origin no longer has p1"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "p1", State: fabriclayout.EntryStopped, WayForward: "origin no longer has p1"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "origin no longer has p1", Gone: true}}},
		},
		{
			"A stop on Bolt carries Bolt's way forward as the stop",
			fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "main", Repo: "bolt", State: fabriclayout.BranchDiverged, WayForward: "resolve board.json in the board worktree"}},
				Entries:  []fabriclayout.EntryOutcome{{Slug: "p1", State: fabriclayout.EntryStopped, WayForward: "resolve board.json in the board worktree"}},
			},
			hibernateengine.WakeReconcile{Stop: "resolve board.json in the board worktree", Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "resolve board.json in the board worktree"}}},
		},
		{
			"A busy Bolt is no stop",
			fabriclayout.OriginReport{
				Branches: []fabriclayout.BranchOutcome{{Branch: "main", Repo: "bolt", State: fabriclayout.BranchBusy, BusyLock: "board push", WayForward: "wait"}},
			},
			hibernateengine.WakeReconcile{},
		},
		{
			"An ended entry with a local pair is a run, and one with none is left out",
			fabriclayout.OriginReport{
				Entries: []fabriclayout.EntryOutcome{
					{Slug: "p1", State: fabriclayout.EntryEnded, Local: true, Busy: true, WayForward: "wait for the driver"},
					{Slug: "p2", State: fabriclayout.EntryEnded, Local: true},
					{Slug: "p3", State: fabriclayout.EntryEnded},
					{Slug: "p4", State: fabriclayout.EntryActive},
				},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{
				{Slug: "p1", Ended: true, Busy: true, WayForward: "wait for the driver"},
				{Slug: "p2", Ended: true},
			}},
		},
		{
			"An owned_elsewhere entry is a run with its owner and adopt subject",
			fabriclayout.OriginReport{
				Entries: []fabriclayout.EntryOutcome{{Slug: "p1", State: fabriclayout.EntryOwnedElsewhere, Local: true, Owner: "laptop", AdoptSubject: "beam", WayForward: "`lym adopt beam` takes it over"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "`lym adopt beam` takes it over", Owner: "laptop", AdoptSubject: "beam"}}},
		},
		{
			"An unreachable report is carried over",
			fabriclayout.OriginReport{Unreachable: true},
			hibernateengine.WakeReconcile{Unreachable: true},
		},
		{
			"A record of a removal and its snapshot tag yields one line per entry",
			withMutations,
			hibernateengine.WakeReconcile{Mutations: []string{"tag_created set-aside/p1/1 (snapshot of uncommitted paths)", "branch_deleted p1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := wakeReconcileFromReport(tt.report)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("wakeReconcileFromReport() mismatch (-want +got):\n%s", cmp.Diff(tt.want, got))
			}
		})
	}
}
