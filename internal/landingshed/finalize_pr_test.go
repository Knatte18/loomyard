//go:build tier0

// finalize_pr_test.go covers Finalize's landing through the run's pull request over a fake GitHub: the decision under the lock, each way it lands or halts, and the merge on GitHub before the parent-side merge.

package landingshed

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestFinalize_PullRequest(t *testing.T) {
	t.Parallel()

	githubOrigin := Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}
	const recordedURL = "https://github.com/acme/widgets/pull/7"
	const wantTrailer = "Co-Authored-By: Test Author <test@example.com>\nLym-Task: me"
	const wantGitHubMerge = "Merge 7|task-head|true|A landing title|A landing body.\n\n" + wantTrailer
	statusScript := func(calls ...scriptedCall) map[string][]scriptedCall {
		return map[string][]scriptedCall{"Status": calls}
	}
	open := scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen}}
	closed := scriptedCall{status: pullrequest.Status{State: pullrequest.StateClosed}}
	merged := scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged}}
	recorded := func(mutate ...func(*pullrequest.Record)) *pullrequest.Record {
		rec := &pullrequest.Record{Number: 7, URL: recordedURL}
		for _, m := range mutate {
			m(rec)
		}
		return rec
	}
	closedForContinue := func(r *pullrequest.Record) { r.ClosedForContinue = true }
	withContinue := func(r *pullrequest.Record) { r.Continue = true }
	refusal := func(kind pullrequest.RefusalKind, message string) scriptedCall {
		return scriptedCall{err: &pullrequest.MergeRefusal{Kind: kind, Message: message}}
	}
	mergeScript := func(open scriptedCall, calls ...scriptedCall) map[string][]scriptedCall {
		return map[string][]scriptedCall{"Status": {open}, "Merge": calls}
	}

	tests := []struct {
		name       string
		parentMode string
		parentErr  error
		origin     *Origin
		record     *pullrequest.Record
		scripts    map[string][]scriptedCall
		squashOff  bool
		pushErr    error
		merges     []mergeCallResult
		ffErr      error
		// taskEmpty makes the task branch hold no commit beyond its parent.
		taskEmpty bool

		wantOutcome shedengine.Outcome
		wantHalt    bool
		wantReason  []string
		wantErr     bool
		wantCalls   []string
		wantOrder   string
		wantMerges  int
		// wantNotInReason are substrings the reason must not hold.
		wantNotInReason []string
		wantNoRecord    bool
	}{
		{
			name: "UnreadableParentModeHaltsNamingTheWorktreeBehindTheLandingPrefix", parentErr: errors.New("bad yaml"),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"worktree wt-parent", "bad yaml", "lym config beam"},
		},
		{
			name: "AResolverHaltIsStuckBehindTheLandingPrefix", parentMode: "required", origin: &Origin{},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"lym loom resume"},
		},
		{
			name: "NoClosesAnOpenRecordedPullRequestRemovesTheRecordAndMergesLocally", parentMode: "off", record: recorded(), scripts: statusScript(open),
			wantOutcome: shedengine.Done, wantMerges: 1, wantNoRecord: true, wantOrder: "merge,mark,push",
			wantCalls: []string{"Status 7", "Close 7 lym closed this pull request: pull request off: the parent's beam.yaml pull_request is off and the entry's pull_request is empty"},
		},
		{
			name: "YesWithNoRecordHaltsNamingGotoPRReview", parentMode: "on",
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"has no pull request recorded", "lym loom goto PR-Review"},
		},
		{
			name: "YesWithAClosedRecordNotClosedForContinueHaltsNamingReopenAndCancel", parentMode: "on", record: recorded(), scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen it on GitHub", "lym loom goto PR-Review", "lym batten cancel me"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AContinueFlagWithoutClosedForContinueHaltsAsWell", parentMode: "on", record: recorded(withContinue), scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"closed without a merge"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "ClosedForContinueMergesLocallyWithNoGitHubMerge", parentMode: "on", record: recorded(closedForContinue), scripts: statusScript(closed),
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,mark,push", wantCalls: []string{"Status 7"},
		},
		{
			name: "AMergedPullRequestMakesNoGitHubMergeAndLandsThroughTheParentSideMerge", parentMode: "on", record: recorded(), scripts: statusScript(merged),
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,mark,push", wantCalls: []string{"Status 7"},
		},
		{
			name: "AnOpenOnePushesMergesOnGitHubWithThePushedHeadFastForwardsAndMarksTheTaskDone", parentMode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}),
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,mark,push", wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "TheMergeMethodFollowsTheSquashSetting", parentMode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), squashOff: true,
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,mark,push",
			wantCalls: []string{"Status 7", strings.Replace(wantGitHubMerge, "|true|", "|false|", 1)},
		},
		{
			name: "AMergeInRequiredErrorAfterTheGitHubMergeRetriesTheLocalMergeWithNoSecondGitHubMerge", parentMode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}),
			merges:      []mergeCallResult{{err: &fabricengine.ErrMergeInRequired{}}, {result: fabricengine.MergeResult{Committed: true}}},
			wantOutcome: shedengine.Done, wantMerges: 2, wantOrder: "task-push,fast-forward,merge,merge,mark,push", wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "APendingRefusalThenASuccessMergesOnceRetried", parentMode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalPending, "Base branch was modified"), scriptedCall{}),
			wantOutcome: shedengine.Done, wantMerges: 1, wantCalls: []string{"Status 7", wantGitHubMerge, wantGitHubMerge},
		},
		{
			name: "APendingRefusalForOverAMinuteHaltsNamingAnApprovalAndTheOperatorsWay", parentMode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalPending, "Base branch was modified")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Base branch was modified", "approves the pull request again", "lym loom goto PR-Review", "lym loom resume"},
			// Retries sleep 1, 2, 4, 8, 16 and 32 seconds, so the seventh refusal is the first past the minute.
			wantCalls: append([]string{"Status 7"}, slices.Repeat([]string{wantGitHubMerge}, 7)...),
		},
		{
			name: "AMethodRefusalHaltsNamingTheSquashSetting", parentMode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalMethod, "Merge commits are not allowed")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Merge commits are not allowed", "lym config landing --set squash=<true|false>", "lym loom resume"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AnotherRefusalHaltsNamingGitHubsMessageAndGotoPRReview", parentMode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalOther, "Required status check is expected")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Required status check is expected", "lym loom goto PR-Review", "ask"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "ATransientMergeFailureIsAnError", parentMode: "on", record: recorded(),
			scripts: mergeScript(open, scriptedCall{err: context.DeadlineExceeded}),
			wantErr: true, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AnUnresolvableTokenWrappedInAURLErrorHaltsNamingGhAuthLogin", parentMode: "on", record: recorded(),
			scripts:     statusScript(scriptedCall{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: githubclient.ErrTokenUnresolvable}}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"gh auth login --hostname github.com", "lym loom resume"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AFailingStatusReadHaltsNamingGitHubsMessageAndNotTheToken", parentMode: "on", record: recorded(),
			scripts:     statusScript(scriptedCall{err: errors.New("404 Not Found")}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"404 Not Found", "fix what GitHub's message names"}, wantNotInReason: []string{"GH_TOKEN"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AnEmptyTaskBranchUnderOnWithNoRecordLandsLocallyWithNoGitHubCall", parentMode: "on", taskEmpty: true,
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,mark,push",
		},
		{
			name: "AnEmptyTaskBranchClosesAnOpenRecordedPullRequestAndLandsLocally", parentMode: "on", taskEmpty: true, record: recorded(), scripts: statusScript(open),
			wantOutcome: shedengine.Done, wantMerges: 1, wantNoRecord: true, wantOrder: "merge,mark,push",
			wantCalls: []string{"Status 7", "Close 7 lym closed this pull request: " + emptyBranchReason},
		},
		{
			name: "AFailingPushHaltsNamingFabricPushAndMergesNothing", parentMode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), pushErr: errors.New("rejected"),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"rejected", "lym fabric push", "lym loom resume"}, wantOrder: "task-push", wantCalls: []string{"Status 7"},
		},
		{
			name: "AFailingFastForwardAfterTheMergeHaltsNamingLoomResume", parentMode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), ffErr: errors.New("not fast-forward"),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"merged on GitHub", "not fast-forward", "lym loom resume"}, wantOrder: "task-push,fast-forward", wantCalls: []string{"Status 7", wantGitHubMerge},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var order []string
			api := &fakePullRequestAPI{scripts: tt.scripts}
			deps := newTestDeps(t)
			deps.Slug = "me"
			deps.Config.Squash = !tt.squashOff
			deps.MarkTaskDone = func() error {
				order = append(order, "mark")
				return nil
			}
			dir := t.TempDir()
			now := prReviewFixtureNow
			mode := tt.parentMode
			origin := githubOrigin
			if tt.origin != nil {
				origin = *tt.origin
			}
			lockHeldAtMode := false
			deps.PullRequest = PullRequestDeps{
				RecordPath:     filepath.Join(dir, "pull-request.json"),
				RecordLockPath: filepath.Join(dir, "pull-request.lock"),
				ParentMode: func() (string, string, error) {
					rec, found, _ := deps.Lock.Store.Read("main")
					lockHeldAtMode = found && rec.Owner.Slug == "me"
					return mode, "wt-parent", tt.parentErr
				},
				EntryField:     func() (string, error) { return "", nil },
				Origin:         func() (Origin, error) { return origin, nil },
				TaskHasCommits: func() (bool, error) { return !tt.taskEmpty, nil },
				API:            func(string) (pullrequest.API, error) { return api, nil },
				PushTask: func() error {
					order = append(order, "task-push")
					return tt.pushErr
				},
				Now: func() time.Time { return now },
				Sleep: func(_ context.Context, d time.Duration) error {
					now = now.Add(d)
					return nil
				},
			}
			if tt.record != nil {
				if err := pullrequest.WriteRecord(deps.PullRequest.RecordPath, deps.PullRequest.RecordLockPath, *tt.record); err != nil {
					t.Fatalf("write record: %v", err)
				}
			}
			merges := tt.merges
			if merges == nil {
				merges = []mergeCallResult{{result: fabricengine.MergeResult{Committed: true}}}
			}
			merger := &recordingParentMerger{results: merges, order: &order, ffErr: tt.ffErr}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, pointer, err := fz.Call(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("Call() error = %v; want error %v", err, tt.wantErr)
			}
			if err == nil {
				if outcome != tt.wantOutcome || pointer.Halt != tt.wantHalt {
					t.Fatalf("Call() = %q, halt %v; want %q, halt %v (reason %q)", outcome, pointer.Halt, tt.wantOutcome, tt.wantHalt, pointer.Reason)
				}
				if tt.wantHalt && !strings.HasPrefix(pointer.Reason, LandingStopPrefix) {
					t.Errorf("Call() reason %q does not open with %q", pointer.Reason, LandingStopPrefix)
				}
				for _, want := range tt.wantReason {
					if !strings.Contains(pointer.Reason, want) {
						t.Errorf("Call() reason %q lacks %q", pointer.Reason, want)
					}
				}
				for _, unwanted := range tt.wantNotInReason {
					if strings.Contains(pointer.Reason, unwanted) {
						t.Errorf("Call() reason %q holds %q", pointer.Reason, unwanted)
					}
				}
			}
			if diff := cmp.Diff(tt.wantCalls, api.calls); diff != "" {
				t.Errorf("GitHub calls mismatch (-want +got):\n%s", diff)
			}
			if tt.wantOrder != "" {
				if got := strings.Join(order, ","); got != tt.wantOrder {
					t.Errorf("call order = %q; want %q", got, tt.wantOrder)
				}
			}
			if len(merger.calls) != tt.wantMerges {
				t.Errorf("parent-side merges = %d; want %d", len(merger.calls), tt.wantMerges)
			}
			if !lockHeldAtMode {
				t.Error("the parent lock was not held when the pull-request mode was read; want the decision made under the lock")
			}
			if _, found, rerr := pullrequest.ReadRecord(deps.PullRequest.RecordPath, deps.PullRequest.RecordLockPath); rerr != nil || (tt.wantNoRecord && found) {
				t.Errorf("ReadRecord() found = %v, error = %v; want no record left: %v", found, rerr, tt.wantNoRecord)
			}
		})
	}
}
