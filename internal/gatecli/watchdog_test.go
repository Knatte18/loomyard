// watchdog_test.go covers the CPU watchdog's quit decisions over a scripted sampler and a fake clock, and the conditions under which the watchdog runs at all.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const (
	testPeriod     = 5 * time.Second
	testBinaryPath = "/tmp/go-build/b001/pkg.test"
	longBinaryPath = "/tmp/go-build/b001/averylongpackagenamethatpassesfifteen.test"
	nonTestBinary  = "/usr/bin/go"
	testBinaryPID  = 42
	watchdogWindow = 10 * time.Second
)

// scriptedSample is what the fake sampler returns on one call.
type scriptedSample struct {
	tree []proc.ProcessCPU
	err  error
}

// series is one sample per cumulative CPU value of the process at path.
func series(path string, cumulative ...time.Duration) []scriptedSample {
	samples := make([]scriptedSample, 0, len(cumulative))
	for _, cpu := range cumulative {
		samples = append(samples, scriptedSample{tree: []proc.ProcessCPU{{PID: testBinaryPID, Path: path, CPU: cpu}}})
	}
	return samples
}

// seconds is a duration of fractional seconds.
func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

// watchdogRun is what one scripted run of the watchdog left behind.
type watchdogRun struct {
	signalled []int
	out       string
	logs      string
	samples   int
}

// runScripted runs a watchdog with window over script and returns once the script's last sample has been taken.
// The fake clock moves one period per wait, so each sample is one period after the one before.
func runScripted(t *testing.T, window time.Duration, script []scriptedSample) watchdogRun {
	t.Helper()
	logs := logcapture.CaptureVerbose(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var result watchdogRun
	var out bytes.Buffer
	clock := time.Unix(1_000_000, 0)
	w := watchdog{
		window: window,
		period: testPeriod,
		sample: func(int) ([]proc.ProcessCPU, error) {
			next := script[result.samples]
			result.samples++
			if result.samples == len(script) {
				cancel()
			}
			return next.tree, next.err
		},
		signal: func(pid int) error {
			result.signalled = append(result.signalled, pid)
			return nil
		},
		now: func() time.Time { return clock },
		after: func(period time.Duration) <-chan time.Time {
			clock = clock.Add(period)
			elapsed := make(chan time.Time, 1)
			elapsed <- clock
			return elapsed
		},
		out: &out,
	}
	w.run(ctx, 1)
	result.out, result.logs = out.String(), logs.String()
	return result
}

// TestWatchdog carries no t.Parallel: the log capture swaps the process-global logger.
func TestWatchdog(t *testing.T) {
	wantLine := gateslot.WatchdogLine("pkg.test", testBinaryPID, watchdogWindow) + "\n"
	wantOnce := []int{testBinaryPID}
	tests := []struct {
		name        string
		script      []scriptedSample
		wantSignals []int
		wantOut     string
		wantLogs    string
		wantSamples int
	}{
		{
			name:        "busy for the whole window is signalled once, after its second busy sample",
			script:      series(testBinaryPath, 0, seconds(5), seconds(10)),
			wantSignals: wantOnce,
			wantOut:     wantLine,
		},
		{
			name:   "one busy sample is short of the window",
			script: series(testBinaryPath, 0, seconds(5)),
		},
		{
			name:        "92 percent of a core for the whole window is signalled",
			script:      series(testBinaryPath, 0, seconds(4.6), seconds(9.2)),
			wantSignals: wantOnce,
			wantOut:     wantLine,
		},
		{
			name:        "staying busy after the quit gets no second signal",
			script:      series(testBinaryPath, 0, seconds(5), seconds(10), seconds(15), seconds(20)),
			wantSignals: wantOnce,
			wantOut:     wantLine,
		},
		{
			name:   "an idle sample restarts the window",
			script: series(testBinaryPath, 0, seconds(5), seconds(5), seconds(10)),
		},
		{
			name:        "two busy samples after an idle one reach the window",
			script:      series(testBinaryPath, 0, seconds(5), seconds(5), seconds(10), seconds(15)),
			wantSignals: wantOnce,
			wantOut:     wantLine,
		},
		{
			name:        "a test binary with a long file name is signalled",
			script:      series(longBinaryPath, 0, seconds(5), seconds(10)),
			wantSignals: wantOnce,
			wantOut:     gateslot.WatchdogLine("averylongpackagenamethatpassesfifteen.test", testBinaryPID, watchdogWindow) + "\n",
		},
		{
			name:   "a busy process that is not a test binary is never signalled",
			script: series(nonTestBinary, 0, seconds(5), seconds(10), seconds(15)),
		},
		{
			name:        "a sampler failure is logged and sampling goes on",
			script:      append([]scriptedSample{{err: errors.New("proc listing failed")}}, series(testBinaryPath, 0, seconds(5), seconds(10))...),
			wantSignals: wantOnce,
			wantOut:     wantLine,
			wantLogs:    "proc listing failed",
		},
		{
			name:        "an unsupported platform ends the watchdog after one log line",
			script:      []scriptedSample{{err: errors.ErrUnsupported}, {}},
			wantLogs:    "no CPU watchdog applies",
			wantSamples: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runScripted(t, watchdogWindow, tc.script)
			if !slices.Equal(got.signalled, tc.wantSignals) {
				t.Errorf("signalled pids = %v; want %v", got.signalled, tc.wantSignals)
			}
			if got.out != tc.wantOut {
				t.Errorf("output = %q; want %q", got.out, tc.wantOut)
			}
			if tc.wantLogs != "" && strings.Count(got.logs, tc.wantLogs) != 1 {
				t.Errorf("logs = %q; want %q once", got.logs, tc.wantLogs)
			}
			if tc.wantSamples != 0 && got.samples != tc.wantSamples {
				t.Errorf("sampler calls = %d; want %d", got.samples, tc.wantSamples)
			}
			if window, found := gateslot.FindWatchdogQuit(got.out); found != (tc.wantOut != "") || (found && window != "10s") {
				t.Errorf("FindWatchdogQuit(%q) = %q, %v; want the window 10s exactly when a line was printed", got.out, window, found)
			}
		})
	}
}

