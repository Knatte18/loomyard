// session_test.go covers Runner's session surface: ReadEvents' offset rules and the SessionCycler-backed ContextTokens, SessionIdle, ClearSession and CompactSession, including the plain-engine refusals.

package shuttleengine

import (
	"errors"
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

// readyCyclerEngine returns a cyclerEngine whose pane starts ready and reads idle.
func readyCyclerEngine() *cyclerEngine {
	return &cyclerEngine{fakeEngine: fakeEngine{StartupScript: []StartupState{StartupReady}}, idle: true}
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

	if _, err := runner.ContextTokens("strand-1", Event{}); err == nil {
		t.Error("ContextTokens on a plain engine = nil error")
	}
	if _, _, err := runner.CompactedSince("strand-1", Event{}, time.Time{}); err == nil || !strings.Contains(err.Error(), "SessionCycler") {
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
		// active makes the fake reed report an operator active in the pane.
		active bool
		want   IdleProbe
		// wantReason is a substring of the held reason; empty when the probe is not held by the reading.
		wantReason string
	}
	const (
		idleFrame   = "IDLE\n❯ "
		needleFrame = "working (esc to interrupt)\n❯ "
		turnEnded   = "START\nSTOP:done\n"
		// suggestionFrame is an idle pane whose box shows only a placeholder, marked the way a styled capture marks it.
		suggestionFrame = "IDLE\n❯ ~Try something~"
	)
	shell := BackgroundTask{Kind: BackgroundShell, ID: "bsh1", Label: "sleep 600", Signal: SignalPayload}
	tests := []struct {
		name string
		// engine settings; the zero value is a live process.
		dead, unproven, interrupted, tooShort bool
		interactive, outputsExist             bool
		// styled gives the engine the styled box read, which blanks a capture's ~…~ spans.
		styled      bool
		outstanding []BackgroundTask
		// progress is what the engine reads of any turn start.
		progress TurnProgress
		probes   []probe
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
		{name: "a draft in the box is held with its reason", probes: []probe{{events: turnEnded, capture: "IDLE\n❯ half a draft", want: IdleProbe{Reason: `the input box holds a draft: "half a draft"`, Draft: true}, wantReason: "draft"}}},
		{name: "a placeholder in the box is idle for a styled box read", styled: true, probes: []probe{{events: turnEnded, capture: suggestionFrame, want: IdleProbe{Idle: true}}}},
		{name: "a placeholder in the box is a draft for a plain box read", probes: []probe{{events: turnEnded, capture: suggestionFrame, want: IdleProbe{Reason: `the input box holds a draft: "~Try something~"`, Draft: true}, wantReason: "draft"}}},
		{name: "an unknown reading over a placeholder falls back to an idle pane for a styled box read", unproven: true, styled: true, probes: []probe{{events: turnEnded, capture: suggestionFrame, want: IdleProbe{Idle: true}}}},
		{name: "an unknown reading falls back to a busy pane holding a draft", unproven: true, probes: []probe{{events: turnEnded, capture: "BUSY\n❯ half a draft", want: IdleProbe{Draft: true}}}},
		{name: "a pane too short is held and reported too short", tooShort: true, probes: []probe{{events: turnEnded, capture: "IDLE", want: IdleProbe{TooShort: true, Reason: readinessReasonPaneTooShort}, wantReason: "too short"}}},
		{
			name:    "a hook turn end beside a pane needle reads idle and logs the disagreement",
			probes:  []probe{{events: turnEnded, capture: needleFrame, want: IdleProbe{Idle: true}}},
			wantLog: "readiness reads ready beside a pane that does not show idle",
		},
		{name: "a pane with no readable box is held for an engine that reads one", probes: []probe{{events: turnEnded, capture: "no box here", want: IdleProbe{Reason: readinessReasonNoInputBox, NoInputBox: true}, wantReason: "no input box"}}},
		{name: "an active operator is held with the activity reason", probes: []probe{{events: turnEnded, capture: idleFrame, active: true, want: IdleProbe{Reason: "the operator typed in this pane within the last 5 s"}, wantReason: "operator typed"}}},
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
		{
			name:     "an in-flight turn start stays held past the idle override",
			progress: TurnInFlight,
			probes: []probe{
				{events: "START\n", capture: idleFrame, want: IdleProbe{Reason: "a turn is running"}, wantReason: "a turn is running"},
				{advance: turnStartIdleOverride, capture: idleFrame, want: IdleProbe{Reason: "a turn is running"}, wantReason: "a turn is running"},
			},
		},
		{name: "a session start after a turn end leaves it idle", probes: []probe{{events: turnEnded + "SESSIONSTART\n", capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{name: "idle-fresh with an empty box is ready", probes: []probe{{events: "SESSIONSTART:startup\n", capture: idleFrame, want: IdleProbe{Idle: true}}}},
		{name: "idle-fresh with a draft is held with its reason", probes: []probe{{events: "SESSIONSTART:clear\n", capture: "IDLE\n❯ half a draft", want: IdleProbe{Reason: `the input box holds a draft: "half a draft"`, Draft: true}, wantReason: "draft"}}},
		{name: "idle-fresh with a pane too short is held and reported too short", tooShort: true, probes: []probe{{events: "SESSIONSTART:startup\n", capture: "IDLE", want: IdleProbe{TooShort: true, Reason: readinessReasonPaneTooShort}, wantReason: "too short"}}},
		{name: "idle-fresh with no readable box is held", probes: []probe{{events: "SESSIONSTART:resume\n", capture: "no box here", want: IdleProbe{Reason: readinessReasonNoInputBox, NoInputBox: true}, wantReason: "no input box"}}},
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
			engine.progress = tt.progress
			engine.outstanding = tt.outstanding
			var probed Engine = engine
			if tt.styled {
				probed = &styledReadinessEngine{readinessEngine: engine}
			}
			fx := newFixture(t, reed, probed, withStrand("strand-1"), withClock(clock))
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
				reed.ActiveQueue = []bool{p.active}
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

	t.Run("a strand with no run record reads the pane for a draft", func(t *testing.T) {
		tests := []struct {
			name   string
			styled bool
			// readsNoBox swaps in an engine that reads no input box.
			readsNoBox bool
			capture    string
			want       IdleProbe
		}{
			{name: "a pane with no box for an engine that reads one", capture: "no box here", want: IdleProbe{NoInputBox: true}},
			{name: "a pane with no box for an engine that reads none", readsNoBox: true, capture: "no box here", want: IdleProbe{}},
			{name: "an idle pane", capture: "IDLE\n❯ ", want: IdleProbe{Idle: true}},
			{name: "a busy pane holding a draft", capture: "BUSY\n❯ half a draft", want: IdleProbe{Draft: true}},
			{name: "a placeholder in the box for a styled box read", styled: true, capture: "IDLE\n❯ ~Try something~", want: IdleProbe{Idle: true}},
		}
		for _, tt := range tests {
			reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{tt.capture}}
			engine := &readinessEngine{}
			var probed Engine = engine
			if tt.styled {
				probed = &styledReadinessEngine{readinessEngine: engine}
			}
			if tt.readsNoBox {
				probed = &cyclerEngine{}
			}
			got, err := newFixture(t, reed, probed).Runner.SessionIdle("strand-1")
			if err != nil || got != tt.want {
				t.Errorf("%s: SessionIdle = %+v, %v; want %+v, nil", tt.name, got, err, tt.want)
			}
		}
	})

	t.Run("a styled capture that fails is the probe's error", func(t *testing.T) {
		captureErr := errors.New("pane gone")
		reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureErr: captureErr}
		runner := newFixture(t, reed, &styledReadinessEngine{readinessEngine: &readinessEngine{}}, withStrand("strand-1")).Runner
		if _, err := runner.SessionIdle("strand-1"); !errors.Is(err, captureErr) {
			t.Errorf("SessionIdle error = %v, want it to wrap %v", err, captureErr)
		}
	})

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

// slashPasteEngine is a paste-path engine whose session commands and color are one slash-command line each.
// noColor makes it answer no color command.
type slashPasteEngine struct {
	*scriptedPasteEngine

	noColor bool
}

func (e *slashPasteEngine) ProcessLiveness(string) Liveness { return LivenessAlive }

// ParseSessionSignals adds the "SESSIONSTART:<source>" lines, after the scripted engine's own signals.
func (e *slashPasteEngine) ParseSessionSignals(data []byte) ([]SessionSignal, int) {
	signals, end := e.scriptedPasteEngine.ParseSessionSignals(data)
	for _, line := range strings.Split(string(data), "\n") {
		if source, found := strings.CutPrefix(strings.TrimSpace(line), "SESSIONSTART:"); found {
			signals = append(signals, SessionSignal{Kind: SessionSignalSessionStart, Source: SessionStartSource(source)})
		}
	}
	return signals, end
}

// pasteLineReed is a fakeReed that appends line to the events file after every paste, as the provider's hook does when a pasted command restarts the session.
// An empty line appends nothing.
type pasteLineReed struct {
	*fakeReed

	eventsPath string
	line       string
}

func (r *pasteLineReed) PasteSubmit(guid, text string, submit bool) error {
	err := r.fakeReed.PasteSubmit(guid, text, submit)
	if r.line == "" {
		return err
	}
	file, openErr := os.OpenFile(r.eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if openErr != nil {
		return openErr
	}
	defer file.Close()
	if _, writeErr := file.WriteString(r.line + "\n"); writeErr != nil {
		return writeErr
	}
	return err
}

func (e *slashPasteEngine) ClearSessionSequence() []PaneInput {
	return []PaneInput{{Text: "/clear", Submit: true}}
}

func (e *slashPasteEngine) ReloadPluginsSequence() []PaneInput {
	return []PaneInput{{Text: "/reload-plugins", Submit: true}}
}

func (e *slashPasteEngine) CompactSessionSequence(focus string) []PaneInput {
	return []PaneInput{{Text: "/compact " + focus, Submit: true}}
}

func (e *slashPasteEngine) ColorSequence(color segmentcolor.Color) []PaneInput {
	if e.noColor {
		return nil
	}
	return []PaneInput{{Text: "/color " + string(color), Submit: true}}
}

// parserlessPasteEngine is a paste-path engine with no SessionSignalParser: it exposes only the Engine, TextPaster, InputBoxReader and SessionCycler capabilities of the engine it wraps.
type parserlessPasteEngine struct {
	Engine
	TextPaster
	InputBoxReader
	SessionCycler
}

// newSlashPasteRunner returns a runner over a live strand "strand-1" on a paste-path engine whose input box reads box and whose pane shows an operator active or not, and the reed it drives.
func newSlashPasteRunner(t *testing.T, box string, active, noColor bool) (*Runner, *fakeReed, string) {
	t.Helper()
	runner, reed, runDir, _ := newSlashPasteRunnerAppending(t, box, active, noColor, "", false)
	return runner, reed, runDir
}

// newSlashPasteRunnerAppending is newSlashPasteRunner with every paste appending pasteLine to the events file, and returns the runner's clock as well.
// parserless hides the engine's session-signal parsing behind parserlessPasteEngine.
func newSlashPasteRunnerAppending(t *testing.T, box string, active, noColor bool, pasteLine string, parserless bool) (*Runner, *fakeReed, string, *fakeClock) {
	t.Helper()
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"IDLE"}, ActiveQueue: []bool{active}}
	appending := &pasteLineReed{fakeReed: reed, line: pasteLine}
	clock := newFakeClock(time.Now())
	boxes := &inputBoxEngine{fakeEngine: readyAgentEngine(), boxes: []inputBoxAnswer{{box, true}}, settle: 100 * time.Millisecond}
	slash := &slashPasteEngine{scriptedPasteEngine: &scriptedPasteEngine{clearableBoxEngine: &clearableBoxEngine{inputBoxEngine: boxes}, pasteSubmits: true}, noColor: noColor}
	var engine Engine = slash
	if parserless {
		engine = parserlessPasteEngine{Engine: slash, TextPaster: slash, InputBoxReader: slash, SessionCycler: slash}
	}
	cfg := Config{SubmitSettleMS: 100, SubmitConfirmTimeoutS: 30, SendReadyTimeoutS: 60}
	fx := newFixture(t, appending, engine, withConfig(cfg), withStrand("strand-1"), withClock(clock))
	appending.eventsPath = fx.EventsPath
	// A finished turn of a session whose process is alive makes the hook-derived session state known, so the idle wait reads the input box.
	if err := os.WriteFile(fx.EventsPath, []byte("START\nSTOP:done\n"), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}
	record := RunState{RunID: "run-1", StrandGUID: "strand-1", SessionID: "session-1", EventsPath: fx.EventsPath}
	if err := saveRunState(filepath.Dir(fx.EventsPath), record); err != nil {
		t.Fatalf("saveRunState: %v", err)
	}
	return fx.Runner, reed, filepath.Dir(fx.EventsPath), clock
}

// checkLandingStartConfirmation drives call, a session command that restarts the session with the source landing, over a paste-path engine whose box reads empty after the paste.
// A session-start line of landing appended by the paste confirms the send at its first read.
// A line of other, or none, leaves the send waiting out the whole submit window before the empty box confirms it.
// A line of landing already in the events file before the send is older than the pre-send offset and is waited out too.
// An engine that parses no session signals confirms on the empty box at once.
func checkLandingStartConfirmation(t *testing.T, landing, other SessionStartSource, call func(*Runner) error) {
	t.Helper()
	window := submitConfirmTimeout(Config{SubmitConfirmTimeoutS: 30})
	tests := []struct {
		name       string
		priorLine  string
		pasteLine  string
		parserless bool
		wantWait   bool
	}{
		{name: "a session start of the matching source confirms the send at once", pasteLine: "SESSIONSTART:" + string(landing)},
		{name: "a session start of the other source is waited out", pasteLine: "SESSIONSTART:" + string(other), wantWait: true},
		{name: "no session start confirms on the empty box once the window closes", wantWait: true},
		{name: "a session start of the matching source before the send is waited out", priorLine: "SESSIONSTART:" + string(landing), wantWait: true},
		{name: "an engine that parses no session signals confirms on the empty box at once", parserless: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner, _, runDir, clock := newSlashPasteRunnerAppending(t, "", false, false, tt.pasteLine, tt.parserless)
			if tt.priorLine != "" {
				appendEventsLine(t, filepath.Join(runDir, eventsFileName), tt.priorLine)
			}
			start := clock.Now()
			if err := call(runner); err != nil {
				t.Fatalf("error = %v, want the command confirmed", err)
			}
			waited := clock.Now().Sub(start)
			if tt.wantWait && waited < window {
				t.Errorf("waited %s, want at least the %s submit window", waited, window)
			}
			if !tt.wantWait && waited >= window {
				t.Errorf("waited %s, want less than the %s submit window", waited, window)
			}
		})
	}
}

// checkPastedSessionCommand drives call, one of Runner's session commands or its color step, over a paste-path engine in three states of the pane the caller's own idle probe would have passed.
// An empty box gets the command as one paste-submit of wantText and no typed text.
// A box holding a draft and an operator active in the pane each hold the command: it fails wrapping ErrSessionBusy and writes nothing.
// A pending record of the command with a turn started since is another turn, so the command is still pasted.
func checkPastedSessionCommand(t *testing.T, wantText string, call func(*Runner) error) {
	t.Helper()
	tests := []struct {
		name     string
		box      string
		active   bool
		pending  bool
		wantHeld bool
	}{
		{name: "an empty box pastes the command once"},
		{name: "a drafted box holds the command", box: "a half-written draft", wantHeld: true},
		{name: "an active operator holds the command", active: true, wantHeld: true},
		{name: "a pending record of the command with a turn started since pastes it afresh", pending: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner, reed, runDir := newSlashPasteRunner(t, tt.box, tt.active, false)
			if tt.pending {
				if err := savePendingSend(runDir, pendingSend{Text: wantText}); err != nil {
					t.Fatalf("savePendingSend: %v", err)
				}
			}
			err := call(runner)
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none on the paste path", reed.SendTextCalls)
			}
			if tt.wantHeld {
				if !errors.Is(err, ErrSessionBusy) {
					t.Errorf("error = %v, want one wrapping ErrSessionBusy", err)
				}
				if len(reed.PasteSubmitCalls) != 0 {
					t.Errorf("PasteSubmit calls = %+v, want none for a held command", reed.PasteSubmitCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want the command confirmed", err)
			}
			if len(reed.PasteSubmitCalls) != 1 || reed.PasteSubmitCalls[0].Text != wantText {
				t.Errorf("PasteSubmit calls = %+v, want one of %q", reed.PasteSubmitCalls, wantText)
			}
		})
	}
}

func TestRunner_ClearSession_PlaysScriptedSequence(t *testing.T) {
	t.Parallel()

	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"IDLE"}}
	engine := readyCyclerEngine()
	engine.clear = []PaneInput{{Key: "Escape"}, {Text: "/clear", Submit: true}}
	runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner

	if err := runner.ClearSession("strand-1"); err != nil {
		t.Fatalf("ClearSession: %v", err)
	}
	if got, want := sentCalls(reed), []string{"SendKey:Escape", "SendText:/clear"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent calls = %v, want %v", got, want)
	}

	checkPastedSessionCommand(t, "/clear", func(r *Runner) error { return r.ClearSession("strand-1") })
	checkLandingStartConfirmation(t, SessionStartClear, SessionStartCompact, func(r *Runner) error { return r.ClearSession("strand-1") })
}

