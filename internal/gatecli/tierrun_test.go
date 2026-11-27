// tierrun_test.go covers the single rerun of a tier that failed on a resource shortage or crashed its test runner.
// It also covers how the closing line names such a run.
// The attempts are fakes that spawn nothing.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/proc"
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
	pressured := tierResult{record: gateslot.RunRecord{ExitCode: 1}, log: []byte("signal: killed\n"), memoryStopped: true, slicePressure: true}
	pressuredTimedOut := tierResult{record: gateslot.RunRecord{ExitCode: 1, TimedOut: true}, memoryStopped: true, slicePressure: true}
	const shortageAnnounced = "gate test: tier0 failed on a resource shortage"
	crashAnnounced := fmt.Sprintf("gate test: tier0 crashed its test runner (exit %d, SIGABRT) with no failing test", abort)
	const pressureAnnounced = "gate test: tier0 had a test binary killed by the memory guard for a memory limit other load filled"
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
		wantPressureStop  bool
		// wantRerunMarked is whether the rerun's record carries the memory pressure mark.
		wantRerunMarked bool
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
		{name: "SlicePressureThenPassEndsPassed", attempts: []tierResult{pressured, failed(0, "ok\n")}, wantAttempts: 2, wantAnnounced: pressureAnnounced},
		{name: "SlicePressureThenOrdinaryFailureCarriesNoPressureStop", attempts: []tierResult{pressured, failed(1, "--- FAIL: TestX\n")}, wantAttempts: 2, wantExit: 1, wantAnnounced: pressureAnnounced},
		{name: "SlicePressureTwiceIsAPressureStop", attempts: []tierResult{pressured, pressured}, wantAttempts: 2, wantExit: 1, wantPressureStop: true, wantRerunMarked: true, wantAnnounced: pressureAnnounced},
		{name: "ShortageThenSlicePressureCarriesNoPressureStop", attempts: []tierResult{eagain, pressured}, wantAttempts: 2, wantExit: 1, wantRerunMarked: true, wantAnnounced: shortageAnnounced},
		{name: "TimedOutSlicePressureIsNotRerun", attempts: []tierResult{pressuredTimedOut}, wantAttempts: 1, wantExit: 1},
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
			if got.record.ExitCode != tt.wantExit || got.shortage != tt.wantShortage || got.crash != tt.wantCrash || got.record.RunnerCrash != tt.wantCrash || got.pressureStop != tt.wantPressureStop {
				t.Errorf("retryTier() = exit %d, shortage %q, crash %q, record crash %q, pressure stop %v; want exit %d, shortage %q, crash %q, pressure stop %v", got.record.ExitCode, got.shortage, got.crash, got.record.RunnerCrash, got.pressureStop, tt.wantExit, tt.wantShortage, tt.wantCrash, tt.wantPressureStop)
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
			wantFirstPressure := tt.wantAnnounced == pressureAnnounced
			if recorded[0].record.MemoryPressure != wantFirstPressure || got.record.MemoryPressure != tt.wantRerunMarked {
				t.Errorf("memory pressure marks = first %v, rerun %v; want %v, %v", recorded[0].record.MemoryPressure, got.record.MemoryPressure, wantFirstPressure, tt.wantRerunMarked)
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

// TestWithTimeoutMin pins the timeout_min bound on a fake clock.
// Either clock's reading past the bound ends the context at the next poll.
// The wall clock's reading is the one that passes it after the host slept.
// A poll waits at most timeoutMinPoll, and no longer than what is left of the bound.
//
//testtiming:keep the poll lengths and the cause of the timeout_min bound on a fake clock
func TestWithTimeoutMin(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type reading struct{ monotonic, wall time.Duration }
	tests := []struct {
		name    string
		timeout time.Duration
		// readings are the time passed on each clock, one per poll in order; the last repeats.
		readings []reading
		// readyPolls is how many polls end at once; every later poll waits until the context ends.
		readyPolls int
		// wantPolls are the waits the bound asks for, in order.
		wantPolls []time.Duration
		// wantCause is the context's cause once the polls are done, nil for a context still open.
		wantCause error
	}{
		{
			name: "AWallClockPastTheBoundEndsTheContextThoughTheMonotonicClockStoodStill", timeout: 35 * time.Minute,
			readings: []reading{{monotonic: time.Minute, wall: 21 * time.Hour}}, wantCause: errTimeoutMinPassed,
		},
		{
			name: "AMonotonicClockPastTheBoundEndsTheContextThoughTheWallClockWasSetBack", timeout: 35 * time.Minute,
			readings: []reading{{monotonic: 36 * time.Minute, wall: -time.Hour}}, wantCause: errTimeoutMinPassed,
		},
		{
			name: "ABoundNotYetPassedKeepsTheContextOpen", timeout: 35 * time.Minute,
			readings: []reading{{time.Minute, time.Minute}}, readyPolls: 1, wantPolls: []time.Duration{timeoutMinPoll, timeoutMinPoll},
		},
		{
			name: "TheLastPollWaitsOnlyWhatIsLeft", timeout: 3 * time.Second,
			readings: []reading{{time.Second, time.Second}, {3 * time.Second, 3 * time.Second}}, readyPolls: 1,
			wantPolls: []time.Duration{2 * time.Second}, wantCause: errTimeoutMinPassed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var read atomic.Int32
			elapsed := func() (time.Duration, time.Duration) {
				at := tt.readings[min(int(read.Add(1))-1, len(tt.readings)-1)]
				return at.monotonic, at.wall
			}
			polls := make(chan time.Duration, len(tt.wantPolls)+1)
			var polled atomic.Int32
			after := func(wait time.Duration) <-chan time.Time {
				polls <- wait
				if int(polled.Add(1)) > tt.readyPolls {
					return nil
				}
				ready := make(chan time.Time, 1)
				ready <- time.Time{}
				return ready
			}

			ctx, cancel := withTimeoutMin(context.Background(), tt.timeout, elapsed, after)
			t.Cleanup(cancel)

			var got []time.Duration
			for range tt.wantPolls {
				got = append(got, <-polls)
			}
			if tt.wantCause != nil {
				<-ctx.Done()
			}
			if !slices.Equal(got, tt.wantPolls) {
				t.Errorf("withTimeoutMin() polled %v; want %v", got, tt.wantPolls)
			}
			if cause := context.Cause(ctx); cause != tt.wantCause {
				t.Errorf("withTimeoutMin() context cause = %v; want %v", cause, tt.wantCause)
			}
		})
	}
}

// TestRunBuildTimeout pins the build step's timeout_min.
// A build that outlives it fails as timed out, prints the build's timeout line and blocks the tiers that opt into it.
// A build that ends in time lets its tier run.
func TestRunBuildTimeout(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the build command is a POSIX shell sleep")
	}

	const unit = 200 * time.Millisecond
	tests := []struct {
		name         string
		command      string
		wantOK       bool
		wantTimedOut bool
	}{
		{name: "ABuildThatEndsInTime", command: "exit 0", wantOK: true},
		{name: "ABuildThatOutlivesItsTimeout", command: "sleep 300", wantTimedOut: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var stderr bytes.Buffer
			e := &execution{
				ctx:       context.Background(),
				out:       io.Discard,
				request:   testRequest{root: dir, stderr: &stderr, timeoutUnit: unit, pipeWaitDelay: defaultPipeWaitDelay},
				cfg:       gateslot.Config{Build: gateslot.BuildStep{Command: tt.command, Dir: dir, TimeoutMin: 1}},
				runLogDir: t.TempDir(),
			}

			result, ok := e.runBuild(tierRun{number: 0}, []string{"PATH=" + os.Getenv("PATH")})

			if ok != tt.wantOK || result.buildFailed == tt.wantOK {
				t.Errorf("runBuild() = ok %v, buildFailed %v; want ok %v", ok, result.buildFailed, tt.wantOK)
			}
			timeoutLine := gateslot.BuildTimeoutLine(unit)
			if got := strings.Contains(stderr.String(), timeoutLine); got != tt.wantTimedOut {
				t.Errorf("runBuild() stderr = %q; want it to hold the timeout line: %v", stderr.String(), tt.wantTimedOut)
			}
			if blocked := e.buildFailure != ""; blocked != tt.wantTimedOut {
				t.Errorf("runBuild() left buildFailure %q; want it set: %v", e.buildFailure, tt.wantTimedOut)
			}
		})
	}
}

