// prreview_wait_test.go covers the wait of the PR-Review row.
// Each state of the pull request and the task worktree ends the wait with a known outcome.
// The wait gates and pushes the commits made in the task worktree.
// It spaces its wakes and GitHub polls on an injected clock.

package landingshed

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// waitWorld is the world a PR-Review wait runs in.
// It scripts the heads and dirty paths, and it runs a clock that moves only when the wait sleeps.
// It also records every push, mark and sleep.
// onSleep runs after each sleep with its 1-based count, so a test changes the world between two wakes.
type waitWorld struct {
	deps   Deps
	api    *fakePullRequestAPI
	tree   *fakeVerifyTree
	cancel context.CancelFunc

	now    time.Time
	sleeps []time.Duration
	head   string
	dirty  []string
	pushes []string
	marks  []string
	// sessions is the session world; a row whose record names firstGUID finds that strand started and live.
	sessions *prSessionWorld
	// missing is what the CommitsMissing seam answers, and missingErr is the error it fails with.
	// missingCalls records each call as "<merged>|<task>".
	missing      []string
	missingErr   error
	missingCalls []string
	pauseCalls   int
	// coveredAt answers whether a pause covers the run at the n-th (1-based) read of the pause record; nil covers nothing.
	coveredAt func(n int) bool
	onSleep   func(w *waitWorld, n int)
	// changes is the channel the record watch delivers on, or nil for a wait left on its timer.
	// watchStops counts the ends of the watch.
	changes    chan struct{}
	watchStops int
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
	w.sessions = newPRSessionWorld(t, recordingStartStop)
	w.sessions.fill(&w.deps)
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
	pr.CommitsMissing = func(merged, task string) ([]string, error) {
		w.missingCalls = append(w.missingCalls, merged+"|"+task)
		return w.missing, w.missingErr
	}
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
	pr.RecordChanges = func() (<-chan struct{}, func()) {
		return w.changes, func() { w.watchStops++ }
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
	tiergate.Require(t, 0)
	// The wait captures the process-global logger, so this test and its subtests stay serial.
	const editSeven = "Edit 7|Change title|" + wantPRBody
	const editNew = "Edit 7|New title|New body\n\nCreated using\n - LoomYard\n - Claude <c@x>"
	const newDescription = "# New title\n\nNew body\n"
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
	withStrand := func(r *pullrequest.Record) {
		r.StrandGUID, r.StrandName, r.SessionID = firstGUID, "pr-strand", "sess-1"
	}
	withContinue := func(r *pullrequest.Record) { r.Continue = true }
	withReviewEnded := func(r *pullrequest.Record) { r.ReviewEnded = true }
	writeContinue := func(w *waitWorld) {
		err := pullrequest.UpdateRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath, func(r *pullrequest.Record) error {
			r.Continue = true
			return nil
		})
		if err != nil {
			t.Errorf("write the continue flag: %v", err)
		}
	}
	// changeDuringFirstSleep returns a setup for a wait whose first sleep advances the clock by elapsed.
	// That sleep writes the continue flag and signals a record change, then ends only when the wait cancels it.
	// A guard fails the test if the sleep never ends.
	changeDuringFirstSleep := func(elapsed time.Duration) func(w *waitWorld) {
		return func(w *waitWorld) {
			w.changes = make(chan struct{}, 1)
			sleep := w.deps.PullRequest.Sleep
			w.deps.PullRequest.Sleep = func(ctx context.Context, d time.Duration) error {
				if len(w.sleeps) > 0 {
					return sleep(ctx, d)
				}
				w.sleeps = append(w.sleeps, d)
				w.now = w.now.Add(elapsed)
				writeContinue(w)
				w.changes <- struct{}{}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Second):
					t.Errorf("the record change did not end the sleep")
					w.cancel()
					return context.Canceled
				}
			}
		}
	}
	// rewriteDescriptionsThenCancel returns a setup that writes each content of writes to the change description.
	// Each content is keyed by the sleep count it is written at, and the setup cancels the wait at the n-th sleep.
	rewriteDescriptionsThenCancel := func(writes map[int]string, n int) func(w *waitWorld) {
		return func(w *waitWorld) {
			w.onSleep = func(w *waitWorld, count int) {
				if content, ok := writes[count]; ok {
					if err := os.WriteFile(w.deps.DescriptionPath, []byte(content), 0o644); err != nil {
						t.Errorf("rewrite the change description: %v", err)
					}
				}
				if count >= n {
					w.cancel()
				}
			}
		}
	}
	// rewriteDescriptionThenCancel returns a setup that writes content to the change description at the first sleep,
	// and the setup cancels the wait at the n-th sleep.
	rewriteDescriptionThenCancel := func(content string, n int) func(w *waitWorld) {
		return rewriteDescriptionsThenCancel(map[int]string{1: content}, n)
	}
	editFailing := func(failures ...scriptedCall) map[string][]scriptedCall {
		return map[string][]scriptedCall{"Status": {open}, "Edit": append([]scriptedCall{{}}, failures...)}
	}
	failingGate := func(w *waitWorld) {
		w.withGateCommand()
		w.tree.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, Log: "/logs/gate.log"}
	}

	// openHoldingText is an open pull request whose title and body on GitHub are the given ones, not what the row set.
	openHoldingText := func(title, body string) scriptedCall {
		return scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen, Title: title, Body: body}}
	}

	// openAtGitHubHead is an open pull request whose branch on GitHub is at head.
	openAtGitHubHead := func(head string) scriptedCall {
		return scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: head}}
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
		// wantReasonExact is the whole reason the halt carries, for a row that pins it to the word.
		wantReasonExact string
		wantErr         error
		// wantErrText is a text the call's error holds, for an error with no sentinel to match.
		wantErrText string
		wantCalls   []string
		wantRecord  *pullrequest.Record
		// noEntryPush marks a row whose entry finds an ungated head recorded, so Call itself pushes nothing.
		noEntryPush bool
		// entryHalt marks a row whose entry halts, so Call pushes nothing and never starts the wait.
		entryHalt bool
		// gatePushes counts the pushes the wait makes after Call's own.
		gatePushes int
		// restsFloorRemainder allows a sleep under a second, the part of the floor a change to the record leaves.
		restsFloorRemainder bool
		check               func(t *testing.T, w *waitWorld, logs string)
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
			name: "MergedAtAnUngatedTaskHeadHaltsWithoutGatingOrPushingIt", record: seed("h1"), scripts: statusScript(mergedAt("h2")), head: "h2", noEntryPush: true,
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
			name: "ReviewEndedOnAClosedPullRequestOnACleanTreeIsDone", record: seed("h1", withReviewEnded), scripts: statusScript(closed),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7"}, wantRecord: seed("h1", withReviewEnded),
		},
		{
			name: "ReviewEndedOnADirtyTreeHaltsNamingTheFiles", record: seed("h1", withReviewEnded), scripts: statusScript(closed), dirty: []string{"wip.go"},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"uncommitted changes: wip.go", "discard them", "lym loom resume"},
		},
		{
			name: "ContinueOnAClosedPullRequestAtTheGatedHeadIsDoneAndRecordsIt", record: seed("h1", withContinue), scripts: statusScript(closed),
			wantOutcome: shedengine.Done, wantRecord: seed("h1", withContinue, withReviewEnded),
		},
		{
			name: "ContinueOnAClosedPullRequestWithAnUngatedCleanHeadGatesItFirst", record: seed("h1", withContinue), scripts: statusScript(closed), head: "h2", noEntryPush: true,
			wantOutcome: shedengine.Done, wantRecord: &pullrequest.Record{Number: 7, URL: "u7", GatedHead: "h2", Continue: true, ReviewEnded: true}, gatePushes: 1,
		},
		{
			name: "ContinueOnAClosedPullRequestOnADirtyTreeKeepsWaiting", record: seed("h1", withContinue), scripts: statusScript(closed), dirty: []string{"wip.go"},
			setup:   func(w *waitWorld) { w.cancelAfter(3) },
			wantErr: context.Canceled, wantCalls: []string{"Status 7"}, wantRecord: seed("h1", withContinue),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if len(w.sleeps) != 3 {
					t.Errorf("wait slept %d times; want 3 wakes before the cancel", len(w.sleeps))
				}
			},
		},
		{
			name: "ContinueOnAMergedPullRequestTakesTheMergedVerdictAndClosesNothing", record: seed("h1", withContinue), scripts: statusScript(mergedAt("h1")),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7"},
		},
		{
			name: "ContinueOnAnOpenPullRequestAtTheGatedHeadRecordsTheEndingLeavesItOpenAndIsDone", record: seed("h1", withContinue), scripts: statusScript(open),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7", editSeven},
			wantRecord: recorded("h1", withContinue, withReviewEnded),
		},
		{
			name: "ReviewEndedOnAnOpenPullRequestOnACleanTreeIsDoneWithoutEditingIt", record: seed("h1", withContinue, withReviewEnded), scripts: statusScript(open),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7"}, wantRecord: seed("h1", withContinue, withReviewEnded),
		},
		{
			name: "ReviewEndedOnAnOpenPullRequestOnADirtyTreeHaltsNamingTheFiles", record: seed("h1", withContinue, withReviewEnded), scripts: statusScript(open), dirty: []string{"wip.go"},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"uncommitted changes: wip.go", "discard them", "lym loom resume"},
		},
		{
			name: "ContinueOnAnOpenPullRequestAtAnUngatedCleanHeadGatesItAndEndsTheReview", record: seed("h1", withContinue), scripts: statusScript(open), head: "h2", noEntryPush: true,
			setup:       func(w *waitWorld) { w.withGateCommand() },
			wantOutcome: shedengine.Done, gatePushes: 1,
			wantCalls:  []string{"Status 7", editSeven},
			wantRecord: recorded("h2", withContinue, withReviewEnded),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 1 {
					t.Errorf("the gate ran %d times; want once over the ungated head", w.tree.verifyCalls)
				}
			},
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
			},
		},
		{
			name: "ApprovalAtTheGatedAndTaskHeadIsDone", record: seed("h1"), scripts: statusScript(approvedAt("h1", false)),
			wantOutcome: shedengine.Done, wantCalls: []string{"Status 7", editSeven}, wantRecord: recorded("h1"),
		},
		{
			name: "ApprovalOnADirtyTreeKeepsWaiting", record: seed("h1"), scripts: statusScript(approvedAt("h1", false)), dirty: []string{"wip.go"},
			setup:   func(w *waitWorld) { w.cancelAfter(2) },
			wantErr: context.Canceled,
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
			name: "ADirtyTreeWaitsWithoutGating", record: seed("h1"), scripts: statusScript(open), head: "h2", noEntryPush: true, dirty: []string{"wip.go"},
			setup:   func(w *waitWorld) { w.withGateCommand(); w.cancelAfter(2) },
			wantErr: context.Canceled, wantRecord: recorded("h1"),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 0 {
					t.Errorf("the gate ran %d times on a dirty tree; want none", w.tree.verifyCalls)
				}
			},
		},
		{
			name: "AMovedCleanHeadIsGatedPushedAndRecorded", record: seed("h1"), scripts: statusScript(open), head: "h2", noEntryPush: true,
			setup: func(w *waitWorld) {
				w.withGateCommand()
				w.cancelAfter(2)
				push := w.deps.PullRequest.PushTask
				w.deps.PullRequest.PushTask = func() error {
					if w.tree.verifyCalls != 1 {
						t.Errorf("pushed after %d gate runs; want the push after the one gate run", w.tree.verifyCalls)
					}
					return push()
				}
			},
			wantErr: context.Canceled, gatePushes: 1, wantRecord: recorded("h2"),
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 1 {
					t.Errorf("the gate ran %d times; want once for the one moved head", w.tree.verifyCalls)
				}
			},
		},
		{
			name: "AGateFailureOnAPullRequestClosedWithNoFlagHaltsAndMailsNothing", record: seed("h1", withStrand), scripts: statusScript(closed), head: "h2", noEntryPush: true,
			setup:       failingGate,
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"refused task head h2", "lym loom resume"},
			wantRecord: seed("h1", withStrand),
		},
		{
			name: "AGateFailureInAnEndedReviewHaltsAndMailsNothing", record: seed("h1", withStrand, withReviewEnded), scripts: statusScript(open),
			setup: func(w *waitWorld) {
				failingGate(w)
				// The entry reads the gated head, and the first wake reads a commit made since.
				reads := 0
				w.deps.TaskHead = func() (string, error) {
					reads++
					if reads == 1 {
						return "h1", nil
					}
					return "h2", nil
				}
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"refused task head h2", "lym loom resume"},
		},
		{
			// The 7th and 8th wakes are the polls after the entry's, and both fail to read the session.
			name: "AFailingSessionReadIsLoggedOnceAndKeepsWaiting", record: seed("h1", withStrand), scripts: statusScript(open),
			setup: func(w *waitWorld) {
				w.onSleep = func(w *waitWorld, n int) {
					if n == 1 {
						w.sessions.blockStatus()
					}
					if n >= 8 {
						w.cancel()
					}
				}
			},
			wantErr: context.Canceled,
			check: func(t *testing.T, w *waitWorld, logs string) {
				if w.api.statusCalls != 3 {
					t.Errorf("Status calls = %d; want the entry's and two polls", w.api.statusCalls)
				}
				if got := strings.Count(logs, "could not read whether the PR-Review session is live"); got != 1 {
					t.Errorf("live-read warnings = %d; want one for the whole episode", got)
				}
			},
		},
		{
			name: "ARunnerCrashHaltsTheRowNamingTheCrashAndMailsNothing", record: seed("h1", withStrand), scripts: statusScript(open), head: "h2", noEntryPush: true,
			setup: func(w *waitWorld) {
				w.withGateCommand()
				w.tree.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 134, RunnerCrash: true, Detail: "gate test runner crashed with no failing test: tier1 (exit 134, SIGABRT)", Log: "/logs/gate.log"}
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{LandingStopPrefix, "tier1 (exit 134, SIGABRT)", "/logs/gate.log", "`lym loom resume`"},
		},
		{
			name: "AForeignGitHubHeadAtEntryHaltsNamingItsCommitsAndPushesNothing", record: seed("h1"), scripts: statusScript(openAtGitHubHead("x9")), entryHalt: true,
			setup:       func(w *waitWorld) { w.missing = []string{"abc1234 hand fix"} },
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReasonExact: "pull request #7's branch task on GitHub is at x9 and holds these commits the task worktree lacks: abc1234 hand fix, " +
				"pushed there by hand, so the run never gated them; " +
				"way forward: bring them into the task branch in the task worktree, or drop them from the branch on GitHub, " +
				"then run lym loom resume, which gates a task head not yet gated before it pushes it",
			wantCalls: []string{"Status 7"},
			check: func(t *testing.T, w *waitWorld, _ string) {
				if diff := cmp.Diff([]string{"h1|x9"}, w.missingCalls); diff != "" {
					t.Errorf("CommitsMissing calls mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "AForeignGitHubHeadBesideATaskHeadMovedDuringTheWaitHaltsBeforeTheGate", record: seed("h1"), scripts: statusScript(open, openAtGitHubHead("x9")),
			setup: func(w *waitWorld) {
				w.withGateCommand()
				w.missing = []string{"abc1234 hand fix"}
				w.onSleep = func(w *waitWorld, n int) {
					if n == 1 {
						w.head = "h2"
					}
				}
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"holds these commits the task worktree lacks: abc1234 hand fix"},
			check: func(t *testing.T, w *waitWorld, _ string) {
				if w.tree.verifyCalls != 0 {
					t.Errorf("the gate ran %d times beside a hand commit; want none", w.tree.verifyCalls)
				}
			},
		},
		{
			name: "AForeignGitHubHeadHaltsAContinuedReviewWithoutRecordingAnEnding", record: seed("h1"), scripts: statusScript(open, openAtGitHubHead("x9")),
			setup: func(w *waitWorld) {
				w.missing = []string{"abc1234 hand fix"}
				w.onSleep = func(w *waitWorld, n int) {
					if n == 1 {
						writeContinue(w)
					}
				}
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"holds these commits the task worktree lacks: abc1234 hand fix"},
			wantRecord: recorded("h1", withContinue),
		},
		{
			name: "AGitHubHeadTheTaskWorktreeCannotReadHaltsNamingTheReadError", record: seed("h1"), scripts: statusScript(openAtGitHubHead("x9")), entryHalt: true,
			setup:       func(w *waitWorld) { w.missingErr = errors.New("unknown revision x9") },
			wantOutcome: shedengine.Stuck, wantHalt: true,
			wantReasonExact: "pull request #7's branch task on GitHub is at x9 and holds commits the task worktree cannot read (unknown revision x9), " +
				"pushed there by hand, so the run never gated them; " +
				"way forward: bring them into the task branch in the task worktree, or drop them from the branch on GitHub, " +
				"then run lym loom resume, which gates a task head not yet gated before it pushes it",
			wantCalls: []string{"Status 7"},
		},
		{
			name: "AGitHubHeadBehindTheTaskHeadKeepsWaiting", record: seed("h1"), scripts: statusScript(openAtGitHubHead("x8")),
			setup:   func(w *waitWorld) { w.cancelAfter(2) },
			wantErr: context.Canceled, wantCalls: []string{"Status 7", editSeven}, wantRecord: recorded("h1"),
			check: func(t *testing.T, w *waitWorld, _ string) {
				// The entry judges the head once and the first wake once more.
				if diff := cmp.Diff([]string{"h1|x8", "h1|x8"}, w.missingCalls); diff != "" {
					t.Errorf("CommitsMissing calls mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "AHeadMovedDuringTheGateDiscardsTheVerdict", record: seed("h1"), scripts: statusScript(open), head: "h2", noEntryPush: true,
			setup: func(w *waitWorld) {
				w.withGateCommand()
				w.tree.onVerify = once(func() { w.head = "h3" })
				w.cancelAfter(1)
			},
			wantErr: context.Canceled, wantRecord: recorded("h1"),
		},
		{
			name: "ATreeDirtiedDuringTheGateDiscardsTheVerdict", record: seed("h1"), scripts: statusScript(open), head: "h2", noEntryPush: true,
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
			name: "AForbiddenPollHaltsNamingTheTokenAndRepositoryAccess", scripts: createdAndStatus(scriptedCall{err: githubErrorResponse(http.StatusForbidden, "Forbidden")}),
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
			name: "ACoveringPauseAtAWakeSkipsTheGateAndThePollAndTheWaitGoesOnOnceUncovered", record: seed("h1"), scripts: statusScript(open), head: "h2", noEntryPush: true,
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
			wantOutcome: shedengine.Done, wantCalls: []string{"FindOpen task main", "Create task main|Change title|" + wantPRBody, "Status 7"},
		},
		{
			name: "TheContinueFlagWrittenBetweenWakesPollsAtOnceAndEndsTheReview", record: seed("h1"), scripts: statusScript(open),
			setup: func(w *waitWorld) {
				w.onSleep = func(w *waitWorld, n int) {
					if n == 1 {
						writeContinue(w)
					}
				}
			},
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7"},
			wantRecord:  recorded("h1", withContinue, withReviewEnded),
		},
		{
			name: "AChangeToTheRecordEndsTheSleepEarlyAndTheContinueFlagEndsTheReviewAfterTheFloor", record: seed("h1"), scripts: statusScript(open),
			setup:       changeDuringFirstSleep(0),
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7"},
			wantRecord:  recorded("h1", withContinue, withReviewEnded),
			check: func(t *testing.T, w *waitWorld, _ string) {
				// The clock stands still in the cut sleep, so the rest of the floor is a whole second.
				want := []time.Duration{time.Second, time.Second}
				if diff := cmp.Diff(want, w.sleeps); diff != "" {
					t.Errorf("sleeps mismatch (-want +got):\n%s", diff)
				}
				if w.watchStops != 1 {
					t.Errorf("the record watch ended %d times; want once", w.watchStops)
				}
			},
		},
		{
			name: "AChangeAfterPartOfTheFloorHasPassedRestsOnlyTheRemainder", record: seed("h1"), scripts: statusScript(open),
			setup:               changeDuringFirstSleep(400 * time.Millisecond),
			restsFloorRemainder: true,
			wantOutcome:         shedengine.Done,
			wantCalls:           []string{"Status 7", editSeven, "Status 7"},
			wantRecord:          recorded("h1", withContinue, withReviewEnded),
			check: func(t *testing.T, w *waitWorld, _ string) {
				want := []time.Duration{time.Second, 600 * time.Millisecond}
				if diff := cmp.Diff(want, w.sleeps); diff != "" {
					t.Errorf("sleeps mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "AChangeAfterTheFloorHasPassedWakesAtOnceWithoutAnotherSleep", record: seed("h1"), scripts: statusScript(open),
			setup:       changeDuringFirstSleep(time.Second),
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7"},
			wantRecord:  recorded("h1", withContinue, withReviewEnded),
			check: func(t *testing.T, w *waitWorld, _ string) {
				want := []time.Duration{time.Second}
				if diff := cmp.Diff(want, w.sleeps); diff != "" {
					t.Errorf("sleeps mismatch (-want +got):\n%s", diff)
				}
			},
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
			// The sleeps run 1, 2, 4, 8, 16, 32 and 60 seconds, so the 7th and 8th wakes are the polls after the entry's.
			name: "AnEditedDescriptionSetsTheTitleAndBodyAgainAtTheNextPollAndOnlyThen", record: seed("h1"), scripts: statusScript(open),
			setup:   rewriteDescriptionThenCancel(newDescription, 8),
			wantErr: context.Canceled,
			wantCalls: []string{
				"Status 7", editSeven, "Status 7",
				editNew,
				"Status 7",
			},
		},
		{
			// The 7th wake is the first poll after the entry's, and it reads a body edited on GitHub by hand.
			name: "ABodyEditedOnGitHubIsSetBackToTheDescriptionAtTheNextPoll", record: seed("h1"),
			scripts:   statusScript(open, openHoldingText("Change title", "A body edited on GitHub")),
			setup:     func(w *waitWorld) { w.cancelAfter(7) },
			wantErr:   context.Canceled,
			wantCalls: []string{"Status 7", editSeven, "Status 7", editSeven},
		},
		{
			name: "ABodyThatDiffersOnlyInLineEndingsAndTrailingWhitespaceIsNotEdited", record: seed("h1"),
			scripts:   statusScript(open, openHoldingText("Change title", strings.ReplaceAll(wantPRBody, "\n", "\r\n")+"  \r\n")),
			setup:     func(w *waitWorld) { w.cancelAfter(7) },
			wantErr:   context.Canceled,
			wantCalls: []string{"Status 7", editSeven, "Status 7"},
		},
		{
			name: "ADescriptionThatYieldsNoTextIsLoggedOnceAndEditsNothing", record: seed("h1"), scripts: statusScript(open),
			setup:     rewriteDescriptionThenCancel("", 8),
			wantErr:   context.Canceled,
			wantCalls: []string{"Status 7", editSeven, "Status 7", "Status 7"},
			check: func(t *testing.T, _ *waitWorld, logs string) {
				if got := strings.Count(logs, "no pull request text could be built"); got != 1 {
					t.Errorf("warnings for the unreadable description = %d; want one over two polls", got)
				}
			},
		},
		{
			// The polls after the entry's are the 7th, 8th and 9th wakes: a bad description, a good one, a bad one again.
			name: "ADescriptionThatYieldsNoTextAgainAfterAGoodOneIsLoggedAgain", record: seed("h1"), scripts: statusScript(open),
			setup:     rewriteDescriptionsThenCancel(map[int]string{1: "", 7: newDescription, 8: ""}, 9),
			wantErr:   context.Canceled,
			wantCalls: []string{"Status 7", editSeven, "Status 7", "Status 7", editNew, "Status 7"},
			check: func(t *testing.T, _ *waitWorld, logs string) {
				if got := strings.Count(logs, "no pull request text could be built"); got != 2 {
					t.Errorf("warnings for the unreadable description = %d; want one per episode, two", got)
				}
			},
		},
		{
			name: "AnEditedDescriptionIsSetWhileAContinuedReviewStillWaits", record: seed("h1", withContinue), scripts: statusScript(open), dirty: []string{"wip.go"},
			setup:     rewriteDescriptionThenCancel(newDescription, 8),
			wantErr:   context.Canceled,
			wantCalls: []string{"Status 7", editSeven, "Status 7", editNew, "Status 7"},
		},
		{
			name: "AContinuedReviewSetsAStaleTextBeforeItEnds", record: seed("h1"), scripts: statusScript(open),
			setup: func(w *waitWorld) {
				rewriteDescriptionThenCancel(newDescription, 20)(w)
				rewrite := w.onSleep
				w.onSleep = func(w *waitWorld, n int) {
					rewrite(w, n)
					if n == 1 {
						writeContinue(w)
					}
				}
			},
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7", editNew},
		},
		{
			name: "ADescriptionThatYieldsNoTextHoldsTheContinuedEnding", record: seed("h1"), scripts: statusScript(open),
			setup: func(w *waitWorld) {
				rewriteDescriptionThenCancel("", 8)(w)
				rewrite := w.onSleep
				w.onSleep = func(w *waitWorld, n int) {
					rewrite(w, n)
					if n == 1 {
						writeContinue(w)
					}
				}
			},
			wantErr: context.Canceled, wantRecord: recorded("h1", withContinue),
			check: func(t *testing.T, _ *waitWorld, logs string) {
				for _, want := range []string{"the review stays open", "way forward: fix what the cause names", "lym loom validate-description", "lym loom pr-continue"} {
					if !strings.Contains(logs, want) {
						t.Errorf("logs = %q; want the Warn to carry %q", logs, want)
					}
				}
			},
		},
		{
			name: "ATransientEditFailureHoldsTheContinuedEndingUntilTheTextIsSet", record: seed("h1"),
			scripts: editFailing(scriptedCall{err: context.DeadlineExceeded}, scriptedCall{}),
			setup: func(w *waitWorld) {
				rewriteDescriptionThenCancel(newDescription, 20)(w)
				rewrite := w.onSleep
				w.onSleep = func(w *waitWorld, n int) {
					rewrite(w, n)
					if n == 1 {
						writeContinue(w)
					}
				}
			},
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7", editNew, "Status 7", editNew},
		},
		{
			name: "ATransientEditFailureHoldsAnApprovalUntilTheTextIsSet", record: seed("h1"),
			scripts:     map[string][]scriptedCall{"Status": {open, approvedAt("h1", false)}, "Edit": {{}, {err: context.DeadlineExceeded}, {}}},
			setup:       rewriteDescriptionThenCancel(newDescription, 20),
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7", editNew, "Status 7", editNew},
		},
		{
			name: "AnEditedDescriptionIsNotSetOnAPullRequestThatIsClosedAtThePoll", record: seed("h1"), scripts: statusScript(open, closed),
			setup:       rewriteDescriptionThenCancel(newDescription, 8),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"reopen it on GitHub"},
			wantCalls: []string{"Status 7", editSeven, "Status 7"},
		},
		{
			name: "AnEditedDescriptionIsNotSetOnAPullRequestThatIsMergedAtThePoll", record: seed("h1"), scripts: statusScript(open, mergedAt("h1")),
			setup:       rewriteDescriptionThenCancel(newDescription, 8),
			wantOutcome: shedengine.Done,
			wantCalls:   []string{"Status 7", editSeven, "Status 7"},
		},
		{
			name: "ATransientEditFailureKeepsWaitingAndSetsTheSameTextAtTheNextPoll", record: seed("h1"), scripts: editFailing(scriptedCall{err: context.DeadlineExceeded}, scriptedCall{}),
			setup:     rewriteDescriptionThenCancel(newDescription, 9),
			wantErr:   context.Canceled,
			wantCalls: []string{"Status 7", editSeven, "Status 7", editNew, "Status 7", editNew, "Status 7"},
			check: func(t *testing.T, _ *waitWorld, logs string) {
				if got := strings.Count(logs, "a GitHub call failed transiently"); got != 1 {
					t.Errorf("transient warnings = %d; want one", got)
				}
			},
		},
		{
			name: "AnEditThatKeepsFailingTransientlyIsRetriedAtEveryPollAndLoggedOnce", record: seed("h1"), scripts: editFailing(scriptedCall{err: context.DeadlineExceeded}),
			setup:     rewriteDescriptionThenCancel(newDescription, 9),
			wantErr:   context.Canceled,
			wantCalls: []string{"Status 7", editSeven, "Status 7", editNew, "Status 7", editNew, "Status 7", editNew},
			check: func(t *testing.T, _ *waitWorld, logs string) {
				if got := strings.Count(logs, "a GitHub call failed transiently"); got != 1 {
					t.Errorf("transient warnings = %d; want one for the whole episode of failed edits", got)
				}
			},
		},
		{
			name: "AnEditThatFailsForGoodHaltsNamingTheEdit", record: seed("h1"), scripts: editFailing(scriptedCall{err: githubErrorResponse(http.StatusUnprocessableEntity, "Unprocessable")}),
			setup:       rewriteDescriptionThenCancel(newDescription, 8),
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"set the pull request's title and body", "422"},
			wantCalls: []string{"Status 7", editSeven, "Status 7", editNew},
		},
		{
			name: "AWaitEnteredOnAnEndedReviewKeepsWaitingThroughATransientPollFailure", record: seed("h1", withReviewEnded),
			scripts: statusScript(scriptedCall{err: context.DeadlineExceeded}),
			setup:   func(w *waitWorld) { w.cancelAfter(2) },
			wantErr: context.Canceled,
		},
		{
			name: "TheMarkIsSetAtTheStartAndAfterAGateAndClearedOnDone", record: seed("h1"), scripts: statusScript(approvedAt("h2", false)), head: "h2", noEntryPush: true,
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
				if tt.record.StrandGUID == firstGUID {
					w.sessions.seedLive(w.deps)
				}
				if err := pullrequest.WriteRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath, *tt.record); err != nil {
					t.Fatalf("write record: %v", err)
				}
			}
			row := NewPRReview("PR-Review", w.deps)
			row.gate.verify = w.tree.verify

			outcome, pointer, err := row.Call(ctx)

			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Call() error = %v; want one wrapping %v", err, tt.wantErr)
				}
			case tt.wantErrText != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("Call() error = %v; want one containing %q", err, tt.wantErrText)
				}
			default:
				if err != nil {
					t.Fatalf("Call() error = %v; want none", err)
				}
				if outcome != tt.wantOutcome || pointer.Halt != tt.wantHalt {
					t.Errorf("Call() = %q, halt %v; want %q, halt %v (reason %q)", outcome, pointer.Halt, tt.wantOutcome, tt.wantHalt, pointer.Reason)
				}
				if tt.wantReasonExact != "" && pointer.Reason != tt.wantReasonExact {
					t.Errorf("Call() reason = %q; want %q", pointer.Reason, tt.wantReasonExact)
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
			wantPushes := tt.gatePushes
			if !tt.noEntryPush && !tt.entryHalt {
				wantPushes++
			}
			if len(w.pushes) != wantPushes {
				t.Errorf("pushed %d times (%q); want %d", len(w.pushes), w.pushes, wantPushes)
			}
			if tt.wantRecord != nil {
				got, found, rerr := pullrequest.ReadRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath)
				if rerr != nil || !found {
					t.Fatalf("ReadRecord() = %+v, %v, %v; want the record", got, found, rerr)
				}
				if diff := cmp.Diff(*tt.wantRecord, got, recordComparison(*tt.wantRecord)...); diff != "" {
					t.Errorf("record mismatch (-want +got):\n%s", diff)
				}
			}
			if tt.check != nil {
				tt.check(t, w, logs.String())
			}

			for _, d := range w.sleeps {
				if (d < time.Second && !tt.restsFloorRemainder) || d <= 0 || d > time.Minute {
					t.Errorf("slept %v; want every wait between a second and a minute", d)
				}
			}
			// A wiring error and a halt at entry end the call before the wait sets its mark.
			if tt.entryHalt && len(w.marks) != 0 {
				t.Errorf("wait marks = %q; want none from a halt at entry", w.marks)
			}
			if tt.wantErrText == "" && !tt.entryHalt && (len(w.marks) < 2 || w.marks[0] != "PR #7" || w.marks[len(w.marks)-1] != "") {
				t.Errorf("wait marks = %q; want the mark set at the start and cleared on the way out", w.marks)
			}
			held, lerr := w.deps.Lock.Store.List()
			if lerr != nil || len(held) != 0 {
				t.Errorf("the parent lock store holds %v (%v); want no record, since the wait takes no parent lock", held, lerr)
			}
		})
	}
}

