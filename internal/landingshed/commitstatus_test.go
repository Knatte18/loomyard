// commitstatus_test.go covers Deps.CommitStatus, the injected loop-owner seam Finalize commits the product's own orchestration status file through before it merges.
// Merge-In's cases live in mergein_test.go.
//
// The ordering assertion is the point of every case here. Committing after the merge would be
// indistinguishable from not committing at all, because fabricengine's merge guard has already
// refused by then -- so each case records the call order rather than merely that both calls
// happened.

package landingshed

import (
	"context"
	"errors"
	"reflect"
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
				if !reflect.DeepEqual(ptr, shedengine.OutputPointer{}) {
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