// TestRunTierProcessCap pins the tier run's wiring of the process cap.
// A tripped cap fails a command that exited 0 and marks the result so no rerun follows.
// A run whose environment carries no run marker starts no cap.
func TestRunTierProcessCap(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell loop")
	}

	const shortage = "resource temporarily unavailable"
	tests := []struct {
		name string
		// marked is whether the run's environment carries the run marker.
		marked bool
		// wait is whether the command waits for the stand-in cap to trip before it exits 0.
		wait       bool
		wantCapped bool
		wantStarts int32
	}{
		{name: "TrippedCapFailsACommandThatExitedZero", marked: true, wait: true, wantCapped: true, wantStarts: 1},
		{name: "NoRunMarkerStartsNoCap", wantCapped: false, wantStarts: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			tripped := filepath.Join(dir, "tripped")
			command := "echo " + shortage
			if tt.wait {
				command = fmt.Sprintf("until [ -f %q ]; do sleep 0.01; done; echo %s", tripped, shortage)
			}
			env := []string{"PATH=" + os.Getenv("PATH")}
			if tt.marked {
				env = append(env, enclosingGateEnv+"=1")
			}
			var starts atomic.Int32
			var stderr bytes.Buffer
			request := testRequest{
				root: dir, stderr: &stderr, samplePeriod: time.Millisecond, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay,
				processCapFor: func(marker string, period time.Duration) processCap {
					starts.Add(1)
					return processCap{
						limit:  1,
						period: period,
						marker: marker,
						list: func(string) ([]int, error) {
							return []int{1, 2}, os.WriteFile(tripped, nil, 0o644)
						},
						kill: func(int) error { return nil },
						after: func(time.Duration) <-chan time.Time {
							elapsed := make(chan time.Time, 1)
							elapsed <- time.Time{}
							return elapsed
						},
					}
				},
			}
			run := tierRun{
				number:      0,
				tier:        gateslot.Tier{Command: command, Counter: counterGoJSON, TimeoutMin: 1},
				invocations: []invocation{{dir: dir}},
			}

			result := runTier(context.Background(), io.Discard, run, env, request)

			if got := starts.Load(); got != tt.wantStarts {
				t.Errorf("runTier() started the process cap %d times; want %d", got, tt.wantStarts)
			}
			if result.processCapped != tt.wantCapped {
				t.Errorf("runTier() processCapped = %v; want %v", result.processCapped, tt.wantCapped)
			}
			if !tt.wantCapped {
				if result.record.ExitCode != 0 || stderr.Len() != 0 {
					t.Errorf("runTier() exit = %d, stderr = %q; want exit 0 and no stderr", result.record.ExitCode, stderr.String())
				}
				return
			}
			capLine := processCapLine(2, 1)
			if result.record.ExitCode != 1 {
				t.Errorf("runTier() exit = %d; want 1 for a command that exited 0 under a tripped cap", result.record.ExitCode)
			}
			if !slices.Contains(result.causes, capLine) || !strings.Contains(stderr.String(), capLine) {
				t.Errorf("runTier() causes = %q, stderr = %q; want the cap line in both", result.causes, stderr.String())
			}
			if resourceShortage(result.log) == "" {
				t.Fatalf("runTier() log = %q; want it to hold the shortage marker the command printed", result.log)
			}
			if got := failedOnShortage(result); got != "" {
				t.Errorf("failedOnShortage() = %q; want none, so a capped run is never rerun", got)
			}
		})
	}
}

