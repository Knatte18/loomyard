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

	"github.com/google/go-cmp/cmp"

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
// It also marks the result so no rerun follows.
// A request that turns the guard off builds none.
func TestRunTierMemoryGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell loop")
	}

	const shortage = "resource temporarily unavailable"
	killLine := memoryGuardLine(filepath.Base(testBinaryPath), testBinaryPID, guardTestDir, guardFilter, "it held 5000 MB, over memory_quit_mb's 4096 MB")
	tests := []struct {
		name string
		// guarding is whether the request turns the memory guard on.
		guarding   bool
		wantKilled bool
		wantStarts int32
	}{
		{name: "AGuardKillFailsACommandThatExitedZero", guarding: true, wantKilled: true, wantStarts: 1},
		{name: "NoGuardLeavesAPassingCommandPassing", guarding: false, wantKilled: false, wantStarts: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			killed := filepath.Join(dir, "killed")
			command := "echo " + shortage
			if tt.guarding {
				// The command reports a test as started, then waits for the stand-in guard's kill before it exits 0.
				command = fmt.Sprintf(`echo '{"Action":"run","Package":"example.com/p","Test":"TestHog"}'; until [ -f %q ]; do sleep 0.01; done; echo %s`, killed, shortage)
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
						sample: func(int) ([]proc.ProcessSample, error) { return guardTree(3000<<20, 2000<<20), nil },
						pressure: func() (proc.MemoryLimit, bool) {
							return proc.MemoryLimit{}, false
						},
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
			if !slices.Contains(result.causes, killLine) || !strings.Contains(stderr.String(), killLine) || !strings.Contains(string(result.log), killLine+"\n") {
				t.Errorf("runTier() causes = %q, stderr = %q, log = %q; want the kill line in each", result.causes, stderr.String(), result.log)
			}
			if !slices.ContainsFunc(result.causes, func(cause string) bool {
				return strings.Contains(cause, "example.com/p") && strings.Contains(cause, "TestHog")
			}) {
				t.Errorf("runTier() causes = %q; want one naming the test TestHog that was running in example.com/p", result.causes)
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
		return "gate test: tier0 ran no test of " + project + classes + ": dotnet test exited 0 and reported no test, as a test project does that sets IsTestProject false unless an MSBuild property is passed, so the gate's pass covers none of its tests; the gate reads only the invocation's total, so it names no property and sees no skip inside an invocation that ran some test; way forward: when those tests must run and the hub's refuse_raw does not list dotnet test, run them by hand with dotnet test <project> -p:<property>=true, the property its project file's IsTestProject condition names, and check that the run's summary line reports them; otherwise report the skipped tests to your parent"
	}
	tests := []struct {
		name        string
		counter     string
		invocations []invocation
		// wantExit is the tier's exit code.
		wantExit int
		want     []string
	}{
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
			run := tierRun{number: 0, tier: gateslot.Tier{Command: command, Counter: tt.counter, TimeoutMin: 1}, invocations: tt.invocations}

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

// TestRunTierReadsTheVSTestResultsFile pins a vstest tier's verdict on the TRX file the gate asks for.
// The command finds the results directory among its args, copies a fixture TRX file there,
// prints the summary a detailed console logger prints, which the console counter cannot read, and exits.
func TestRunTierReadsTheVSTestResultsFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell script")
	}

	trx := func(results, runInfo string) string {
		return `<?xml version="1.0" encoding="utf-8"?>
<TestRun xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <Results>` + results + `</Results>
  <ResultSummary outcome="Completed"><RunInfos>` + runInfo + `</RunInfos></ResultSummary>
</TestRun>
`
	}
	const passed = `<UnitTestResult testName="Ns.Tests.Kept" outcome="Passed" />`
	const failed = `<UnitTestResult testName="Ns.Tests.Broken" outcome="Failed"><Output><ErrorInfo>
<Message>Assert.That failed
  Expected: 1</Message><StackTrace>   at Ns.Tests.Broken() in Tests.cs:line 9</StackTrace></ErrorInfo></Output></UnitTestResult>`
	const crash = `<RunInfo outcome="Error"><Text>The active test run was aborted. Reason: Test host process crashed : Fatal error.
The test running when the crash occurred: Ns.Tests.Kept</Text></RunInfo>`
	crashLine := "gate test: the test results file of tier0 reports: The active test run was aborted. Reason: Test host process crashed : Fatal error.; The test running when the crash occurred: Ns.Tests.Kept"
	tests := []struct {
		name         string
		trx          string
		exit         int
		wantRecord   gateslot.RunRecord
		wantFailures []runFailure
		// wantLine is the line stderr, the log and the causes hold, or "" when stderr stays empty.
		wantLine string
	}{
		{
			name:       "ADetailedConsoleLoggerLeavesTheCountsToTheResultsFile",
			trx:        trx(passed, ""),
			wantRecord: gateslot.RunRecord{Discovered: 1, Passed: 1},
		},
		{
			name:         "AFailedTestIsReadFromTheResultsFile",
			trx:          trx(passed+failed, ""),
			exit:         1,
			wantRecord:   gateslot.RunRecord{ExitCode: 1, Discovered: 2, Passed: 1, Failed: 1},
			wantFailures: []runFailure{{test: "Ns.Tests.Broken", lines: []string{"Assert.That failed", "Expected: 1", "at Ns.Tests.Broken() in Tests.cs:line 9"}}},
		},
		{
			name:       "ACrashedTestHostNamesItsCause",
			trx:        trx("", crash),
			exit:       1,
			wantRecord: gateslot.RunRecord{ExitCode: 1},
			wantLine:   crashLine,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			fixture := filepath.Join(dir, "fixture.trx")
			if err := os.WriteFile(fixture, []byte(tt.trx), 0o644); err != nil {
				t.Fatal(err)
			}
			seen := filepath.Join(dir, "results-dir")
			command := fmt.Sprintf(`set -- {args}; while [ "$1" != --results-directory ]; do shift; done; printf %%s "$2" > %q; cp %q "$2/gate_net8.0_1.trx"; echo 'Total tests: 1'; echo '     Passed: 1'; exit %d`, seen, fixture, tt.exit)
			var stderr bytes.Buffer
			request := testRequest{root: dir, stderr: &stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay, args: []string{"--logger", "console;verbosity=detailed"}}
			run := tierRun{number: 0, tier: gateslot.Tier{Command: command, Counter: counterVSTest, TimeoutMin: 1}, invocations: []invocation{{dir: dir}}, args: request.args}

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			if result.refusal != "" {
				t.Fatalf("runTier() refusal = %q; want none", result.refusal)
			}
			got := result.record
			want := tt.wantRecord
			if got.ExitCode != want.ExitCode || got.Discovered != want.Discovered || got.Passed != want.Passed || got.Failed != want.Failed || got.Skipped != want.Skipped {
				t.Errorf("runTier() record = exit %d, discovered %d, passed %d, failed %d, skipped %d; want exit %d, discovered %d, passed %d, failed %d, skipped %d",
					got.ExitCode, got.Discovered, got.Passed, got.Failed, got.Skipped, want.ExitCode, want.Discovered, want.Passed, want.Failed, want.Skipped)
			}
			if diff := cmp.Diff(tt.wantFailures, result.failures, cmp.AllowUnexported(runFailure{})); diff != "" {
				t.Errorf("runTier() failures mismatch (-want +got):\n%s", diff)
			}
			wantStderr := ""
			if tt.wantLine != "" {
				wantStderr = tt.wantLine + "\n"
				if !slices.Contains(result.causes, tt.wantLine) || !strings.Contains(string(result.log), tt.wantLine+"\n") {
					t.Errorf("runTier() causes = %q, log = %q; want both to hold %q", result.causes, result.log, tt.wantLine)
				}
			}
			if stderr.String() != wantStderr {
				t.Errorf("runTier() stderr = %q; want %q", stderr.String(), wantStderr)
			}
			resultsDir, err := os.ReadFile(seen)
			if err != nil {
				t.Fatalf("read the results directory the command saw: %v", err)
			}
			if _, err := os.Stat(string(resultsDir)); !os.IsNotExist(err) {
				t.Errorf("results directory %s after the run: stat error %v; want it removed", resultsDir, err)
			}
		})
	}
}

// TestRunTierNamesEnvThatDiffersFromTheSession pins the line a tier run prints for its env values the session sets otherwise.
func TestRunTierNamesEnvThatDiffersFromTheSession(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tier command is a POSIX shell builtin")
	}

	session := []string{"COMMONLIB_REPO=/session/CommonLib", "SAME=x"}
	tests := []struct {
		name string
		env  map[string]string
		// wantLine is the line stderr and the log hold, or "" when stderr stays empty.
		wantLine string
	}{
		{
			name: "AValueTheSessionSetsOtherwiseIsNamed",
			env:  map[string]string{"COMMONLIB_REPO": "/tier/CommonLib", "SAME": "x", "ONLY_TIER": "y"},
			wantLine: "gate test: tier1 runs with env values that differ from this session's environment: COMMONLIB_REPO=/tier/CommonLib (the session has /session/CommonLib); " +
				"a result of tier1 reflects the tier's values, not the session's; " +
				"way forward: read the result under those values, or change tiers.tier1.env with \"lym config gate\" from the prime" + roleClause,
		},
		{name: "ValuesTheSessionSharesOrLacksNameNothing", env: map[string]string{"SAME": "x", "ONLY_TIER": "y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var stderr bytes.Buffer
			request := testRequest{root: dir, stderr: &stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay, sessionEnv: session}
			run := tierRun{number: 1, tier: gateslot.Tier{Command: "true", Env: tt.env, Counter: counterGoJSON, TimeoutMin: 1}, invocations: []invocation{{dir: dir}}}

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			if result.refusal != "" || result.record.ExitCode != 0 {
				t.Fatalf("runTier() = refusal %q, exit %d; want exit 0 and no refusal", result.refusal, result.record.ExitCode)
			}
			wantStderr := ""
			if tt.wantLine != "" {
				wantStderr = tt.wantLine + "\n"
			}
			if stderr.String() != wantStderr {
				t.Errorf("runTier() stderr = %q; want %q", stderr.String(), wantStderr)
			}
			if tt.wantLine != "" && !strings.Contains(string(result.log), tt.wantLine+"\n") {
				t.Errorf("runTier() log = %q; want it to hold %q", result.log, tt.wantLine)
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
