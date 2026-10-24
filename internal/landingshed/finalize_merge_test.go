//go:build tier0

// finalize_merge_test.go drives Finalize's merge of the parent branch into the task and the judgement of that merge, over a fake task merger, a fake resolver and the gate fixture.

package landingshed

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fakeTaskMerger is the test's taskMerger: its MergeIn answers a scripted result, conflicts included, and it records whether MergeAbort ran.
type fakeTaskMerger struct {
	result  fabricengine.MergeResult
	err     error
	merges  []string
	aborted bool
	// onMerge, when set, runs at each MergeIn, standing in for the merge commit it makes.
	onMerge func()
}

func (f *fakeTaskMerger) MergeIn(source string) (fabricengine.MergeResult, error) {
	f.merges = append(f.merges, source)
	if f.onMerge != nil {
		f.onMerge()
	}
	return f.result, f.err
}

func (f *fakeTaskMerger) MergeAbort() (fabricengine.MergeResult, error) {
	f.aborted = true
	return fabricengine.MergeResult{}, nil
}

// TestFinalize_MergeJudgement pins Finalize's merge of the parent branch into the task and the judgement of that merge:
// when the merge proceeds to the gate, when it reroutes with no gate run, how a conflict ends for a loom and a darn Finalize, what the parent side is measured from, and what the merge record carries afterwards.
func TestFinalize_MergeJudgement(t *testing.T) {
	const earlierTip, newTip, taskBase, aligned = "p1", "p2", "b0", "a1"
	committed := fabricengine.MergeResult{Committed: true}
	tests := []struct {
		name string
		// loom tells Deps.ConflictReroutes; a darn Finalize merges through its resolver, which starts a conflict session when session is set.
		loom    bool
		merge   fabricengine.MergeResult
		session bool
		// record is the merge record at entry: "keep" holds earlierTip, "unreadable" does not decode and "absent" is missing.
		record string
		// paths answers ChangedPaths by "from..to".
		paths map[string][]string
		// wantReroute is whether the call reroutes with no gate run, and wantInReason what its reason names.
		wantReroute  bool
		wantInReason string
		wantAborted  bool
		// wantChanged are the ChangedPaths calls, in order.
		wantChanged []string
		// wantRecord is whether the merge record is rewritten, and wantMergeCommit and wantOverlap what it then carries.
		wantRecord      bool
		wantMergeCommit string
		wantOverlap     []string
		wantUpToDate    bool
		// wantNote are the fragments the overlap note holds; nil means no note is written.
		wantNote []string
	}{
		{
			name: "an up-to-date merge proceeds to the gate", loom: true, merge: fabricengine.MergeResult{AlreadyUpToDate: true}, record: "keep",
			wantRecord: true, wantUpToDate: true,
		},
		{
			name: "a loom conflict aborts the merge and reroutes with no gate run", loom: true, merge: fabricengine.MergeResult{Conflicts: []string{"a.go"}}, record: "keep",
			wantReroute: true, wantInReason: "a.go", wantAborted: true,
		},
		{
			name: "an unreadable merge record measures the parent side from the task's merge base", loom: true, merge: committed, record: "unreadable",
			paths:       map[string][]string{"b0..p2": {"x.go"}, "b0..t1": {"x.go"}},
			wantReroute: true, wantInReason: "x.go", wantChanged: []string{"b0..p2", "b0..t1"}, wantRecord: true, wantMergeCommit: "t2", wantOverlap: []string{"x.go"},
		},
		{
			name: "a darn conflict runs the conflict session and reroutes as a large merge", merge: committed, session: true, record: "keep",
			wantReroute: true, wantInReason: "conflict session", wantChanged: []string{"p1..p2", "b0..t1"}, wantRecord: true, wantMergeCommit: "t2",
			wantNote: []string{"conflict session", "since p1"},
		},
		{
			name: "a darn overlap reroute writes the note from the merged tip the record names", merge: committed, record: "keep",
			paths:       map[string][]string{"p1..p2": {"a.go", "b.go"}, "b0..t1": {"b.go", "c.go"}},
			wantReroute: true, wantInReason: "b.go", wantChanged: []string{"p1..p2", "b0..t1"}, wantRecord: true, wantMergeCommit: "t2", wantOverlap: []string{"b.go"},
			wantNote: []string{"since its tip p1", "- b.go", "git diff p1 "},
		},
		{
			name: "a darn overlap reroute with no merge record writes the note from the task's merge base", merge: committed, record: "absent",
			paths:       map[string][]string{"b0..p2": {"x.go"}, "b0..t1": {"x.go"}},
			wantReroute: true, wantInReason: "x.go", wantChanged: []string{"b0..p2", "b0..t1"}, wantRecord: true, wantMergeCommit: "t2", wantOverlap: []string{"x.go"},
			wantNote: []string{"since its tip b0", "- x.go", "git diff b0 "},
		},
		{
			name: "an overlapping parent change reroutes with the merge commit kept", loom: true, merge: committed, record: "keep",
			paths:       map[string][]string{"p1..p2": {"a.go", "b.go"}, "b0..t1": {"b.go", "c.go"}},
			wantReroute: true, wantInReason: "b.go", wantChanged: []string{"p1..p2", "b0..t1"}, wantRecord: true, wantMergeCommit: "t2", wantOverlap: []string{"b.go"},
		},
		{
			name: "a non-overlapping parent change proceeds to the gate", loom: true, merge: committed, record: "keep",
			paths:       map[string][]string{"p1..p2": {"a.go"}, "b0..t1": {"c.go"}},
			wantChanged: []string{"p1..p2", "b0..t1"}, wantRecord: true, wantMergeCommit: "t2",
		},
		{
			name: "a darn run with no merge record measures the parent side from the task's merge base", merge: committed, record: "absent",
			wantChanged: []string{"b0..p2", "b0..t1"}, wantRecord: true, wantMergeCommit: "t2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.ConflictReroutes = tt.loom
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
			merger := &recordingParentMerger{results: []mergeCallResult{{result: committed}}}
			taskMerger := &fakeTaskMerger{result: tt.merge, onMerge: func() {
				if len(tt.merge.Conflicts) == 0 && !tt.merge.AlreadyUpToDate {
					head = "t2"
				}
			}}
			fz := &Finalize{deps: deps, taskMerger: taskMerger, gate: gate.gate, sessions: &conflictSessions{}, parentOpener: func() (parentMerger, error) { return merger, nil }}
			fz.resolver = &eventResolver{events: new([]string), result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}, onResolve: func() {
				head = "t2"
				if tt.session {
					fz.sessions.started.Add(1)
				}
			}}

			outcome, ptr, err := fz.Call(context.Background())

			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if tt.wantReroute {
				if outcome != shedengine.Stuck || !ptr.Reroute || ptr.Halt || !strings.Contains(ptr.Reason, tt.wantInReason) {
					t.Fatalf("Call() = %q, %+v; want a reroute naming %q", outcome, ptr, tt.wantInReason)
				}
				if gate.fake.verifyCalls != 0 || len(merger.calls) != 0 {
					t.Errorf("verify calls = %d, parent-side merges = %d; want none after a reroute", gate.fake.verifyCalls, len(merger.calls))
				}
			} else if outcome != shedengine.Done || gate.fake.verifyCalls != 1 {
				t.Fatalf("Call() = %q, %+v with %d verify call(s); want Done after one", outcome, ptr, gate.fake.verifyCalls)
			}
			note, noteErr := os.ReadFile(OverlapNotePath(deps.ScratchDir))
			if tt.wantNote == nil {
				if noteErr == nil {
					t.Errorf("overlap note = %q; want none written", note)
				}
			} else {
				for _, fragment := range tt.wantNote {
					if !strings.Contains(string(note), fragment) {
						t.Errorf("overlap note = %q, %v; want it to hold %q", note, noteErr, fragment)
					}
				}
			}
			if taskMerger.aborted != tt.wantAborted {
				t.Errorf("MergeAbort ran = %v; want %v", taskMerger.aborted, tt.wantAborted)
			}
			if got, want := strings.Join(changed, ","), strings.Join(tt.wantChanged, ","); got != want {
				t.Errorf("ChangedPaths calls = %q; want %q", got, want)
			}

			rec, found, _ := ReadMergeRecord(deps.MergeRecordPath)
			if !tt.wantRecord {
				if rec.MergedTip != earlierTip {
					t.Errorf("merge record = %+v; want it left as it was", rec)
				}
				return
			}
			if !found || rec.MergedTip != newTip || rec.TaskBase != taskBase || rec.TaskHead != "t1" || rec.MergeCommit != tt.wantMergeCommit || rec.UpToDate != tt.wantUpToDate {
				t.Errorf("merge record = %+v, %v; want merged tip %s, task base %s, task head t1, merge commit %q, up to date %v", rec, found, newTip, taskBase, tt.wantMergeCommit, tt.wantUpToDate)
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