// sentCalls returns the keys and texts reed was asked to write, in order.
func sentCalls(reed *fakeReed) []string {
	var sent []string
	for _, call := range reed.CallLog {
		if strings.HasPrefix(call, "SendKey:") || strings.HasPrefix(call, "SendText:") {
			sent = append(sent, call)
		}
	}
	return sent
}

func TestRunner_ReloadPlugins_PlaysSequenceOnALiveShuttleStrand(t *testing.T) {
	t.Parallel()

	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"IDLE"}}
	runner := newFixture(t, reed, readyCyclerEngine(), withStrand("strand-1")).Runner

	if err := runner.ReloadPlugins("strand-1"); err != nil {
		t.Fatalf("ReloadPlugins: %v", err)
	}
	if got, want := sentCalls(reed), []string{"SendText:/reload-plugins"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent calls = %v, want %v", got, want)
	}

	checkPastedSessionCommand(t, "/reload-plugins", func(r *Runner) error { return r.ReloadPlugins("strand-1") })

	refusedReed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	refused := newFixture(t, refusedReed, &cyclerEngine{}, withStrand("strand-1")).Runner
	if err := refused.ReloadPlugins("nope"); err == nil {
		t.Error("ReloadPlugins(unknown guid) = nil error")
	}
	if len(refusedReed.CallLog) != 0 {
		t.Errorf("reed touched: %v", refusedReed.CallLog)
	}
}