// TestRunTierMemoryGuard pins the tier run's wiring of the memory guard.
// A guard kill fails a command that exited 0.
// It puts the kill's line and the tests it left running in the log and the causes.
// It marks the result of a kill over the cap so no rerun follows, and that of a slice-pressure kill so one does.
// A slice-pressure kill beside a failed test, or beside a failed package whose binary was not killed, is not marked so.
// A request that turns the guard off builds none.
func TestRunTierMemoryGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell loop")
	}

	const shortage = "resource temporarily unavailable"
	const (
		overCapReason  = "it held 5000 MB, over gate.memory_quit_mb's 4096 MB"
		pressureReason = "the memory limit of /user.slice had 512 MB left of 16384 MB, and this run's processes held only 1500 MB of the 15872 MB in use; " +
			"no other process under it holds memory the guard could read; it was the largest test binary at 1500 MB"
	)
	// killedPackageEvents are what go test reports for the package of a killed binary.
	killedPackageEvents := []string{
		`{"Action":"output","Package":"example.com/m/p","Test":"TestHog","Output":"signal: killed\n"}`,
		`{"Action":"fail","Package":"example.com/m/p"}`,
	}
	tests := []struct {
		name string
		// guarding is whether the request turns the memory guard on.
		guarding bool
		// slicePressure has the guard see a small run under a memory limit other load filled, instead of a tree over the cap.
		slicePressure bool
		// events are further test events the command prints.
		events     []string
		wantKilled bool
		// wantRerun is whether the result is marked for the slice-pressure rerun.
		wantRerun  bool
		wantStarts int32
	}{
		{name: "AGuardKillFailsACommandThatExitedZero", guarding: true, wantKilled: true, wantStarts: 1},
		{name: "ASlicePressureKillMarksTheRunForARerun", guarding: true, slicePressure: true, wantKilled: true, wantRerun: true, wantStarts: 1},
		{
			name: "TheKilledPackagesOwnFailureKeepsTheRerun", guarding: true, slicePressure: true,
			events: killedPackageEvents, wantKilled: true, wantRerun: true, wantStarts: 1,
		},
		{
			name: "AnotherPackagesFailureBesideASlicePressureKillIsJudged", guarding: true, slicePressure: true,
			events: append(slices.Clone(killedPackageEvents), `{"Action":"fail","Package":"example.com/m/other"}`), wantKilled: true, wantStarts: 1,
		},
		{
			name: "AFailedTestBesideASlicePressureKillIsJudged", guarding: true, slicePressure: true,
			events: []string{`{"Action":"fail","Package":"example.com/m/p","Test":"TestOther"}`}, wantKilled: true, wantStarts: 1,
		},
		{name: "NoGuardLeavesAPassingCommandPassing", guarding: false, wantKilled: false, wantStarts: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			reason := overCapReason
			if tt.slicePressure {
				reason = pressureReason
			}
			wantLine := memoryGuardLine(filepath.Base(testBinaryPath), testBinaryPID, guardTestDir, guardFilter, reason, tt.slicePressure)
			killed := filepath.Join(dir, "killed")
			command := "echo " + shortage
			if tt.guarding {
				// The command reports a test as started, then waits for the stand-in guard's kill before it exits 0.
				// printf takes each event verbatim, where echo would expand the JSON's own escapes.
				events := append([]string{`{"Action":"run","Package":"example.com/m/p","Test":"TestHog"}`}, tt.events...)
				command = fmt.Sprintf(`printf '%%s\n' '%s'; until [ -f %q ]; do sleep 0.01; done; echo %s`, strings.Join(events, "' '"), killed, shortage)
			}
			var starts atomic.Int32
			var stderr bytes.Buffer
			request := testRequest{
				root: dir, stderr: &stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay, memoryGuarding: tt.guarding,
				memoryGuardFor: func(limit int64, out io.Writer) memoryGuard {
					starts.Add(1)
					return memoryGuard{
						limit:  guardLimit,
						floor:  guardFloor,
						period: time.Millisecond,
						sample: func(root int) ([]proc.ProcessSample, error) {
							if tt.slicePressure {
								return []proc.ProcessSample{{PID: testBinaryPID, Parent: root, Path: testBinaryPath, RSS: 1500 << 20}}, nil
							}
							return guardTree(3000<<20, 2000<<20), nil
						},
						own: func() ([]proc.ProcessSample, error) {
							return []proc.ProcessSample{{PID: testBinaryPID, Path: testBinaryPath, RSS: 1500 << 20}}, nil
						},
						pressure: func() (proc.MemoryLimit, bool) {
							if tt.slicePressure {
								return proc.MemoryLimit{Used: 16384<<20 - 512<<20, Max: 16384 << 20, Cgroup: "/user.slice"}, true
							}
							return proc.MemoryLimit{}, false
						},
						holders: func(string) ([]proc.ProcessSample, error) { return nil, nil },
						describe: func(pid int) (proc.Process, bool) {
							process := proc.Process{PID: pid, Cwd: guardTestDir, Argv: []string{testBinaryPath, testRunFlagPrefix + guardFilter}}
							return process, pid == testBinaryPID
						},
						kill:  func(int) error { return os.WriteFile(killed, nil, 0o644) },
						after: time.After,
						out:   out,
					}
				},
			}
			run := tierRun{
				number:      0,
				tier:        gateslot.Tier{Command: command, Counter: counterGoJSON, TimeoutMin: 1},
				invocations: []invocation{{dir: dir}},
			}

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			if got := starts.Load(); got != tt.wantStarts {
				t.Errorf("runTier() built the memory guard %d times; want %d", got, tt.wantStarts)
			}
			if result.memoryStopped != tt.wantKilled {
				t.Errorf("runTier() memoryStopped = %v; want %v", result.memoryStopped, tt.wantKilled)
			}
			if !tt.wantKilled {
				if result.record.ExitCode != 0 || stderr.Len() != 0 {
					t.Errorf("runTier() exit = %d, stderr = %q; want exit 0 and no stderr", result.record.ExitCode, stderr.String())
				}
				return
			}
			if result.record.ExitCode != 1 {
				t.Errorf("runTier() exit = %d; want 1 for a command that exited 0 after a guard kill", result.record.ExitCode)
			}
			if !slices.Contains(result.causes, wantLine) || !strings.Contains(stderr.String(), wantLine) || !strings.Contains(string(result.log), wantLine+"\n") {
				t.Errorf("runTier() causes = %q, stderr = %q, log = %q; want the kill line %q in each", result.causes, stderr.String(), result.log, wantLine)
			}
			if result.slicePressure != tt.wantRerun || stoppedOnSlicePressure(result) != tt.wantRerun {
				t.Errorf("runTier() slicePressure = %v, stoppedOnSlicePressure() = %v; want %v for both", result.slicePressure, stoppedOnSlicePressure(result), tt.wantRerun)
			}
			if !slices.ContainsFunc(result.causes, func(cause string) bool {
				return strings.Contains(cause, "example.com/m/p") && strings.Contains(cause, "TestHog")
			}) {
				t.Errorf("runTier() causes = %q; want one naming the test TestHog that was running in example.com/m/p", result.causes)
			}
			if resourceShortage(result.log) == "" {
				t.Fatalf("runTier() log = %q; want it to hold the shortage marker the command printed", result.log)
			}
			if got := failedOnShortage(result); got != "" {
				t.Errorf("failedOnShortage() = %q; want none, so a run the guard killed is never rerun", got)
			}
		})
	}
}

