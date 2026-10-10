// premerge_test.go covers the pre-merge step both producers run ahead of their merge-in, driven through each producer's Call with the package's fake resolver and recording seam closures.
// It captures the process-global logger output, so its tests do not run in parallel.

package landingshed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const (
	mergeWayForwardClause = `conclude it with "lyx fabric merge --continue" or discard it with "lyx fabric merge --abort", then resume`
	resolverRanReason     = "the resolver ran"
)

// TestPreMerge_ClearsOwnParkedMergeIn pins, for both producers, what the step does before the merge-in for each merge state the pair can be in:
// the seam calls it makes, whether the merge-in follows, and the Stuck reason or error it ends the call with.
// A last subtest pins that Finalize probes again before the merge-in it repeats after the parent moved.
func TestPreMerge_ClearsOwnParkedMergeIn(t *testing.T) {
	probeErr := errors.New("probe boom")
	statusErr := errors.New("strand table unreadable")
	stopErr := errors.New("stop boom")
	abortErr := errors.New("abort boom")
	ownLeftover := fabricengine.MidMergeState{
		Kind: fabricengine.MidMergeParked, Verb: "merge-in", Source: "main",
		SourceSHA: "sourcesha", StartSHA: "startsha",
	}

	tests := []struct {
		name         string
		state        fabricengine.MidMergeState
		probeErr     error
		stopGUID     string
		stopErr      error
		abortErr     error
		nilSeams     bool
		wantCalls    string
		wantResolver bool
		wantReason   string
		wantErr      error
		wantLog      []string
	}{
		{
			name:         "a clean pair proceeds without a stop or an abort",
			state:        fabricengine.MidMergeState{Kind: fabricengine.MidMergeNone},
			wantResolver: true,
		},
		{
			name:         "the row's own parked merge-in is stopped over, aborted and logged, then merged afresh",
			state:        ownLeftover,
			wantCalls:    "stop,abort",
			wantResolver: true,
			wantLog:      []string{"producer=", "source=main", "source_sha=sourcesha", "start_sha=startsha"},
		},
		{
			name:       "a conflict session that cannot be stopped leaves the merge untouched",
			state:      ownLeftover,
			stopGUID:   "guid-1",
			stopErr:    stopErr,
			wantCalls:  "stop",
			wantReason: `a conflict session of this run (strand guid-1) is still live and could not be stopped: stop boom; way forward: run "lyx reed remove guid-1", then "lyx loom resume"`,
		},
		{
			name:      "an unreadable strand table is an error with no abort",
			state:     ownLeftover,
			stopErr:   statusErr,
			wantCalls: "stop",
			wantErr:   statusErr,
		},
		{
			name:       "an abort that fails after a successful stop is Stuck naming the parent branch",
			state:      ownLeftover,
			abortErr:   abortErr,
			wantCalls:  "stop,abort",
			wantReason: `the parked merge-in of parent branch "main" could not be aborted: abort boom; way forward: ` + mergeWayForwardClause,
		},
		{
			name:       "a parked merge verb is never stopped over",
			state:      fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Verb: "merge", Source: "main"},
			wantReason: "a fabric merge (merge of main) is in progress in the task worktree; way forward: " + mergeWayForwardClause,
		},
		{
			name:       "a parked merge-in of another source is never stopped over",
			state:      fabricengine.MidMergeState{Kind: fabricengine.MidMergeParked, Verb: "merge-in", Source: "other"},
			wantReason: "a fabric merge (merge-in of other) is in progress in the task worktree; way forward: " + mergeWayForwardClause,
		},
		{
			name:       "foreign git merge state is never touched",
			state:      fabricengine.MidMergeState{Kind: fabricengine.MidMergeForeign, Conflicts: []string{"a.txt"}},
			wantReason: "a git merge, cherry-pick or squash that fabric did not start is in progress in the task worktree; way forward: conclude or abort it with git, then resume",
		},
		{
			name:         "a nil probe seam skips the step",
			state:        ownLeftover,
			nilSeams:     true,
			wantResolver: true,
		},
		{
			name:     "a probe error is returned",
			probeErr: probeErr,
			wantErr:  probeErr,
		},
	}

	for _, producer := range []string{publishName, finalizeName} {
		for _, tt := range tests {
			t.Run(producer+"/"+tt.name, func(t *testing.T) {
				logs := logcapture.CaptureVerbose(t)
				var calls []string
				pm := preMerge{}
				if !tt.nilSeams {
					pm = preMerge{
						mergeState: func() (fabricengine.MidMergeState, error) { return tt.state, tt.probeErr },
						abortMerge: func() error { calls = append(calls, "abort"); return tt.abortErr },
						stopConflictSession: func() (string, error) {
							calls = append(calls, "stop")
							return tt.stopGUID, tt.stopErr
						},
					}
				}
				res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeStuck, Reason: resolverRanReason}}

				outcome, ptr, err := callProducer(t, producer, pm, res)

				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Call() error = %v; want %v", err, tt.wantErr)
				}
				if got := strings.Join(calls, ","); got != tt.wantCalls {
					t.Errorf("seam calls = %q; want %q", got, tt.wantCalls)
				}
				if res.called != tt.wantResolver {
					t.Errorf("merge-in ran = %v; want %v", res.called, tt.wantResolver)
				}
				if tt.wantErr == nil {
					wantReason := tt.wantReason
					if tt.wantResolver {
						wantReason = resolverRanReason
					}
					if outcome != shedengine.Stuck || ptr.Reason != wantReason {
						t.Errorf("Call() = (%q, %q); want (Stuck, %q)", outcome, ptr.Reason, wantReason)
					}
				}
				for _, want := range tt.wantLog {
					if !strings.Contains(logs.String(), want) {
						t.Errorf("log output lacks %q:\n%s", want, logs.String())
					}
				}
			})
		}
	}

	t.Run("Finalize's retry pass after the parent moved probes again", func(t *testing.T) {
		deps := newTestDeps(t)
		deps.MarkTaskDone = func() error { return nil }
		probes := 0
		pm := preMerge{mergeState: func() (fabricengine.MidMergeState, error) {
			probes++
			return fabricengine.MidMergeState{Kind: fabricengine.MidMergeNone}, nil
		}}
		res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
		merger := &recordingParentMerger{results: []mergeCallResult{
			{err: &fabricengine.ErrMergeInRequired{Source: deps.TaskBranch}},
			{result: fabricengine.MergeResult{Committed: true}},
		}}
		fz := &Finalize{deps: deps, resolver: res, premerge: pm, parentOpener: func() (parentMerger, error) { return merger, nil }}

		outcome, ptr, err := fz.Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = (%q, %q, %v); want Done", outcome, ptr.Reason, err)
		}
		if probes != 2 {
			t.Errorf("merge-state probes = %d; want 2 (one per merge-in pass)", probes)
		}
	})
}

// callProducer runs the named producer's Call over a deps value, the fake resolver and the given pre-merge seams.
func callProducer(t *testing.T, producer string, pm preMerge, res *recordingResolver) (shedengine.Outcome, shedengine.OutputPointer, error) {
	t.Helper()
	deps := newTestDeps(t)
	switch producer {
	case publishName:
		return (&Publish{deps: deps, resolver: res, premerge: pm}).Call(context.Background())
	case finalizeName:
		return (&Finalize{deps: deps, resolver: res, premerge: pm}).Call(context.Background())
	default:
		t.Fatalf("unknown producer %q", producer)
		return "", shedengine.OutputPointer{}, nil
	}
}