// TestRunner_TypeColor_PlaysTheSequenceAndWaitsItOut sends the engine's color command into a live shuttle strand through the guarded send path.
// A pasted `/color <name>` is confirmed by the empty box, and a held color returns an error wrapping ErrSessionBusy so the caller may retry.
// A color with no command returns nil with no paste, even beside an active operator, so no idle wait or activity query runs for it.
// An unknown strand is refused before reed is touched.
func TestRunner_TypeColor_PlaysTheSequenceAndWaitsItOut(t *testing.T) {
	t.Parallel()

	checkPastedSessionCommand(t, "/color green", func(r *Runner) error { return r.TypeColor("strand-1", segmentcolor.Green) })

	t.Run("a color with no command types nothing and asks nothing", func(t *testing.T) {
		t.Parallel()
		runner, reed, _ := newSlashPasteRunner(t, "a half-written draft", true, true)
		if err := runner.TypeColor("strand-1", segmentcolor.Green); err != nil {
			t.Errorf("TypeColor() = %v, want nil", err)
		}
		if len(reed.PasteSubmitCalls) != 0 || len(reed.CallLog) != 0 {
			t.Errorf("reed touched: pastes %+v, calls %v", reed.PasteSubmitCalls, reed.CallLog)
		}
	})

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
	t.Parallel()

	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"IDLE"}}
	engine := readyCyclerEngine()
	runner := newFixture(t, reed, engine, withStrand("strand-1")).Runner

	if err := runner.CompactSession("strand-1", "the plan"); err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if !reflect.DeepEqual(engine.foci, []string{"the plan"}) {
		t.Errorf("focus passed to the engine = %v, want [the plan]", engine.foci)
	}
	if got, want := sentCalls(reed), []string{"SendText:/compact the plan"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent calls = %v, want %v", got, want)
	}

	checkPastedSessionCommand(t, "/compact the plan", func(r *Runner) error { return r.CompactSession("strand-1", "the plan") })
	checkLandingStartConfirmation(t, SessionStartCompact, SessionStartClear, func(r *Runner) error { return r.CompactSession("strand-1", "the plan") })
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
	got, err := runner.ClassifySkillLoad("strand-1", Event{Kind: EventStop}, []string{"a", "b"})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ClassifySkillLoad = %+v, %v; want %+v, nil", got, err, want)
	}
}

