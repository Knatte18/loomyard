package shuttleengine

import (
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// shadowRun is what one Wait over a scenario returned and logged.
type shadowRun struct {
	result  Result
	notices []string
	log     string
}

var (
	sessionStateLine      = regexp.MustCompile(`shuttle: session state" .*state=(\S+) cause=(\S+) .*loop=(\S+)`)
	shadowPayloadShell    = []BackgroundTask{{Kind: BackgroundShell, ID: "shell-1", Label: "sleep 600", Signal: SignalPayload}}
	shadowTranscriptShell = []BackgroundTask{{Kind: BackgroundShell, ID: "shell-1", Label: "sleep 600", Signal: SignalTranscript}}
)

// stateChanges returns the "state/cause loop" of every `shuttle: session state` line in log, in order.
func stateChanges(log string) []string {
	var changes []string
	for _, line := range strings.Split(log, "\n") {
		if m := sessionStateLine.FindStringSubmatch(line); m != nil {
			changes = append(changes, m[1]+"/"+m[2]+" "+m[3])
		}
	}
	return changes
}

// TestWait_LogsSessionStateBesideItsClassification drives Wait over scripted events with an engine that parses session signals,
// and again over the same events with an engine that cannot:
// the state changes and disagreements are logged by the first run only, and both runs return the same Result and notify the same lines.
// The log is captured process-wide, so the test does not run in parallel.
func TestWait_LogsSessionStateBesideItsClassification(t *testing.T) {
	tests := []struct {
		name           string
		events         string
		outstanding    []BackgroundTask
		liveness       Liveness
		outputsAtStart bool
		gate           func() GateSpec
		timeout        time.Duration
		// jump is the clock advance per Sleep, zero for the scripted steps below.
		jump time.Duration
		// script returns the agent's actions between ticks, run once per Sleep.
		script func(appendLine func(string), touchOutput func()) []func()

		wantChanges       []string
		wantDisagreements int
	}{
		{
			name:   "a turn start, a waiting turn end, a held turn end and done log one line each with the loop's classification",
			events: "START\n", outstanding: shadowPayloadShell, liveness: LivenessAlive, timeout: time.Hour,
			script: func(appendLine func(string), touchOutput func()) []func() {
				return []func(){
					func() { appendLine("WAIT:background work") },
					func() { appendLine("STOP:what now?") },
					func() { touchOutput(); appendLine("STOP:finished") },
				}
			},
			wantChanges: []string{"busy/turn running", "busy/background waiting", "idle-stalled/no-output held", "idle-done/done done"},
		},
		{
			name:   "a dead reading while the loop reads running logs one disagreement across several ticks",
			events: "START\n", liveness: LivenessDead, timeout: 30 * time.Minute, jump: 10 * time.Minute,
			wantChanges:       []string{"dead/process-gone running"},
			wantDisagreements: 1,
		},
		{
			name:   "a pending gate entry beside idle-done with every output present is expected",
			events: "STOP:finished\n", liveness: LivenessAlive, outputsAtStart: true, timeout: time.Hour, jump: 2 * time.Hour,
			gate: func() GateSpec {
				pending := func() (GateResult, error) { return GateResult{Pending: true}, nil }
				return GateSpec{{Name: "parent-review", Gate: pending, Final: pending, Attempts: 3, PassOnCap: true}}
			},
			wantChanges: []string{"idle-done/done waiting"},
		},
		{
			name:   "a held turn end after transcript-reported shells expired beside busy on background work is expected",
			events: "WAIT:background work\n", outstanding: shadowTranscriptShell, liveness: LivenessAlive, timeout: time.Hour, jump: 6 * time.Minute,
			wantChanges: []string{"busy/background waiting"},
		},
		{
			name:   "done beside busy on background work is expected",
			events: "WAIT:background work\n", outstanding: shadowPayloadShell, liveness: LivenessAlive, outputsAtStart: true, timeout: time.Hour,
			wantChanges: []string{"busy/background done"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drive := func(engine Engine) shadowRun {
				buf := logcapture.CaptureVerbose(t)
				fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, engine, withConfig(gateConfig))
				fc := newFakeClock(time.Now())
				var clk Clock = fc
				var steps *multiStepClock
				switch {
				case tt.jump > 0:
					clk = &jumpClock{fakeClock: fc, jump: tt.jump}
				case tt.script != nil:
					steps = &multiStepClock{fakeClock: fc}
					clk = steps
				}
				outputFile := filepath.Join(t.TempDir(), "out.md")
				if tt.outputsAtStart {
					touchOutputFile(t, outputFile)
				}
				var gate GateSpec
				if tt.gate != nil {
					gate = tt.gate()
				}
				run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: tt.timeout},
					withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
					withRunEvents(tt.events),
					withRunClock(clk, fc.Now().Add(tt.timeout)),
					withRunGate(gate))
				if steps != nil {
					steps.steps = tt.script(
						func(line string) { appendEventsLine(t, run.state.EventsPath, line) },
						func() { touchOutputFile(t, outputFile) })
				}
				var got shadowRun
				fx.Runner.SetNotifier(func(line string) error {
					got.notices = append(got.notices, line)
					return nil
				})
				result, err := run.Wait()
				if err != nil {
					t.Fatalf("Wait() error: %v", err)
				}
				result.RunDir = ""
				got.result, got.log = result, buf.String()
				return got
			}

			shadowed := drive(&sessionFakeEngine{waitingEngine: waitingEngine{outstanding: tt.outstanding}, liveness: tt.liveness})
			plain := drive(&waitingEngine{outstanding: tt.outstanding})

			if got := stateChanges(shadowed.log); !reflect.DeepEqual(got, tt.wantChanges) {
				t.Errorf("state changes = %q, want %q in %q", got, tt.wantChanges, shadowed.log)
			}
			if got := strings.Count(shadowed.log, "shuttle: session state disagrees"); got != tt.wantDisagreements {
				t.Errorf("disagreements = %d, want %d in %q", got, tt.wantDisagreements, shadowed.log)
			}
			if got := stateChanges(plain.log); len(got) != 0 || strings.Contains(plain.log, "disagrees") {
				t.Errorf("an engine without the parser logged session state: %q", plain.log)
			}
			if !reflect.DeepEqual(shadowed.result, plain.result) {
				t.Errorf("result with the parser = %+v, without = %+v", shadowed.result, plain.result)
			}
			if !reflect.DeepEqual(shadowed.notices, plain.notices) {
				t.Errorf("notices with the parser = %q, without = %q", shadowed.notices, plain.notices)
			}
		})
	}
}
