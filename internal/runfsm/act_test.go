// act_test.go pins the act rule's order, each refusal's exact text and the effect of each aside.

package runfsm_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	testSlug  = "slug"
	testRetry = "the verb"
)

// refusalText returns the error text of err, empty for nil.
func refusalText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestAct(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		pushOwed      = `run "slug" cannot act: a push to origin is owed (last error: dial tcp: refused); way forward: run ` + "`lym fabric push`" + ` in /wt/slug once origin answers, then run the verb again`
		pushOwedBare  = `run "slug" cannot act: a push to origin is owed; way forward: run the verb again once the owed push lands`
		pushOwedNamed = `run "slug" cannot act: a push to origin is owed (last error: refused); way forward: run ` + "`lym fabric push`" + ` in /hub/task to retry the push; then run the verb again`
		owned         = "run \"slug\" cannot act: machine neve owns the run; way forward: run the same verb on neve, or run `lym adopt sub` on this machine first"
		codeTip       = `run "slug" cannot act: the code tip abc123 the run's last transition names is not on origin yet; machine neve owes it; way forward: run ` +
			"`lym fabric push`" + ` in the task worktree on machine neve, then run the verb again once the code push lands`
		codeTipNoMachine = `run "slug" cannot act: the code tip abc123 the run's last transition names is not on origin yet; way forward: run the verb again once the code push lands`
		held             = `run "slug" cannot act: an adopt on machine another machine holds the hub; way forward: wait for that adopt's hold to clear, then run the verb again; if machine another machine has stopped, ask the operator`
		heldNamed        = `run "slug" cannot act: an adopt on machine neve holds the hub; way forward: wait for that adopt's hold to clear, then run the verb again; if machine neve has stopped, ask the operator`
		pauseCover       = "run \"slug\" cannot act: a pause covers the run; way forward: run `lym tender resume slug`, then run the verb again"
		pausedIntent     = "run \"slug\" cannot act: the run is paused; way forward: run `lym tender resume slug`, then run the verb again"
		pausedShed       = "run \"slug\" cannot act: the run is paused; way forward: run `lym batten run slug` from the prime to resume the run, " +
			"or `lym loom resume` in the task worktree to resume the inner run"
		cancelKeep   = "run \"slug\" cannot act: a cancel started on machine neve has not finished; way forward: run `lym batten cancel slug` on machine neve to finish it"
		cancelRemove = "run \"slug\" cannot act: a cancel started on machine neve has not finished; way forward: run `lym batten cancel slug --remove` on machine neve to finish it"
		cancelled    = "run \"slug\" cannot act: the run was cancelled; way forward: run `lym batten cancel slug --remove`, which removes its record and pair, before a fresh run"
	)

	t.Run("Act", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			in         runfsm.Inputs
			wantAnswer runfsm.Answer
			wantReason runfsm.Reason
			wantText   string
		}{
			{"PushOwedWaitsFirst", runfsm.Inputs{State: runfsm.Running, PushOwed: true, PushOwedError: "dial tcp: refused", PushOwedWorktree: "/wt/slug", OwnedElsewhere: true, Owner: "neve", AdoptSubject: "sub"}, runfsm.AnswerWait, runfsm.ReasonPushOwed, pushOwed},
			{"PushOwedWithoutErrorOrWorktreeIsBare", runfsm.Inputs{State: runfsm.Running, PushOwed: true}, runfsm.AnswerWait, runfsm.ReasonPushOwed, pushOwedBare},
			{"PushOwedNamesTheRecordsOwnWayForwardInPlaceOfTheOwedWorktree", runfsm.Inputs{State: runfsm.Running, PushOwed: true, PushOwedError: "refused", PushOwedWorktree: "/hub/task-records", PushOwedWayForward: "run `lym fabric push` in /hub/task to retry the push"}, runfsm.AnswerWait, runfsm.ReasonPushOwed, pushOwedNamed},
			{"OwnedElsewhereHandsOver", runfsm.Inputs{State: runfsm.Running, OwnedElsewhere: true, Owner: "neve", AdoptSubject: "sub"}, runfsm.AnswerHandOver, runfsm.ReasonOwnedElsewhere, owned},
			{"OwnedElsewhereLandingPastMergeHandsOver", runfsm.Inputs{State: runfsm.Landing, PastParentMerge: true, OwnedElsewhere: true, Owner: "neve", AdoptSubject: "sub"}, runfsm.AnswerHandOver, runfsm.ReasonOwnedElsewhere, owned},
			{"MissingCodeTipWaits", runfsm.Inputs{State: runfsm.Running, CodeTipMissing: true, CodeTip: "abc123", CodeTipMachine: "neve"}, runfsm.AnswerWait, runfsm.ReasonCodeTip, codeTip},
			{"MissingCodeTipWithoutMachineLeavesOutThePushClause", runfsm.Inputs{State: runfsm.Running, CodeTipMissing: true, CodeTip: "abc123"}, runfsm.AnswerWait, runfsm.ReasonCodeTip, codeTipNoMachine},
			{"HeldLandingPastMergeActs", runfsm.Inputs{State: runfsm.Landing, PastParentMerge: true, Held: true}, runfsm.AnswerAct, "", ""},
			{"HeldLandingPastMergeWithCoveringPauseWaits", runfsm.Inputs{State: runfsm.Landing, PastParentMerge: true, Held: true, PauseCovers: true}, runfsm.AnswerWait, runfsm.ReasonHeld, held},
			{"LandingPastMergeWithCoveringPauseWaits", runfsm.Inputs{State: runfsm.Landing, PastParentMerge: true, PauseCovers: true}, runfsm.AnswerWait, runfsm.ReasonPauseCovers, pauseCover},
			{"HeldLandingPastMergeWithPushOwedWaits", runfsm.Inputs{State: runfsm.Landing, PastParentMerge: true, Held: true, PushOwed: true, PushOwedError: "dial tcp: refused", PushOwedWorktree: "/wt/slug"}, runfsm.AnswerWait, runfsm.ReasonPushOwed, pushOwed},
			{"HeldLandingBeforeMergeWaits", runfsm.Inputs{State: runfsm.Landing, Held: true}, runfsm.AnswerWait, runfsm.ReasonHeld, held},
			{"HeldWaitsAndNamesTheHoldsMachine", runfsm.Inputs{State: runfsm.Running, Held: true, HoldMachine: "neve"}, runfsm.AnswerWait, runfsm.ReasonHeld, heldNamed},
			{"HeldWithNoMachineNamesAnotherMachine", runfsm.Inputs{State: runfsm.Running, Held: true}, runfsm.AnswerWait, runfsm.ReasonHeld, held},
			{"UnheldLandingBeforeTheMergeActs", runfsm.Inputs{State: runfsm.Landing}, runfsm.AnswerAct, "", ""},
			{"CoveringPauseWaits", runfsm.Inputs{State: runfsm.Running, PauseCovers: true}, runfsm.AnswerWait, runfsm.ReasonPauseCovers, pauseCover},
			{"PausedWaits", runfsm.Inputs{State: runfsm.Paused}, runfsm.AnswerWait, runfsm.ReasonPaused, pausedShed},
			{"CancellingWaits", runfsm.Inputs{State: runfsm.Cancelling}, runfsm.AnswerWait, runfsm.ReasonCancelling, "run \"slug\" cannot act: a cancel started on machine  has not finished; way forward: run `lym batten cancel slug` on machine  to finish it"},
			{"CancelledWaits", runfsm.Inputs{State: runfsm.Cancelled}, runfsm.AnswerWait, runfsm.ReasonTerminal, cancelled},
			{"RunningActs", runfsm.Inputs{State: runfsm.Running}, runfsm.AnswerAct, "", ""},
			{"QueuedActs", runfsm.Inputs{State: runfsm.Queued}, runfsm.AnswerAct, "", ""},
			{"NoRunActs", runfsm.Inputs{State: runfsm.NoRun}, runfsm.AnswerAct, "", ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := runfsm.Act(tt.in)
				if got.Answer != tt.wantAnswer || got.Reason != tt.wantReason {
					t.Errorf("Act(%+v) = %q, %q; want %q, %q", tt.in, got.Answer, got.Reason, tt.wantAnswer, tt.wantReason)
				}
				if text := refusalText(got.Refusal(testSlug, testRetry)); text != tt.wantText {
					t.Errorf("Refusal() = %q; want %q", text, tt.wantText)
				}
			})
		}
	})

	t.Run("LandedRefusalNamesEveryStepToRunTheSlugAgain", func(t *testing.T) {
		t.Parallel()
		text := refusalText(runfsm.Act(runfsm.Inputs{State: runfsm.Landed}).Refusal(testSlug, testRetry))
		for _, want := range []string{
			`run "slug" cannot act: the slug has landed`,
			"`lym batten cleanup --apply` from the prime, which removes its run directory",
			"`lym fabric remove slug` if its pair still stands",
			"`lym board set-status '{\"slug\":\"slug\",\"status\":null}'`, which releases the entry",
			"deleting the leftover task branch, locally and on the remote",
			"give the new task another slug",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("Refusal() = %q; want it to contain %q", text, want)
			}
		}
	})

	t.Run("Judge", func(t *testing.T) {
		t.Parallel()
		running := runfsm.Records{BattenFound: true, BattenStarted: true, Batten: shedengine.Status{State: shedengine.StateRunning}}
		withBatten := func(st shedengine.Status) runfsm.Records {
			return runfsm.Records{BattenFound: true, BattenStarted: true, Batten: st}
		}
		cancelIntent := func(remove bool) *shedengine.CancelIntent {
			return &shedengine.CancelIntent{Machine: "neve", Remove: remove}
		}
		finishedCancel := withBatten(shedengine.Status{State: shedengine.StateRunning, Cancel: &shedengine.CancelIntent{Machine: "neve", Finish: &shedengine.CancelFinish{EndedAt: "2026-11-15T09:05:00Z"}}})
		pausedByIntent := withBatten(shedengine.Status{State: shedengine.StateRunning, PauseIntent: &shedengine.PauseIntent{By: "orch"}})
		pausedByShed := withBatten(shedengine.Status{State: shedengine.StatePaused})
		tests := []struct {
			name       string
			records    runfsm.Records
			in         runfsm.Inputs
			aside      runfsm.Aside
			wantState  runfsm.State
			wantAnswer runfsm.Answer
			wantText   string
		}{
			{"PausedByIntentNamesTenderResume", pausedByIntent, runfsm.Inputs{}, runfsm.Aside{}, runfsm.Paused, runfsm.AnswerWait, pausedIntent},
			{"PausedByShedStateNamesBattenRun", pausedByShed, runfsm.Inputs{}, runfsm.Aside{}, runfsm.Paused, runfsm.AnswerWait, pausedShed},
			{"PauseAsideDropsTheIntent", pausedByIntent, runfsm.Inputs{}, runfsm.Aside{Pause: true}, runfsm.Running, runfsm.AnswerAct, ""},
			{"PauseAsideDropsACoveringPause", running, runfsm.Inputs{PauseCovers: true}, runfsm.Aside{Pause: true}, runfsm.Running, runfsm.AnswerAct, ""},
			{"ShedPauseAsideReadsItAsNotPaused", pausedByShed, runfsm.Inputs{}, runfsm.Aside{ShedPause: true}, runfsm.Running, runfsm.AnswerAct, ""},
			{"PauseAsideKeepsAShedPause", pausedByShed, runfsm.Inputs{}, runfsm.Aside{Pause: true}, runfsm.Paused, runfsm.AnswerWait, pausedShed},
			{"CancellingNamesTheKeepFinish", withBatten(shedengine.Status{State: shedengine.StateRunning, Cancel: cancelIntent(false)}), runfsm.Inputs{}, runfsm.Aside{}, runfsm.Cancelling, runfsm.AnswerWait, cancelKeep},
			{"CancellingNamesTheRemoveFinish", withBatten(shedengine.Status{State: shedengine.StateRunning, Cancel: cancelIntent(true)}), runfsm.Inputs{}, runfsm.Aside{}, runfsm.Cancelling, runfsm.AnswerWait, cancelRemove},
			{"CancelAsideDropsTheIntent", withBatten(shedengine.Status{State: shedengine.StateRunning, Cancel: cancelIntent(true)}), runfsm.Inputs{}, runfsm.Aside{Cancel: true}, runfsm.Running, runfsm.AnswerAct, ""},
			{"CancelAsideDropsAFinishedCancel", finishedCancel, runfsm.Inputs{}, runfsm.Aside{Cancel: true}, runfsm.Running, runfsm.AnswerAct, ""},
			{"UnfinishedCancelAsideDropsTheIntent", withBatten(shedengine.Status{State: shedengine.StateRunning, Cancel: cancelIntent(false)}), runfsm.Inputs{}, runfsm.Aside{UnfinishedCancel: true}, runfsm.Running, runfsm.AnswerAct, ""},
			{"UnfinishedCancelAsideKeepsAFinishedCancel", finishedCancel, runfsm.Inputs{}, runfsm.Aside{UnfinishedCancel: true}, runfsm.Cancelled, runfsm.AnswerWait, cancelled},
			{"HoldAsideDropsTheHold", running, runfsm.Inputs{Held: true}, runfsm.Aside{Hold: true}, runfsm.Running, runfsm.AnswerAct, ""},
			{"PushOwedAsideDropsTheOwedPush", running, runfsm.Inputs{PushOwed: true, PushOwedError: "refused", PushOwedWorktree: "/wt/slug"}, runfsm.Aside{PushOwed: true}, runfsm.Running, runfsm.AnswerAct, ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				reading, verdict := runfsm.Judge(tt.records, tt.in, tt.aside)
				if reading.State != tt.wantState || verdict.Answer != tt.wantAnswer {
					t.Errorf("Judge() = state %q, answer %q; want %q, %q", reading.State, verdict.Answer, tt.wantState, tt.wantAnswer)
				}
				if text := refusalText(verdict.Refusal(testSlug, testRetry)); text != tt.wantText {
					t.Errorf("Refusal() = %q; want %q", text, tt.wantText)
				}
			})
		}
	})
}