func TestPRReview_ReEntryPastAnEndedReviewGatesTheHeadBeforeAnyPush(t *testing.T) {
	tiergate.Require(t, 0)
	// The row captures the process-global logger, so this test stays serial.
	logcapture.CaptureVerbose(t)
	w, ctx := newWaitWorld(t, nil)
	w.withGateCommand()
	w.tree.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, Log: "/logs/gate.log"}
	w.head = "h2"
	rec := pullrequest.Record{Number: 7, URL: "u7", GatedHead: "h1", ReviewEnded: true}
	if err := pullrequest.WriteRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath, rec); err != nil {
		t.Fatalf("write record: %v", err)
	}
	row := NewPRReview("PR-Review", w.deps)
	row.gate.verify = w.tree.verify

	outcome, pointer, err := row.Call(ctx)

	if err != nil || outcome != shedengine.Stuck || !pointer.Halt || !strings.Contains(pointer.Reason, "refused task head h2") {
		t.Fatalf("Call() = %q, halt %v, reason %q, error %v; want a halt on the refused head", outcome, pointer.Halt, pointer.Reason, err)
	}
	if len(w.pushes) != 0 || len(w.api.calls) != 0 {
		t.Errorf("pushes = %q, GitHub calls = %q; want none", w.pushes, w.api.calls)
	}
	got, found, err := pullrequest.ReadRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath)
	if diff := cmp.Diff(rec, got); err != nil || !found || diff != "" {
		t.Errorf("record found = %v, error %v, mismatch (-want +got):\n%s", found, err, diff)
	}
}

