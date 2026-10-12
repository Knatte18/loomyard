// dialog_test.go covers the notice Wait sends a parent for a permission or input dialog an autonomous run's pane shows past a bound:
// when it is sent, how often, and the offset it persists.

package shuttleengine

import (
	"cmp"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// dialogReed is a fakeReed whose pane capture is whatever the test's script last set, or an error while failCapture is set.
type dialogReed struct {
	*fakeReed

	pane        string
	failCapture bool
}

func (r *dialogReed) CapturePane(string) (string, error) {
	if r.failCapture {
		return "", errors.New("pane gone")
	}
	return r.pane, nil
}

var _ ReedOps = (*dialogReed)(nil)

// dialogEnv is what a scripted step changes between two ticks.
type dialogEnv struct {
	appendEvents func(line string)
	reed         *dialogReed
}

const (
	// dialogPaneNoBox is a pane with no caret, so the readinessEngine reads no input box.
	dialogPaneNoBox = "Do you want to proceed?\n1. Yes\n2. No"
	// dialogPaneBox is a pane showing an empty caret input box.
	dialogPaneBox = "❯ "
	// dialogTick is the clock jump per Sleep.
	dialogTick = 5 * time.Second
)

func TestWait_DialogNotice(t *testing.T) {
	t.Parallel()
	const strandName = "lyx:slug:driver"
	shell := []BackgroundTask{{Kind: BackgroundShell, ID: "sh-1", Label: "sleep 9999", Signal: SignalTranscript}}
	// steps[k] runs after the (k+1)th sleep, so at (k+1)*dialogTick on the run clock, just before that tick's wait-loop pass.
	tests := []struct {
		name   string
		events string
		// noReader makes the engine parse session signals but read no input box.
		noReader   bool
		tooShort   bool
		outstand   []BackgroundTask
		interact   bool
		quiet      bool
		noNotifier bool
		strand     string
		// dialogOffset is the persisted DialogNotifiedOffset the run starts with.
		dialogOffset int64
		failCapture  bool
		steps        map[int]func(env *dialogEnv)
		// deferral builds the run's Spec.HoldDeferral from the time elapsed since Wait began.
		deferral func(elapsed func() time.Duration) func() bool
		// wantAt is the elapsed time of each dialog notice, and wantPersisted the DialogNotifiedOffset persisted at each notifier call.
		wantAt        []time.Duration
		wantPersisted []int64
		// wantFinal is the DialogNotifiedOffset the record holds when Wait returns.
		wantFinal int64
		wantHold  int
	}{
		{
			name: "an ask with no input box notifies once at the bound, with the offset persisted before the call", events: "ASK\n", strand: strandName,
			wantAt: []time.Duration{permissionNoticeBound}, wantPersisted: []int64{4}, wantFinal: 4,
		},
		{
			name: "an ask followed by a turn end with outstanding tasks still notifies", events: "ASK\nWAIT:background work\n", outstand: shell,
			wantAt: []time.Duration{permissionNoticeBound}, wantPersisted: []int64{int64(len("ASK\nWAIT:background work\n"))}, wantFinal: int64(len("ASK\nWAIT:background work\n")),
		},
		{
			name: "an input box shown before the bound sends nothing", events: "ASK\n",
			steps: map[int]func(*dialogEnv){1: func(env *dialogEnv) { env.reed.pane = dialogPaneBox }},
		},
		{name: "an interactive run sends nothing", events: "ASK\n", interact: true},
		{name: "a runner with no notifier sends nothing and persists no offset", events: "ASK\n", noNotifier: true},
		{
			name: "a quiet-hold run still notifies", events: "ASK\n", quiet: true,
			wantAt: []time.Duration{permissionNoticeBound}, wantPersisted: []int64{4}, wantFinal: 4,
		},
		{
			name: "two asks within the bound notify once", events: "ASK\n",
			steps:  map[int]func(*dialogEnv){1: func(env *dialogEnv) { env.appendEvents("ASK") }},
			wantAt: []time.Duration{permissionNoticeBound}, wantPersisted: []int64{8}, wantFinal: 8,
		},
		{
			name: "an input box between two asks restarts the bound from the second ask", events: "ASK\n",
			steps: map[int]func(*dialogEnv){
				1:  func(env *dialogEnv) { env.reed.pane = dialogPaneBox },
				2:  func(env *dialogEnv) { env.reed.pane = dialogPaneNoBox },
				10: func(env *dialogEnv) { env.appendEvents("ASK") },
			},
			wantAt: []time.Duration{55*time.Second + permissionNoticeBound}, wantPersisted: []int64{8}, wantFinal: 8,
		},
		{
			name: "a notified dialog answered and then asked again notifies a second time", events: "ASK\n",
			steps: map[int]func(*dialogEnv){
				13: func(env *dialogEnv) { env.reed.pane = dialogPaneBox },
				14: func(env *dialogEnv) { env.reed.pane = dialogPaneNoBox },
				15: func(env *dialogEnv) { env.appendEvents("ASK") },
			},
			wantAt: []time.Duration{permissionNoticeBound, 80*time.Second + permissionNoticeBound}, wantPersisted: []int64{4, 8}, wantFinal: 8,
		},
		{
			name: "a hold deferral's pending held-turn-end notice is still sent after a dialog notice", events: "STOP:hello\nASK\n",
			deferral: func(elapsed func() time.Duration) func() bool {
				return func() bool { return elapsed() < 100*time.Second }
			},
			wantAt: []time.Duration{permissionNoticeBound}, wantPersisted: []int64{int64(len("STOP:hello\nASK\n"))}, wantFinal: int64(len("STOP:hello\nASK\n")), wantHold: 1,
		},
		{
			name: "a failed capture sends nothing on that tick and the notice follows on a later tick that reads no box", events: "ASK\n", failCapture: true,
			steps:  map[int]func(*dialogEnv){12: func(env *dialogEnv) { env.reed.failCapture = false }},
			wantAt: []time.Duration{65 * time.Second}, wantPersisted: []int64{4}, wantFinal: 4,
		},
		{name: "an engine that reads no input box sends nothing", events: "ASK\n", noReader: true},
		{name: "a pane too short to draw the box sends nothing", events: "ASK\n", tooShort: true},
		{
			name: "a further ask after the notice advances the persisted offset past it", events: "ASK\n",
			steps:  map[int]func(*dialogEnv){15: func(env *dialogEnv) { env.appendEvents("ASK") }},
			wantAt: []time.Duration{permissionNoticeBound}, wantPersisted: []int64{4}, wantFinal: 8,
		},
		{
			name: "a reattach with a pending ask past the persisted offset notifies once", events: "ASK\nASK\n", dialogOffset: 4,
			wantAt: []time.Duration{permissionNoticeBound}, wantPersisted: []int64{8}, wantFinal: 8,
		},
		{
			name: "a reattach whose persisted offset is past every ask sends nothing", events: "ASK\nASK\n", dialogOffset: 8,
			wantFinal: 8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var engine Engine
			if tt.noReader {
				engine = &sessionFakeEngine{waitingEngine: waitingEngine{outstanding: tt.outstand}}
			} else {
				engine = &readinessEngine{idleEngine: idleEngine{sessionFakeEngine: sessionFakeEngine{waitingEngine: waitingEngine{outstanding: tt.outstand}}}, tooShort: tt.tooShort}
			}
			reed := &dialogReed{fakeReed: &fakeReed{StatusQueue: liveStrandStatus(true)}, pane: dialogPaneNoBox, failCapture: tt.failCapture}
			fx := newFixture(t, reed, engine, withConfig(gateConfig))
			fc := newFakeClock(time.Now())
			start := fc.Now()
			elapsed := func() time.Duration { return fc.Now().Sub(start) }
			steps := make([]func(), 1+slices.Max(append([]int{0}, slices.Collect(maps.Keys(tt.steps))...)))
			for index := range steps {
				steps[index] = func() {}
			}
			clk := &jumpStepClock{multiStepClock: &multiStepClock{fakeClock: fc, steps: steps}, jump: dialogTick}
			var holdDeferral func() bool
			if tt.deferral != nil {
				holdDeferral = tt.deferral(elapsed)
			}
			timeout := 5 * time.Minute
			run := fx.newRun(Spec{OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")}, Timeout: timeout, Interactive: tt.interact, QuietHold: tt.quiet, HoldDeferral: holdDeferral},
				withRunState(RunState{StrandGUID: "strand-1", StrandName: tt.strand, SessionID: "session-1", DialogNotifiedOffset: tt.dialogOffset}),
				withRunEvents(tt.events),
				withRunClock(clk, fc.Now().Add(timeout)))
			env := &dialogEnv{appendEvents: func(line string) { appendEventsLine(t, run.state.EventsPath, line) }, reed: reed}
			for index, step := range tt.steps {
				steps[index] = func() { step(env) }
			}

			var gotAt []time.Duration
			var gotPersisted []int64
			var gotHold int
			if !tt.noNotifier {
				wantName := cmp.Or(tt.strand, "strand-1")
				fx.Runner.SetNotifier(func(line string) error {
					if !strings.Contains(line, "permission or input dialog") {
						gotHold++
						return nil
					}
					if line != dialogNotice(wantName) || !strings.HasPrefix(line, "Shuttle notice:") || strings.ContainsAny(line, "\n\r") {
						t.Errorf("dialog notice = %q, want the single line %q", line, dialogNotice(wantName))
					}
					state, _, err := loadRunState(run.runDir)
					if err != nil {
						t.Fatalf("loadRunState in notifier: %v", err)
					}
					gotAt = append(gotAt, elapsed())
					gotPersisted = append(gotPersisted, state.DialogNotifiedOffset)
					return nil
				})
			}
			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeTimeout {
				t.Errorf("outcome = %q, want %q: a dialog notice changes no outcome", result.Outcome, OutcomeTimeout)
			}
			if !slices.Equal(gotAt, tt.wantAt) {
				t.Errorf("dialog notices at %v, want %v", gotAt, tt.wantAt)
			}
			if !slices.Equal(gotPersisted, tt.wantPersisted) {
				t.Errorf("DialogNotifiedOffset at each notifier call = %v, want %v", gotPersisted, tt.wantPersisted)
			}
			if gotHold != tt.wantHold {
				t.Errorf("hold notices = %d, want %d", gotHold, tt.wantHold)
			}
			final, _, err := loadRunState(run.runDir)
			if err != nil {
				t.Fatalf("loadRunState after Wait: %v", err)
			}
			if final.DialogNotifiedOffset != tt.wantFinal {
				t.Errorf("final DialogNotifiedOffset = %d, want %d", final.DialogNotifiedOffset, tt.wantFinal)
			}
		})
	}
}