// TestRunTierStartFailureKeepsTheShellRefusal pins the refusal of a tier whose command cannot be started.
// The command's directory is missing, so the start fails before the command runs.
// Under a memory_mb that is no cap failure, so the refusal keeps the shell's way forward and names no cap.
func TestRunTierStartFailureKeepsTheShellRefusal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell")
	}

	tests := []struct {
		name     string
		memoryMB int
		want     []string
		wantNot  string
	}{
		{name: "UnderAMemoryCapTheShellRefusalStands", memoryMB: 64, want: []string{"could not start the shell for tier0"}, wantNot: "memory_mb"},
		{name: "WithNoCapTheShellRefusalStands", want: []string{"could not start the shell for tier0"}, wantNot: "memory_mb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			missing := filepath.Join(t.TempDir(), "missing")
			request := testRequest{root: t.TempDir(), timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay}
			run := tierRun{
				number:      0,
				tier:        gateslot.Tier{Command: "true", Counter: counterGoJSON, TimeoutMin: 1, MemoryMB: tt.memoryMB},
				invocations: []invocation{{dir: missing}},
			}

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			for _, want := range tt.want {
				if !strings.Contains(result.refusal, want) {
					t.Errorf("runTier() refusal = %q; want it to hold %q", result.refusal, want)
				}
			}
			if tt.wantNot != "" && strings.Contains(result.refusal, tt.wantNot) {
				t.Errorf("runTier() refusal = %q; want it without %q", result.refusal, tt.wantNot)
			}
		})
	}
}

