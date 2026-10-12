// parentlock_test.go covers the seams of Run-Shed's parent lock backstop: the record lookup by run and the moved-parent read.

package battencli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
)

// TestChildParentLock asserts Read finds the lock record whose owner is the child's run by slug and pair and Release frees it,
// and that ParentMoved reads a moved parent from the merge record and the upstream tip: an absent record, a parent with no upstream, an upstream still at the merged tip and an upstream holding the task's work all read as not moved.
func TestChildParentLock(t *testing.T) {
	t.Parallel()

	t.Run("ReadFindsTheRecordOfTheChildsRunAndReleaseFreesIt", func(t *testing.T) {
		t.Parallel()
		boardDir := t.TempDir()
		taskLocation := &lyxcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "a-slug", AnchorRel: "."}
		store := &parentlock.Store{Dir: parentlock.Dir(boardDir)}
		other := parentlock.Owner{Slug: "other-slug", Pair: "/hub/other-slug", RunID: "run-2", PID: 7, StartTime: "other-start"}
		sameSlugOtherPair := parentlock.Owner{Slug: "a-slug", Pair: "/elsewhere/a-slug", RunID: "run-3", PID: 8, StartTime: "elsewhere-start"}
		child := parentlock.Owner{Slug: "a-slug", Pair: taskLocation.WorktreePath(), RunID: "run-1", PID: 9, StartTime: "child-start"}
		for branch, owner := range map[string]parentlock.Owner{"main": other, "develop": sameSlugOtherPair, "release": child} {
			if attempt, err := store.TryAcquire(branch, owner); err != nil || !attempt.Granted {
				t.Fatalf("TryAcquire(%s) = %+v, %v; want granted", branch, attempt, err)
			}
		}

		lock := childParentLock("a-slug", func() (*lyxcwd.Location, error) { return taskLocation, nil }, boardDir, 0)
		record, found, err := lock.Read()
		if err != nil || !found || record.Branch != "release" || record.Owner != child {
			t.Fatalf("Read() = %+v, %v, %v; want the child's record on release", record, found, err)
		}
		if released, err := lock.Release(record); err != nil || !released {
			t.Fatalf("Release() = %v, %v; want true, nil", released, err)
		}
		if _, found, err := lock.Read(); err != nil || found {
			t.Errorf("Read() after the release = found %v, error %v; want none", found, err)
		}
	})

	t.Run("ParentMovedReadsTheUpstreamTipAgainstTheMergeRecord", func(t *testing.T) {
		t.Parallel()
		const (
			mergedTip = "merged"
			taskHead  = "head"
		)
		tests := []struct {
			name      string
			record    bool
			tips      fabricengine.ParentTips
			tipsErr   error
			holds     bool
			wantMoved bool
			wantErr   bool
		}{
			{name: "no merge record is not moved"},
			{name: "no upstream is not moved", record: true, tips: fabricengine.ParentTips{Local: "local", Relation: fabricengine.ParentNoUpstream}},
			{name: "upstream at the merged tip is not moved", record: true, tips: fabricengine.ParentTips{Upstream: mergedTip, Relation: fabricengine.ParentEqual}},
			{name: "upstream holding the task's work is not moved", record: true, tips: fabricengine.ParentTips{Upstream: "squashed", Relation: fabricengine.ParentBehind}, holds: true},
			{name: "upstream past the merged tip is moved", record: true, tips: fabricengine.ParentTips{Upstream: "newer", Relation: fabricengine.ParentBehind}, wantMoved: true},
			{name: "unreadable tips are an error", record: true, tipsErr: errors.New("fetch failed"), wantErr: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				recordPath := landingshed.MergeRecordPath(t.TempDir())
				if tt.record {
					if err := landingshed.WriteMergeRecord(recordPath, landingshed.MergeRecord{ParentBranch: "main", MergedTip: mergedTip}); err != nil {
						t.Fatal(err)
					}
				}
				tips := func(branch string, fetch bool) (fabricengine.ParentTips, error) {
					if branch != "main" || !fetch {
						t.Errorf("tips(%q, %v); want the record's parent branch, fetched", branch, fetch)
					}
					return tt.tips, tt.tipsErr
				}
				head := func() (string, error) { return taskHead, nil }
				holdsWork := func(base, source string) (bool, error) {
					if base != tt.tips.Upstream || source != taskHead {
						t.Errorf("holdsWork(%q, %q); want the upstream tip and the task head", base, source)
					}
					return tt.holds, nil
				}

				moved, err := parentMovedSinceMergeIn(recordPath, tips, head, holdsWork)

				if moved != tt.wantMoved || (err != nil) != tt.wantErr {
					t.Errorf("parentMovedSinceMergeIn() = %v, %v; want %v, error %v", moved, err, tt.wantMoved, tt.wantErr)
				}
			})
		}
	})
}
