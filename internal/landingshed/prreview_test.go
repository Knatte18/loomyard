//go:build tier0

// prreview_test.go covers the PR-Review row against a fake pull-request API and a temp record: the mode decision, the push, finding or opening the pull request, the record it keeps and the failures it halts or returns on.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// scriptedCall is one scripted answer of a fakePullRequestAPI method.
type scriptedCall struct {
	ref    pullrequest.Ref
	found  bool
	status pullrequest.Status
	err    error
}

// fakePullRequestAPI is a pullrequest.API that records every call with its arguments and answers each method from a list of scripted results, one per call, the last repeating.
// A method with no script answers the zero result.
type fakePullRequestAPI struct {
	mu      sync.Mutex
	calls   []string
	scripts map[string][]scriptedCall
	used    map[string]int
	// statusCalls counts the Status calls, which the once-a-minute poll bound is read from.
	statusCalls int
}

var _ pullrequest.API = (*fakePullRequestAPI)(nil)

func (f *fakePullRequestAPI) next(method, record string) scriptedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method+" "+record)
	if method == "Status" {
		f.statusCalls++
	}
	script := f.scripts[method]
	if len(script) == 0 {
		return scriptedCall{}
	}
	if f.used == nil {
		f.used = map[string]int{}
	}
	i := min(f.used[method], len(script)-1)
	f.used[method]++
	return script[i]
}

func (f *fakePullRequestAPI) FindOpen(_ context.Context, head, base string) (pullrequest.Ref, bool, error) {
	s := f.next("FindOpen", head+" "+base)
	return s.ref, s.found, s.err
}

func (f *fakePullRequestAPI) Create(_ context.Context, head, base, title, body string) (pullrequest.Ref, error) {
	s := f.next("Create", fmt.Sprintf("%s %s|%s|%s", head, base, title, body))
	return s.ref, s.err
}

func (f *fakePullRequestAPI) Edit(_ context.Context, number int, title, body string) error {
	return f.next("Edit", fmt.Sprintf("%d|%s|%s", number, title, body)).err
}

func (f *fakePullRequestAPI) Status(_ context.Context, number int) (pullrequest.Status, error) {
	s := f.next("Status", fmt.Sprint(number))
	return s.status, s.err
}

func (f *fakePullRequestAPI) Close(_ context.Context, number int, comment string) error {
	return f.next("Close", fmt.Sprintf("%d %s", number, comment)).err
}

func (f *fakePullRequestAPI) Merge(_ context.Context, number int, expectedHead string, squash bool, title, message string) error {
	return f.next("Merge", fmt.Sprintf("%d|%s|%v|%s|%s", number, expectedHead, squash, title, message)).err
}

