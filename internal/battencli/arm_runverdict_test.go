// arm_runverdict_test.go pins judgeAfterCodeTipFetch over fake reads: when a verdict fetches and is judged again.

package battencli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestJudgeAfterCodeTipFetch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	missingTip := runfsm.Inputs{CodeTipMissing: true, CodeTip: "abc123", CodeTipPairAbsent: true}
	clean := runfsm.Inputs{}
	readFailed := errors.New("records unreadable")

	tests := []struct {
		name string
		// inputs are what each read answers in turn; the last repeats.
		inputs []runfsm.Inputs
		// readErrs is what each read fails with in turn, nil for a read that succeeds; the last repeats.
		readErrs   []error
		fetchErr   error
		wantAnswer runfsm.Answer
		wantReason runfsm.Reason
		wantFetch  int
		wantReads  int
		wantErr    error
	}{
		{
			name:   "AFetchThatDropsAStaleRecordsRefLetsTheRunAct",
			inputs: []runfsm.Inputs{missingTip, clean}, wantAnswer: runfsm.AnswerAct, wantFetch: 1, wantReads: 2,
		},
		{
			name:   "ATipStillMissingAfterTheFetchWaits",
			inputs: []runfsm.Inputs{missingTip}, wantAnswer: runfsm.AnswerWait, wantReason: runfsm.ReasonCodeTip, wantFetch: 1, wantReads: 2,
		},
		{
			name:   "AFailedFetchKeepsTheCodeTipWait",
			inputs: []runfsm.Inputs{missingTip, clean}, fetchErr: errors.New("origin unreachable"),
			wantAnswer: runfsm.AnswerWait, wantReason: runfsm.ReasonCodeTip, wantFetch: 1, wantReads: 1,
		},
		{
			name:   "AnotherWaitFetchesNothing",
			inputs: []runfsm.Inputs{{Held: true}}, wantAnswer: runfsm.AnswerWait, wantReason: runfsm.ReasonHeld, wantReads: 1,
		},
		{
			name:   "AnActVerdictFetchesNothing",
			inputs: []runfsm.Inputs{clean}, wantAnswer: runfsm.AnswerAct, wantReads: 1,
		},
		{
			name:     "AFailedReadReturnsItsError",
			readErrs: []error{readFailed}, wantReads: 1, wantErr: readFailed,
		},
		{
			name:   "AFailedReadAfterTheFetchReturnsItsError",
			inputs: []runfsm.Inputs{missingTip}, readErrs: []error{nil, readFailed}, wantFetch: 1, wantReads: 2, wantErr: readFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reads, fetches := 0, 0
			read := func() (runfsm.Records, runfsm.Inputs, error) {
				reads++
				if len(tt.readErrs) > 0 {
					if err := tt.readErrs[min(reads, len(tt.readErrs))-1]; err != nil {
						return runfsm.Records{}, runfsm.Inputs{}, err
					}
				}
				return runfsm.Records{}, tt.inputs[min(reads, len(tt.inputs))-1], nil
			}
			fetch := func() error { fetches++; return tt.fetchErr }

			verdict, err := judgeAfterCodeTipFetch(read, fetch)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("judgeAfterCodeTipFetch() error = %v; want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (verdict.Answer != tt.wantAnswer || verdict.Reason != tt.wantReason) {
				t.Errorf("judgeAfterCodeTipFetch() = %q, %q; want %q, %q", verdict.Answer, verdict.Reason, tt.wantAnswer, tt.wantReason)
			}
			if fetches != tt.wantFetch || reads != tt.wantReads {
				t.Errorf("judgeAfterCodeTipFetch() fetched %d and read %d times; want %d and %d", fetches, reads, tt.wantFetch, tt.wantReads)
			}
		})
	}
}
