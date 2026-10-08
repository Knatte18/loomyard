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
	idle     bool
	tooShort bool
	clear    []PaneInput
	captures []string
	foci     []string
}

func (e *cyclerEngine) ContextTokens(Event) ContextReading { return ContextReading{} }
func (e *cyclerEngine) CompactedSince(Event, time.Time) (CompactionBoundary, bool) {
	return CompactionBoundary{}, false
}
func (e *cyclerEngine) IdleSession(capture string) bool {
	e.captures = append(e.captures, capture)
	return e.idle
}
func (e *cyclerEngine) PaneTooShort(string) bool          { return e.tooShort }
func (e *cyclerEngine) ClearSessionSequence() []PaneInput { return e.clear }
func (e *cyclerEngine) ReloadPluginsSequence() []PaneInput {
	return []PaneInput{{Text: "/reload-plugins", Submit: true}}
}
func (e *cyclerEngine) CompactSessionSequence(focus string) []PaneInput {
	e.foci = append(e.foci, focus)
	return []PaneInput{{Text: "/compact " + focus, Submit: true}}
}

// TestRunner_ReadEvents covers ReadEvents' offset rules: it advances past complete lines only, so a
// partial trailing line is left for the next read; an absent events file keeps the caller's offset;
// and an unknown strand guid is an error.
func TestRunner_ReadEvents(t *testing.T) {
	const complete = "STOP:one\nSTOP:two\n"
	tests := []struct {
		name string
		// events is the events file's seed; nil leaves it absent.
		events      *string
		guid        string
		fromOffset  int64
		wantMessage []string
		wantOffset  int64
		wantErr     bool
	}{
		{
			name: "advances past complete lines only", events: ptrTo(complete + "STOP:par"), guid: "strand-1",
			wantMessage: []string{"one", "two"}, wantOffset: int64(len(complete)),
		},
		{name: "an absent file keeps the offset", guid: "strand-1", fromOffset: 7, wantOffset: 7},
		{name: "an unknown guid is an error", guid: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t, &fakeReed{}, &fakeEngine{}, withStrand("strand-1"))
			runner, eventsPath := fx.Runner, fx.EventsPath
			if tt.events != nil {
				if err := os.WriteFile(eventsPath, []byte(*tt.events), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			events, off, err := runner.ReadEvents(tt.guid, tt.fromOffset)
			if tt.wantErr {
				if err == nil {
					t.Error("ReadEvents(unknown guid) = nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			var got []string
			for _, event := range events {
				got = append(got, event.Message)
			}
			if !reflect.DeepEqual(got, tt.wantMessage) {
				t.Errorf("events = %+v, want messages %v", events, tt.wantMessage)
			}
			if off != tt.wantOffset {
				t.Errorf("offset = %d, want %d", off, tt.wantOffset)
			}

			// A re-read from the returned offset finds nothing new and leaves the offset where it is.
			again, off2, err := runner.ReadEvents(tt.guid, off)
			if err != nil || len(again) != 0 || off2 != off {
				t.Errorf("re-read = %+v, %d, %v; want no events and offset %d", again, off2, err, off)
			}
		})
	}
}

// ptrTo returns a pointer to value, for table rows that distinguish an absent string from an empty one.
func ptrTo[T any](value T) *T { return &value }

func TestRunner_SessionMethods_ErrorOnPlainEngine(t *testing.T) {
	t.Parallel()

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
	err = runner.ReloadPlugins("strand-1")
	if err == nil || !strings.Contains(err.Error(), "ReloadPluginsSequence") {
		t.Errorf("ReloadPlugins on a plain engine = %v, want an error naming the capability", err)
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed touched despite missing capability: %v", reed.CallLog)
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

func TestRunner_ReloadPlugins_PlaysSequenceOnALiveShuttleStrand(t *testing.T) {
	t.Parallel()

	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newFixture(t, reed, &cyclerEngine{}, withStrand("strand-1")).Runner

	if err := runner.ReloadPlugins("strand-1"); err != nil {
		t.Fatalf("ReloadPlugins: %v", err)
	}
	want := []string{"Status", "SendText:/reload-plugins"}
	if !reflect.DeepEqual(reed.CallLog, want) {
		t.Errorf("CallLog = %v, want %v", reed.CallLog, want)
	}

	refusedReed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	refused := newFixture(t, refusedReed, &cyclerEngine{}, withStrand("strand-1")).Runner
	if err := refused.ReloadPlugins("nope"); err == nil {
		t.Error("ReloadPlugins(unknown guid) = nil error")
	}
	if len(refusedReed.CallLog) != 0 {
		t.Errorf("reed touched: %v", refusedReed.CallLog)
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

func TestRunner_LoadSkills_GuardStrands(t *testing.T) {
	t.Parallel()
	refused := []struct {
		name string
		guid string
		live bool
	}{
		{"unknown guid", "nope", true},
		{"dead strand", "strand-1", false},
	}
	for _, tc := range refused {
		reed := &fakeReed{StatusQueue: liveStrandStatus(tc.live)}
		runner := newFixture(t, reed, &skillFakeEngine{fakeEngine: &fakeEngine{}}, withStrand("strand-1")).Runner
		if err := runner.LoadSkills(tc.guid, []string{"a", "b"}); err == nil {
			t.Errorf("%s: LoadSkills = nil error", tc.name)
		}
		if len(reed.SendTextCalls) != 0 {
			t.Errorf("%s: typed %+v; want nothing", tc.name, reed.SendTextCalls)
		}
	}
}

func TestRunner_LoadSkillsAndClassifySkillLoad_DriveTheEngine(t *testing.T) {
	t.Parallel()
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"❯ ", "❯ LOAD:a,b"}}
	want := SkillLoadReport{Verified: true, Loaded: []string{"a"}, Missing: []string{"b"}}
	engine := &skillFakeEngine{fakeEngine: &fakeEngine{}, Reports: []SkillLoadReport{want}}
	runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner
	if err := runner.LoadSkills("strand-1", []string{"a", "b"}); err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}
	if len(reed.SendTextCalls) != 1 || reed.SendTextCalls[0].Text != "LOAD:a,b" || !reed.SendTextCalls[0].Submit {
		t.Errorf("SendTextCalls = %+v; want the engine's one-turn load message", reed.SendTextCalls)
	}
	got, err := runner.ClassifySkillLoad(Event{Kind: EventStop}, []string{"a", "b"})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ClassifySkillLoad = %+v, %v; want %+v, nil", got, err, want)
	}
}

func TestRunner_LoadSkillsAndClassifySkillLoad_ErrorOnPlainEngine(t *testing.T) {
	runner := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &fakeEngine{}, withStrand("strand-1")).Runner
	if err := runner.LoadSkills("strand-1", []string{"a"}); err == nil || !strings.Contains(err.Error(), "SkillLoader") {
		t.Errorf("LoadSkills error = %v; want one naming SkillLoader", err)
	}
	if _, err := runner.ClassifySkillLoad(Event{Kind: EventStop}, []string{"a"}); err == nil || !strings.Contains(err.Error(), "SkillLoader") {
		t.Errorf("ClassifySkillLoad error = %v; want one naming SkillLoader", err)
	}
}

// TestRunner_SessionState covers the state read of one run by guid: the found run reads at the runner's clock, and an unknown guid is an error.
func TestRunner_SessionState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)
	runner := newFixture(t, &fakeReed{}, &fakeEngine{}, withStrand("strand-1"), withClock(&frozenClock{now: now})).Runner

	got, err := runner.SessionState("strand-1")
	if err != nil {
		t.Fatalf("SessionState(found guid): %v", err)
	}
	if got.StrandGUID != "strand-1" || got.StrandName != "strand-1" || got.State.Name != SessionUnknown || got.State.Cause != SessionCauseUnsupported || !got.State.Since.Equal(now) {
		t.Errorf("SessionState = %+v; want strand-1 unknown/unsupported since %v", got, now)
	}

	if _, err := runner.SessionState("nope"); err == nil {
		t.Error("SessionState(unknown guid) = nil error")
	}
}
