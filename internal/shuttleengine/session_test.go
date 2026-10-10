// session_test.go covers Runner's session surface: ReadEvents' offset rules and the SessionCycler-backed ContextTokens, SessionIdle, ClearSession and CompactSession, including the plain-engine refusals.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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

// TestRunner_SessionIdle_ReturnsScriptedClassification drives Runner.SessionIdle over a fake reed and an engine with the signal parser, the cycler and the box reader.
// Each row is a series of probes of one strand on one runner, each appending events and moving the clock before it reads one pane capture:
// a ready, held or unknown readiness reading and its fallback to the pane, a draft, a pane too short, a hook turn end beside a pane needle, a turn start held then released by a later turn end, an interrupt report and the idle override across polls.
// An engine without the signal parser falls back to the pane probe, whose TooShort is reported only for a pane that is not idle.
//
// It is not parallel: logcapture redirects the process-global logger.
func TestRunner_SessionIdle_ReturnsScriptedClassification(t *testing.T) {
	type probe struct {
		// events is appended to the run's events file before the probe.
		events string
		// advance moves the fake clock before the probe.
		advance time.Duration
		capture string
		want    IdleProbe
		// wantReason is a substring of the held reason; empty when the probe is not held by the reading.
		wantReason string
	}
	const (
		idleFrame   = "IDLE\n❯ "
		needleFrame = "working (esc to interrupt)\n❯ "
		turnEnded   = "START\nSTOP:done\n"
	)
	shell := BackgroundTask{Kind: BackgroundShell, ID: "bsh1", Label: "sleep 600", Signal: SignalPayload}
	tests := []struct {
		name string
		// engine settings; the zero value is a live process.
		dead, unproven, interrupted, tooShort bool
		interactive, outputsExist             bool
		outstanding                           []BackgroundTask
		probes                                []probe
		// wantLog is a substring the probes' log must hold.
		wantLog string
	}{
		{name: "idle-done is ready", outputsExist: true, probes: []probe{{events: turnEnded, capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{name: "idle-stalled is ready", probes: []probe{{events: turnEnded, capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{name: "asking at an awaited turn end is ready", interactive: true, probes: []probe{{events: turnEnded, capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{name: "busy on background work is ready", outstanding: []BackgroundTask{shell}, probes: []probe{{events: "START\nWAIT:bg\n", capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{name: "a running turn is held", probes: []probe{{events: "START\n", capture: idleFrame, want: IdleProbe{Reason: "a turn is running"}, wantReason: "a turn is running"}}},
		{name: "an ask is held", probes: []probe{{events: "START\nASK\n", capture: idleFrame, want: IdleProbe{Reason: "the session waits on an answer"}, wantReason: "waits on an answer"}}},
		{name: "a dead process is held", dead: true, probes: []probe{{events: turnEnded, capture: idleFrame, want: IdleProbe{Reason: "the session's process is gone"}, wantReason: "process is gone"}}},
		{name: "an unknown reading falls back to an idle pane", unproven: true, probes: []probe{{events: turnEnded, capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{name: "an unknown reading falls back to a busy pane, too short or not", unproven: true, tooShort: true, probes: []probe{{events: turnEnded, capture: needleFrame, want: IdleProbe{TooShort: true}}}},
		{name: "a draft in the box is held with its reason", probes: []probe{{events: turnEnded, capture: "IDLE\n❯ half a draft", want: IdleProbe{Reason: `the input box holds a draft: "half a draft"`}, wantReason: "draft"}}},
		{name: "a pane too short is held and reported too short", tooShort: true, probes: []probe{{events: turnEnded, capture: "IDLE", want: IdleProbe{TooShort: true, Reason: readinessReasonPaneTooShort}, wantReason: "too short"}}},
		{
			name:    "a hook turn end beside a pane needle reads idle and logs the disagreement",
			probes:  []probe{{events: turnEnded, capture: needleFrame, want: IdleProbe{Idle: true}}},
			wantLog: "readiness reads ready beside a pane that does not show idle",
		},
		{name: "a hook turn end beside a pane with no box reads idle", probes: []probe{{events: turnEnded, capture: "no box here", want: IdleProbe{Idle: true}}}},
		{
			name: "a turn start with no turn end is held, then released by a later turn end",
			probes: []probe{
				{events: "START\n", capture: idleFrame, want: IdleProbe{Reason: "a turn is running"}, wantReason: "a turn is running"},
				{events: "STOP:done\n", capture: idleFrame, want: IdleProbe{Idle: true}},
			},
		},
		{name: "a reported interrupt releases the turn start", interrupted: true, probes: []probe{{events: "START\n", capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{
			name: "the idle override releases a turn start the pane has read idle across polls",
			probes: []probe{
				{events: "START\n", capture: idleFrame, want: IdleProbe{Reason: "a turn is running"}, wantReason: "a turn is running"},
				{advance: turnStartIdleOverride, capture: idleFrame, want: IdleProbe{Idle: true}},
			},
			wantLog: "released an unmatched turn start",
		},
		{name: "a session start after a turn end leaves it idle", probes: []probe{{events: turnEnded + "SESSIONSTART\n", capture: idleFrame, want: IdleProbe{Idle: true}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			logger.SetVerbosity(2)
			clock := newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))
			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			engine := &readinessEngine{tooShort: tt.tooShort}
			engine.StartupScript = []StartupState{StartupReady}
			engine.liveness = LivenessAlive
			if tt.dead {
				engine.liveness = LivenessDead
			}
			if tt.unproven {
				engine.liveness = LivenessUnproven
			}
			engine.interrupted = tt.interrupted
			engine.outstanding = tt.outstanding
			fx := newFixture(t, reed, engine, withStrand("strand-1"), withClock(clock))
			runDir := filepath.Dir(fx.EventsPath)
			state := RunState{RunID: "run-1", StrandGUID: "strand-1", EventsPath: fx.EventsPath, Interactive: tt.interactive}
			if tt.outputsExist {
				output := filepath.Join(runDir, "report.md")
				if err := os.WriteFile(output, []byte("done"), 0o644); err != nil {
					t.Fatal(err)
				}
				state.OutputFiles = []string{output}
			}
			if err := saveRunState(runDir, state); err != nil {
				t.Fatalf("saveRunState: %v", err)
			}

			events := ""
			for i, p := range tt.probes {
				events += p.events
				if err := os.WriteFile(fx.EventsPath, []byte(events), 0o644); err != nil {
					t.Fatal(err)
				}
				clock.Sleep(p.advance)
				reed.CaptureQueue = []string{p.capture}
				got, err := fx.Runner.SessionIdle("strand-1")
				if err != nil || got != p.want {
					t.Errorf("probe %d: SessionIdle = %+v, %v; want %+v, nil", i, got, err, p.want)
				}
				if p.wantReason != "" && !strings.Contains(got.Reason, p.wantReason) {
					t.Errorf("probe %d: reason %q does not contain %q", i, got.Reason, p.wantReason)
				}
			}
			if tt.wantLog != "" && !strings.Contains(buf.String(), tt.wantLog) {
				t.Errorf("log does not contain %q: %s", tt.wantLog, buf.String())
			}
		})
	}

	t.Run("an engine without the signal parser reads the pane", func(t *testing.T) {
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
	})
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

// TestRunner_TypeColor_PlaysTheSequenceAndWaitsItOut plays the engine's color sequence into a live shuttle strand and drives the wait that follows through an engine with the InputBoxReader capability.
// A box empty at once ends the wait on the first read,
// a command lingering in the input box is waited out without an extra key,
// and one that never leaves the box is submitted again by Enters each followed by one read until the submit window closes, and only logged, never failing the call.
// Start's color step shares the same helper.
// An unknown strand is refused before reed is touched.
// It is not parallel: each row replaces the package-level inputSleep and captures the global logger.
func TestRunner_TypeColor_PlaysTheSequenceAndWaitsItOut(t *testing.T) {
	const command = "COLOR:green"
	const settle = 300 * time.Millisecond
	poll := []string{"Sleep:" + colorSettleInterval.String(), "CapturePane"}
	enterThenRead := []string{"SendKey:Enter", "CapturePane"}
	join := func(parts ...[]string) []string {
		var out []string
		for _, part := range parts {
			out = append(out, part...)
		}
		return out
	}
	repeat := func(part []string, n int) []string {
		var out []string
		for range n {
			out = append(out, part...)
		}
		return out
	}
	prefix := []string{"Status", "SendText:" + command, "CapturePane"}

	tests := []struct {
		name    string
		boxes   []inputBoxAnswer
		wantLog []string
		// wantEnterReads is whether wantLog is followed by one or more enterThenRead pairs, their count set by the submit window.
		wantEnterReads bool
		wantWarn       string
	}{
		{
			name:    "empty box ends the wait at once",
			boxes:   []inputBoxAnswer{{"", true}},
			wantLog: prefix,
		},
		{
			name:    "lingering command is waited out",
			boxes:   []inputBoxAnswer{{command, true}, {command, true}, {"", true}},
			wantLog: join(prefix, poll, poll),
		},
		{
			name:           "command that never leaves the box only logs",
			boxes:          []inputBoxAnswer{{command, true}},
			wantLog:        join(prefix, repeat(poll, colorSettleAttempts-1)),
			wantEnterReads: true,
			wantWarn:       "the segment color command stayed in the input box",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			orig := inputSleep
			inputSleep = func(d time.Duration) {
				reed.mu.Lock()
				defer reed.mu.Unlock()
				reed.CallLog = append(reed.CallLog, "Sleep:"+d.String())
			}
			t.Cleanup(func() { inputSleep = orig })
			engine := &inputBoxEngine{fakeEngine: readyAgentEngine(), boxes: tt.boxes, settle: settle}
			runner := newFixture(t, reed, engine, withStrand("strand-1"), withClock(newFakeClock(time.Unix(0, 0)))).Runner

			if err := runner.TypeColor("strand-1", segmentcolor.Green); err != nil {
				t.Fatalf("TypeColor: %v, want nil since the color is display only", err)
			}
			gotLog := reed.CallLog
			if tt.wantEnterReads && len(gotLog) > len(tt.wantLog) {
				enterReads := gotLog[len(tt.wantLog):]
				if len(enterReads)%len(enterThenRead) != 0 || !reflect.DeepEqual(enterReads, repeat(enterThenRead, len(enterReads)/len(enterThenRead))) {
					t.Errorf("CallLog after the polls = %v, want only %v pairs", enterReads, enterThenRead)
				}
				gotLog = gotLog[:len(tt.wantLog)]
			} else if tt.wantEnterReads {
				t.Errorf("CallLog = %v, want %v followed by %v pairs", gotLog, tt.wantLog, enterThenRead)
			}
			if !reflect.DeepEqual(gotLog, tt.wantLog) {
				t.Errorf("CallLog = %v, want %v", gotLog, tt.wantLog)
			}
			got := logs.String()
			if tt.wantWarn == "" && strings.Contains(got, "segment color command") {
				t.Errorf("log = %q, want no color warning", got)
			}
			if tt.wantWarn != "" && !strings.Contains(got, tt.wantWarn) {
				t.Errorf("log = %q, want it to name %q", got, tt.wantWarn)
			}
		})
	}

	refusedReed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	refused := newFixture(t, refusedReed, &fakeEngine{}, withStrand("strand-1")).Runner
	if err := refused.TypeColor("nope", segmentcolor.Green); err == nil {
		t.Error("TypeColor(unknown guid) = nil error")
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
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"❯ ", "❯ ", "❯ LOAD:a,b"}}
	want := SkillLoadReport{Verified: true, Loaded: []string{"a"}, Missing: []string{"b"}}
	engine := &skillFakeEngine{fakeEngine: readyAgentEngine(), Reports: []SkillLoadReport{want}}
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
