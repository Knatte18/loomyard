// session_test.go covers Runner's session surface: ReadEvents' offset rules and the SessionCycler-backed ContextTokens, SessionIdle, ClearSession and CompactSession, including the plain-engine refusals.

package shuttleengine

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// cyclerEngine is a fakeEngine that also implements SessionCycler with scripted answers.
type cyclerEngine struct {
	fakeEngine
	tokens   int
	known    bool
	idle     bool
	tooShort bool
	clear    []PaneInput
	captures []string
	foci     []string

	compactedAt time.Time
	sinceAsks   []time.Time
}

func (e *cyclerEngine) ContextTokens(Event) ContextReading {
	return ContextReading{Tokens: e.tokens, Known: e.known}
}
func (e *cyclerEngine) CompactedSince(_ Event, since time.Time) (time.Time, bool) {
	e.sinceAsks = append(e.sinceAsks, since)
	return e.compactedAt, !e.compactedAt.IsZero()
}
func (e *cyclerEngine) IdleSession(capture string) bool {
	e.captures = append(e.captures, capture)
	return e.idle
}
func (e *cyclerEngine) PaneTooShort(string) bool          { return e.tooShort }
func (e *cyclerEngine) ClearSessionSequence() []PaneInput { return e.clear }
func (e *cyclerEngine) CompactSessionSequence(focus string) []PaneInput {
	e.foci = append(e.foci, focus)
	return []PaneInput{{Text: "/compact " + focus, Submit: true}}
}

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
	if _, _, err := runner.CompactedSince(Event{}, time.Time{}); err == nil || !strings.Contains(err.Error(), "SessionCycler") {
		t.Errorf("CompactedSince on a plain engine = %v, want an error naming SessionCycler", err)
	}
	if _, err := runner.SessionIdle("strand-1"); err == nil {
		t.Error("SessionIdle on a plain engine = nil error")
	}
	if err := runner.ClearSession("strand-1"); err == nil {
		t.Error("ClearSession on a plain engine = nil error")
	}
	err := runner.CompactSession("strand-1", "focus")
	if err == nil || !strings.Contains(err.Error(), "CompactSessionSequence") {
		t.Errorf("CompactSession on a plain engine = %v, want an error naming the capability", err)
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

func TestRunner_CompactedSince_Delegates(t *testing.T) {
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	since := at.Add(-time.Minute)
	engine := &cyclerEngine{compactedAt: at}
	runner := newFixture(t, &fakeReed{}, engine, withStrand("strand-1")).Runner
	got, found, err := runner.CompactedSince(Event{}, since)
	if err != nil || !found || !got.Equal(at) {
		t.Errorf("CompactedSince = %v, %v, %v; want %v, true, nil", got, found, err, at)
	}
	if len(engine.sinceAsks) != 1 || !engine.sinceAsks[0].Equal(since) {
		t.Errorf("engine asked %v, want one ask at %v", engine.sinceAsks, since)
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
	// TooShort is reported only for a pane that is not idle.
	for _, want := range []IdleProbe{{Idle: true}, {}, {TooShort: true}} {
		reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"the pane"}}
		engine := &cyclerEngine{idle: want.Idle, tooShort: want.TooShort || want.Idle}
		runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner
		got, err := runner.SessionIdle("strand-1")
		if err != nil || got != want {
			t.Errorf("SessionIdle = %+v, %v; want %+v, nil", got, err, want)
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

func TestRunner_CompactSession_PlaysSequenceForFocus(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	engine := &cyclerEngine{}
	runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner

	if err := runner.CompactSession("strand-1", "the plan"); err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if !reflect.DeepEqual(engine.foci, []string{"the plan"}) {
		t.Errorf("focus passed to the engine = %v, want [the plan]", engine.foci)
	}
	want := []string{"Status", "SendText:/compact the plan"}
	if !reflect.DeepEqual(reed.CallLog, want) {
		t.Errorf("CallLog = %v, want %v", reed.CallLog, want)
	}
}

func TestRunner_CompactSession_RefusesMultiLineFocus(t *testing.T) {
	for _, focus := range []string{"a\nb", "a\r\nb"} {
		reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
		engine := &cyclerEngine{}
		runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner
		if err := runner.CompactSession("strand-1", focus); err == nil {
			t.Errorf("CompactSession(%q) = nil error", focus)
		}
		if len(reed.CallLog) != 0 || len(engine.foci) != 0 {
			t.Errorf("played despite refusal: calls %v, foci %v", reed.CallLog, engine.foci)
		}
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

func TestRunner_LoadSkillAndSkillUnknown_DelegateToCapability(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"NOSKILL ghost"}}
	engine := &skillFakeEngine{fakeEngine: &fakeEngine{}}
	runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner

	if err := runner.LoadSkill("strand-1", "a"); err != nil {
		t.Fatalf("LoadSkill: %v", err)
	}
	if len(reed.SendTextCalls) != 1 || reed.SendTextCalls[0].Text != "LOAD:a" || !reed.SendTextCalls[0].Submit {
		t.Errorf("SendTextCalls = %+v; want the engine's load sequence", reed.SendTextCalls)
	}
	if unknown, err := runner.SkillUnknown("strand-1", "ghost"); err != nil || !unknown {
		t.Errorf("SkillUnknown(ghost) = %v, %v; want true, nil", unknown, err)
	}
	if unknown, err := runner.SkillUnknown("strand-1", "other"); err != nil || unknown {
		t.Errorf("SkillUnknown(other) = %v, %v; want false, nil", unknown, err)
	}
}

func TestRunner_LoadSkillAndSkillUnknown_ErrorOnPlainEngine(t *testing.T) {
	runner := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &fakeEngine{}, withStrand("strand-1")).Runner
	if err := runner.LoadSkill("strand-1", "a"); err == nil || !strings.Contains(err.Error(), "SkillLoader") {
		t.Errorf("LoadSkill error = %v; want one naming SkillLoader", err)
	}
	if _, err := runner.SkillUnknown("strand-1", "a"); err == nil || !strings.Contains(err.Error(), "SkillLoader") {
		t.Errorf("SkillUnknown error = %v; want one naming SkillLoader", err)
	}
}
