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

// TestFinalize_CommitStatus pins the seam's ordering and disposition in Finalize: it runs before the merge-in, a failure is an error that never merges, and an unwired seam is not an error.
// It stays serial because newTestDeps swaps the package-level NewGitHubClient.
func TestFinalize_CommitStatus(t *testing.T) {
	sentinel := errors.New("git index locked")
	cases := []struct {
		name       string
		commitErr  error
		nilSeam    bool
		wantEvents []string
		wantMerges int
	}{
		{"runs before the merge-in", nil, false, []string{"commit", "merge"}, 1},
		{"failure is an error and never merges", sentinel, false, []string{"commit"}, 0},
		{"nil seam is not an error", nil, true, []string{"merge"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			deps := newTestDeps(t)
			deps.CommitStatus = commitStatusRecorder(&events, tc.commitErr)
			if tc.nilSeam {
				deps.CommitStatus = nil
			}
			res := &orderRecordingResolver{events: &events, result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			merger := &recordingParentMerger{results: []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}}
			fz := &Finalize{deps: deps, resolver: res, parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, ptr, err := fz.Call(context.Background())
			if tc.commitErr != nil {
				if !errors.Is(err, tc.commitErr) {
					t.Fatalf("Call() error = %v; want errors.Is(err, sentinel)", err)
				}
				if outcome != "" {
					t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
				}
				if ptr != (shedengine.OutputPointer{}) {
					t.Errorf("Call() pointer = %+v; want empty", ptr)
				}
			} else if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			// A commit after the merge is the same as no commit at all: the merge guard would already have refused.
			if !slices.Equal(events, tc.wantEvents) {
				t.Errorf("call order = %v; want %v", events, tc.wantEvents)
			}
			if len(merger.calls) != tc.wantMerges {
				t.Errorf("parent-side merge calls = %d; want %d", len(merger.calls), tc.wantMerges)
			}
		})
	}
}

// TestPublish_CommitStatus pins the same seam in Publish: it runs before the merge-in, a failure is an error that never merges, and a run needing no pull request never calls it.
// It stays serial because newTestDeps swaps the package-level NewGitHubClient.
func TestPublish_CommitStatus(t *testing.T) {
	sentinel := errors.New("git index locked")
	cases := []struct {
		name       string
		commitErr  error
		mutate     func(*Deps)
		resolved   mergeresolve.Result
		wantOut    shedengine.Outcome
		wantEvents []string
	}{
		{
			// A stuck merge-in returns before the push and before any GitHub call, so this case exercises the ordering without reaching the network.
			name:       "runs before the merge-in",
			resolved:   mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: "conflict"},
			wantOut:    shedengine.Stuck,
			wantEvents: []string{"commit", "merge"},
		},
		{
			name:       "failure is an error and never merges",
			commitErr:  sentinel,
			resolved:   mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			wantEvents: []string{"commit"},
		},
		{
			name:     "not called when no pull request is required",
			mutate:   func(d *Deps) { d.Config.RequirePRToBase = []string{"some-other-branch"} },
			resolved: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved},
			wantOut:  shedengine.Done,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			deps := newTestDeps(t)
			deps.PushSkipped = false
			deps.PushBranch = func() error { return nil }
			deps.CommitStatus = commitStatusRecorder(&events, tc.commitErr)
			if tc.mutate != nil {
				tc.mutate(&deps)
			}
			res := &orderRecordingResolver{events: &events, result: tc.resolved}
			p := &Publish{deps: deps, resolver: res}

			outcome, ptr, err := p.Call(context.Background())
			if tc.commitErr != nil {
				if !errors.Is(err, tc.commitErr) {
					t.Fatalf("Call() error = %v; want errors.Is(err, sentinel)", err)
				}
				if outcome != "" {
					t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
				}
				if ptr != (shedengine.OutputPointer{}) {
					t.Errorf("Call() pointer = %+v; want empty", ptr)
				}
			} else if err != nil || outcome != tc.wantOut {
				t.Fatalf("Call() = %q, %v; want %q, nil", outcome, err, tc.wantOut)
			}
			if !slices.Equal(events, tc.wantEvents) {
				t.Errorf("call order = %v; want %v", events, tc.wantEvents)
			}
		})
	}
}
