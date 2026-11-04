// finalize_pr_test.go covers Finalize's landing through the run's pull request over a fake GitHub: the decision under the lock, each way it lands or halts, and the merge on GitHub before the parent-side merge.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFinalize_PullRequest(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	githubOrigin := Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}
	const recordedURL = "https://github.com/acme/widgets/pull/7"
	const wantCredit = "Created using\n - LoomYard\n - Test Author <test@example.com>"
	const wantGitHubMerge = "Merge 7|task-head|true|A landing title (#7)|A landing body.\n\n" + wantCredit
	const wantCreate = "Create task-branch main|A landing title|A landing body.\n\n" + wantCredit
	const wantGitHubMergeWithVerdict = "Merge 7|task-head|true|A landing title (#7)|A landing body.\n\n" + testVerdict + "\n\n" + wantCredit
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
	reviewEnded := func(r *pullrequest.Record) { r.ReviewEnded = true }
	// openedRecord is the record Finalize writes for a pull request it opens itself under a merge parent.
	openedRecord := &pullrequest.Record{Number: 7, URL: recordedURL, Base: "main", Head: "task-branch", GatedHead: "task-head", ReviewEnded: true}
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
		// verdict is what Deps.DriftVerdict answers; empty leaves the seam nil.
		verdict string
		pushErr error
		merges  []mergeCallResult
		ffErr   error
		// taskEmpty makes the task branch hold no commit beyond its parent.
		taskEmpty bool
		// taskErr makes the TaskHasCommits seam fail.
		taskErr error
		// parentPushErr makes the parent branch's push fail.
		parentPushErr error
		// wantSquashedMerges asserts every parent-side merge was asked to squash.
		wantSquashedMerges bool
		// parentOwnedElsewhere makes the owner checks find the parent owned by another machine.
		parentOwnedElsewhere bool
		// wantRecord is the pull request record the landing must leave behind.
		wantRecord *pullrequest.Record
		// wantOwnerHalt makes the halt reason open with OwnerHaltPrefix instead of LandingStopPrefix.
		wantOwnerHalt bool

		wantOutcome shedengine.Outcome
		wantHalt    bool
		wantReason  []string
		wantErr     bool
		// wantErrText is a substring the returned error must hold.
		wantErrText string
		wantCalls   []string
		wantOrder   string
		wantMerges  int
		// wantNotInReason are substrings the reason must not hold.
		wantNotInReason []string
		wantNoRecord    bool
		// wantRecordKept asserts the record's file is still there.
		wantRecordKept bool
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
			wantCalls: []string{"RequiresPullRequest main", "Status 7", "Close 7 lym closed this pull request: pull request off: the parent's beam.yaml pull_request is off and the entry's pull_request is empty"},
		},
		{
			name: "AnOffParentWhoseBranchRulesRequireAPullRequestMergesTheRecordedOneOnGitHub", parentMode: "off", record: recorded(),
			scripts:     map[string][]scriptedCall{"RequiresPullRequest": {{found: true}}, "Status": {open}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,mark,push",
			wantCalls: []string{"RequiresPullRequest main", "Status 7", wantGitHubMerge},
		},
		{
			name: "ATransientParentRuleReadIsAnErrorStopWithNoMerge", parentMode: "off", record: recorded(),
			scripts: map[string][]scriptedCall{"RequiresPullRequest": {{err: context.DeadlineExceeded}}},
			wantErr: true, wantErrText: "read GitHub's rules for the parent branch", wantCalls: []string{"RequiresPullRequest main"},
		},
		{
			name: "AnUnreadableParentRuleWithARecordedPullRequestHaltsClosesNothingAndKeepsTheRecord", parentMode: "off", record: recorded(),
			scripts:     map[string][]scriptedCall{"RequiresPullRequest": {{err: githubErrorResponse(http.StatusNotFound, "Not Found")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"read GitHub's rules for the parent branch", "Not Found", "lym loom resume"},
			wantCalls: []string{"RequiresPullRequest main"}, wantRecordKept: true,
		},
		{
			name: "AnUnresolvableTokenOnTheParentRuleReadReadsAsNoRuleAndLandsLocally", parentMode: "off",
			scripts:     map[string][]scriptedCall{"RequiresPullRequest": {{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: githubclient.ErrTokenUnresolvable}}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,mark,push", wantCalls: []string{"RequiresPullRequest main"},
		},
		{
			name: "YesWithNoRecordHaltsNamingGotoPRReview", parentMode: "on",
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"has no pull request recorded", "lym loom goto PR-Review"},
		},
		{
			name: "AMergeParentWithNoRecordOpensThePullRequestAndSquashMergesItWhateverTheSquashSetting", parentMode: "merge", squashOff: true,
			scripts:     map[string][]scriptedCall{"Create": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}}}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,mark,push", wantRecordKept: true,
			wantSquashedMerges: true, wantRecord: openedRecord,
			wantCalls: []string{"FindOpen task-branch main", wantCreate, wantGitHubMerge},
		},
		{
			name: "AMergeParentFindsTheOpenPullRequestOnTheTaskBranchSetsItsTextAndMergesIt", parentMode: "merge",
			scripts:     map[string][]scriptedCall{"FindOpen": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}, found: true}}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,mark,push", wantRecordKept: true, wantRecord: openedRecord,
			wantCalls: []string{"FindOpen task-branch main", "Edit 7|A landing title|A landing body.\n\n" + wantCredit, wantGitHubMerge},
		},
		{
			name: "AFailedOpenUnderAMergeParentHaltsNamingGitHubsMessageAndMergesNothing", parentMode: "merge",
			scripts:     map[string][]scriptedCall{"Create": {{err: githubErrorResponse(http.StatusUnprocessableEntity, "No commits between main and task-branch")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"open the pull request", "No commits between", "lym loom resume"},
			wantOrder: "task-push", wantCalls: []string{"FindOpen task-branch main", wantCreate},
		},
		{
			name: "AMethodRefusalUnderAMergeParentNamesTheRepositorySettingsAndNotTheSquashSetting", parentMode: "merge", record: recorded(), squashOff: true,
			scripts:     mergeScript(open, refusal(pullrequest.RefusalMethod, "Squash merges are not allowed")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Squash merges are not allowed", "allow squash merging", "lym loom resume"},
			wantNotInReason: []string{"lym config landing"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "YesWithAClosedRecordWithoutTheOperatorsEndingHaltsNamingReopenAndCancel", parentMode: "on", record: recorded(), scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen it on GitHub", "lym loom goto PR-Review", "lym batten cancel me"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AContinueFlagWithoutTheRecordedEndingHaltsAsWell", parentMode: "on", record: recorded(withContinue), scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"closed without a merge"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "TheOperatorsEndingOnAClosedPullRequestReopensItAndMergesOnGitHub", parentMode: "on", record: recorded(reviewEnded),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "reopen,task-push,fast-forward,merge,mark,push",
			wantCalls: []string{"Status 7", "Reopen 7", wantGitHubMerge},
		},
		{
			name: "AReopenedPullRequestUnderAMergeParentSquashMergesWhateverTheSquashSetting", parentMode: "merge", record: recorded(reviewEnded), squashOff: true,
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "task-push,reopen,fast-forward,merge,mark,push",
			wantCalls: []string{"Status 7", "Reopen 7", wantGitHubMerge},
		},
		{
			name: "AMergeParentReopensAClosedRecordedPullRequestWithNoOperatorEndingAfterTheTaskPush", parentMode: "merge", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,reopen,fast-forward,merge,mark,push",
			wantCalls: []string{"Status 7", "Reopen 7", wantGitHubMerge},
		},
		{
			name: "AnOwnerHaltUnderAMergeParentReopensNothingOnGitHub", parentMode: "merge", record: recorded(), scripts: statusScript(closed), parentOwnedElsewhere: true,
			wantOutcome: shedengine.Stuck, wantHalt: true, wantOwnerHalt: true, wantReason: []string{"owned by machine"},
			wantOrder: "", wantCalls: []string{"Status 7"},
		},
		{
			name: "AFailedReopenUnderAMergeParentHaltsNamingGitHubsMessageAndMergesNothing", parentMode: "merge", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Reopen": {{err: errors.New("state cannot be changed")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen the pull request", "state cannot be changed", "lym loom resume"},
			wantOrder: "task-push,reopen", wantCalls: []string{"Status 7", "Reopen 7"},
		},
		{
			name: "AProtectedParentPushRefusalUnderAMergeParentOmitsTheSettingAdvice", parentMode: "merge", record: recorded(), scripts: mergeScript(open, scriptedCall{}),
			parentPushErr: errors.New("gitrepo: push rejected: ! [remote rejected] main -> main (protected branch hook declined)"),
			wantOutcome:   shedengine.Stuck, wantMerges: 1, wantReason: []string{"protected branch hook declined", `"git reset --keep origin/main"`},
			wantNotInReason: []string{"lym config beam"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AFailedReopenHaltsNamingGitHubsMessageAndMergesNothing", parentMode: "on", record: recorded(reviewEnded),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Reopen": {{err: errors.New("state cannot be changed")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen the pull request", "state cannot be changed", "lym loom resume"},
			wantOrder: "reopen", wantCalls: []string{"Status 7", "Reopen 7"},
		},
		{
			name: "AMergedPullRequestUnderAMergeParentLandsThroughASquashingParentSideMerge", parentMode: "merge", record: recorded(), squashOff: true, scripts: statusScript(merged),
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "merge,mark,push", wantCalls: []string{"Status 7"},
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
			name: "TheGitHubMergeMessageCarriesTheBodyThenTheRegressionCheckThenTheCredit", parentMode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), verdict: testVerdict,
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,mark,push", wantCalls: []string{"Status 7", wantGitHubMergeWithVerdict},
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
			name: "APendingRefusalForOverAMinuteHaltsNamingAnApprovalAndTheCancel", parentMode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalPending, "Base branch was modified")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Base branch was modified", "approves the pull request again", "lym batten cancel me", "lym loom resume"},
			wantNotInReason: []string{"locally", "lym loom goto PR-Review"},
			// Retries sleep 1, 2, 4, 8, 16 and 32 seconds, so the seventh refusal is the first past the minute.
			wantCalls: append([]string{"Status 7"}, slices.Repeat([]string{wantGitHubMerge}, 7)...),
		},
		{
			name: "AMethodRefusalHaltsNamingTheSquashSetting", parentMode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalMethod, "Merge commits are not allowed")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Merge commits are not allowed", "lym config landing --set squash=<true|false>", "lym loom resume"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AnotherRefusalHaltsNamingGitHubsMessageAndTheCancel", parentMode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalOther, "Required status check is expected")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Required status check is expected", "lym batten cancel me"}, wantCalls: []string{"Status 7", wantGitHubMerge},
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
			name: "ARejectedTokenWrappedInAURLErrorHaltsNamingTheTokenAndNotATransientError", parentMode: "on", record: recorded(),
			scripts:     statusScript(scriptedCall{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: fmt.Errorf("%w: the token from GH_TOKEN (401)", githubclient.ErrTokenRejected)}}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"GH_TOKEN, GITHUB_TOKEN", "lym loom resume"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AFailingTaskCommitReadIsAnErrorStopWithNoGitHubCallAndNoMerge", parentMode: "on", record: recorded(),
			taskErr: errors.New("rev-list failed"),
			wantErr: true, wantErrText: "read whether the task branch has commits beyond its parent",
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
			api.onCall = func(method string) {
				if method == "Reopen" {
					order = append(order, "reopen")
				}
			}
			deps := newTestDeps(t)
			deps.Slug = "me"
			deps.Config.Squash = !tt.squashOff
			if tt.parentOwnedElsewhere {
				deps.ParentOwner = func() (ParentOwnership, error) { return ParentOwnership{Owner: "other-machine"}, nil }
			}
			if tt.verdict != "" {
				deps.DriftVerdict = func() (string, error) { return tt.verdict, nil }
			}
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
				TaskHasCommits: func() (bool, error) { return !tt.taskEmpty, tt.taskErr },
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
			merger := &recordingParentMerger{results: merges, order: &order, ffErr: tt.ffErr, pushErr: tt.parentPushErr}
			res := &recordingResolver{result: mergeresolve.Result{Outcome: mergeresolve.OutcomeResolved}}
			fz := &Finalize{deps: deps, resolver: res, guard: newParentGuard(deps), parentOpener: func() (parentMerger, error) { return merger, nil }}

			outcome, pointer, err := fz.Call(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("Call() error = %v; want error %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("Call() error = %q; want it to hold %q", err, tt.wantErrText)
			}
			if err == nil {
				if outcome != tt.wantOutcome || pointer.Halt != tt.wantHalt {
					t.Fatalf("Call() = %q, halt %v; want %q, halt %v (reason %q)", outcome, pointer.Halt, tt.wantOutcome, tt.wantHalt, pointer.Reason)
				}
				wantPrefix := LandingStopPrefix
				if tt.wantOwnerHalt {
					wantPrefix = OwnerHaltPrefix
				}
				if tt.wantHalt && !strings.HasPrefix(pointer.Reason, wantPrefix) {
					t.Errorf("Call() reason %q does not open with %q", pointer.Reason, wantPrefix)
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
			for i, call := range merger.calls {
				if tt.wantSquashedMerges && !call.opts.Squash {
					t.Errorf("parent-side merge %d Squash = false; want true", i)
				}
			}
			if tt.wantRecord != nil {
				got, found, rerr := pullrequest.ReadRecord(deps.PullRequest.RecordPath, deps.PullRequest.RecordLockPath)
				key := func(r pullrequest.Record) string {
					return fmt.Sprintf("%d|%s|%s|%s|%s|%t", r.Number, r.URL, r.Base, r.Head, r.GatedHead, r.ReviewEnded)
				}
				if rerr != nil || !found || key(got) != key(*tt.wantRecord) {
					t.Errorf("ReadRecord() = %q, found %v, error %v; want %q", key(got), found, rerr, key(*tt.wantRecord))
				}
			}
			if !lockHeldAtMode {
				t.Error("the parent lock was not held when the pull-request mode was read; want the decision made under the lock")
			}
			if _, found, rerr := pullrequest.ReadRecord(deps.PullRequest.RecordPath, deps.PullRequest.RecordLockPath); rerr != nil || (tt.wantNoRecord && found) || (tt.wantRecordKept && !found) {
				t.Errorf("ReadRecord() found = %v, error = %v; want no record left: %v, record kept: %v", found, rerr, tt.wantNoRecord, tt.wantRecordKept)
			}
		})
	}
}

// TestWithSection asserts a section follows the trimmed body after a blank line, stands alone after a body of whitespace, and leaves the body as it is when empty.
func TestWithSection(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		section string
		want    string
	}{
		{"EmptySectionLeavesTheBodyUnchanged", "Body\n", "", "Body\n"},
		{"SectionFollowsTheTrimmedBodyAfterABlankLine", "Body\n\n", testVerdict, "Body\n\n" + testVerdict},
		{"WhitespaceOnlyBodyIsTheSectionAlone", " \n\t\n", testVerdict, testVerdict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := withSection(tt.body, tt.section); got != tt.want {
				t.Errorf("withSection(%q, %q) = %q; want %q", tt.body, tt.section, got, tt.want)
			}
		})
	}
}
