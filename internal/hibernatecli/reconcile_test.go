// reconcile_test.go pins the mapping of an origin reconcile report onto the hibernate engine's own view of it.

package hibernatecli

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
)

func TestWakeReconcileFromReport(t *testing.T) {
	t.Parallel()
	recorder := fabricengine.NewMutations("")
	recorder.AppendRef(fabricengine.KindTagCreated, "set-aside/p1/1", "snapshot of uncommitted paths")
	recorder.AppendRef(fabricengine.KindBranchDeleted, "p1", "")
	withMutations := fabricengine.OriginReport{MutationRecord: fabricengine.MutationRecord{Mutations: recorder.Snapshot()}}

	tests := []struct {
		name   string
		report fabricengine.OriginReport
		want   hibernateengine.WakeReconcile
	}{
		{
			"A stopped branch's way forward reaches its entry's run exactly",
			fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "p1", Repo: "records", Slug: "p1", State: fabricengine.BranchDiverged, WayForward: "resolve a.txt, then `lyx fabric push`"}},
				Entries:  []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryStopped, WayForward: "resolve a.txt, then `lyx fabric push`"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "resolve a.txt, then `lyx fabric push`"}}},
		},
		{
			"A busy records branch's push lock becomes the run's parked-push flag",
			fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "p1", Repo: "records", Slug: "p1", State: fabricengine.BranchBusy, PushLockHeld: true, WayForward: "`lyx resume p1`"}},
				Entries:  []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryStopped, WayForward: "`lyx resume p1`"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "`lyx resume p1`", Busy: true, ParkedPush: true}}},
		},
		{
			"A busy branch holding another lock is busy without a parked push",
			fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "p1", Repo: "records", Slug: "p1", State: fabricengine.BranchBusy, BusyLock: "records write", WayForward: "wait for the holder of the records write lock to end"}},
				Entries:  []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryStopped, WayForward: "wait for the holder of the records write lock to end"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "wait for the holder of the records write lock to end", Busy: true}}},
		},
		{
			"A branch stopped as gone sets the run's gone flag",
			fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "p1", Repo: "code", Slug: "p1", State: fabricengine.BranchGone, WayForward: "origin no longer has p1"}},
				Entries:  []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryStopped, WayForward: "origin no longer has p1"}},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "origin no longer has p1", Gone: true}}},
		},
		{
			"A stop on Bolt carries Bolt's way forward as the stop",
			fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "main", Repo: "bolt", State: fabricengine.BranchDiverged, WayForward: "resolve board.json in the board worktree"}},
				Entries:  []fabricengine.EntryOutcome{{Slug: "p1", State: fabricengine.EntryStopped, WayForward: "resolve board.json in the board worktree"}},
			},
			hibernateengine.WakeReconcile{Stop: "resolve board.json in the board worktree", Runs: []hibernateengine.WakeRun{{Slug: "p1", WayForward: "resolve board.json in the board worktree"}}},
		},
		{
			"A busy Bolt is no stop",
			fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "main", Repo: "bolt", State: fabricengine.BranchBusy, BusyLock: "board push", WayForward: "wait"}},
			},
			hibernateengine.WakeReconcile{},
		},
		{
			"An ended entry with a local pair is a run, and one with none is left out",
			fabricengine.OriginReport{
				Entries: []fabricengine.EntryOutcome{
					{Slug: "p1", State: fabricengine.EntryEnded, Local: true, Busy: true, WayForward: "wait for the driver"},
					{Slug: "p2", State: fabricengine.EntryEnded, Local: true},
					{Slug: "p3", State: fabricengine.EntryEnded},
					{Slug: "p4", State: fabricengine.EntryActive},
				},
			},
			hibernateengine.WakeReconcile{Runs: []hibernateengine.WakeRun{
				{Slug: "p1", Ended: true, Busy: true, WayForward: "wait for the driver"},
				{Slug: "p2", Ended: true},
			}},
		},
		{
			"An unreachable report is carried over",
			fabricengine.OriginReport{Unreachable: true},
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