func TestPRReview_WaitThatStartsWithNoTextSetEditsAPullRequestWhoseTextDiffers(t *testing.T) {
	tiergate.Require(t, 0)
	// The wait captures the process-global logger, so this test stays serial.
	logcapture.CaptureVerbose(t)
	handEdited := pullrequest.Status{State: pullrequest.StateOpen, Title: "Hand title", Body: "Hand body"}
	w, ctx := newWaitWorld(t, map[string][]scriptedCall{"Status": {{status: handEdited}}})
	rec := pullrequest.Record{Number: 7, URL: "u7", GatedHead: "h1"}
	if err := pullrequest.WriteRecord(w.deps.PullRequest.RecordPath, w.deps.PullRequest.RecordLockPath, rec); err != nil {
		t.Fatalf("write record: %v", err)
	}
	w.cancelAfter(1)
	row := NewPRReview("PR-Review", w.deps)
	row.gate.verify = w.tree.verify

	err := row.wait(ctx, w.api, rec, nil, time.Time{}, false).err

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wait() error = %v; want one wrapping %v", err, context.Canceled)
	}
	if diff := cmp.Diff([]string{"Status 7", "Edit 7|Change title|" + wantPRBody}, w.api.calls); diff != "" {
		t.Errorf("GitHub calls mismatch (-want +got):\n%s", diff)
	}
}
