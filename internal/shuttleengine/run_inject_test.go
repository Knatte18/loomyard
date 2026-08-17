// run_inject_test.go covers Runner.Inject: the happy path plays every input in order through the
// reed seam, a dead strand's pane refuses delivery, an unknown guid refuses before ever touching
// reed, and empty inputs is a rejected no-op.

package shuttleengine

import (
	"testing"
)

// newInjectTestRunner returns a Runner over reed/engine, with a run seeded
// under its run-dir root (seedRun) so FindRun can resolve guid — Inject's
// out-of-process entry point, unlike (*Run).Interrupt/Send, has no in-process
// Run handle to draw StrandGUID from and must resolve it from run.json.
func newInjectTestRunner(t *testing.T, reed ReedOps, engine Engine, guid string) *Runner {
	t.Helper()
	root := t.TempDir()
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}
	runner := NewRunner(reed, engine, root, root, cfg)
	if guid != "" {
		seedRun(t, runDirRoot(cfg, root), "run-1", guid)
	}
	return runner
}

func TestRunner_Inject_HappyPath_PlaysEveryInputInOrder(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newInjectTestRunner(t, reed, &fakeEngine{}, "strand-1")

	inputs := []PaneInput{
		{Key: "Escape"},
		{Text: "go test ./...", Submit: true},
	}
	if err := runner.Inject("strand-1", inputs); err != nil {
		t.Fatalf("Inject() error: %v", err)
	}

	// playInputs' own ordering is proven by run_test.go's Send/Interrupt
	// coverage; here the assertion is that Inject reached playInputs at all,
	// with every step delivered, in order, through the reed seam.
	if len(reed.SendKeyCalls) != 1 || reed.SendKeyCalls[0].Key != "Escape" {
		t.Errorf("SendKey calls = %+v, want exactly one Escape", reed.SendKeyCalls)
	}
	if len(reed.SendTextCalls) != 1 || reed.SendTextCalls[0].Text != "go test ./..." || !reed.SendTextCalls[0].Submit {
		t.Errorf("SendText calls = %+v, want one submitted \"go test ./...\"", reed.SendTextCalls)
	}
	// CallLog leads with Status (requireLiveStrand's liveness check) before
	// the two played inputs land, in order.
	if len(reed.CallLog) != 3 || reed.CallLog[0] != "Status" || reed.CallLog[1] != "SendKey:Escape" || reed.CallLog[2] != "SendText:go test ./..." {
		t.Errorf("CallLog = %v, want [Status, SendKey:Escape, SendText:go test ./...]", reed.CallLog)
	}
}

func TestRunner_Inject_DeadStrand_Refuses(t *testing.T) {
	// Unlike Send/Interrupt's requireReadyAgentPane, Inject deliberately does
	// NOT require the pane to show an input-ready TUI — only that the strand
	// is live at all (requireLiveStrand). A dead pane must still refuse.
	reed := &fakeReed{StatusQueue: liveStrandStatus(false)}
	runner := newInjectTestRunner(t, reed, &fakeEngine{}, "strand-1")

	if err := runner.Inject("strand-1", []PaneInput{{Key: "Escape"}}); err == nil {
		t.Error("Inject() = nil error, want a dead-strand refusal")
	}
	if len(reed.SendKeyCalls) != 0 || len(reed.SendTextCalls) != 0 {
		t.Errorf("keys reached the pane despite refusal: SendKey=%+v SendText=%+v", reed.SendKeyCalls, reed.SendTextCalls)
	}
}

func TestRunner_Inject_UnknownGUID_RefusesBeforeTouchingReed(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	// No run seeded at all: FindRun must fail before Status/SendKey/SendText
	// are ever called.
	runner := newInjectTestRunner(t, reed, &fakeEngine{}, "")

	if err := runner.Inject("does-not-exist", []PaneInput{{Key: "Escape"}}); err == nil {
		t.Error("Inject() = nil error, want an unknown-guid refusal")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed was touched despite an unresolvable guid: CallLog = %v", reed.CallLog)
	}
}

func TestRunner_Inject_EmptyInputs_IsARejectedNoOp(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newInjectTestRunner(t, reed, &fakeEngine{}, "strand-1")

	if err := runner.Inject("strand-1", nil); err == nil {
		t.Error("Inject(nil) = nil error, want empty-inputs to be rejected as a no-op")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed was touched despite empty inputs: CallLog = %v", reed.CallLog)
	}
}
