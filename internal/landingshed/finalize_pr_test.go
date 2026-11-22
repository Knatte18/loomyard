// finalize_pr_test.go covers Finalize's landing through the run's pull request over a fake GitHub:
// the decision under the lock, each way it lands or halts, and the merge on GitHub before the parent-side merge.

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
	mergedAt := func(head string) scriptedCall {
		return scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: head}}
	}
	recordedBeam := &BeamPullRequest{Number: 7, URL: recordedURL}
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
	// openAt is an open pull request at the task head in GitHub's mergeable state mergeable.
	openAt := func(mergeable string) scriptedCall {
		return scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: "task-head", MergeableState: mergeable}}
	}
	notMergeable := refusal(pullrequest.RefusalOther, "Pull Request is not mergeable")
	// mergeStateRead is the GitHub calls of one merge-state reading of a pull request GitHub has computed.
	mergeStateRead := []string{"Status 7", "RequiredChecks main task-head"}

	tests := []struct {
		name string
		// mode is beam.yaml's pull_request for the landing run, as the Mode seam reads it.
		mode string
		// modeErr makes the Mode seam fail.
		modeErr   error
		origin    *Origin
		record    *pullrequest.Record
		scripts   map[string][]scriptedCall
		squashOff bool
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
		// wantTrueMerges asserts no parent-side merge was asked to squash.
		wantTrueMerges bool
		// parentOwnedElsewhere makes the owner checks find the parent owned by another machine.
		parentOwnedElsewhere bool
		// wantRecord is the pull request record the landing must leave behind.
		wantRecord *pullrequest.Record
		// wantOwnerHalt makes the halt reason open with `OwnerHaltPrefix`.
		wantOwnerHalt bool
		// beam makes the run a beam's: its landing record is read, and a spent one is dropped.
		beam bool
		// beamPR is the pull request the beam's landing record holds; nil leaves none.
		beamPR *BeamPullRequest
		// beamReadErr makes the read of the beam's landing record fail.
		beamReadErr error
		// commitsMissing lists the task commits the merged pull request lacks.
		commitsMissing []string

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
		// commitErr is what the CommitRecord seam fails with.
		commitErr error
		// wantCommits is how many times the landing commits the record.
		wantCommits int
	}{
		{
			name: "AFailedModeLookupHaltsNamingTheErrorAndTheFixBehindTheLandingPrefix", modeErr: errors.New("bad yaml"),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"beam.yaml's pull_request for this landing could not be read", "bad yaml", "lym config beam", "--layer default", "run_settings", "lym loom resume"},
		},
		{
			name: "AResolverHaltIsStuckBehindTheLandingPrefix", mode: "merge", origin: &Origin{},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"lym loom resume"},
		},
		{
			name: "NoClosesAnOpenRecordedPullRequestRemovesTheRecordAndMergesLocally", mode: "off", record: recorded(), scripts: statusScript(open), wantCommits: 1,
			wantOutcome: shedengine.Done, wantMerges: 1, wantNoRecord: true, wantOrder: "merge,push,mark",
			wantCalls: []string{"RequiresPullRequest main", "Status 7", "Close 7 lym closed this pull request: pull request off: beam.yaml's pull_request for this landing is off"},
		},
		{
			name: "AnOffParentWhoseBranchRulesRequireAPullRequestMergesTheRecordedOneOnGitHub", mode: "off", record: recorded(),
			scripts:     map[string][]scriptedCall{"RequiresPullRequest": {{found: true}}, "Status": {open}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,push,mark",
			wantCalls: []string{"RequiresPullRequest main", "Status 7", wantGitHubMerge},
		},
		{
			name: "ATransientParentRuleReadIsAnErrorStopWithNoMerge", mode: "off", record: recorded(),
			scripts: map[string][]scriptedCall{"RequiresPullRequest": {{err: context.DeadlineExceeded}}},
			wantErr: true, wantErrText: "read GitHub's rules for the parent branch", wantCalls: []string{"RequiresPullRequest main"},
		},
		{
			name: "AnUnreadableParentRuleWithARecordedPullRequestHaltsClosesNothingAndKeepsTheRecord", mode: "off", record: recorded(),
			scripts:     map[string][]scriptedCall{"RequiresPullRequest": {{err: githubErrorResponse(http.StatusNotFound, "Not Found")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"read GitHub's rules for the parent branch", "Not Found", "lym loom resume"},
			wantCalls: []string{"RequiresPullRequest main"}, wantRecordKept: true,
		},
		{
			name: "AnUnresolvableTokenOnTheParentRuleReadReadsAsNoRuleAndLandsLocally", mode: "off",
			scripts:     map[string][]scriptedCall{"RequiresPullRequest": {{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: githubclient.ErrTokenUnresolvable}}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,push,mark", wantCalls: []string{"RequiresPullRequest main"},
		},
		{
			name: "YesWithNoRecordHaltsNamingGotoPRReview", mode: "on",
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"has no pull request recorded", "lym loom goto PR-Review"},
		},
		{
			name: "AMergeParentWithNoRecordOpensThePullRequestAndSquashMergesItWhateverTheSquashSetting", mode: "merge", squashOff: true, wantCommits: 1,
			scripts:     map[string][]scriptedCall{"Create": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}}}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,push,mark", wantRecordKept: true,
			wantSquashedMerges: true, wantRecord: openedRecord,
			wantCalls: []string{"FindOpen task-branch main", wantCreate, wantGitHubMerge},
		},
		{
			name: "AMergeParentFindsTheOpenPullRequestOnTheTaskBranchSetsItsTextAndMergesIt", mode: "merge", wantCommits: 1,
			scripts:     map[string][]scriptedCall{"FindOpen": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}, found: true}}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,push,mark", wantRecordKept: true, wantRecord: openedRecord,
			wantCalls: []string{"FindOpen task-branch main", "Edit 7|A landing title|A landing body.\n\n" + wantCredit, wantGitHubMerge},
		},
		{
			name: "AFailedOpenUnderAMergeParentHaltsNamingGitHubsMessageAndMergesNothing", mode: "merge",
			scripts:     map[string][]scriptedCall{"Create": {{err: githubErrorResponse(http.StatusUnprocessableEntity, "No commits between main and task-branch")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"open the pull request", "No commits between", "lym loom resume"},
			wantOrder: "task-push", wantCalls: []string{"FindOpen task-branch main", wantCreate},
		},
		{
			name: "AMethodRefusalUnderAMergeParentNamesTheRepositorySettingsAndNotTheSquashSetting", mode: "merge", record: recorded(), squashOff: true,
			scripts:     mergeScript(open, refusal(pullrequest.RefusalMethod, "Squash merges are not allowed")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Squash merges are not allowed", "allow squash merging", "lym loom resume"},
			wantNotInReason: []string{"lym config landing"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "YesWithAClosedRecordWithoutTheOperatorsEndingHaltsNamingReopenAndCancel", mode: "on", record: recorded(), scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen it on GitHub", "lym loom goto PR-Review", "lym batten cancel me"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AContinueFlagWithoutTheRecordedEndingHaltsAsWell", mode: "on", record: recorded(withContinue), scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"closed without a merge"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "TheOperatorsEndingOnAClosedPullRequestReopensItAndMergesOnGitHub", mode: "on", record: recorded(reviewEnded),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "reopen,task-push,fast-forward,merge,push,mark",
			wantCalls: []string{"Status 7", "Reopen 7", wantGitHubMerge},
		},
		{
			name: "AReopenedPullRequestUnderAMergeParentSquashMergesWhateverTheSquashSetting", mode: "merge", record: recorded(reviewEnded), squashOff: true,
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "task-push,reopen,fast-forward,merge,push,mark",
			wantCalls: []string{"Status 7", "Reopen 7", wantGitHubMerge},
		},
		{
			name: "AMergeParentReopensAClosedRecordedPullRequestWithNoOperatorEndingAfterTheTaskPush", mode: "merge", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,reopen,fast-forward,merge,push,mark",
			wantCalls: []string{"Status 7", "Reopen 7", wantGitHubMerge},
		},
		{
			name: "AnOwnerHaltUnderAMergeParentReopensNothingOnGitHub", mode: "merge", record: recorded(), scripts: statusScript(closed), parentOwnedElsewhere: true,
			wantOutcome: shedengine.Stuck, wantHalt: true, wantOwnerHalt: true, wantReason: []string{"owned by machine"},
			wantOrder: "", wantCalls: []string{"Status 7"},
		},
		{
			name: "AFailedReopenUnderAMergeParentHaltsNamingGitHubsMessageAndMergesNothing", mode: "merge", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Reopen": {{err: errors.New("state cannot be changed")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen the pull request", "state cannot be changed", "lym loom resume"},
			wantOrder: "task-push,reopen", wantCalls: []string{"Status 7", "Reopen 7"},
		},
		{
			name: "AProtectedParentPushRefusalUnderAMergeParentOmitsTheSettingAdvice", mode: "merge", record: recorded(), scripts: mergeScript(open, scriptedCall{}),
			parentPushErr: errors.New("gitrepo: push rejected: ! [remote rejected] main -> main (protected branch hook declined)"),
			wantOutcome:   shedengine.Stuck, wantMerges: 1, wantReason: []string{"protected branch hook declined", `"git reset --keep origin/main"`},
			wantNotInReason: []string{"lym config beam"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AMergeInModeLandsByATrueMergeWithNoPullRequestAndNoRuleRead", mode: "merge-in",
			wantOutcome: shedengine.Done, wantMerges: 1, wantTrueMerges: true, wantOrder: "merge,push,mark",
		},
		{
			name: "AProtectedParentPushRefusalOfAMergeInModeOmitsTheSettingAdvice", mode: "merge-in",
			parentPushErr: errors.New("gitrepo: push rejected: ! [remote rejected] main -> main (protected branch hook declined)"),
			wantOutcome:   shedengine.Stuck, wantMerges: 1, wantTrueMerges: true, wantReason: []string{"protected branch hook declined", `"git reset --keep origin/main"`},
			wantNotInReason: []string{"lym config beam"},
		},
		{
			name: "AFailedReopenHaltsNamingGitHubsMessageAndMergesNothing", mode: "on", record: recorded(reviewEnded),
			scripts:     map[string][]scriptedCall{"Status": {closed}, "Reopen": {{err: errors.New("state cannot be changed")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen the pull request", "state cannot be changed", "lym loom resume"},
			wantOrder: "reopen", wantCalls: []string{"Status 7", "Reopen 7"},
		},
		{
			name: "AMergedPullRequestUnderAMergeParentLandsThroughASquashingParentSideMerge", mode: "merge", record: recorded(), squashOff: true, scripts: statusScript(merged),
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "merge,push,mark", wantCalls: []string{"Status 7"},
		},
		{
			name: "AMergedPullRequestMakesNoGitHubMergeAndLandsThroughTheParentSideMerge", mode: "on", record: recorded(), scripts: statusScript(merged),
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,push,mark", wantCalls: []string{"Status 7"},
		},
		{
			name: "AnOpenOnePushesMergesOnGitHubWithThePushedHeadFastForwardsAndMarksTheTaskDone", mode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}),
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,push,mark", wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "TheMergeMethodFollowsTheSquashSetting", mode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), squashOff: true,
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,push,mark",
			wantCalls: []string{"Status 7", strings.Replace(wantGitHubMerge, "|true|", "|false|", 1)},
		},
		{
			name: "TheGitHubMergeMessageCarriesTheBodyThenTheRegressionCheckThenTheCredit", mode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), verdict: testVerdict,
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,push,mark", wantCalls: []string{"Status 7", wantGitHubMergeWithVerdict},
		},
		{
			name: "AMergeInRequiredErrorAfterTheGitHubMergeRetriesTheLocalMergeWithNoSecondGitHubMerge", mode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}),
			merges:      []mergeCallResult{{err: &fabricengine.ErrMergeInRequired{}}, {result: fabricengine.MergeResult{Committed: true}}},
			wantOutcome: shedengine.Done, wantMerges: 2, wantOrder: "task-push,fast-forward,merge,merge,push,mark", wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "APendingRefusalThenASuccessMergesOnceRetried", mode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalPending, "Base branch was modified"), scriptedCall{}),
			wantOutcome: shedengine.Done, wantMerges: 1, wantCalls: []string{"Status 7", wantGitHubMerge, wantGitHubMerge},
		},
		{
			name: "APendingRefusalForOverAMinuteHaltsNamingAnApprovalAndTheCancel", mode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalPending, "Base branch was modified")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Base branch was modified", "approves the pull request again", "lym batten cancel me", "lym loom resume"},
			wantNotInReason: []string{"locally", "lym loom goto PR-Review"},
			// Retries sleep 1, 2, 4, 8, 16 and 32 seconds, so the seventh refusal is the first past the minute.
			wantCalls: append([]string{"Status 7"}, slices.Repeat([]string{wantGitHubMerge}, 7)...),
		},
		{
			name: "AMethodRefusalHaltsNamingTheSquashSetting", mode: "on", record: recorded(),
			scripts:     mergeScript(open, refusal(pullrequest.RefusalMethod, "Merge commits are not allowed")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Merge commits are not allowed", "lym config landing --set squash=<true|false>", "lym loom resume"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AnotherRefusalHaltsNamingGitHubsMessageTheCauseGitHubReportsAndTheCancel", mode: "on", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {open, openAt("blocked")}, "Merge": {refusal(pullrequest.RefusalOther, "Required status check is expected")}},
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReason: []string{"Required status check is expected", "GitHub reports: blocked: a review or a required check is still owed", "lym batten cancel me"},
			wantCalls:  append([]string{"Status 7", wantGitHubMerge}, mergeStateRead...),
		},
		{
			name: "ANotMergeableRefusalWhileGitHubComputesMergeabilityWaitsItOutAndMerges", mode: "on", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {open, openAt("unknown"), openAt("clean")}, "Merge": {notMergeable, {}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,fast-forward,merge,push,mark",
			wantCalls: slices.Concat([]string{"Status 7", wantGitHubMerge, "Status 7"}, mergeStateRead, []string{wantGitHubMerge}),
		},
		{
			name: "AMergeabilityGitHubNeverComputesHaltsNamingItAfterTheWindow", mode: "on", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {open, openAt("unknown")}, "Merge": {notMergeable}},
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReason:      []string{"Pull Request is not mergeable", "GitHub reports: GitHub has not yet computed whether it can merge", "lym loom resume"},
			wantNotInReason: []string{"approves", "lym batten cancel"},
			// The reading sleeps 1, 2, 4, 8, 16 and then 29 seconds, the rest of the minute, so it reads the status seven times.
			wantCalls: slices.Concat([]string{"Status 7", wantGitHubMerge}, slices.Repeat([]string{"Status 7"}, 7), []string{"RequiredChecks main task-head"}),
		},
		{
			name: "AFailedMergeStateReadAfterANotMergeableRefusalHaltsAndMergesNoSecondTime", mode: "on", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {open, {err: errors.New("404 Not Found")}}, "Merge": {notMergeable}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"read whether the pull request can merge", "404 Not Found"},
			wantCalls: []string{"Status 7", wantGitHubMerge, "Status 7"},
		},
		{
			name: "ARefusalThatOutlastsTheWindowWhileGitHubReportsItCanMergeHaltsNamingOnlyTheResume", mode: "on", record: recorded(),
			scripts:     map[string][]scriptedCall{"Status": {open, openAt("clean")}, "Merge": {notMergeable}},
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReason:      []string{"Pull Request is not mergeable", "GitHub now reports it can merge", "lym loom resume"},
			wantNotInReason: []string{"lym batten cancel", "approves"},
			// Retries sleep 1, 2, 4, 8, 16 and 32 seconds, so the seventh refusal is the first past the minute.
			wantCalls: append([]string{"Status 7"}, slices.Repeat(append([]string{wantGitHubMerge}, mergeStateRead...), 7)...),
		},
		{
			name: "ATransientMergeFailureIsAnError", mode: "on", record: recorded(),
			scripts: mergeScript(open, scriptedCall{err: context.DeadlineExceeded}),
			wantErr: true, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AnUnresolvableTokenWrappedInAURLErrorHaltsNamingGhAuthLogin", mode: "on", record: recorded(),
			scripts:     statusScript(scriptedCall{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: githubclient.ErrTokenUnresolvable}}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"gh auth login --hostname github.com", "lym loom resume"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "ARejectedTokenWrappedInAURLErrorHaltsNamingTheTokenAndNotATransientError", mode: "on", record: recorded(),
			scripts:     statusScript(scriptedCall{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: fmt.Errorf("%w: the token from GH_TOKEN (401)", githubclient.ErrTokenRejected)}}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"GH_TOKEN, GITHUB_TOKEN", "lym loom resume"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AFailingTaskCommitReadIsAnErrorStopWithNoGitHubCallAndNoMerge", mode: "on", record: recorded(),
			taskErr: errors.New("rev-list failed"),
			wantErr: true, wantErrText: "read whether the task branch has commits beyond its parent",
		},
		{
			name: "AFailingStatusReadHaltsNamingGitHubsMessageAndNotTheToken", mode: "on", record: recorded(),
			scripts:     statusScript(scriptedCall{err: errors.New("404 Not Found")}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"404 Not Found", "fix what GitHub's message names"}, wantNotInReason: []string{"GH_TOKEN"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "AnEmptyTaskBranchUnderOnWithNoRecordLandsLocallyWithNoGitHubCall", mode: "on", taskEmpty: true,
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,push,mark",
		},
		{
			name: "AnEmptyTaskBranchClosesAnOpenRecordedPullRequestAndLandsLocally", mode: "on", taskEmpty: true, record: recorded(), scripts: statusScript(open), wantCommits: 1,
			wantOutcome: shedengine.Done, wantMerges: 1, wantNoRecord: true, wantOrder: "merge,push,mark",
			wantCalls: []string{"Status 7", "Close 7 lym closed this pull request: " + emptyBranchReason},
		},
		{
			name: "AFailingPushHaltsNamingFabricPushAndMergesNothing", mode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), pushErr: errors.New("rejected"),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"rejected", "lym fabric push", "lym loom resume"}, wantOrder: "task-push", wantCalls: []string{"Status 7"},
		},
		{
			name: "AFailingFastForwardAfterTheMergeHaltsNamingLoomResume", mode: "on", record: recorded(), scripts: mergeScript(open, scriptedCall{}), ffErr: errors.New("not fast-forward"),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"merged on GitHub", "not fast-forward", "lym loom resume"}, wantOrder: "task-push,fast-forward", wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "ABeamsRecordedOpenPullRequestIsSquashMergedOnGitHubWithNoEditAndTheRecordDropsAfterTheParentPush", mode: "off", squashOff: true,
			beam: true, beamPR: recordedBeam, scripts: mergeScript(open, scriptedCall{}),
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "task-push,fast-forward,merge,push,mark,drop-record",
			wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "ACrashReentryFindingTheBeamsPullRequestMergedAtTheTaskHeadLandsThroughTheAlreadyLandedPathAndDropsTheRecord", mode: "off",
			beam: true, beamPR: recordedBeam, scripts: statusScript(mergedAt("task-head")),
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "merge,push,mark,drop-record", wantCalls: []string{"Status 7"},
		},
		{
			name: "AMergeAtAnotherHeadLackingNoTaskCommitLandsThroughTheAlreadyLandedPathAndDropsTheRecord", mode: "off",
			beam: true, beamPR: recordedBeam, scripts: statusScript(mergedAt("older-head")),
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "commits-missing,merge,push,mark,drop-record", wantCalls: []string{"Status 7"},
		},
		{
			name: "AMergeAtAnotherHeadLackingATaskCommitUnderMergeOpensANewPullRequestMergesItAndDropsTheRecord", mode: "merge", wantCommits: 1,
			beam: true, beamPR: recordedBeam, commitsMissing: []string{"abc1234 a late commit"},
			scripts:     map[string][]scriptedCall{"Status": {mergedAt("older-head")}, "Create": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}}}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantRecordKept: true, wantRecord: openedRecord,
			wantOrder: "commits-missing,task-push,fast-forward,merge,push,mark,drop-record",
			wantCalls: []string{"Status 7", "FindOpen task-branch main", wantCreate, wantGitHubMerge},
		},
		{
			name: "AMergeAtAnotherHeadLackingATaskCommitUnderOnLandsLocallyAndDropsTheRecord", mode: "on",
			beam: true, beamPR: recordedBeam, commitsMissing: []string{"abc1234 a late commit"}, scripts: statusScript(mergedAt("older-head")),
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "commits-missing,merge,push,mark,drop-record",
			wantCalls: []string{"Status 7", "RequiresPullRequest main"},
		},
		{
			name: "ABeamsClosedPullRequestHaltsNamingTheOperatorReopeningItAndLoomResume", mode: "on",
			beam: true, beamPR: recordedBeam, scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopens it on GitHub", "lym loom resume"},
			wantNotInReason: []string{"lym batten cancel", "lym loom goto PR-Review"}, wantCalls: []string{"Status 7"},
		},
		{
			name: "ABeamsPendingRefusalForOverAMinuteHaltsNamingTheStatusVerbAndNotACancel", mode: "on",
			beam: true, beamPR: recordedBeam, scripts: mergeScript(open, refusal(pullrequest.RefusalPending, "Base branch was modified")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Base branch was modified", "lym beam pr status me", "lym loom resume"},
			wantNotInReason: []string{"lym batten cancel"},
			wantCalls:       append([]string{"Status 7"}, slices.Repeat([]string{wantGitHubMerge}, 7)...),
		},
		{
			name: "ABeamsUncomputedMergeabilityHaltsNamingTheWaitTheStatusVerbAndNotACancel", mode: "on",
			beam: true, beamPR: recordedBeam,
			scripts:     map[string][]scriptedCall{"Status": {open, openAt("unknown")}, "Merge": {notMergeable}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"GitHub has not yet computed", "wait until GitHub computes", "lym beam pr status me", "lym loom resume"},
			wantNotInReason: []string{"lym batten cancel", "approves"},
			wantCalls:       slices.Concat([]string{"Status 7", wantGitHubMerge}, slices.Repeat([]string{"Status 7"}, 7), []string{"RequiredChecks main task-head"}),
		},
		{
			name: "ABeamsOtherRefusalHaltsNamingTheStatusVerbAndNotACancel", mode: "on",
			beam: true, beamPR: recordedBeam,
			scripts:     map[string][]scriptedCall{"Status": {open, openAt("blocked")}, "Merge": {refusal(pullrequest.RefusalOther, "Required status check is expected")}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Required status check is expected", "GitHub reports: blocked", "lym beam pr status me", "lym loom resume"},
			wantNotInReason: []string{"lym batten cancel"}, wantCalls: append([]string{"Status 7", wantGitHubMerge}, mergeStateRead...),
		},
		{
			name: "ABeamsMethodRefusalNamesTheRepositorySettingsTheStatusVerbAndNotACancel", mode: "on",
			beam: true, beamPR: recordedBeam, scripts: mergeScript(open, refusal(pullrequest.RefusalMethod, "Squash merges are not allowed")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"Squash merges are not allowed", "allow squash merging", "lym beam pr status me", "lym loom resume"},
			wantNotInReason: []string{"lym batten cancel", "lym config landing"}, wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "AFailingBeamRecordReadIsAnErrorStopWithNoGitHubCall", mode: "on",
			beam: true, beamReadErr: errors.New("landing record unreadable"),
			wantErr: true, wantErrText: "read the beam landing record",
		},
		{
			name: "ABeamWithNoRecordedPullRequestAndAnOpenOneInTheRunsRecordMergesItWithNoEdit", mode: "off", record: recorded(),
			beam: true, scripts: mergeScript(open, scriptedCall{}),
			wantOutcome: shedengine.Done, wantMerges: 1, wantSquashedMerges: true, wantOrder: "task-push,fast-forward,merge,push,mark",
			wantCalls: []string{"Status 7", wantGitHubMerge},
		},
		{
			name: "ABeamWithNoRecordedPullRequestUnderOffDropsAClosedOneInTheRunsRecordAndLandsLocallyWithNoReopen", mode: "off", record: recorded(), wantCommits: 1,
			beam: true, scripts: statusScript(closed),
			wantOutcome: shedengine.Done, wantMerges: 1, wantNoRecord: true, wantOrder: "merge,push,mark",
			wantCalls: []string{"Status 7", "RequiresPullRequest main", "Status 7"},
		},
		{
			name: "ABeamWithNoRecordedPullRequestUnderMergeLeavesAClosedOneInTheRunsRecordToTheResolverWhichReopensAndMergesIt", mode: "merge", record: recorded(),
			beam: true, scripts: map[string][]scriptedCall{"Status": {closed}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "task-push,reopen,fast-forward,merge,push,mark",
			wantCalls: []string{"Status 7", "Status 7", "Reopen 7", wantGitHubMerge},
		},
		{
			name: "ABeamWithNoRecordedPullRequestUnderMergeLandsAMergedOneInTheRunsRecordThroughTheAlreadyLandedPath", mode: "merge", record: recorded(),
			beam: true, scripts: statusScript(merged),
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,push,mark",
			wantCalls: []string{"Status 7", "Status 7"},
		},
		{
			name: "ABeamWithNoRecordedPullRequestUnderMergeFindsTheOpenOneAndMergesItWithNoEdit", mode: "merge", wantCommits: 1,
			beam:        true,
			scripts:     map[string][]scriptedCall{"FindOpen": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}, found: true}}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantRecordKept: true, wantRecord: openedRecord, wantOrder: "task-push,fast-forward,merge,push,mark",
			wantCalls: []string{"FindOpen task-branch main", wantGitHubMerge},
		},
		{
			name: "ABeamWithNoRecordedPullRequestUnderMergeOpensOneFromTheDescriptionAndMergesIt", mode: "merge", wantCommits: 1,
			beam:        true,
			scripts:     map[string][]scriptedCall{"Create": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}}}, "Merge": {{}}},
			wantOutcome: shedengine.Done, wantMerges: 1, wantRecordKept: true, wantRecord: openedRecord, wantOrder: "task-push,fast-forward,merge,push,mark",
			wantCalls: []string{"FindOpen task-branch main", wantCreate, wantGitHubMerge},
		},
		{
			name: "ABeamWithNoRecordedPullRequestUnderOnLandsLocallyWithNoGitHubCall", mode: "on", origin: &Origin{URL: "/srv/hub.git", Upstream: true},
			beam:        true,
			wantOutcome: shedengine.Done, wantMerges: 1, wantOrder: "merge,push,mark",
		},
		{
			name: "AFailingCommitOfTheRecordRemovalIsAnErrorStopWithNoMerge", mode: "off", record: recorded(), scripts: statusScript(open),
			commitErr: errors.New("push stopped"),
			wantErr:   true, wantErrText: "commit the pull request record", wantCommits: 1,
			wantCalls: []string{"RequiresPullRequest main", "Status 7", "Close 7 lym closed this pull request: pull request off: beam.yaml's pull_request for this landing is off"},
		},
		{
			name: "AFailingCommitOfTheOpenedRecordIsAnErrorStopWithNoGitHubMerge", mode: "merge",
			scripts:   map[string][]scriptedCall{"Create": {{ref: pullrequest.Ref{Number: 7, URL: recordedURL}}}},
			commitErr: errors.New("push stopped"),
			wantErr:   true, wantErrText: "commit the pull request record", wantCommits: 1,
			wantCalls: []string{"FindOpen task-branch main", wantCreate},
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
			mode := tt.mode
			origin := githubOrigin
			if tt.origin != nil {
				origin = *tt.origin
			}
			lockHeldAtMode := false
			commits := &commitRecorder{err: tt.commitErr}
			deps.PullRequest = PullRequestDeps{
				RecordPath:     filepath.Join(dir, "pull-request.json"),
				RecordLockPath: filepath.Join(dir, "pull-request.lock"),
				CommitRecord:   commits.commit,
				Mode: func() (string, error) {
					rec, found, _ := deps.Lock.Store.Read("main")
					lockHeldAtMode = found && rec.Owner.Slug == "me"
					return mode, tt.modeErr
				},
				CommitsMissing: func(string, string) ([]string, error) {
					order = append(order, "commits-missing")
					return tt.commitsMissing, nil
				},
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
			if tt.beam {
				deps.PullRequest.BeamRecord = func() (BeamPullRequest, bool, error) {
					if tt.beamReadErr != nil || tt.beamPR == nil {
						return BeamPullRequest{}, false, tt.beamReadErr
					}
					return *tt.beamPR, true, nil
				}
				deps.PullRequest.DropBeamRecord = func() error {
					order = append(order, "drop-record")
					return nil
				}
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
			if commits.calls != tt.wantCommits {
				t.Errorf("record commits = %d; want %d", commits.calls, tt.wantCommits)
			}
			for i, call := range merger.calls {
				if tt.wantSquashedMerges && !call.opts.Squash {
					t.Errorf("parent-side merge %d Squash = false; want true", i)
				}
				if tt.wantTrueMerges && call.opts.Squash {
					t.Errorf("parent-side merge %d Squash = true; want false", i)
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

// TestWithSection asserts that a section follows the trimmed body after a blank line.
// A whitespace-only body yields the section alone,
// and an empty section leaves the body as it is.
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
