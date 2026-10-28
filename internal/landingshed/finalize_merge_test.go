//go:build tier0

// finalize_merge_test.go drives Finalize's merge of the parent branch into the task and the judgement of that merge, over a fake resolver and the gate fixture.

package landingshed

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestFinalize_MergeJudgement pins Finalize's merge of the parent branch into the task and the judgement of that merge:
// every merge runs under the parent lock and proceeds to the gate, a large merge included, what the parent side is measured from, and what the merge record carries afterwards.
func TestFinalize_MergeJudgement(t *testing.T) {
	const earlierTip, newTip, taskBase, aligned = "p1", "p2", "b0", "a1"
	tests := []struct {
		name string
		// upToDate is whether the merge finds the task already holding the parent's tip, which moves no head.
		upToDate bool
		// record is the merge record at entry: "keep" holds earlierTip, "unreadable" does not decode and "absent" is missing.
		record string
		// paths answers ChangedPaths by "from..to".
		paths map[string][]string
		// wantChanged are the ChangedPaths calls, in order.
		wantChanged []string
		// wantMergeCommit and wantOverlap are what the rewritten merge record carries.
		wantMergeCommit string
		wantOverlap     []string
	}{
		{
			name: "an up-to-date merge proceeds to the gate", upToDate: true, record: "keep",
		},
		{
			name: "an unreadable merge record measures the parent side from the task's merge base and lands", record: "unreadable",
			paths:       map[string][]string{"b0..p2": {"x.go"}, "b0..t1": {"x.go"}},
			wantChanged: []string{"b0..p2", "b0..t1"}, wantMergeCommit: "t2", wantOverlap: []string{"x.go"},
		},
		{
			name: "an overlapping parent change lands with the merge commit kept", record: "keep",
			paths:       map[string][]string{"p1..p2": {"a.go", "b.go"}, "b0..t1": {"b.go", "c.go"}},
			wantChanged: []string{"p1..p2", "b0..t1"}, wantMergeCommit: "t2", wantOverlap: []string{"b.go"},
		},
		{
			name: "a non-overlapping parent change proceeds to the gate", record: "keep",
			paths:       map[string][]string{"p1..p2": {"a.go"}, "b0..t1": {"c.go"}},
			wantChanged: []string{"p1..p2", "b0..t1"}, wantMergeCommit: "t2",
		},
		{
			name: "a run with no merge record measures the parent side from the task's merge base", record: "absent",
			wantChanged: []string{"b0..p2", "b0..t1"}, wantMergeCommit: "t2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			head := "t1"
			deps.TaskHead = func() (string, error) { return head, nil }
			deps.MergeBase = func(string) (string, error) { return taskBase, nil }
			deps.ReadParentTips = func(bool) (fabricengine.ParentTips, error) {
				return fabricengine.ParentTips{Local: newTip, Upstream: newTip, Relation: fabricengine.ParentEqual}, nil
			}
			var changed []string
			deps.ChangedPaths = func(from, to string) ([]string, error) {
				changed = append(changed, from+".."+to)
				return tt.paths[from+".."+to], nil
			}
			switch tt.record {
			case "keep":
				if err := WriteMergeRecord(deps.MergeRecordPath, MergeRecord{ParentBranch: "main", MergedTip: earlierTip, AlignedTip: aligned}); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if err := os.WriteFile(deps.MergeRecordPath, []byte("{{{"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "absent":
				if err := os.Remove(deps.MergeRecordPath); err != nil {
					t.Fatal(err)
				}
			}
			gate := newGateFixture(t, "finalize", "go test ./...", nil)
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
			resolver := &eventResolver{
				events: new([]string),
				result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved, AlreadyUpToDate: tt.upToDate},
				lockHeld: func() bool {
					rec, found, _ := deps.Lock.Store.Read("main")
					return found && rec.Owner.Slug == "me"
				},
				onResolve: func() {
					if !tt.upToDate {
						head = "t2"
					}
				},
			}
			fz := &Finalize{deps: deps, resolver: resolver, gate: gate.gate, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, ptr, err := fz.Call(context.Background())

			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if outcome != shedengine.Done || gate.fake.verifyCalls != 1 {
				t.Fatalf("Call() = %q, %+v with %d verify call(s); want Done after one", outcome, ptr, gate.fake.verifyCalls)
			}
			if !resolver.heldAtMerge {
				t.Error("the merge-in ran without the parent lock held; want it, and any conflict session it starts, under the lock")
			}
			if _, statErr := os.Stat(filepath.Join(deps.ScratchDir, "overlap-note.md")); statErr == nil {
				t.Error("overlap-note.md exists; want none written")
			}
			if got, want := strings.Join(changed, ","), strings.Join(tt.wantChanged, ","); got != want {
				t.Errorf("ChangedPaths calls = %q; want %q", got, want)
			}

			rec, found, _ := ReadMergeRecord(deps.MergeRecordPath)
			if !found || rec.MergedTip != newTip || rec.TaskBase != taskBase || rec.TaskHead != "t1" || rec.MergeCommit != tt.wantMergeCommit || rec.UpToDate != tt.upToDate {
				t.Errorf("merge record = %+v, %v; want merged tip %s, task base %s, task head t1, merge commit %q, up to date %v", rec, found, newTip, taskBase, tt.wantMergeCommit, tt.upToDate)
			}
			if !slices.Equal(rec.Overlap.Files, tt.wantOverlap) {
				t.Errorf("merge record overlap files = %v; want %v", rec.Overlap.Files, tt.wantOverlap)
			}
			if tt.record == "keep" && rec.AlignedTip != aligned {
				t.Errorf("merge record aligned tip = %q; want it kept at %q", rec.AlignedTip, aligned)
			}
		})
	}
}