func TestWatchdogApplies(t *testing.T) {
	t.Parallel()

	const ancestorPID = 77
	isAncestor := func(pid int) bool { return pid == ancestorPID }
	tests := []struct {
		name      string
		window    time.Duration
		flags     []string
		enclosing string
		want      bool
	}{
		{name: "a window of 0 turns it off", window: 0, want: false},
		{name: "-bench=. turns it off", window: watchdogWindow, flags: []string{"-run", "X", "-bench=."}, want: false},
		{name: "-test.bench turns it off", window: watchdogWindow, flags: []string{"-test.bench", "."}, want: false},
		{name: "--fuzz=FuzzX turns it off", window: watchdogWindow, flags: []string{"--fuzz=FuzzX"}, want: false},
		{name: "an enclosing run that is an ancestor turns it off", window: watchdogWindow, enclosing: "77", want: false},
		{name: "an enclosing pid that is no ancestor leaves it on", window: watchdogWindow, enclosing: "78", want: true},
		{name: "an empty enclosing value leaves it on", window: watchdogWindow, want: true},
		{name: "an unparsable enclosing value leaves it on", window: watchdogWindow, enclosing: "not-a-pid", want: true},
		{name: "ordinary go test flags leave it on", window: watchdogWindow, flags: []string{"-run", "Bench", "-count=1"}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, reason := watchdogApplies(tc.window, tc.flags, tc.enclosing, isAncestor)
			if got != tc.want || reason == "" {
				t.Errorf("watchdogApplies(%v, %q, %q) = %v, %q; want %v and a reason", tc.window, tc.flags, tc.enclosing, got, reason, tc.want)
			}
		})
	}
}
