//go:build tier0

// prreview_wait_test.go covers the wait of the PR-Review row: what each state of the pull request and the task worktree ends it with, how it gates and pushes the agent's commits, and how it spaces its wakes and its GitHub polls on an injected clock.

package landingshed

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// waitWorld is the world a PR-Review wait runs in: scripted heads and dirty paths, a clock that moves only when the wait sleeps, and a record of every push, mail, mark and sleep.
// onSleep runs after each sleep with its 1-based count, so a test changes the world between two wakes.
type waitWorld struct {
	deps   Deps
	api    *fakePullRequestAPI
	tree   *fakeVerifyTree
	cancel context.CancelFunc

	now        time.Time
	sleeps     []time.Duration
	head       string
	dirty      []string
	pushes     []string
	mails      []string
	marks      []string
	missing    []string
	mailErr    error
	pauseCalls int
	// liveAtRead records whether the session was live at each read of the task worktree's uncommitted paths, for a test that installs a recording DirtyPaths.
	liveAtRead []bool
	// coveredAt answers whether a pause covers the run at the n-th (1-based) read of the pause record; nil covers nothing.
	coveredAt func(n int) bool
	onSleep   func(w *waitWorld, n int)
}

// newWaitWorld builds a world over scripts, with the task head "h1", a clean tree and no gate command.
func newWaitWorld(t *testing.T, scripts map[string][]scriptedCall) (*waitWorld, context.Context) {
	t.Helper()
	api := &fakePullRequestAPI{scripts: scripts}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w := &waitWorld{
		deps:   prReviewTestDeps(t, api),
		api:    api,
		tree:   &fakeVerifyTree{result: verifytree.Result{Status: verifytree.StatusPassed}},
		cancel: cancel,
		now:    prReviewFixtureNow,
		head:   "h1",
	}
	w.deps.TaskHead = func() (string, error) { return w.head, nil }
	w.deps.GateKey = "pr-review"
	w.deps.VerifyWaitMark = func(label string, _ time.Time) error {
		w.marks = append(w.marks, label)
		return nil
	}
	pr := &w.deps.PullRequest
	pr.DirtyPaths = func() ([]string, error) { return w.dirty, nil }
	pr.PushTask = func() error {
		w.pushes = append(w.pushes, w.head)
		return nil
	}
	pr.MailAgent = func(strand, line string) error {
		w.mails = append(w.mails, strand+"|"+line)
		return w.mailErr
	}
	pr.CommitsMissing = func(string, string) ([]string, error) { return w.missing, nil }
	pr.PauseCovered = func() (bool, error) {
		w.pauseCalls++
		return w.coveredAt != nil && w.coveredAt(w.pauseCalls), nil
	}
	pr.Now = func() time.Time { return w.now }
	pr.Sleep = func(ctx context.Context, d time.Duration) error {
		w.sleeps = append(w.sleeps, d)
		w.now = w.now.Add(d)
		if w.onSleep != nil {
			w.onSleep(w, len(w.sleeps))
		}
		return ctx.Err()
	}
	return w, ctx
}

// withGateCommand makes the row's gate run a command, which the world's fake verify tree answers.
func (w *waitWorld) withGateCommand() {
	w.deps.VerifyCommand = func(string) (string, error) { return "gate cmd", nil }
}

// cancelAfter cancels the wait's context at the n-th sleep, ending a wait that would otherwise keep waiting.
func (w *waitWorld) cancelAfter(n int) {
	w.onSleep = func(w *waitWorld, count int) {
		if count >= n {
			w.cancel()
		}
	}
}

// once wraps fn so that it runs at its first call only.
func once(fn func()) func() {
	done := false
	return func() {
		if !done {
			done = true
			fn()
		}
	}
}