func TestPRReview_Call(t *testing.T) {
	t.Parallel()

	githubOrigin := Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}
	localOrigin := Origin{}
	open := scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen}}
	created := scriptedCall{ref: pullrequest.Ref{Number: 7, URL: "https://github.com/acme/widgets/pull/7"}}
	closed := scriptedCall{status: pullrequest.Status{State: pullrequest.StateClosed}}
	const (
		wantCreateCall  = "Create task main|Change title|\nChange body\n"
		wantEditSeven   = "Edit 7|Change title|\nChange body\n"
		localNoReason   = "no pull request: the parent's beam.yaml pull_request is on and the task pair has no origin remote"
		taskHead        = "head-new"
		recordedGated   = "head-old"
		recordedURLSeen = "https://github.com/acme/widgets/pull/7"
	)
	// mergedAtHead ends the wait of a row whose task head is the gated one: the pull request is merged at that head.
	mergedAtHead := scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: taskHead}}

	tests := []struct {
		name       string
		parentMode string
		parentErr  error
		field      string
		origin     Origin
		record     *pullrequest.Record
		dirty      []string
		scripts    map[string][]scriptedCall
		pushErr    error
		// taskEmpty makes the task branch hold no commit beyond its parent.
		taskEmpty bool
		// taskErr makes the TaskHasCommits seam fail.
		taskErr error

		wantOutcome shedengine.Outcome
		wantHalt    bool
		wantReason  []string
		// wantNotInReason are substrings the reason must not hold.
		wantNotInReason []string
		wantErr         bool
		// wantErrText is a substring the returned error must hold.
		wantErrText string
		wantCalls   []string
		wantPushes  int
		wantRecord  *pullrequest.Record
		// wantNoRecord asserts the record's file is gone; a case with neither it nor wantRecord leaves the record unchecked.
		wantNoRecord bool
	}{
		{
			name: "OffWithNoFieldIsDoneAndCallsNoGitHub", parentMode: "off", origin: githubOrigin,
			wantOutcome: shedengine.Done,
		},
		{
			name: "NoClosesTheRecordedOpenPullRequestAndRemovesTheRecord", parentMode: "on", origin: localOrigin,
			record:      &pullrequest.Record{Number: 7},
			scripts:     map[string][]scriptedCall{"Status": {open}},
			wantOutcome: shedengine.Done, wantNoRecord: true,
			wantCalls: []string{"Status 7", "Close 7 lym closed this pull request: " + localNoReason},
		},
		{
			name: "NoLeavesAMergedPullRequestAndRemovesTheRecord", parentMode: "off", origin: githubOrigin,
			record:      &pullrequest.Record{Number: 7},
			scripts:     map[string][]scriptedCall{"Status": {{status: pullrequest.Status{State: pullrequest.StateMerged}}}},
			wantOutcome: shedengine.Done, wantNoRecord: true,
			wantCalls: []string{"Status 7"},
		},
		{
			name: "UnreadableParentModeHaltsNamingTheWorktree", parentErr: errors.New("bad yaml"), origin: githubOrigin,
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReason: []string{"worktree wt-parent", "bad yaml", "lym config beam --set pull_request=", "lym loom resume"},
		},
		{
			name: "FieldOnWithALocalOriginHaltsWithBothWaysForward", parentMode: "off", field: "on", origin: localOrigin,
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReason: []string{"lym loom resume", "lym batten cancel the-slug"},
		},
		{
			name: "FirstYesPushesCreatesAndRecordsThePullRequest", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			wantOutcome: shedengine.Done, wantPushes: 1,
			wantCalls:  []string{"FindOpen task main", wantCreateCall, "Status 7"},
			wantRecord: &pullrequest.Record{Number: 7, URL: recordedURLSeen, Base: "main", Head: "task", GatedHead: taskHead},
		},
		{
			name: "ReentryEditsTheRecordedPullRequestKeepsItsGatedHeadAndTheContinueFlag", parentMode: "on", origin: githubOrigin,
			record:      &pullrequest.Record{Number: 7, URL: recordedURLSeen, GatedHead: recordedGated, Continue: true},
			dirty:       []string{"wip.go"},
			scripts:     map[string][]scriptedCall{"Status": {open, {status: pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: recordedGated}}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantReason: []string{"uncommitted changes: wip.go"},
			wantCalls:  []string{"Status 7", wantEditSeven, "Status 7"},
			wantRecord: &pullrequest.Record{Number: 7, URL: recordedURLSeen, Base: "main", Head: "task", GatedHead: recordedGated, Continue: true},
		},
		{
			name: "YesWithNoneRecordedButOneOpenReusesIt", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"FindOpen": {{ref: pullrequest.Ref{Number: 9, URL: "u9"}, found: true}}, "Status": {mergedAtHead}},
			wantOutcome: shedengine.Done, wantPushes: 1,
			wantCalls:  []string{"FindOpen task main", "Edit 9|Change title|\nChange body\n", "Status 9"},
			wantRecord: &pullrequest.Record{Number: 9, URL: "u9", Base: "main", Head: "task", GatedHead: taskHead},
		},
		{
			name: "ClosedForContinueAtTheGatedHeadKeepsTheRecordAndWaitsOnlyToJudgeIt", parentMode: "on", origin: githubOrigin,
			record:      &pullrequest.Record{Number: 7, ClosedForContinue: true, GatedHead: taskHead},
			scripts:     map[string][]scriptedCall{"Status": {closed}},
			wantOutcome: shedengine.Done, wantPushes: 1,
			wantCalls:  []string{"Status 7"},
			wantRecord: &pullrequest.Record{Number: 7, ClosedForContinue: true, GatedHead: taskHead},
		},
		{
			name: "ClosedForContinuePastTheGatedHeadOpensANewPullRequest", parentMode: "on", origin: githubOrigin,
			record:      &pullrequest.Record{Number: 7, ClosedForContinue: true, GatedHead: recordedGated},
			scripts:     map[string][]scriptedCall{"Create": {{ref: pullrequest.Ref{Number: 8, URL: "u8"}}}, "Status": {mergedAtHead}},
			wantOutcome: shedengine.Done, wantPushes: 1,
			wantCalls:  []string{"FindOpen task main", wantCreateCall, "Status 8"},
			wantRecord: &pullrequest.Record{Number: 8, URL: "u8", Base: "main", Head: "task", GatedHead: taskHead},
		},
		{
			name: "NonTransientPushFailureHaltsNamingFabricPush", parentMode: "on", origin: githubOrigin,
			pushErr:     errors.New("rejected"),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantReason: []string{"rejected", "lym fabric push", "lym loom resume"},
		},
		{
			name: "TransientPushFailureIsAnError", parentMode: "on", origin: githubOrigin,
			pushErr: context.DeadlineExceeded,
			wantErr: true, wantPushes: 1,
		},
		{
			name: "UnresolvableTokenWrappedInAURLErrorHaltsNamingGhAuthLogin", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"FindOpen": {{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: githubclient.ErrTokenUnresolvable}}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantCalls:  []string{"FindOpen task main"},
			wantReason: []string{"gh auth login --hostname github.com", "lym loom resume"},
		},
		{
			name: "EmptyTaskBranchUnderOnIsDoneWithNoPushAndNoGitHubCall", parentMode: "on", origin: githubOrigin, taskEmpty: true,
			wantOutcome: shedengine.Done, wantNoRecord: true,
		},
		{
			name: "EmptyTaskBranchClosesTheRecordedOpenPullRequestAndRemovesTheRecord", parentMode: "on", origin: githubOrigin, taskEmpty: true,
			record:      &pullrequest.Record{Number: 7},
			scripts:     map[string][]scriptedCall{"Status": {open}},
			wantOutcome: shedengine.Done, wantNoRecord: true,
			wantCalls: []string{"Status 7", "Close 7 lym closed this pull request: " + emptyBranchReason},
		},
		{
			name: "FailingTaskCommitReadIsAnErrorNamingItWithNoPushAndNoGitHubCall", parentMode: "on", origin: githubOrigin,
			taskErr: errors.New("rev-list failed"),
			wantErr: true, wantErrText: "read whether the task branch has commits beyond its parent",
		},
		{
			name: "RejectedTokenWrappedInAURLErrorHaltsNamingTheToken", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"FindOpen": {{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: fmt.Errorf("%w: the token from GH_TOKEN (401)", githubclient.ErrTokenRejected)}}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantCalls:  []string{"FindOpen task main"},
			wantReason: []string{"GH_TOKEN, GITHUB_TOKEN", "lym loom resume"},
		},
		{
			name: "UnauthorizedResponseHaltsNamingTheToken", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"FindOpen": {{err: githubErrorResponse(http.StatusUnauthorized, "Bad credentials")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantCalls:  []string{"FindOpen task main"},
			wantReason: []string{"Bad credentials", "GH_TOKEN", "can reach the repository", "lym loom resume"},
		},
		{
			name: "ForbiddenResponseHaltsNamingTheToken", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"FindOpen": {{err: githubErrorResponse(http.StatusForbidden, "Resource not accessible")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantCalls:  []string{"FindOpen task main"},
			wantReason: []string{"Resource not accessible", "GH_TOKEN", "lym loom resume"},
		},
		{
			name: "UnprocessableResponseHaltsSayingWhatA422MeansAndNotNamingTheToken", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"Create": {{err: githubErrorResponse(http.StatusUnprocessableEntity, "No commits between main and task")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantCalls:       []string{"FindOpen task main", wantCreateCall},
			wantReason:      []string{"No commits between main and task", "(422)", "accepted the token but not the request", "lym loom resume"},
			wantNotInReason: []string{"GH_TOKEN"},
		},
		{
			name: "NotFoundHaltsNamingGitHubsMessageAndNotTheToken", parentMode: "on", origin: githubOrigin,
			scripts:     map[string][]scriptedCall{"FindOpen": {{err: githubErrorResponse(http.StatusNotFound, "Not Found")}}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantPushes: 1,
			wantCalls:       []string{"FindOpen task main"},
			wantReason:      []string{"Not Found", "fix what GitHub's message names", "lym loom resume"},
			wantNotInReason: []string{"GH_TOKEN"},
		},
		{
			name: "TransientGitHubErrorIsAnError", parentMode: "on", origin: githubOrigin,
			scripts: map[string][]scriptedCall{"FindOpen": {{err: context.DeadlineExceeded}}},
			wantErr: true, wantPushes: 1,
			wantCalls: []string{"FindOpen task main"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newPRReviewFixture(t, tt.parentMode, tt.parentErr, tt.field, tt.origin, tt.record, tt.scripts, tt.pushErr)
			fx.dirty = tt.dirty
			fx.deps.PullRequest.TaskHasCommits = func() (bool, error) { return !tt.taskEmpty, tt.taskErr }

			outcome, pointer, err := NewPRReview("PR-Review", fx.deps).Call(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("Call() error = %v; want error %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("Call() error = %q; want it to hold %q", err, tt.wantErrText)
			}
			if err == nil {
				if outcome != tt.wantOutcome || pointer.Halt != tt.wantHalt {
					t.Errorf("Call() = %q, halt %v; want %q, halt %v (reason %q)", outcome, pointer.Halt, tt.wantOutcome, tt.wantHalt, pointer.Reason)
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
			if diff := cmp.Diff(tt.wantCalls, fx.api.calls); diff != "" {
				t.Errorf("GitHub calls mismatch (-want +got):\n%s", diff)
			}
			if fx.pushes != tt.wantPushes {
				t.Errorf("Call() pushed %d times; want %d", fx.pushes, tt.wantPushes)
			}
			got, found, rerr := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath)
			if rerr != nil {
				t.Fatalf("ReadRecord() error = %v", rerr)
			}
			if tt.wantRecord == nil {
				if tt.wantNoRecord && found {
					t.Errorf("Call() left the record %+v; want none", got)
				}
				return
			}
			if !found {
				t.Fatalf("Call() left no record; want %+v", *tt.wantRecord)
			}
			got = withSessionOf(got, *tt.wantRecord)
			if diff := cmp.Diff(*tt.wantRecord, got); diff != "" {
				t.Errorf("record mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("NilSeamIsAnErrorNamingIt", func(t *testing.T) {
		t.Parallel()
		unsetters := map[string]func(d *Deps){
			"PullRequest.RecordPath":      func(d *Deps) { d.PullRequest.RecordPath = "" },
			"PullRequest.RecordLockPath":  func(d *Deps) { d.PullRequest.RecordLockPath = "" },
			"PullRequest.ParentMode":      func(d *Deps) { d.PullRequest.ParentMode = nil },
			"PullRequest.EntryField":      func(d *Deps) { d.PullRequest.EntryField = nil },
			"PullRequest.Origin":          func(d *Deps) { d.PullRequest.Origin = nil },
			"PullRequest.API":             func(d *Deps) { d.PullRequest.API = nil },
			"PullRequest.PushTask":        func(d *Deps) { d.PullRequest.PushTask = nil },
			"PullRequest.CommitsMissing":  func(d *Deps) { d.PullRequest.CommitsMissing = nil },
			"PullRequest.TaskHasCommits":  func(d *Deps) { d.PullRequest.TaskHasCommits = nil },
			"PullRequest.DirtyPaths":      func(d *Deps) { d.PullRequest.DirtyPaths = nil },
			"PullRequest.MailAgent":       func(d *Deps) { d.PullRequest.MailAgent = nil },
			"PullRequest.PauseCovered":    func(d *Deps) { d.PullRequest.PauseCovered = nil },
			"PullRequest.Now":             func(d *Deps) { d.PullRequest.Now = nil },
			"PullRequest.Sleep":           func(d *Deps) { d.PullRequest.Sleep = nil },
			"PullRequest.Sessions":        func(d *Deps) { d.PullRequest.Sessions = nil },
			"PullRequest.SessionLive":     func(d *Deps) { d.PullRequest.SessionLive = nil },
			"PullRequest.RegisterSession": func(d *Deps) { d.PullRequest.RegisterSession = nil },
			"TaskHead":                    func(d *Deps) { d.TaskHead = nil },
		}
		for field, unset := range unsetters {
			fx := newPRReviewFixture(t, "on", nil, "", githubOrigin, nil, nil, nil)
			unset(&fx.deps)

			_, _, err := NewPRReview("PR-Review", fx.deps).Call(context.Background())

			if err == nil || !strings.Contains(err.Error(), "Deps."+field) {
				t.Errorf("Call() with %s unset = %v; want an error naming Deps.%s", field, err, field)
			}
		}
	})

	t.Run("CancelledContextIsAnError", func(t *testing.T) {
		t.Parallel()
		fx := newPRReviewFixture(t, "on", nil, "", githubOrigin, nil, nil, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, _, err := NewPRReview("PR-Review", fx.deps).Call(ctx)

		if !errors.Is(err, context.Canceled) || outcome != "" {
			t.Errorf("Call() = %q, %v; want an empty outcome and an error wrapping context.Canceled", outcome, err)
		}
	})
}

// emptyBranchReason is the resolver's reason for an on parent whose task branch holds no commit beyond it.
const emptyBranchReason = "no pull request: the parent's beam.yaml pull_request is on, but the task branch has no commits beyond its parent, so the landing is empty"

// githubErrorResponse is the error go-github returns for a response GitHub answered with status and message.
func githubErrorResponse(status int, message string) *github.ErrorResponse {
	return &github.ErrorResponse{Response: &http.Response{StatusCode: status}, Message: message}
}

// prReviewFixture is the Deps a PR-Review test runs under, with the fake API, the push count and the dirty paths it reads back.
type prReviewFixture struct {
	deps   Deps
	api    *fakePullRequestAPI
	pushes int
	// dirty is what the task worktree's uncommitted-paths seam reports.
	dirty []string
}

// prReviewFixtureNow is the instant every fake clock of the PR-Review tests starts at.
var prReviewFixtureNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

// prReviewTestDeps builds a Deps over a temp record path and a change description titled "Change title" whose body, as summaryparser reads it, is "\nChange body\n".
// Every PullRequest seam is filled: the origin is a GitHub one, the parent's mode is on, the task head is "head-new", the worktree is clean, no pause covers the run, and the clock moves only when the wait sleeps.
// A test replaces the seams it scripts.
func prReviewTestDeps(t *testing.T, api *fakePullRequestAPI) Deps {
	t.Helper()
	dir := t.TempDir()
	description := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(description, []byte("# Change title\n\nChange body\n"), 0o644); err != nil {
		t.Fatalf("write description: %v", err)
	}
	now := prReviewFixtureNow
	sessions := newFakePRSessions()
	return Deps{
		Slug:            "the-slug",
		WorktreeRoot:    "/wt/task",
		TaskBranch:      "task",
		ParentBranch:    "main",
		DescriptionPath: description,
		StencilsDir:     stencilkit.Seed(t),
		ParentName:      "parent-session",
		Config:          Config{PRReviewer: "claude:fixture-model"},
		TaskHead:        func() (string, error) { return "head-new", nil },
		GateTiers:       func(string) ([]int, error) { return []int{0, 1}, nil },
		Lock:            ParentLockDeps{Store: &parentlock.Store{Dir: filepath.Join(dir, "parent-lock")}},
		PullRequest: PullRequestDeps{
			Sessions:        sessions,
			SessionLive:     sessions.isLive,
			RegisterSession: sessions.register,
			RecordPath:      filepath.Join(dir, "pull-request.json"),
			RecordLockPath:  filepath.Join(dir, "pull-request.lock"),
			ParentMode:      func() (string, string, error) { return "on", "wt-parent", nil },
			EntryField:      func() (string, error) { return "", nil },
			Origin:          func() (Origin, error) { return Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}, nil },
			API:             func(string) (pullrequest.API, error) { return api, nil },
			PushTask:        func() error { return nil },
			CommitsMissing:  func(string, string) ([]string, error) { return nil, nil },
			TaskHasCommits:  func() (bool, error) { return true, nil },
			DirtyPaths:      func() ([]string, error) { return nil, nil },
			MailAgent:       func(string, string) error { return nil },
			PauseCovered:    func() (bool, error) { return false, nil },
			Now:             func() time.Time { return now },
			Sleep: func(_ context.Context, d time.Duration) error {
				now = now.Add(d)
				return nil
			},
		},
	}
}

// newPRReviewFixture builds a prReviewTestDeps Deps with the given mode, field, origin and push result.
// record, when set, is written before the row runs; the task head reads "head-new".
func newPRReviewFixture(t *testing.T, parentMode string, parentErr error, field string, origin Origin, record *pullrequest.Record, scripts map[string][]scriptedCall, pushErr error) *prReviewFixture {
	t.Helper()
	fx := &prReviewFixture{api: &fakePullRequestAPI{scripts: scripts}}
	fx.deps = prReviewTestDeps(t, fx.api)
	pr := &fx.deps.PullRequest
	pr.ParentMode = func() (string, string, error) { return parentMode, "wt-parent", parentErr }
	pr.EntryField = func() (string, error) { return field, nil }
	pr.Origin = func() (Origin, error) { return origin, nil }
	pr.DirtyPaths = func() ([]string, error) { return fx.dirty, nil }
	pr.PushTask = func() error {
		fx.pushes++
		return pushErr
	}
	if record != nil {
		if err := pullrequest.WriteRecord(pr.RecordPath, pr.RecordLockPath, *record); err != nil {
			t.Fatalf("write record: %v", err)
		}
	}
	return fx
}