// TestRunTierNoTestNotice pins the line a passing `vstest` invocation prints when its output reported no test.
// The command reports one test for a target holding "ran" and prints only a restore line for any other.
func TestRunTierNoTestNotice(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell case")
	}

	const summary = "Passed!  - Failed:     0, Passed:     1, Skipped:     0, Total:     1, Duration: 1 s - ran.dll (net8.0)"
	command := "case \"{targets}\" in *ran*) echo '" + summary + "';; *bad*) exit 3;; *) echo 'Restored the project';; esac"
	// notice builds the expected line; project "{dir}" stands for the invocation's run directory.
	notice := func(project, classes string) string {
		return "gate test: tier0 ran no test of " + project + classes + ": dotnet test exited 0 and reported no test, as a test project does that sets IsTestProject false unless an MSBuild property is passed, so the gate's pass covers none of its tests; the gate reads only the invocation's total, so it names no property and sees no skip inside an invocation that ran some test; way forward: when those tests must run and the hub's gate.refuse_raw does not list dotnet test, run them by hand with dotnet test <project> -p:<property>=true, the property its project file's IsTestProject condition names, and check that the run's summary line reports them; otherwise report the skipped tests to your parent"
	}
	filtered := func(others, wayForward string) string {
		return "gate test: tier0 ran no test of slow.csproj: dotnet test exited 0 and no test of slow.csproj matched the filter among the words after \"--\"; " + others + "; way forward: " + wayForward
	}
	tests := []struct {
		name        string
		counter     string
		invocations []invocation
		// args are the call's own words after `--`, and otherTiers the declared tiers the call does not run.
		args       []string
		otherTiers []int
		// wantExit is the tier's exit code.
		wantExit int
		want     []string
	}{
		{
			name: "ACallerFilterThatMatchesNoTestNamesTheTiersTheCallDoesNotRun", counter: counterVSTest,
			invocations: []invocation{{targets: []string{"slow.csproj"}}}, args: []string{"--filter", "FullyQualifiedName~Ns.Slow"}, otherTiers: []int{1, 3},
			want: []string{filtered("the hub also declares tier1, tier3, which this call does not run, and the test may belong to one of them",
				"run the test through the tier that holds it, with lym gate test --tier <N> <its package or test project> -- <args> and the same words after \"--\"")},
		},
		{
			name: "ACallerFilterInAHubWithNoOtherTierNamesTheFilter", counter: counterVSTest,
			invocations: []invocation{{targets: []string{"slow.csproj"}}}, args: []string{"-p:VSTestTestCaseFilter=Name=X"},
			want: []string{filtered("the hub declares no tier besides those this call runs", "check that the filter selects a test of the project, then re-run the same command")},
		},
		{name: "AProjectThatReportsATestPrintsNoLine", counter: counterVSTest, invocations: []invocation{{targets: []string{"ran.csproj"}}}},
		{
			name:    "ANarrowedProjectThatReportsNoTestIsNamedWithItsClassFilter",
			counter: counterVSTest,
			invocations: []invocation{
				{targets: []string{"ran.csproj"}},
				{targets: []string{"slow.csproj"}, terms: []string{"FullyQualifiedName~Ns.SlowA", "FullyQualifiedName~Ns.SlowB"}, partial: true},
			},
			want: []string{notice("slow.csproj", " under the class filter FullyQualifiedName~Ns.SlowA|FullyQualifiedName~Ns.SlowB")},
		},
		{name: "AWholeRunThatReportsNoTestIsNamed", counter: counterVSTest, invocations: []invocation{{targets: []string{"all.sln"}}}, want: []string{notice("all.sln", "")}},
		{name: "AnInvocationWithNoTargetsNamesItsRunDirectory", counter: counterVSTest, invocations: []invocation{{}}, want: []string{notice("{dir}", "")}},
		{name: "AFailingInvocationPrintsNoLine", counter: counterVSTest, invocations: []invocation{{targets: []string{"bad.csproj"}}}, wantExit: 3},
		{name: "AGoJSONRunPrintsNoLine", counter: counterGoJSON, invocations: []invocation{{targets: []string{"slow"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for i := range tt.invocations {
				tt.invocations[i].dir = dir
			}
			var stderr bytes.Buffer
			request := testRequest{root: dir, stderr: &stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay}
			run := tierRun{number: 0, tier: gateslot.Tier{Command: command, Counter: tt.counter, TimeoutMin: 1}, invocations: tt.invocations, args: tt.args, otherTiers: tt.otherTiers}

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			if result.refusal != "" || result.record.ExitCode != tt.wantExit {
				t.Fatalf("runTier() = refusal %q, exit %d; want exit %d and no refusal", result.refusal, result.record.ExitCode, tt.wantExit)
			}
			var want []string
			for _, line := range tt.want {
				want = append(want, strings.ReplaceAll(line, "{dir}", dir))
			}
			got := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
			if stderr.Len() == 0 {
				got = nil
			}
			if !slices.Equal(got, want) {
				t.Errorf("runTier() stderr lines = %q; want %q", got, want)
			}
			for _, line := range want {
				if !strings.Contains(string(result.log), line+"\n") {
					t.Errorf("runTier() log = %q; want it to hold %q", result.log, line)
				}
			}
		})
	}
}

// A `Total tests:` summary one invocation leaves open takes no outcome line of the next, on a tier with no results file.
func TestRunTierEndsAVSTestSummaryWithItsInvocation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell case")
	}

	command := "case \"{targets}\" in *cut*) echo 'Total tests: 1';; *) echo '     Passed: 1';; esac"
	dir := t.TempDir()
	request := testRequest{root: dir, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay}
	run := tierRun{
		tier:        gateslot.Tier{Command: command, Counter: counterVSTest, TimeoutMin: 1},
		invocations: []invocation{{dir: dir, targets: []string{"cut.csproj"}}, {dir: dir, targets: []string{"next.csproj"}}},
	}

	result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

	if result.refusal != "" || result.record.Discovered != 1 || result.record.Passed != 0 {
		t.Errorf("runTier() = refusal %q, discovered %d, passed %d; want no refusal, 1 and 0", result.refusal, result.record.Discovered, result.record.Passed)
	}
}

