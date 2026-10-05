// session_test.go covers Runner's session surface: ReadEvents' offset rules and the SessionCycler-backed ContextTokens, SessionIdle and ClearSession, including the plain-engine refusals.

package shuttleengine

import (
	"os"
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

func (e *cyclerEngine) ContextTokens(Event) ContextReading {
	return ContextReading{Tokens: e.tokens, Known: e.known}
}
func (e *cyclerEngine) IdleSession(capture string) bool {
	e.captures = append(e.captures, capture)
	return e.idle
}
func (e *cyclerEngine) ClearSessionSequence() []PaneInput { return e.clear }

func TestRunner_ReadEvents_AdvancesPastCompleteLinesOnly(t *testing.T) {
	fx := newFixture(t, &fakeReed{}, &fakeEngine{}, withStrand("strand-1"))
	runner, eventsPath := fx.Runner, fx.EventsPath
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
	runner := newFixture(t, &fakeReed{}, &fakeEngine{}, withStrand("strand-1")).Runner
	events, off, err := runner.ReadEvents("strand-1", 7)
	if err != nil || len(events) != 0 || off != 7 {
		t.Errorf("ReadEvents = %+v, %d, %v; want none, 7, nil", events, off, err)
	}
}

func TestRunner_ReadEvents_UnknownGUID(t *testing.T) {
	runner := newFixture(t, &fakeReed{}, &fakeEngine{}, withStrand("strand-1")).Runner
	if _, _, err := runner.ReadEvents("nope", 0); err == nil {
		t.Error("ReadEvents(unknown guid) = nil error")
	}
}

func TestRunner_SessionMethods_ErrorOnPlainEngine(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newFixture(t, reed, &fakeEngine{}, withStrand("strand-1")).Runner

	if _, err := runner.ContextTokens(Event{}); err == nil {
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
	runner := newFixture(t, &fakeReed{}, engine, withStrand("strand-1")).Runner
	reading, err := runner.ContextTokens(Event{})
	if err != nil || reading.Tokens != 1234 || !reading.Known {
		t.Errorf("ContextTokens = %+v, %v; want 1234 known, nil", reading, err)
	}
}

func TestRunner_SessionIdle_DeadStrandErrors(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(false)}
	runner := newFixture(t, reed, &cyclerEngine{idle: true}, withStrand("strand-1")).Runner
	if _, err := runner.SessionIdle("strand-1"); err == nil {
		t.Error("SessionIdle on a dead strand = nil error")
	}
}

func TestRunner_SessionIdle_ReturnsScriptedClassification(t *testing.T) {
	for _, want := range []bool{true, false} {
		reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"the pane"}}
		engine := &cyclerEngine{idle: want}
		runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner
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
	runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner

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
	runner := newFixture(t, reed, &cyclerEngine{}, withStrand("strand-1")).Runner
	if err := runner.ClearSession("nope"); err == nil {
		t.Error("ClearSession(unknown guid) = nil error")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed touched: %v", reed.CallLog)
	}
}
