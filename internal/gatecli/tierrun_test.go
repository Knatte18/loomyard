// tierrun_test.go covers the single rerun of a tier that failed on a resource shortage or crashed its test runner.
// It also covers how the closing line names such a run.
// The attempts are fakes that spawn nothing.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cancelOnWrite cancels a call at its first write.
// It stands in for a cancellation that arrives while a tier waits to rerun.
type cancelOnWrite struct {
	io.Writer
	cancel context.CancelFunc
}

func (w cancelOnWrite) Write(p []byte) (int, error) {
	w.cancel()
	return w.Writer.Write(p)
}

func TestRetryTier(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	failed := func(exit int, log string) tierResult {
		return tierResult{record: gateslot.RunRecord{ExitCode: exit}, log: []byte(log)}
	}
	eagain := failed(2, "fork/exec /usr/bin/git: Resource temporarily unavailable\n")
	thread := failed(2, "runtime: failed to create new OS thread (have 12 already; errno=11)\n")
	abort := 128 + int(syscall.SIGABRT)
	aborted := failed(abort, "Fatal error. Internal CLR error. (0x80131506)\nAborted (core dumped)\n")
	segv := failed(128+int(syscall.SIGSEGV), "Segmentation fault (core dumped)\n")
	abortedWithFailure := tierResult{record: gateslot.RunRecord{ExitCode: abort, Failed: 1}, log: aborted.log}
	const shortageAnnounced = "gate test: tier0 failed on a resource shortage"
	crashAnnounced := fmt.Sprintf("gate test: tier0 crashed its test runner (exit %d, SIGABRT) with no failing test", abort)
	tests := []struct {
		name      string
		attempts  []tierResult
		cancelled bool
		// cancelDuringDelay cancels the call when the rerun is announced, so the wait before the rerun ends on the cancellation.
		cancelDuringDelay bool
		wantAttempts      int
		wantExit          int
		wantShortage      string
		wantCrash         string
		// wantAnnounced opens stderr when the rerun is announced, and is empty when it is not.
		wantAnnounced string
	}{
		{name: "PassIsNotRerun", attempts: []tierResult{failed(0, "Resource temporarily unavailable\n")}, wantAttempts: 1},
		{name: "FailureWithoutMarkerIsNotRerun", attempts: []tierResult{failed(1, "--- FAIL: TestX\n")}, wantAttempts: 1, wantExit: 1},
		{name: "TimeoutIsNotRerun", attempts: []tierResult{{record: gateslot.RunRecord{ExitCode: 1, TimedOut: true}, log: eagain.log}}, wantAttempts: 1, wantExit: 1},
		{name: "RefusalIsNotRerun", attempts: []tierResult{{refusal: "no shell", log: eagain.log}}, wantAttempts: 1},
		{name: "CancelledCallIsNotRerun", attempts: []tierResult{eagain}, cancelled: true, wantAttempts: 1, wantExit: 2},
		{name: "CancelDuringTheDelayReturnsTheFirstResult", attempts: []tierResult{eagain}, cancelDuringDelay: true, wantAttempts: 1, wantExit: 2, wantAnnounced: shortageAnnounced},
		{name: "ShortageThenPassEndsPassed", attempts: []tierResult{eagain, failed(0, "ok\n")}, wantAttempts: 2, wantAnnounced: shortageAnnounced},
		{name: "ShortageThenOrdinaryFailureCarriesNoShortage", attempts: []tierResult{thread, failed(1, "--- FAIL: TestX\n")}, wantAttempts: 2, wantExit: 1, wantAnnounced: shortageAnnounced},
		{name: "ShortageTwiceCarriesTheMarker", attempts: []tierResult{eagain, thread}, wantAttempts: 2, wantExit: 2, wantShortage: "failed to create new os thread", wantAnnounced: shortageAnnounced},
		{name: "ShellForkFailureIsAShortage", attempts: []tierResult{failed(1, "sh: Cannot fork\n"), failed(1, "bash: fork: Cannot fork\n")}, wantAttempts: 2, wantExit: 1, wantShortage: "cannot fork", wantAnnounced: shortageAnnounced},
		{name: "CrashThenPassEndsPassed", attempts: []tierResult{aborted, failed(0, "Passed!  - Failed: 0, Passed: 138, Skipped: 0, Total: 138\n")}, wantAttempts: 2, wantAnnounced: crashAnnounced},
		{name: "CrashTwiceCarriesTheSignal", attempts: []tierResult{aborted, segv}, wantAttempts: 2, wantExit: 128 + int(syscall.SIGSEGV), wantCrash: "SIGSEGV", wantAnnounced: crashAnnounced},
		{name: "CrashThenFailingTestCarriesNoCrash", attempts: []tierResult{aborted, abortedWithFailure}, wantAttempts: 2, wantExit: abort, wantAnnounced: crashAnnounced},
		{name: "ShortageThenCrashCarriesNoCrash", attempts: []tierResult{eagain, aborted}, wantAttempts: 2, wantExit: abort, wantAnnounced: shortageAnnounced},
		{name: "SignalExitWithAFailingTestIsNotRerun", attempts: []tierResult{abortedWithFailure}, wantAttempts: 1, wantExit: abort},
		{name: "TerminatedRunIsNoCrash", attempts: []tierResult{failed(128+int(syscall.SIGTERM), "Terminated\n")}, wantAttempts: 1, wantExit: 128 + int(syscall.SIGTERM)},
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
			var recorded []tierResult
			got := retryTier(ctx, out, "tier0", delay, func() tierResult {
				calls++
				return tt.attempts[calls-1]
			}, func(first tierResult) { recorded = append(recorded, first) })
			if calls != tt.wantAttempts {
				t.Errorf("retryTier ran %d attempts; want %d", calls, tt.wantAttempts)
			}
			if got.record.ExitCode != tt.wantExit || got.shortage != tt.wantShortage || got.crash != tt.wantCrash || got.record.RunnerCrash != tt.wantCrash {
				t.Errorf("retryTier() = exit %d, shortage %q, crash %q, record crash %q; want exit %d, shortage %q, crash %q", got.record.ExitCode, got.shortage, got.crash, got.record.RunnerCrash, tt.wantExit, tt.wantShortage, tt.wantCrash)
			}
			retried := tt.wantAnnounced != ""
			if announced := retried && strings.HasPrefix(stderr.String(), tt.wantAnnounced); announced != retried || (!retried && stderr.Len() > 0) {
				t.Errorf("stderr = %q; want the rerun announced with %q", stderr.String(), tt.wantAnnounced)
			}
			if calls != 2 {
				if len(recorded) != 0 {
					t.Errorf("recorded %d first runs; want none for a run that was not rerun", len(recorded))
				}
				return
			}
			if !strings.HasPrefix(string(got.log), stderr.String()) || !got.record.Rerun {
				t.Errorf("rerun log = %q, rerun mark %v; want the log to open with the announcement %q and the record marked a rerun", got.log, got.record.Rerun, stderr.String())
			}
			wantFirstCrash := ""
			if tt.wantAnnounced == crashAnnounced {
				wantFirstCrash = "SIGABRT"
			}
			if len(recorded) != 1 || recorded[0].record.ExitCode != tt.attempts[0].record.ExitCode || recorded[0].record.Rerun || recorded[0].record.RunnerCrash != wantFirstCrash {
				t.Errorf("recorded first runs = %+v; want the first run once, not marked a rerun, with runner crash %q", recorded, wantFirstCrash)
			}
		})
	}
}

func TestFailedRunLabel(t *testing.T) {
	tiergate.Require(t, 0)
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
		{
			name: "CrashTwice",
			end:  runEnd{code: 134, ran: true, crash: "SIGABRT"},
			want: "tier1 (exit 134, a runner crash, SIGABRT, in its run and its rerun with no failing test: the code under test was not judged)",
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
