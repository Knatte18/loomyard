// commitstatus_test.go covers Deps.CommitStatus, the injected loop-owner seam both producers commit
// the product's own orchestration status file through before they merge. The seam spans both
// producers, so its cases live in one file rather than being duplicated into publish_test.go and
// finalize_test.go; both reuse those files' own in-package fakes.
//
// The ordering assertion is the point of every case here. Committing after the merge would be
// indistinguishable from not committing at all, because fabricengine's merge guard has already
// refused by then -- so each case records the call order rather than merely that both calls
// happened.

package landingshed

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// orderRecordingResolver is a resolver fake that appends "merge" to a shared event slice, so a test
// can assert the commit seam ran before the merge rather than merely that both ran.
type orderRecordingResolver struct {
	events *[]string
	result mergeresolve.Result
	err    error
}

func (r *orderRecordingResolver) Resolve(_ context.Context, _ string) (mergeresolve.Result, error) {
	*r.events = append(*r.events, "merge")
	return r.result, r.err
}

// commitStatusRecorder returns a CommitStatus closure appending "commit" to events and returning
// err, so a test scripts both the ordering and the failure disposition from one helper.
func commitStatusRecorder(events *[]string, err error) func() error {
	return func() error {
		*events = append(*events, "commit")
		return err
	}
}

func TestFinalize_CommitStatus_RunsBeforeTheMergeIn(t *testing.T) {
	var events []string
	deps := newFinalizeDeps(t)
	deps.CommitStatus = commitStatusRecorder(&events, nil)
	res := &orderRecordingResolver{events: &events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	want := []string{"commit", "merge"}
	if !slices.Equal(events, want) {
		t.Errorf("call order = %v; want %v (a commit after the merge is the same as no commit at all)", events, want)
	}
}

func TestFinalize_CommitStatus_FailureIsAnErrorAndNeverMerges(t *testing.T) {
	sentinel := errors.New("git index locked")
	var events []string
	deps := newFinalizeDeps(t)
	deps.CommitStatus = commitStatusRecorder(&events, sentinel)
	res := &orderRecordingResolver{events: &events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, ptr, err := fz.Call(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("Call() error = %v; want errors.Is(err, sentinel)", err)
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if slices.Contains(events, "merge") {
		t.Errorf("call order = %v; want no merge after a failed commit -- the guard would refuse anyway", events)
	}
	if len(merger.calls) != 0 {
		t.Errorf("parent-side merge calls = %d; want 0", len(merger.calls))
	}
}

func TestFinalize_CommitStatus_NilIsNotAnError(t *testing.T) {
	deps := newFinalizeDeps(t)
	deps.CommitStatus = nil
	res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
	fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

	outcome, _, err := fz.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil (a nil seam means \"no status file to commit\")", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
}

func TestPublish_CommitStatus_RunsBeforeTheMergeIn(t *testing.T) {
	var events []string
	deps := newTestDeps(t)
	deps.PushSkipped = false
	deps.PushBranch = func() error { return nil }
	deps.CommitStatus = commitStatusRecorder(&events, nil)
	// A stuck merge-in returns before the push and before any GitHub call, so this case exercises
	// the ordering without reaching the network.
	res := &orderRecordingResolver{events: &events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict"}}
	p := &Publish{deps: deps, resolver: res}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	want := []string{"commit", "merge"}
	if !slices.Equal(events, want) {
		t.Errorf("call order = %v; want %v", events, want)
	}
}

func TestPublish_CommitStatus_NotCalledWhenNoPullRequestIsRequired(t *testing.T) {
	var events []string
	deps := newTestDeps(t)
	deps.Config.RequirePRToBase = []string{"some-other-branch"}
	deps.CommitStatus = commitStatusRecorder(&events, nil)
	res := &orderRecordingResolver{events: &events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if len(events) != 0 {
		t.Errorf("recorded calls = %v; want none -- a run that never merges has nothing to commit for", events)
	}
}

func TestPublish_CommitStatus_FailureIsAnErrorAndNeverMerges(t *testing.T) {
	sentinel := errors.New("git index locked")
	var events []string
	deps := newTestDeps(t)
	deps.PushSkipped = false
	deps.PushBranch = func() error { return nil }
	deps.CommitStatus = commitStatusRecorder(&events, sentinel)
	res := &orderRecordingResolver{events: &events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
	p := &Publish{deps: deps, resolver: res}

	outcome, ptr, err := p.Call(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("Call() error = %v; want errors.Is(err, sentinel)", err)
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if slices.Contains(events, "merge") {
		t.Errorf("call order = %v; want no merge after a failed commit", events)
	}
}
