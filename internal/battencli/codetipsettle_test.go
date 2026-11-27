// codetipsettle_test.go pins settleCodeTip over fake deps: which branch of the owner's code-tip wait each reading takes.

package battencli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// codeTipCalls counts what a settleCodeTip call did through its deps.
type codeTipCalls struct {
	fetch, push, giveUp, fastForward int
}

// TestSettleCodeTip does not call t.Parallel: logcapture redirects the process-global logger.
func TestSettleCodeTip(t *testing.T) {
	tiergate.Require(t, 0)

	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	missing := func(age time.Duration) fabriclayout.CodeTipReading {
		return fabriclayout.CodeTipReading{Tip: "abc123", Missing: true, Machine: "oslo", At: now.Add(-age)}
	}
	landed := fabriclayout.CodeTipReading{Tip: "abc123", Machine: "oslo", At: now.Add(-time.Hour)}
	pushRefused := errors.New("origin refused the push")

	tests := []struct {
		name string
		// reason is the verdict's reason; the empty reason is an act verdict.
		reason runfsm.Reason
		read   fabriclayout.CodeTipReading
		// readAfterFetch is what the read after the fetch answers; nil answers read again.
		readAfterFetch *fabriclayout.CodeTipReading
		readErr        error
		held           bool
		pushErr        error
		giveUpReturned bool
		giveUpErr      error
		wantCalls      codeTipCalls
		wantErr        bool
		// wantLog is a text the logs hold; empty wants no Warn at all.
		wantLog string
	}{
		{
			name: "AMissingTipInsideTheGraceGivesNothingUp", reason: runfsm.ReasonCodeTip,
			read: missing(5 * time.Minute), wantCalls: codeTipCalls{fetch: 1},
		},
		{
			name: "AMissingTipPastTheGraceIsGivenUpOnce", reason: runfsm.ReasonCodeTip,
			read: missing(31 * time.Minute), giveUpReturned: true, wantCalls: codeTipCalls{fetch: 1, giveUp: 1},
		},
		{
			name: "AMissingTipTheLocalBranchHoldsIsPushedInsideTheGrace", reason: runfsm.ReasonCodeTip,
			read: missing(5 * time.Minute), held: true, wantCalls: codeTipCalls{fetch: 1, push: 1},
		},
		{
			name: "AMissingTipTheLocalBranchHoldsIsPushedNotGivenUpPastTheGrace", reason: runfsm.ReasonCodeTip,
			read: missing(31 * time.Minute), held: true, wantCalls: codeTipCalls{fetch: 1, push: 1},
		},
		{
			name: "AFailingPushOfAHeldTipPastTheGraceLogsTheWayForward", reason: runfsm.ReasonCodeTip,
			read: missing(31 * time.Minute), held: true, pushErr: pushRefused, wantCalls: codeTipCalls{fetch: 1, push: 1},
			wantLog: "lym fabric push",
		},
		{
			name: "ATipTheFetchShowsLandedGivesNothingUpAndFastForwards", reason: runfsm.ReasonCodeTip,
			read: missing(31 * time.Minute), readAfterFetch: &landed, wantCalls: codeTipCalls{fetch: 1, fastForward: 1},
		},
		{
			name: "AGiveUpThatReturnsFalseLogsTheOwingMachine", reason: runfsm.ReasonCodeTip,
			read: missing(31 * time.Minute), wantCalls: codeTipCalls{fetch: 1, giveUp: 1},
			wantLog: "machine oslo",
		},
		{
			name: "AGiveUpWhosePushFailsReturnsNilForTheNextTick", reason: runfsm.ReasonCodeTip,
			read: missing(31 * time.Minute), giveUpReturned: true, giveUpErr: pushRefused, wantCalls: codeTipCalls{fetch: 1, giveUp: 1},
			wantLog: "give-up",
		},
		{
			name: "AFailedReadReturnsItsError", reason: runfsm.ReasonCodeTip,
			readErr: errors.New("records unreadable"), wantErr: true,
		},
		{
			name: "ALandedTipWithTheBranchBehindFastForwards",
			read: landed, wantCalls: codeTipCalls{fastForward: 1},
		},
		{
			name: "AnActVerdictWithALevelBranchDoesNothing",
			read: landed, held: true,
		},
		{
			name: "AMissingTipUnderAnotherVerdictLeavesThatVerdictsWait", reason: runfsm.ReasonPushOwed,
			read: missing(31 * time.Minute), held: true,
		},
		{
			name: "ARunWithNoTransitionOnOriginDoesNothing", reason: runfsm.ReasonCodeTip,
			read: fabriclayout.CodeTipReading{}, wantCalls: codeTipCalls{fetch: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			var calls codeTipCalls
			reads := 0
			deps := codeTipDeps{
				Read: func(string) (fabriclayout.CodeTipReading, error) {
					reads++
					if tt.readErr != nil {
						return fabriclayout.CodeTipReading{}, tt.readErr
					}
					if reads > 1 && tt.readAfterFetch != nil {
						return *tt.readAfterFetch, nil
					}
					return tt.read, nil
				},
				Fetch:       func(string) error { calls.fetch++; return nil },
				HoldsTip:    func(string, string) (bool, error) { return tt.held, nil },
				PushTip:     func(string) error { calls.push++; return tt.pushErr },
				GiveUp:      func(string) (bool, error) { calls.giveUp++; return tt.giveUpReturned, tt.giveUpErr },
				FastForward: func(string) error { calls.fastForward++; return nil },
			}

			err := settleCodeTip(deps, tt.name, runfsm.Verdict{Reason: tt.reason}, now)

			if (err != nil) != tt.wantErr {
				t.Errorf("settleCodeTip() error = %v; want error %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("settleCodeTip() calls = %+v; want %+v", calls, tt.wantCalls)
			}
			switch got := logs.String(); {
			case tt.wantLog == "" && got != "":
				t.Errorf("settleCodeTip() logged %q; want nothing", got)
			case tt.wantLog != "" && !strings.Contains(got, tt.wantLog):
				t.Errorf("settleCodeTip() logged %q; want it to hold %q", got, tt.wantLog)
			}
		})
	}
}
