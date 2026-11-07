// run_inject_test.go covers Runner.Inject.
// The happy path plays every input in order through the reed seam.
// A dead strand's pane refuses delivery, and an unknown guid refuses before touching reed.
// Empty inputs are a rejected no-op.

package shuttleengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunner_Inject_HappyPath_PlaysEveryInputInOrder(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newFixture(t, reed, &fakeEngine{}, withStrand("strand-1")).Runner

	inputs := []PaneInput{
		{Key: "C-u"},
		{Text: "go test ./...", Submit: true},
	}
	if err := runner.Inject("strand-1", inputs); err != nil {
		t.Fatalf("Inject() error: %v", err)
	}

	// playInputs' own ordering is covered by run_test.go's Send and Interrupt tests.
	// Here the assertion is that Inject reaches playInputs, and every step is delivered in order through the reed seam.
	if len(reed.SendKeyCalls) != 1 || reed.SendKeyCalls[0].Key != "C-u" {
		t.Errorf("SendKey calls = %+v, want exactly one C-u", reed.SendKeyCalls)
	}
	if len(reed.SendTextCalls) != 1 || reed.SendTextCalls[0].Text != "go test ./..." || !reed.SendTextCalls[0].Submit {
		t.Errorf("SendText calls = %+v, want one submitted \"go test ./...\"", reed.SendTextCalls)
	}
	// CallLog leads with Status, the requireLiveStrand liveness check, before the two played inputs land in order.
	if len(reed.CallLog) != 3 || reed.CallLog[0] != "Status" || reed.CallLog[1] != "SendKey:C-u" || reed.CallLog[2] != "SendText:go test ./..." {
		t.Errorf("CallLog = %v, want [Status, SendKey:C-u, SendText:go test ./...]", reed.CallLog)
	}
}

func TestRunner_Inject_DeadStrand_Refuses(t *testing.T) {
	tiergate.Require(t, 0)
	// Inject does not require the pane to show an input-ready TUI.
	// It requires only that the strand is live (requireLiveStrand), so a dead pane must still refuse.
	reed := &fakeReed{StatusQueue: liveStrandStatus(false)}
	runner := newFixture(t, reed, &fakeEngine{}, withStrand("strand-1")).Runner

	if err := runner.Inject("strand-1", []PaneInput{{Key: "C-u"}}); err == nil {
		t.Error("Inject() = nil error, want a dead-strand refusal")
	}
	if len(reed.SendKeyCalls) != 0 || len(reed.SendTextCalls) != 0 {
		t.Errorf("keys reached the pane despite refusal: SendKey=%+v SendText=%+v", reed.SendKeyCalls, reed.SendTextCalls)
	}
}

func TestRunner_Inject_UnknownGUID_RefusesBeforeTouchingReed(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	// With no run seeded, FindRun must fail before Status, SendKey or SendText is called.
	runner := newFixture(t, reed, &fakeEngine{}).Runner

	if err := runner.Inject("does-not-exist", []PaneInput{{Key: "C-u"}}); err == nil {
		t.Error("Inject() = nil error, want an unknown-guid refusal")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed was touched despite an unresolvable guid: CallLog = %v", reed.CallLog)
	}
}

func TestRunner_Inject_RefusesAnyInterruptKey(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		inputs []PaneInput
		wantIn string
	}{
		{"a lone Escape", []PaneInput{{Key: "Escape"}}, ""},
		{"an Escape with a settle", []PaneInput{{Key: "Escape", SettleMS: 200}}, ""},
		{"two Escapes", []PaneInput{{Key: "Escape"}, {Key: "Escape"}}, ""},
		{"an Escape between other inputs", []PaneInput{{Key: "C-u"}, {Key: "Escape"}, {Text: "go test ./...", Submit: true}}, ""},
		{"the C-[ spelling", []PaneInput{{Key: "C-["}}, ""},
		{"a lowercase spelling", []PaneInput{{Key: "escape"}}, ""},
		{"an ESC byte in text", []PaneInput{{Text: "\x1b"}}, "control character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			runner := newFixture(t, reed, &fakeEngine{}, withStrand("strand-1")).Runner

			err := runner.Inject("strand-1", tt.inputs)
			if err == nil || !strings.Contains(err.Error(), "lym creel interrupt") {
				t.Fatalf("Inject() error = %v; want a refusal naming \"lym creel interrupt\"", err)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Inject() error = %v; want it to contain %q", err, tt.wantIn)
			}
			if len(reed.CallLog) != 0 || len(reed.SendKeyCalls) != 0 || len(reed.SendTextCalls) != 0 {
				t.Errorf("reed was touched despite the refusal: CallLog=%v SendKey=%+v SendText=%+v", reed.CallLog, reed.SendKeyCalls, reed.SendTextCalls)
			}
		})
	}
}

func TestRunner_Inject_EmptyInputs_IsARejectedNoOp(t *testing.T) {
	tiergate.Require(t, 0)
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newFixture(t, reed, &fakeEngine{}, withStrand("strand-1")).Runner

	if err := runner.Inject("strand-1", nil); err == nil {
		t.Error("Inject(nil) = nil error, want empty-inputs to be rejected as a no-op")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed was touched despite empty inputs: CallLog = %v", reed.CallLog)
	}
}