func TestRunner_LoadSkillsAndClassifySkillLoad_ErrorOnPlainEngine(t *testing.T) {
	runner := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &fakeEngine{}, withStrand("strand-1")).Runner
	if err := runner.LoadSkills("strand-1", []string{"a"}); err == nil || !strings.Contains(err.Error(), "SkillLoader") {
		t.Errorf("LoadSkills error = %v; want one naming SkillLoader", err)
	}
	if _, err := runner.ClassifySkillLoad("strand-1", Event{Kind: EventStop}, []string{"a"}); err == nil || !strings.Contains(err.Error(), "SkillLoader") {
		t.Errorf("ClassifySkillLoad error = %v; want one naming SkillLoader", err)
	}
}

// TestRunner_SessionState covers the state read of one run by guid: the found run reads at the runner's clock, and an unknown guid is an error.
func TestRunner_SessionState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)
	fx := newFixture(t, &fakeReed{}, &fakeEngine{}, withStrand("strand-1"), withClock(&frozenClock{now: now}))
	runner := fx.Runner

	got, err := runner.SessionState("strand-1")
	if err != nil {
		t.Fatalf("SessionState(found guid): %v", err)
	}
	if got.StrandGUID != "strand-1" || got.StrandName != "strand-1" || got.State.Name != SessionUnknown || got.State.Cause != SessionCauseUnsupported || !got.State.Since.Equal(now) {
		t.Errorf("SessionState = %+v; want strand-1 unknown/unsupported since %v", got, now)
	}

	fromHandle, err := fx.newRun(Spec{}, withRunState(RunState{StrandGUID: "strand-1"})).SessionState()
	if err != nil {
		t.Fatalf("Run.SessionState: %v", err)
	}
	if !reflect.DeepEqual(fromHandle, got) {
		t.Errorf("Run.SessionState = %+v; want Runner.SessionState's %+v", fromHandle, got)
	}

	if _, err := runner.SessionState("nope"); err == nil {
		t.Error("SessionState(unknown guid) = nil error")
	}
}
