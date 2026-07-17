// run_inject_test.go covers Runner.Inject: the happy path plays every input
// in order through the mux seam, a dead strand's pane refuses delivery, an
// unknown guid refuses before ever touching mux, and empty inputs is a
// rejected no-op.

package shuttleengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// newInjectTestRunner returns a Runner over mux/engine, with a run seeded
// under its run-dir root (seedRun) so FindRun can resolve guid — Inject's
// out-of-process entry point, unlike (*Run).Interrupt/Send, has no in-process
// Run handle to draw StrandGUID from and must resolve it from run.json.
func newInjectTestRunner(t *testing.T, mux MuxOps, engine Engine, guid string) *Runner {
	t.Helper()
	root := t.TempDir()
	layout := &hubgeometry.Layout{Cwd: root, WorktreeRoot: root}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}
	runner := NewRunner(mux, engine, layout, cfg)
	if guid != "" {
		seedRun(t, runDirRoot(cfg, layout), "run-1", guid)
	}
	return runner
}

func TestRunner_Inject_HappyPath_PlaysEveryInputInOrder(t *testing.T) {
	mux := &fakeMux{StatusQueue: liveStrandStatus(true)}
	runner := newInjectTestRunner(t, mux, &fakeEngine{}, "strand-1")

	inputs := []PaneInput{
		{Key: "Escape"},
		{Text: "go test ./...", Submit: true},
	}
	if err := runner.Inject("strand-1", inputs); err != nil {
		t.Fatalf("Inject() error: %v", err)
	}

	// playInputs' own ordering is proven by run_test.go's Send/Interrupt
	// coverage; here the assertion is that Inject reached playInputs at all,
	// with every step delivered, in order, through the mux seam.
	if len(mux.SendKeyCalls) != 1 || mux.SendKeyCalls[0].Key != "Escape" {
		t.Errorf("SendKey calls = %+v, want exactly one Escape", mux.SendKeyCalls)
	}
	if len(mux.SendTextCalls) != 1 || mux.SendTextCalls[0].Text != "go test ./..." || !mux.SendTextCalls[0].Submit {
		t.Errorf("SendText calls = %+v, want one submitted \"go test ./...\"", mux.SendTextCalls)
	}
	// CallLog leads with Status (requireLiveStrand's liveness check) before
	// the two played inputs land, in order.
	if len(mux.CallLog) != 3 || mux.CallLog[0] != "Status" || mux.CallLog[1] != "SendKey:Escape" || mux.CallLog[2] != "SendText:go test ./..." {
		t.Errorf("CallLog = %v, want [Status, SendKey:Escape, SendText:go test ./...]", mux.CallLog)
	}
}

func TestRunner_Inject_DeadStrand_Refuses(t *testing.T) {
	// Unlike Send/Interrupt's requireReadyAgentPane, Inject deliberately does
	// NOT require the pane to show an input-ready TUI — only that the strand
	// is live at all (requireLiveStrand). A dead pane must still refuse.
	mux := &fakeMux{StatusQueue: liveStrandStatus(false)}
	runner := newInjectTestRunner(t, mux, &fakeEngine{}, "strand-1")

	if err := runner.Inject("strand-1", []PaneInput{{Key: "Escape"}}); err == nil {
		t.Error("Inject() = nil error, want a dead-strand refusal")
	}
	if len(mux.SendKeyCalls) != 0 || len(mux.SendTextCalls) != 0 {
		t.Errorf("keys reached the pane despite refusal: SendKey=%+v SendText=%+v", mux.SendKeyCalls, mux.SendTextCalls)
	}
}

func TestRunner_Inject_UnknownGUID_RefusesBeforeTouchingMux(t *testing.T) {
	mux := &fakeMux{StatusQueue: liveStrandStatus(true)}
	// No run seeded at all: FindRun must fail before Status/SendKey/SendText
	// are ever called.
	runner := newInjectTestRunner(t, mux, &fakeEngine{}, "")

	if err := runner.Inject("does-not-exist", []PaneInput{{Key: "Escape"}}); err == nil {
		t.Error("Inject() = nil error, want an unknown-guid refusal")
	}
	if len(mux.CallLog) != 0 {
		t.Errorf("mux was touched despite an unresolvable guid: CallLog = %v", mux.CallLog)
	}
}

func TestRunner_Inject_EmptyInputs_IsARejectedNoOp(t *testing.T) {
	mux := &fakeMux{StatusQueue: liveStrandStatus(true)}
	runner := newInjectTestRunner(t, mux, &fakeEngine{}, "strand-1")

	if err := runner.Inject("strand-1", nil); err == nil {
		t.Error("Inject(nil) = nil error, want empty-inputs to be rejected as a no-op")
	}
	if len(mux.CallLog) != 0 {
		t.Errorf("mux was touched despite empty inputs: CallLog = %v", mux.CallLog)
	}
}