func TestPRReview_Wait(t *testing.T) {
	// The wait captures the process-global logger, so this test and its subtests stay serial.
	const continueClose = "Close 7 " + continueComment
	const editSeven = "Edit 7|Change title|\nChange body\n"
	open := scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen}}
	closed := scriptedCall{status: pullrequest.Status{State: pullrequest.StateClosed}}
	mergedAt := func(head string) scriptedCall {
		return scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: head}}
	}
	approvedAt := func(head string, changesRequested bool) scriptedCall {
		return scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: head, Approved: true, ChangesRequested: changesRequested}}
	}
	statusScript := func(calls ...scriptedCall) map[string][]scriptedCall {
		return map[string][]scriptedCall{"Status": calls}
	}
	createdAndStatus := func(calls ...scriptedCall) map[string][]scriptedCall {
		return map[string][]scriptedCall{"Create": {{ref: pullrequest.Ref{Number: 7, URL: "u7"}}}, "Status": calls}
	}
	seed := func(gated string, mutate ...func(*pullrequest.Record)) *pullrequest.Record {
		rec := &pullrequest.Record{Number: 7, URL: "u7", GatedHead: gated}
		for _, m := range mutate {
			m(rec)
		}
		return rec
	}
	recorded := func(gated string, mutate ...func(*pullrequest.Record)) *pullrequest.Record {
		rec := seed(gated, mutate...)
		rec.Base, rec.Head = "main", "task"
		return rec
	}
	withContinue := func(r *pullrequest.Record) { r.Continue = true }
	withClosedForContinue := func(r *pullrequest.Record) { r.ClosedForContinue = true }
	withStrand := func(r *pullrequest.Record) { r.StrandName = "pr-strand" }
	writeContinue := func(w *waitWorld) {
		err := pullrequest.UpdateRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath, func(r *pullrequest.Record) error {
			r.Continue = true
			return nil
		})
		if err != nil {
			t.Errorf("write the continue flag: %v", err)
		}
	}
	failingGate := func(w *waitWorld) {
		w.withGateCommand()
		w.tree.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, Log: "/logs/gate.log"}
	}

	tests := []struct {
		name    string
		record  *pullrequest.Record
		scripts map[string][]scriptedCall
		head    string
		dirty   []string
		setup   func(w *waitWorld)

		wantOutcome shedengine.Outcome
		wantHalt    bool
		wantReason  []string
		wantErr     error
		wantCalls   []string
		wantRecord  *pullrequest.Record
		// gatePushes counts the pushes the wait makes after Call's own.
		gatePushes int
		check      func(t *testing.T, w *waitWorld, logs string)
	}{
		{
			name: "MergedAtTheGatedHeadOnACleanTreeIsDone", record: seed("h1"), scripts: statusScript(mergedAt("h1")),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7"}, wantRecord: seed("h1"),
		},
		{
			name: "MergedOnADirtyTreeHaltsNamingTheFiles", record: seed("h1"), scripts: statusScript(mergedAt("h1")), dirty: []string{"wip.go"},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"uncommitted changes: wip.go", "lym loom resume"},
		},
		{
			name: "MergedWithoutTaskCommitsHaltsNamingThemAndTheBoardWayForward", record: seed("h1"), scripts: statusScript(mergedAt("h1")),
			setup:       func(w *waitWorld) { w.missing = []string{"abc1234 late fix"} },
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReason: []string{"abc1234 late fix", "lym batten cancel the-slug", "lym board set-status the-slug done"},
		},
		{
			name: "MergedAtAnotherHeadHaltsNamingBothHeadsAndGotoFinalize", record: seed("h1"), scripts: statusScript(mergedAt("m9")),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"m9", "h1", "lym loom goto Finalize"},
		},
		{
			name: "MergedAtAnUngatedTaskHeadHaltsWithoutGatingOrPushingIt", record: seed("h1"), scripts: statusScript(mergedAt("h2")), head: "h2",
			setup:       func(w *waitWorld) { w.withGateCommand() },
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"h2", "h1", "lym loom goto Finalize"},
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 0 {
					t.Errorf("the gate ran %d times on a merged pull request; want none", w.tree.verifyCalls)
				}
			},
		},
		{
			name: "AHeadMovedAfterTheEntryPollIsNotGatedWhenTheForcedPollReadsItMerged", record: seed("h1"), scripts: statusScript(open, mergedAt("h2")),
			setup: func(w *waitWorld) {
				w.withGateCommand()
				w.onSleep = func(w *waitWorld, n int) {
					if n == 1 {
						w.head = "h2"
					}
				}
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"h2", "h1", "lym loom goto Finalize"},
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 0 {
					t.Errorf("the gate ran %d times on a merged pull request; want none", w.tree.verifyCalls)
				}
			},
		},
		{
			name: "ClosedWithoutAFlagHaltsNamingReopenAndCancel", record: seed("h1"), scripts: statusScript(closed),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen it on GitHub", "lym batten cancel the-slug"},
		},
		{
			name: "ClosedForContinueOnACleanTreeIsDone", record: seed("h1", withClosedForContinue), scripts: statusScript(closed),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7"}, wantRecord: seed("h1", withClosedForContinue),
		},
		{
			name: "ClosedForContinueOnADirtyTreeHaltsNamingTheFiles", record: seed("h1", withClosedForContinue), scripts: statusScript(closed), dirty: []string{"wip.go"},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"uncommitted changes: wip.go", "discard them", "lym loom resume"},
		},
		{
			name: "ContinueOnAClosedPullRequestAtTheGatedHeadIsDoneAndRecordsIt", record: seed("h1", withContinue), scripts: statusScript(closed),
			wantOutcome: shedengine.Done, wantRecord: seed("h1", withContinue, withClosedForContinue),
		},
		{
			name: "ContinueOnAClosedPullRequestWithAnUngatedCleanHeadGatesItFirst", record: seed("h1", withContinue), scripts: statusScript(closed), head: "h2",
			wantOutcome: shedengine.Done, wantRecord: &pullrequest.Record{Number: 7, URL: "u7", GatedHead: "h2", Continue: true, ClosedForContinue: true}, gatePushes: 1,
		},
		{
			name: "ContinueOnAClosedPullRequestOnADirtyTreeKeepsWaiting", record: seed("h1", withContinue), scripts: statusScript(closed), dirty: []string{"wip.go"},
			setup:   func(w *waitWorld) { w.cancelAfter(3) },
			wantErr: context.Canceled, wantCalls: []string{"Status 7"}, wantRecord: seed("h1", withContinue),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if len(w.sleeps) != 3 {
					t.Errorf("wait slept %d times; want 3 wakes before the cancel", len(w.sleeps))
				}
				assertSessionCounts(t, w.deps, 2, 1)
			},
		},
		{
			name: "ContinueOnAMergedPullRequestTakesTheMergedVerdictAndClosesNothing", record: seed("h1", withContinue), scripts: statusScript(mergedAt("h1")),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7"},
		},
		{
			name: "ContinueOnAnOpenPullRequestAtTheGatedHeadClosesItAndIsDone", record: seed("h1", withContinue), scripts: statusScript(open),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7", editSeven, continueClose},
			wantRecord: recorded("h1", withContinue, withClosedForContinue),
		},
		{
			name:   "ContinueOnAnOpenPullRequestAtAnUngatedCleanHeadKeepsWaitingWithoutClosing",
			record: seed("h1", withContinue, func(r *pullrequest.Record) { r.MailedHead = "h2" }), scripts: statusScript(open), head: "h2",
			setup:   func(w *waitWorld) { w.cancelAfter(2) },
			wantErr: context.Canceled, wantCalls: []string{"Status 7", editSeven},
		},
		{
			name: "ContinueOnADirtyTreeKeepsWaitingWithOneStatusCallInTheFirstMinute", record: seed("h1", withContinue), scripts: statusScript(open), dirty: []string{"wip.go"},
			setup:   func(w *waitWorld) { w.cancelAfter(5) },
			wantErr: context.Canceled, wantCalls: []string{"Status 7", editSeven},
			check: func(t *testing.T, w *waitWorld, _ string) {
				want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
				if diff := cmp.Diff(want, w.sleeps); diff != "" {
					t.Errorf("wake intervals mismatch (-want +got):\n%s", diff)
				}
				assertSessionCounts(t, w.deps, 2, 1)
			},
		},
		{
			name: "ApprovalAtTheGatedAndTaskHeadIsDone", record: seed("h1"), scripts: statusScript(approvedAt("h1", false)),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7", editSeven}, wantRecord: recorded("h1"),
		},
		{
			name: "ApprovalOnADirtyTreeStopsTheSessionBeforeTheReReadAndResumesItWhileWaiting", record: seed("h1"), scripts: statusScript(approvedAt("h1", false)), dirty: []string{"wip.go"},
			setup:   func(w *waitWorld) { w.cancelAfter(2) },
			wantErr: context.Canceled,
			check: func(t *testing.T, w *waitWorld, _ string) {
				assertSessionCounts(t, w.deps, 2, 1)
			},
		},
		{
			// No record, so the entry opens the pull request and starts the session a merged entry status would not.
			name: "MergedStopsTheSessionBeforeTheReReadOfTheTree", scripts: createdAndStatus(mergedAt("h1")),
			setup: func(w *waitWorld) {
				sessions := sessionsOf(w.deps)
				w.deps.PullRequest.DirtyPaths = func() ([]string, error) {
					live, _ := sessions.isLive("guid-1")
					w.liveAtRead = append(w.liveAtRead, live)
					return w.dirty, nil
				}
			},
			wantOutcome: shedengine.Done, wantCalls: []string{"FindOpen task main", "Create task main|Change title|\nChange body\n", "Status 7"},
			check: func(t *testing.T, w *waitWorld, _ string) {
				if diff := cmp.Diff([]bool{true, false}, w.liveAtRead); diff != "" {
					t.Errorf("session liveness at each tree read mismatch (-want +got):\n%s", diff)
				}
				assertSessionCounts(t, w.deps, 1, 1)
			},
		},
		{
			name: "ApprovalOfAnOlderHeadKeepsWaiting", record: seed("h1"), scripts: statusScript(approvedAt("h0", false)),
			setup: func(w *waitWorld) { w.cancelAfter(2) }, wantErr: context.Canceled,
		},
		{
			name: "ACommentAfterAChangeRequestKeepsWaiting", record: seed("h1"), scripts: statusScript(approvedAt("h1", true)),
			setup: func(w *waitWorld) { w.cancelAfter(2) }, wantErr: context.Canceled,
		},
		{
			name: "ADirtyTreeWaitsWithoutGating", record: seed("h1"), scripts: statusScript(open), head: "h2", dirty: []string{"wip.go"},
			setup:   func(w *waitWorld) { w.withGateCommand(); w.cancelAfter(2) },
			wantErr: context.Canceled, wantRecord: recorded("h1"),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 0 {
					t.Errorf("the gate ran %d times on a dirty tree; want none", w.tree.verifyCalls)
				}
			},
		},
		{
			name: "AMovedCleanHeadIsGatedPushedAndRecorded", record: seed("h1"), scripts: statusScript(open), head: "h2",
			setup:   func(w *waitWorld) { w.withGateCommand(); w.cancelAfter(2) },
			wantErr: context.Canceled, gatePushes: 1, wantRecord: recorded("h2"),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 1 {
					t.Errorf("the gate ran %d times; want once for the one moved head", w.tree.verifyCalls)
				}
			},
		},
		{
			name: "AFailingGateMailsOncePerHeadAndPushesNothing", record: seed("h1", withStrand), scripts: statusScript(open), head: "h2",
			setup:   func(w *waitWorld) { failingGate(w); w.cancelAfter(3) },
			wantErr: context.Canceled, wantRecord: recorded("h1", withStrand, func(r *pullrequest.Record) { r.MailedHead = "h2" }),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if len(w.mails) != 1 {
					t.Fatalf("mails = %q; want exactly one over two wakes of the failing head", w.mails)
				}
				for _, want := range []string{"pr-strand|", "tier 1", "/logs/gate.log", "new commit"} {
					if !strings.Contains(w.mails[0], want) {
						t.Errorf("mail %q lacks %q", w.mails[0], want)
					}
				}
				for _, banned := range []string{"merging parent branch", "resume"} {
					if strings.Contains(w.mails[0], banned) {
						t.Errorf("mail %q holds %q, which does not fit a mail to the agent", w.mails[0], banned)
					}
				}
			},
		},
		{
			name: "AFailedMailPostIsLoggedAndNotRetriedForThatHead", record: seed("h1", withStrand), scripts: statusScript(open), head: "h2",
			setup:   func(w *waitWorld) { failingGate(w); w.mailErr = errors.New("creel down"); w.cancelAfter(3) },
			wantErr: context.Canceled,
			check: func(t *testing.T, w *waitWorld, logs string) {
				if len(w.mails) != 1 {
					t.Errorf("mail posts = %d; want one, not retried for the same head", len(w.mails))
				}
				if !strings.Contains(logs, "could not tell the PR-Review session") {
					t.Errorf("log lacks the failed post's warning:\n%s", logs)
				}
			},
			wantRecord: recorded("h1", withStrand, func(r *pullrequest.Record) { r.MailedHead = "h2" }),
		},
		{
			name: "AFailingGateWithNoSessionRecordedLogsTheReason", record: seed("h1"), scripts: statusScript(open), head: "h2",
			// A session that reports no strand name leaves the record with none to mail.
			setup:   func(w *waitWorld) { failingGate(w); sessionsOf(w.deps).result.StrandName = ""; w.cancelAfter(2) },
			wantErr: context.Canceled,
			check: func(t *testing.T, w *waitWorld, logs string) {
				if len(w.mails) != 0 || !strings.Contains(logs, "no PR-Review session is recorded") {
					t.Errorf("mails = %q, log = %q; want no mail and a log naming the missing session", w.mails, logs)
				}
			},
		},
		{
			name: "AHeadMovedDuringTheGateDiscardsTheVerdict", record: seed("h1"), scripts: statusScript(open), head: "h2",
			setup: func(w *waitWorld) {
				w.withGateCommand()
				w.tree.onVerify = once(func() { w.head = "h3" })
				w.cancelAfter(1)
			},
			wantErr: context.Canceled, wantRecord: recorded("h1"),
		},
		{
			name: "ATreeDirtiedDuringTheGateDiscardsTheVerdict", record: seed("h1"), scripts: statusScript(open), head: "h2",
			setup: func(w *waitWorld) {
				w.withGateCommand()
				w.tree.onVerify = once(func() { w.dirty = []string{"wip.go"} })
				w.cancelAfter(1)
			},
			wantErr: context.Canceled, wantRecord: recorded("h1"),
		},
		{
			name: "ATransientPollErrorLogsOnceOverTwoFailingPollsAndKeepsWaiting", scripts: createdAndStatus(scriptedCall{err: context.DeadlineExceeded}),
			setup:   func(w *waitWorld) { w.cancelAfter(7) },
			wantErr: context.Canceled,
			check: func(t *testing.T, w *waitWorld, logs string) {
				if w.api.statusCalls != 2 {
					t.Errorf("Status calls = %d; want the two polls a minute apart", w.api.statusCalls)
				}
				if got := strings.Count(logs, "a GitHub call failed transiently"); got != 1 {
					t.Errorf("transient warnings = %d; want one for the whole episode", got)
				}
			},
		},
		{
			name:        "AnUnresolvableTokenWrappedInAURLErrorHaltsNamingGhAuthLogin",
			scripts:     createdAndStatus(scriptedCall{err: &url.Error{Op: "Get", URL: "https://api.github.com", Err: githubclient.ErrTokenUnresolvable}}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"gh auth login --hostname github.com", "lym loom resume"},
		},
		{
			name: "AForbiddenPollHaltsNamingTheTokenAndRepositoryAccess", scripts: createdAndStatus(scriptedCall{err: errors.New("403 Forbidden")}),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"403 Forbidden", "GH_TOKEN", "can reach the repository"},
		},
		{
			name: "TheWakeIntervalsDoubleToAMinuteAndFallBackToASecondWhenTheHeadMoves", record: seed("h1"), scripts: statusScript(open),
			setup: func(w *waitWorld) {
				w.onSleep = func(w *waitWorld, n int) {
					if n == 3 {
						w.head = "h2"
					}
					if n >= 10 {
						w.cancel()
					}
				}
			},
			wantErr: context.Canceled, gatePushes: 1,
			check: func(t *testing.T, w *waitWorld, _ string) {
				want := []time.Duration{1, 2, 4, 1, 2, 4, 8, 16, 32, 60}
				for i := range want {
					want[i] *= time.Second
				}
				if diff := cmp.Diff(want, w.sleeps); diff != "" {
					t.Errorf("wake intervals mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "ACoveringPauseAtAWakeSkipsTheGateAndThePollAndTheWaitGoesOnOnceUncovered", record: seed("h1"), scripts: statusScript(open), head: "h2",
			setup: func(w *waitWorld) {
				w.withGateCommand()
				// Read 1 is the entry's, read 2 and 3 are the first wake's.
				w.coveredAt = func(n int) bool { return n == 2 || n == 3 }
				w.onSleep = func(w *waitWorld, n int) {
					if n <= 2 && (w.tree.verifyCalls != 0 || w.api.statusCalls != 1) {
						t.Errorf("sleep %d under a pause saw %d gate runs and %d Status calls; want none beyond the entry's read", n, w.tree.verifyCalls, w.api.statusCalls)
					}
					if n >= 3 {
						w.cancel()
					}
				}
			},
			wantErr: context.Canceled, gatePushes: 1, wantRecord: recorded("h2"),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 1 {
					t.Errorf("the gate ran %d times; want once after the pause lifted", w.tree.verifyCalls)
				}
				// The pause stopped the session at its first covered read and the first uncovered one started it again.
				assertSessionCounts(t, w.deps, 2, 1)
			},
		},
		{
			name: "ACoveringPauseAtEntryPushesNothingAndCallsNoGitHubUntilUncovered", scripts: createdAndStatus(mergedAt("h1")),
			setup: func(w *waitWorld) {
				w.coveredAt = func(n int) bool { return n <= 2 }
				w.onSleep = func(w *waitWorld, n int) {
					if len(w.pushes) != 0 || len(w.api.calls) != 0 {
						t.Errorf("sleep %d under a pause saw %d pushes and GitHub calls %q; want none", n, len(w.pushes), w.api.calls)
					}
				}
			},
			wantOutcome: shedengine.Done, wantCalls: []string{"FindOpen task main", "Create task main|Change title|\nChange body\n", "Status 7"},
		},
		{
			name: "TheContinueFlagWrittenBetweenWakesPollsAtOnceAndClosesThePullRequest", record: seed("h1"), scripts: statusScript(open),
			setup: func(w *waitWorld) {
				w.onSleep = func(w *waitWorld, n int) {
					if n == 1 {
						writeContinue(w)
					}
				}
			},
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7", continueClose},
			wantRecord:  recorded("h1", withContinue, withClosedForContinue),
		},
		{
			name: "TheContinueFlagWrittenOnADirtyTreeKeepsWaitingWithOneExtraStatusCall", record: seed("h1"), scripts: statusScript(open), dirty: []string{"wip.go"},
			setup: func(w *waitWorld) {
				w.onSleep = func(w *waitWorld, n int) {
					if n == 1 {
						writeContinue(w)
					}
					if n >= 6 {
						w.cancel()
					}
				}
			},
			wantErr: context.Canceled, wantCalls: []string{"Status 7", editSeven, "Status 7"},
		},
		{
			name: "TheMarkIsSetAtTheStartAndAfterAGateAndClearedOnDone", record: seed("h1"), scripts: statusScript(approvedAt("h2", false)), head: "h2",
			setup:       func(w *waitWorld) { w.withGateCommand() },
			wantOutcome: shedengine.Done, gatePushes: 1,
			check: func(t *testing.T, w *waitWorld, _ string) {
				want := []string{"PR #7", "", "verify PR-Review", "", "PR #7", ""}
				if diff := cmp.Diff(want, w.marks); diff != "" {
					t.Errorf("wait marks mismatch (-want +got):\n%s", diff)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			w, ctx := newWaitWorld(t, tt.scripts)
			if tt.head != "" {
				w.head = tt.head
			}
			w.dirty = tt.dirty
			if tt.setup != nil {
				tt.setup(w)
			}
			if tt.record != nil {
				if err := pullrequest.WriteRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath, *tt.record); err != nil {
					t.Fatalf("write record: %v", err)
				}
			}
			row := NewPRReview("PR-Review", w.deps)
			row.gate.verify = w.tree.verify

			outcome, pointer, err := row.Call(ctx)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Call() error = %v; want one wrapping %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Call() error = %v; want none", err)
				}
				if outcome != tt.wantOutcome || pointer.Halt != tt.wantHalt {
					t.Errorf("Call() = %q, halt %v; want %q, halt %v (reason %q)", outcome, pointer.Halt, tt.wantOutcome, tt.wantHalt, pointer.Reason)
				}
				for _, want := range tt.wantReason {
					if !strings.Contains(pointer.Reason, want) {
						t.Errorf("Call() reason %q lacks %q", pointer.Reason, want)
					}
				}
			}
			if tt.wantCalls != nil {
				if diff := cmp.Diff(tt.wantCalls, w.api.calls); diff != "" {
					t.Errorf("GitHub calls mismatch (-want +got):\n%s", diff)
				}
			}
			if len(w.pushes) != 1+tt.gatePushes {
				t.Errorf("pushed %d times (%q); want %d", len(w.pushes), w.pushes, 1+tt.gatePushes)
			}
			if tt.wantRecord != nil {
				got, found, rerr := pullrequest.ReadRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath)
				if rerr != nil || !found {
					t.Fatalf("ReadRecord() = %+v, %v, %v; want the record", got, found, rerr)
				}
				if diff := cmp.Diff(*tt.wantRecord, withSessionOf(got, *tt.wantRecord)); diff != "" {
					t.Errorf("record mismatch (-want +got):\n%s", diff)
				}
			}
			if tt.check != nil {
				tt.check(t, w, logs.String())
			}

			for _, d := range w.sleeps {
				if d < time.Second || d > time.Minute {
					t.Errorf("slept %v; want every wait between a second and a minute", d)
				}
			}
			if len(w.marks) < 2 || w.marks[0] != "PR #7" || w.marks[len(w.marks)-1] != "" {
				t.Errorf("wait marks = %q; want the mark set at the start and cleared on the way out", w.marks)
			}
			held, lerr := w.deps.Lock.Store.List()
			if lerr != nil || len(held) != 0 {
				t.Errorf("the parent lock store holds %v (%v); want no record, since the wait takes no parent lock", held, lerr)
			}
		})
	}
}
