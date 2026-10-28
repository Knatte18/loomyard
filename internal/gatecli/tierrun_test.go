//go:build tier0

// tierrun_test.go covers the one rerun of a tier that failed on a resource shortage and how the closing line names such a run, over fake attempts that spawn nothing.

package gatecli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

// cancelOnWrite cancels a call at its first write, standing in for a cancellation that arrives while a tier waits to rerun.
type cancelOnWrite struct {
	io.Writer
	cancel context.CancelFunc
}

func (w cancelOnWrite) Write(p []byte) (int, error) {
	w.cancel()
	return w.Writer.Write(p)
}

func TestRetryOnShortage(t *testing.T) {
	t.Parallel()

	failed := func(exit int, log string) tierResult {
		return tierResult{record: gateslot.RunRecord{ExitCode: exit}, log: []byte(log)}
	}
	eagain := failed(2, "fork/exec /usr/bin/git: Resource temporarily unavailable\n")
	thread := failed(2, "runtime: failed to create new OS thread (have 12 already; errno=11)\n")
	tests := []struct {
		name      string
		attempts  []tierResult
		cancelled bool
		// cancelDuringDelay cancels the call when the rerun is announced, so the wait before the rerun ends on the cancellation.
		cancelDuringDelay bool
		wantAttempts      int
		wantExit          int
		wantShortage      string
		wantRetry         bool
	}{
		{name: "PassIsNotRerun", attempts: []tierResult{failed(0, "Resource temporarily unavailable\n")}, wantAttempts: 1},
		{name: "FailureWithoutMarkerIsNotRerun", attempts: []tierResult{failed(1, "--- FAIL: TestX\n")}, wantAttempts: 1, wantExit: 1},
		{name: "TimeoutIsNotRerun", attempts: []tierResult{{record: gateslot.RunRecord{ExitCode: 1, TimedOut: true}, log: eagain.log}}, wantAttempts: 1, wantExit: 1},
		{name: "RefusalIsNotRerun", attempts: []tierResult{{refusal: "no shell", log: eagain.log}}, wantAttempts: 1},
		{name: "CancelledCallIsNotRerun", attempts: []tierResult{eagain}, cancelled: true, wantAttempts: 1, wantExit: 2},
		{name: "CancelDuringTheDelayReturnsTheFirstResult", attempts: []tierResult{eagain}, cancelDuringDelay: true, wantAttempts: 1, wantExit: 2, wantRetry: true},
		{name: "ShortageThenPassEndsPassed", attempts: []tierResult{eagain, failed(0, "ok\n")}, wantAttempts: 2, wantRetry: true},
		{name: "ShortageThenOrdinaryFailureCarriesNoShortage", attempts: []tierResult{thread, failed(1, "--- FAIL: TestX\n")}, wantAttempts: 2, wantExit: 1, wantRetry: true},
		{name: "ShortageTwiceCarriesTheMarker", attempts: []tierResult{eagain, thread}, wantAttempts: 2, wantExit: 2, wantShortage: "failed to create new os thread", wantRetry: true},
		{name: "ShellForkFailureIsAShortage", attempts: []tierResult{failed(1, "sh: Cannot fork\n"), failed(1, "bash: fork: Cannot fork\n")}, wantAttempts: 2, wantExit: 1, wantShortage: "cannot fork", wantRetry: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			var stderr bytes.Buffer
			var out io.Writer = &stderr
			delay := time.Duration(0)
			if tt.cancelDuringDelay {
				out = cancelOnWrite{Writer: &stderr, cancel: cancel}
				delay = time.Hour
			}
			calls := 0
			got := retryOnShortage(ctx, out, "tier0", delay, func() tierResult {
				calls++
				return tt.attempts[calls-1]
			})
			if calls != tt.wantAttempts {
				t.Errorf("retryOnShortage ran %d attempts; want %d", calls, tt.wantAttempts)
			}
			if got.record.ExitCode != tt.wantExit || got.shortage != tt.wantShortage {
				t.Errorf("retryOnShortage() = exit %d, shortage %q; want exit %d, shortage %q", got.record.ExitCode, got.shortage, tt.wantExit, tt.wantShortage)
			}
			const announced = "gate test: tier0 failed on a resource shortage"
			if retried := strings.HasPrefix(stderr.String(), announced); retried != tt.wantRetry {
				t.Errorf("stderr = %q; want the rerun announced: %v", stderr.String(), tt.wantRetry)
			}
			if tt.wantRetry && calls == 2 && !strings.HasPrefix(string(got.log), stderr.String()) {
				t.Errorf("rerun log = %q; want it to open with the announcement %q", got.log, stderr.String())
			}
		})
	}
}

func TestFailedRunLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		end  runEnd
		want string
	}{
		{name: "OrdinaryFailure", end: runEnd{code: 1, ran: true}, want: "tier1 (exit 1)"},
		{
			name: "ShortageTwice",
			end:  runEnd{code: 2, ran: true, shortage: "cannot fork"},
			want: `tier1 (exit 2, a resource shortage, "cannot fork", in its run and its rerun: an environment error, not a test failure; way forward: re-run the same command once the machine's process load drops)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := failedRunLabel(tierRun{number: 1}, tt.end); got != tt.want {
				t.Errorf("failedRunLabel() = %q; want %q", got, tt.want)
			}
		})
	}
}
