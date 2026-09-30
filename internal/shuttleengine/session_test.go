// session_test.go covers Runner's session surface: ReadEvents' offset rules and the SessionCycler-backed ContextTokens, SessionIdle and ClearSession, including the plain-engine refusals.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// cyclerEngine is a fakeEngine that also implements SessionCycler with scripted answers.
type cyclerEngine struct {
	fakeEngine
	tokens   int
	known    bool
	idle     bool
	clear    []PaneInput
	captures []string
}

func (e *cyclerEngine) ContextTokens(Event) (int, bool) { return e.tokens, e.known }
func (e *cyclerEngine) IdleSession(capture string) bool {
	e.captures = append(e.captures, capture)
	return e.idle
}
func (e *cyclerEngine) ClearSessionSequence() []PaneInput { return e.clear }

// newSessionTestRunner seeds a run for guid whose events file is eventsPath (returned) and returns a Runner over reed/engine.
func newSessionTestRunner(t *testing.T, reed ReedOps, engine Engine, guid string) (*Runner, string) {
	t.Helper()
	worktreeRoot := t.TempDir()
	anchorPath := filepath.Join(worktreeRoot, "sub")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor: %v", err)
	}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}
	runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
	runDir := filepath.Join(runDirRoot(cfg, anchorPath), "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if err := saveRunState(runDir, RunState{RunID: "run-1", StrandGUID: guid, EventsPath: eventsPath}); err != nil {
		t.Fatalf("saveRunState: %v", err)
	}
	return runner, eventsPath
}

func TestRunner_ReadEvents_AdvancesPastCompleteLinesOnly(t *testing.T) {
	runner, eventsPath := newSessionTestRunner(t, &fakeReed{}, &fakeEngine{}, "strand-1")
	full := "STOP:one\nSTOP:two\n"
	if err := os.WriteFile(eventsPath, []byte(full+"STOP:par"), 0o644); err != nil {
		t.Fatal(err)
	}

	events, off, err := runner.ReadEvents("strand-1", 0)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 2 || events[0].Message != "one" || events[1].Message != "two" {
		t.Errorf("events = %+v, want one and two", events)
	}
	if off != int64(len(full)) {
		t.Errorf("offset = %d, want %d", off, len(full))
	}

	events, off2, err := runner.ReadEvents("strand-1", off)
	if err != nil || len(events) != 0 || off2 != off {
		t.Errorf("re-read = %+v, %d, %v; want no events and offset %d", events, off2, err, off)
	}
}

func TestRunner_ReadEvents_AbsentFileKeepsOffset(t *testing.T) {
	runner, _ := newSessionTestRunner(t, &fakeReed{}, &fakeEngine{}, "strand-1")
	events, off, err := runner.ReadEvents("strand-1", 7)
	if err != nil || len(events) != 0 || off != 7 {
		t.Errorf("ReadEvents = %+v, %d, %v; want none, 7, nil", events, off, err)
	}
}

func TestRunner_ReadEvents_UnknownGUID(t *testing.T) {
	runner, _ := newSessionTestRunner(t, &fakeReed{}, &fakeEngine{}, "strand-1")
	if _, _, err := runner.ReadEvents("nope", 0); err == nil {
		t.Error("ReadEvents(unknown guid) = nil error")
	}
}

func TestRunner_SessionMethods_ErrorOnPlainEngine(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner, _ := newSessionTestRunner(t, reed, &fakeEngine{}, "strand-1")

	if _, _, err := runner.ContextTokens(Event{}); err == nil {
		t.Error("ContextTokens on a plain engine = nil error")
	}
	if _, err := runner.SessionIdle("strand-1"); err == nil {
		t.Error("SessionIdle on a plain engine = nil error")
	}
	if err := runner.ClearSession("strand-1"); err == nil {
		t.Error("ClearSession on a plain engine = nil error")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed touched despite missing capability: %v", reed.CallLog)
	}
}

func TestRunner_ContextTokens_Delegates(t *testing.T) {
	engine := &cyclerEngine{tokens: 1234, known: true}
	runner, _ := newSessionTestRunner(t, &fakeReed{}, engine, "strand-1")
	tokens, known, err := runner.ContextTokens(Event{})
	if err != nil || tokens != 1234 || !known {
		t.Errorf("ContextTokens = %d, %v, %v; want 1234, true, nil", tokens, known, err)
	}
}

func TestRunner_SessionIdle_DeadStrandErrors(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(false)}
	runner, _ := newSessionTestRunner(t, reed, &cyclerEngine{idle: true}, "strand-1")
	if _, err := runner.SessionIdle("strand-1"); err == nil {
		t.Error("SessionIdle on a dead strand = nil error")
	}
}

func TestRunner_SessionIdle_ReturnsScriptedClassification(t *testing.T) {
	for _, want := range []bool{true, false} {
		reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"the pane"}}
		engine := &cyclerEngine{idle: want}
		runner, _ := newSessionTestRunner(t, reed, engine, "strand-1")
		got, err := runner.SessionIdle("strand-1")
		if err != nil || got != want {
			t.Errorf("SessionIdle = %v, %v; want %v, nil", got, err, want)
		}
		if !reflect.DeepEqual(engine.captures, []string{"the pane"}) {
			t.Errorf("classified captures = %v, want [the pane]", engine.captures)
		}
	}
}

func TestRunner_ClearSession_PlaysScriptedSequence(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	engine := &cyclerEngine{clear: []PaneInput{{Key: "Escape"}, {Text: "/clear", Submit: true}}}
	runner, _ := newSessionTestRunner(t, reed, engine, "strand-1")

	if err := runner.ClearSession("strand-1"); err != nil {
		t.Fatalf("ClearSession: %v", err)
	}
	want := []string{"Status", "SendKey:Escape", "SendText:/clear"}
	if !reflect.DeepEqual(reed.CallLog, want) {
		t.Errorf("CallLog = %v, want %v", reed.CallLog, want)
	}
}

func TestRunner_ClearSession_UnknownGUID(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner, _ := newSessionTestRunner(t, reed, &cyclerEngine{}, "strand-1")
	if err := runner.ClearSession("nope"); err == nil {
		t.Error("ClearSession(unknown guid) = nil error")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed touched: %v", reed.CallLog)
	}
}