// A run's stops are slice pressure alone only when every one is a slice-pressure kill of the memory guard.
func TestRunStopsSlicePressureOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pressure := guardKill{line: "pressure kill", slicePressure: true}
	overCap := guardKill{line: "over-cap kill"}
	tests := []struct {
		name  string
		stops runStops
		want  bool
	}{
		{"NoStop", runStops{}, false},
		{"SlicePressureKills", runStops{guardKills: []guardKill{pressure, pressure}}, true},
		{"BesideAnOverCapKill", runStops{guardKills: []guardKill{pressure, overCap}}, false},
		{"BesideTheMemoryCap", runStops{guardKills: []guardKill{pressure}, memoryCap: "memory cap"}, false},
		{"BesideTheProcessCap", runStops{guardKills: []guardKill{pressure}, processCap: "process cap"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.stops.slicePressureOnly(); got != tt.want {
				t.Errorf("slicePressureOnly() = %v; want %v", got, tt.want)
			}
		})
	}
}

// A call closes with the memory pressure or runner crash line only when no failure judged the code.
// A run that judged no code stops no --stop-on-fail call.
func TestUnjudgedRunsClosingLineAndStopsCall(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	pressure := runEnd{code: 1, ran: true, pressureStop: true}
	crash := runEnd{code: 134, ran: true, crash: "SIGABRT"}
	failure := runEnd{code: 2, ran: true}
	pressureLine := gateslot.MemoryPressureLine("tier1 (exit 1)")
	tests := []struct {
		name string
		ends []runEnd
		// lintFailed is whether a lint step found something.
		lintFailed bool
		want       string
		// wantStops is whether each of ends stops a --stop-on-fail call.
		wantStops []bool
	}{
		{name: "NothingFailed"},
		{name: "APressureStop", ends: []runEnd{pressure}, want: pressureLine, wantStops: []bool{false}},
		{name: "ACrash", ends: []runEnd{crash}, want: gateslot.RunnerCrashLine("tier0 (exit 134, SIGABRT)"), wantStops: []bool{false}},
		{name: "ACrashAndAPressureStopCloseWithThePressureLine", ends: []runEnd{crash, pressure}, want: gateslot.MemoryPressureLine("tier1 (exit 1)"), wantStops: []bool{false, false}},
		{name: "APressureStopBesideAFailure", ends: []runEnd{pressure, failure}, wantStops: []bool{false, true}},
		{name: "APressureStopBesideALintFinding", ends: []runEnd{pressure}, lintFailed: true, wantStops: []bool{false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var runs unjudgedRuns
			for i, end := range tt.ends {
				label := "tier0"
				if end.pressureStop {
					label = "tier1"
				}
				runs.add(label, end)
				if got := stopsCall(end); got != tt.wantStops[i] {
					t.Errorf("stopsCall(%+v) = %v; want %v", end, got, tt.wantStops[i])
				}
			}
			runs.judged = runs.judged || tt.lintFailed
			if got := runs.closingLine(); got != tt.want {
				t.Errorf("closingLine() = %q; want %q", got, tt.want)
			}
		})
	}
}

// The kill line names the tier's memory cap in full, as the key to raise.
func TestTierMemoryCapLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := tierMemoryCapLine("tier1", 64)

	for _, want := range []string{"gate test: tier1 held more than its gate.tiers.tier1.memory_mb of 64 MB", `or raise gate.tiers.tier1.memory_mb with "lym config gate" from the prime`} {
		if !strings.Contains(got, want) {
			t.Errorf("tierMemoryCapLine(tier1, 64) = %q; want it to hold %q", got, want)
		}
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
		{
			name: "PressureStopTwice",
			end:  runEnd{code: 1, ran: true, pressureStop: true},
			want: "tier1 (exit 1, a memory guard kill in its run and its rerun for a memory limit other load filled: an infrastructure stop, not a test failure; way forward: re-run the same command once that load ends)",
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
