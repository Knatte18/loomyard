// tierrun.go runs one tier whose gate slots are already held:
// its prebuild, its command per invocation, its counter, its timeout and the record of how it ended.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// tierResult is how one tier run ended.
type tierResult struct {
	// record is what the tier run knows, its per-package launch list included; the caller adds the rest before writing it.
	record gateslot.RunRecord
	// log is the run's whole output as the counter passed it on, with the timeout line when the tier timed out.
	log []byte
	// refusal is set instead of an outcome when the tier cannot run at all: prebuild, render or shell start failed.
	refusal string
	// trimpathNote is the line saying the tier built with `-trimpath`, empty when it did not or ran no command.
	trimpathNote string
	// buildFailed is set when the build step before the tier failed or timed out; the tier then ran no command.
	buildFailed bool
	// shortage is the marker of a tier that failed on a shortage in its run and again in its rerun; empty otherwise.
	shortage string
	// crash is the signal of a tier that crashed its test runner in its run and again in its rerun; empty otherwise.
	crash string
	// processCapped is set when the process cap killed the run's processes; such a run is never rerun.
	processCapped bool
	// memoryStopped is set when the memory guard or the tier's memory cap killed a process of the run;
	// such a run is never rerun, bar one whose stops were all slice-pressure kills.
	memoryStopped bool
	// slicePressure is set when other load filled the memory limit, so the run judged no code and is rerun once.
	// Every stop of such a run was a slice-pressure kill of the memory guard.
	// No test of it failed, and no package of it failed without its test binary being killed.
	slicePressure bool
	// pressureStop is set on a rerun when the run and its rerun both ended on slice-pressure kills alone; false otherwise.
	pressureStop bool
	// failures, causes and unparsed feed the failure report: failed tests, lines the verb states, and unparsed output.
	failures []runFailure
	causes   []string
	unparsed []string
	// strayLine is the stray-file line the run failed on, empty when it left no new untracked file.
	strayLine string
	// copies holds the result of each copy when the tier ran as --copies copies; empty for a tier run that ran once.
	copies []tierResult
}

// runTier runs the tier of run in ctx with its slots held, once or as copies.
// A request with copiesRun above 0 runs that many copies of the tier side by side, each under the same bounds.
// A run with an invocation lists the worktree's untracked, non-ignored files before and after it.
// It fails when the run left a file the first listing lacked.
// A worktree git cannot list is not checked.
// A run with an invocation first prints envOverrideLine for the tier's env against the session's.
// The line is printed once, whatever the copies, and heads the result's log.
func runTier(ctx context.Context, out io.Writer, run tierRun, env []string, request testRequest) tierResult {
	before, listed := untrackedBeforeRun(run, request.root)
	return runTierChecked(ctx, out, run, env, request, before, listed)
}

// runTierChecked runs the tier of run in ctx with its slots held, once or as copies, and fails it on a stray file.
// before is the worktree's untracked, non-ignored files from before the tier's first attempt.
// listed says that listing could be taken; without it the run is not checked.
// A caller that reruns the tier passes the same listing to every attempt,
// so a file an earlier attempt left still fails the run.
func runTierChecked(ctx context.Context, out io.Writer, run tierRun, env []string, request testRequest, before map[string]bool, listed bool) tierResult {
	envLine := ""
	if len(run.invocations) > 0 {
		envLine = envOverrideLine(tierName(run.number), renderTierEnv(run.tier), request.sessionEnv)
	}
	if envLine != "" {
		printLine(request.stderr, envLine)
	}
	var result tierResult
	if request.copiesRun > 0 {
		result = runTierCopies(ctx, run, env, request)
	} else {
		result = runTierCommands(ctx, out, run, env, request)
	}
	if envLine != "" && result.refusal == "" {
		result.log = append([]byte(envLine+"\n"), result.log...)
	}
	if !listed || result.refusal != "" {
		return result
	}
	return checkStrayFiles(result, before, tierName(run.number), request)
}

// runTierCommands runs the tier of run once in ctx with its slots held.
// It prebuilds `lym` when the tier asks and a test the run executes may use it.
// Each invocation's command then runs through the platform shell as a tied child.
// The run stops at the first non-zero exit.
// The tier's gate.tiers.<tier>.timeout_min bounds the prebuild and every invocation together, counted from the call;
// a tier that outlives it has its process tree killed, exits 1 and prints and logs gateslot.TierTimeoutLine.
// env is the environment every command gets, before the tier's own env is added.
// When the resulting GOFLAGS carries `-trimpath`, it logs a line saying so before the first command,
// and it returns that line for the caller to print.
// Every command's GOFLAGS then opens with gateslot.TestCacheOffFlag, so `go test` keeps no test result cache.
// A passing `vstest` invocation that reported no test prints the line naming it to stderr and the log.
// A failed run prints and logs each run-level message its test results files report, such as a test host crash.
// It counts each among the causes.
// A run with no invocation passes at once with nothing discovered.
func runTierCommands(ctx context.Context, out io.Writer, run tierRun, env []string, request testRequest) tierResult {
	started := time.Now()
	timeout := time.Duration(run.tier.TimeoutMin) * request.timeoutUnit
	record := gateslot.RunRecord{Tier: run.number, ScopeForm: run.scopeForm, Slots: run.tier.Slots, GuardStep: run.guardStep, Start: started}

	var logBuffer bytes.Buffer
	counter, err := newCounter(run.tier.Counter, io.MultiWriter(out, &logBuffer))
	if err != nil {
		return tierResult{refusal: fmt.Sprintf("gate test: %v; way forward: fix the counter of %s with \"lym config gate\" from the prime%s", err, tierName(run.number), roleClause)}
	}

	tierCtx, cancel := withTimeoutMin(ctx, timeout, elapsedSinceNow(), time.After)
	defer cancel()

	tierEnv := slices.Concat(env, renderTierEnv(run.tier))
	trimpathNote := ""
	if len(run.invocations) > 0 {
		trimpathNote = trimpathLine(tierName(run.number), tierEnv)
	}
	if trimpathNote != "" {
		logBuffer.WriteString(trimpathNote + "\n")
	}
	notice := func(line string) {
		logBuffer.WriteString(line + "\n")
		printLine(request.stderr, line)
	}
	exit, commands, stops, refusal := runInvocations(tierCtx, run, counter, gateslot.WithTestCacheOff(tierEnv), request, notice)
	if refusal != "" {
		return tierResult{refusal: refusal}
	}
	if err := counter.Flush(); err != nil {
		logger.Warn("gate test: could not flush the tier's output", "tier", tierName(run.number), "cause", err)
	}

	var causes []string
	if exit != 0 {
		// A crashed test host fails the run with no failed test, so its results file's message is the only cause.
		for _, line := range resultsFileLines(counter, tierName(run.number)) {
			logBuffer.WriteString(line + "\n")
			printLine(request.stderr, line)
			causes = append(causes, line)
		}
	}
	timedOut := ctx.Err() == nil && errors.Is(context.Cause(tierCtx), errTimeoutMinPassed) && exit != 0
	if timedOut {
		exit = 1
		line := gateslot.TierTimeoutLine(tierName(run.number), timeout)
		logBuffer.WriteString(line + "\n")
		printLine(request.stderr, line)
		causes = append(causes, line)
	}
	if stops.processCap != "" {
		if exit == 0 {
			exit = 1
		}
		logBuffer.WriteString(stops.processCap + "\n")
		printLine(request.stderr, stops.processCap)
		causes = append(causes, stops.processCap)
	}
	// The memory guard printed each kill's line as it happened; the tier's cap line is printed here.
	if stops.memoryCap != "" {
		printLine(request.stderr, stops.memoryCap)
	}
	memoryLines := stops.memoryLines()
	for _, line := range memoryLines {
		if exit == 0 {
			exit = 1
		}
		logBuffer.WriteString(line + "\n")
		causes = append(causes, line)
	}
	if line := unfinishedTestsLine(counter.Unfinished()); line != "" && len(memoryLines) > 0 {
		logBuffer.WriteString(line + "\n")
		causes = append(causes, line)
	}

	if exit == 0 {
		if line := emptyTierFailure(run, request.root, counter.TestsRun(), request.args); line != "" {
			exit = 1
			logBuffer.WriteString(line + "\n")
			printLine(request.stderr, line)
			causes = append(causes, line)
		}
	}

	counts := counter.Counts()
	end := time.Now()
	record.Command = strings.Join(commands, "; ")
	if len(run.invocations) > 0 {
		record.RunDir = resolveRunDir(request.root, run.invocations[0].dir)
	}
	record.End = end
	record.WallSec = end.Sub(started).Seconds()
	record.ExitCode = exit
	record.TimedOut = timedOut
	record.Outcome = "passed"
	if exit != 0 {
		record.Outcome = "failed"
	}
	record.Discovered, record.Passed, record.Failed, record.Skipped = counts.discovered, counts.passed, counts.failed, counts.skipped
	record.Launches = counter.Launches()
	failures := counter.Failures()
	record.Failures = testFailures(failures)
	return tierResult{
		record: record, log: logBuffer.Bytes(), trimpathNote: trimpathNote,
		processCapped: stops.processCap != "", memoryStopped: len(stops.memoryLines()) > 0,
		slicePressure: stops.slicePressureOnly() && record.Failed == 0 && !failedBeyondKills(failures),
		failures:      failures, causes: causes, unparsed: counter.Unparsed(),
	}
}

// timeoutMinPoll is the longest a gate.tiers.<tier>.timeout_min bound goes without reading the clock.
const timeoutMinPoll = 10 * time.Second

// errTimeoutMinPassed is the cause of a context its gate.tiers.<tier>.timeout_min bound ended.
var errTimeoutMinPassed = errors.New("the timeout_min bound passed")

// withTimeoutMin returns a context under ctx that ends with the cause errTimeoutMinPassed once timeout has passed.
// The time passed counts from the call on the monotonic clock and on the wall clock, whichever has run further.
// The monotonic clock stands still while the host sleeps,
// so a bound the host slept through still ends the context within timeoutMinPoll of its waking.
// elapsed reads the time passed since the call on both clocks, and after waits as time.After does.
// The cancel it returns ends the context and the goroutine that watches the bound.
func withTimeoutMin(ctx context.Context, timeout time.Duration, elapsed func() (monotonic, wall time.Duration), after func(time.Duration) <-chan time.Time) (context.Context, context.CancelFunc) {
	bounded, cancel := context.WithCancelCause(ctx)
	go func() {
		for {
			monotonic, wall := elapsed()
			left := timeout - max(monotonic, wall)
			if left <= 0 {
				cancel(errTimeoutMinPassed)
				return
			}
			select {
			case <-bounded.Done():
				return
			case <-after(min(left, timeoutMinPoll)):
			}
		}
	}()
	return bounded, func() { cancel(context.Canceled) }
}

// elapsedSinceNow returns a reading of the time passed since this call, on the monotonic clock and on the wall clock.
func elapsedSinceNow() func() (monotonic, wall time.Duration) {
	start := time.Now()
	return func() (time.Duration, time.Duration) {
		// Round(0) strips the monotonic reading, so the second difference is the wall clock's.
		return time.Since(start), time.Now().Round(0).Sub(start.Round(0))
	}
}

// envOverrideLine returns the line naming each entry of tierEnv whose variable session sets to another value.
// tierEnv holds the tier's own env entries, and session the environment of the session that started the call.
// A result the tier gives reflects the tier's values, so a session that measures through the gate must know them.
// The line names each variable with the tier's value only, since the session's value may be a secret the tier overrides.
// A variable the session leaves unset is not named, since the session reads no value of its own for it.
// Variable names compare without case on Windows, where the environment does.
// It returns "" when no entry differs.
func envOverrideLine(tier string, tierEnv, session []string) string {
	envName := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(name)
		}
		return name
	}
	sessionValues := make(map[string]string, len(session))
	for _, entry := range session {
		if name, value, ok := strings.Cut(entry, "="); ok {
			sessionValues[envName(name)] = value
		}
	}
	var differing []string
	for _, entry := range tierEnv {
		name, value, _ := strings.Cut(entry, "=")
		if held, set := sessionValues[envName(name)]; set && held != value {
			differing = append(differing, name+"="+value)
		}
	}
	if len(differing) == 0 {
		return ""
	}
	return fmt.Sprintf("gate test: %s runs with env values that differ from this session's environment: %s; "+
		"a result of %s reflects the tier's values, not the session's; "+
		"way forward: read the result under those values, or change tiers.%s.env with \"lym config gate\" from the prime",
		tier, strings.Join(differing, ", "), tier, tier)
}

// runStops is what the guards over a tier's commands stopped.
type runStops struct {
	// processCap is the process cap's line once it killed the run's processes, empty otherwise.
	processCap string
	// guardKills holds the memory guard's kill of each test binary it killed.
	guardKills []guardKill
	// memoryCap is the line of a kill at the tier's memory cap, empty otherwise.
	memoryCap string
}

// memoryLines returns the memory guard's lines followed by the tier's memory cap line, when there is one.
func (s runStops) memoryLines() []string {
	var lines []string
	for _, kill := range s.guardKills {
		lines = append(lines, kill.line)
	}
	if s.memoryCap != "" {
		lines = append(lines, s.memoryCap)
	}
	return lines
}

// slicePressureOnly reports whether the guards' only stops of the run were the memory guard's slice-pressure kills.
func (s runStops) slicePressureOnly() bool {
	if len(s.guardKills) == 0 || s.memoryCap != "" || s.processCap != "" {
		return false
	}
	for _, kill := range s.guardKills {
		if !kill.slicePressure {
			return false
		}
	}
	return true
}

// failedBeyondKills reports whether failures holds a failure a killed test binary does not explain:
// a failed test, or a package that failed outside any test with its binary not killed.
func failedBeyondKills(failures []runFailure) bool {
	return slices.ContainsFunc(failures, func(failure runFailure) bool {
		return failure.test != "" || !failure.killed
	})
}

// stopped reports whether any guard stopped part of the run.
func (s runStops) stopped() bool {
	return s.processCap != "" || len(s.memoryLines()) > 0
}

// tierMemoryCapLine is the line a tier run prints and logs once the kernel killed a process of it at its memory cap.
func tierMemoryCapLine(tier string, memoryMB int) string {
	capKey := tierKeyName(tier, tierMemoryMBLeaf)
	return fmt.Sprintf("gate test: %s held more than its %s of %d MB, its builds and test binaries together, "+
		"so the kernel killed the largest of them and the run fails; "+
		"way forward: find the package whose tests hold memory without bound and fix it, then re-run the same command, "+
		"or raise %s with \"lym config gate\" from the prime%s", tier, capKey, memoryMB, capKey, roleClause)
}

// unfinishedTestsShown caps the tests the line of unfinished tests names per package.
const unfinishedTestsShown = 10

// unfinishedTestsLine returns the line naming the tests a memory kill left running, or "" when there are none.
// unfinished maps a package to the tests that started in it and never reported a result, as the counter reads them.
// Each package names at most unfinishedTestsShown tests.
func unfinishedTestsLine(unfinished map[string][]string) string {
	if len(unfinished) == 0 {
		return ""
	}
	var packages []string
	for _, pkg := range slices.Sorted(maps.Keys(unfinished)) {
		tests := unfinished[pkg]
		shown := tests[:min(len(tests), unfinishedTestsShown)]
		named := pkg + ": " + strings.Join(shown, ", ")
		if more := len(tests) - len(shown); more > 0 {
			named += fmt.Sprintf(" and %d more", more)
		}
		packages = append(packages, named)
	}
	return "gate test: the tests running when the memory kill ended their test binary, which never reported a result: " + strings.Join(packages, "; ")
}

// tierMemoryCapRefusal is the refusal of a tier whose command did not run under its memory cap of memoryMB.
// err wraps proc.ErrMemoryCap and says why: a scope that never ran the command, or a cap that could not be set.
func tierMemoryCapRefusal(tier string, memoryMB int, dir string, err error) string {
	capKey := tierKeyName(tier, tierMemoryMBLeaf)
	return fmt.Sprintf("gate test: the command of %s in %s did not run under its %s of %d MB: %v; "+
		"way forward: run from a session that reaches the systemd user manager, which must hand the memory controller to its scopes, "+
		"or remove %s with \"lym config gate\" from the prime, then re-run the same command%s", tier, dir, capKey, memoryMB, err, capKey, roleClause)
}

// testFailures returns the failed tests of failures as the run record carries them.
// Each keeps its first line; package failures are left out.
func testFailures(failures []runFailure) []gateslot.TestFailure {
	var tests []gateslot.TestFailure
	for _, failure := range failures {
		if failure.test == "" {
			continue
		}
		first := ""
		if len(failure.lines) > 0 {
			first = failure.lines[0]
		}
		tests = append(tests, gateslot.TestFailure{Package: failure.pkg, Test: failure.test, FirstLine: first})
	}
	return tests
}

// shortageMarkers are lowercase fragments of the errors a process or thread creation prints.
// They appear when the user's task limit or the machine's process table is momentarily exhausted:
// Go's fork/exec and thread-creation EAGAIN and a shell's failed fork.
var shortageMarkers = []string{"resource temporarily unavailable", "failed to create new os thread", "cannot fork"}

// resourceShortage returns the first of shortageMarkers that log holds, matched without case.
// It returns "" when log holds none.
func resourceShortage(log []byte) string {
	lower := bytes.ToLower(log)
	for _, marker := range shortageMarkers {
		if bytes.Contains(lower, []byte(marker)) {
			return marker
		}
	}
	return ""
}

// failedOnShortage returns the resource-shortage marker of a tier run that ran to a failing exit without timing out.
// It returns "" for any other result, a run the process cap or a memory guard stopped included:
// its shortage is the runaway's own, which a rerun would only repeat.
func failedOnShortage(result tierResult) string {
	if result.refusal != "" || result.buildFailed || result.record.ExitCode == 0 || result.record.TimedOut || result.processCapped || result.memoryStopped {
		return ""
	}
	return resourceShortage(result.log)
}

// crashSignals names the signals a test runner dies of when it crashes, as opposed to being stopped from outside:
// an abort, a bad instruction, a trap, a bus or segmentation fault and a floating-point fault.
var crashSignals = map[syscall.Signal]string{
	syscall.SIGABRT: "SIGABRT",
	syscall.SIGILL:  "SIGILL",
	syscall.SIGTRAP: "SIGTRAP",
	syscall.SIGBUS:  "SIGBUS",
	syscall.SIGSEGV: "SIGSEGV",
	syscall.SIGFPE:  "SIGFPE",
}

// runnerCrash returns the name of the crash signal that ended a tier run with no failing test.
// It reads the signal from an exit code of 128 plus the signal's number, as a shell reports it.
// It returns "" for any other result.
// A refusal, a blocked or timed-out run and a run with a failing test are no crash:
// each says something about the code under test, or none ran.
func runnerCrash(result tierResult) string {
	if result.refusal != "" || result.buildFailed || result.record.TimedOut || result.record.Failed > 0 || len(result.record.Failures) > 0 {
		return ""
	}
	return crashSignals[syscall.Signal(result.record.ExitCode-128)]
}

// stoppedOnSlicePressure reports whether a tier run that ran and did not time out ended on slice-pressure kills alone.
func stoppedOnSlicePressure(result tierResult) bool {
	return result.refusal == "" && !result.buildFailed && !result.record.TimedOut && result.slicePressure
}

// retryTier runs attempt, the tier named tier, and returns its result.
// It reruns the tier after delay if the run failed on a shortage, or crashed its test runner with no failing test.
// It does so too if the memory guard stopped the run for a memory limit other load filled.
// None of them judges the code under test.
// The record of every attempt stopped so, the first or the rerun, carries MemoryPressure.
// The line announcing the rerun goes to stderr and opens the rerun's log.
// Once the delay has passed, recordFirst gets the first run's result,
// so its record stays on file beside the rerun's, which is marked Rerun.
// A rerun that fails on a shortage again carries its marker in shortage,
// and a rerun that crashes after a first run that crashed carries its signal in crash and its record's RunnerCrash.
// A rerun that the guard stops so after a first run it stopped so carries pressureStop.
// A rerun that crashes after a first run that failed on a shortage carries no crash:
// only one of the two runs crashed.
// A ctx cancelled before or during the delay returns the first result.
func retryTier(ctx context.Context, stderr io.Writer, tier string, delay time.Duration, attempt func() tierResult, recordFirst func(tierResult)) tierResult {
	result := attempt()
	result.record.MemoryPressure = stoppedOnSlicePressure(result)
	marker := failedOnShortage(result)
	signal := ""
	if marker == "" {
		signal = runnerCrash(result)
	}
	pressured := marker == "" && signal == "" && stoppedOnSlicePressure(result)
	if (marker == "" && signal == "" && !pressured) || ctx.Err() != nil {
		return result
	}
	cause := fmt.Sprintf("failed on a resource shortage (%q in its output), an environment error a rerun clears", marker)
	if signal != "" {
		cause = fmt.Sprintf("crashed its test runner (exit %d, %s) with no failing test, which judges no code", result.record.ExitCode, signal)
		result.record.RunnerCrash = signal
	}
	if pressured {
		cause = "had a test binary killed by the memory guard for a memory limit other load filled, which judges no code"
	}
	line := fmt.Sprintf("gate test: %s %s; running it once more in %s", tier, cause, delay)
	printLine(stderr, line)
	logger.Warn("gate test: rerunning a tier that failed on its environment", "tier", tier, "marker", marker, "signal", signal, "delay", delay)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return result
	case <-timer.C:
	}
	recordFirst(result)
	retried := attempt()
	retried.record.Rerun = true
	retried.shortage = failedOnShortage(retried)
	if retried.shortage == "" && signal != "" {
		retried.crash = runnerCrash(retried)
		retried.record.RunnerCrash = retried.crash
	}
	retried.pressureStop = pressured && stoppedOnSlicePressure(retried)
	retried.record.MemoryPressure = stoppedOnSlicePressure(retried)
	retried.log = append([]byte(line+"\n"), retried.log...)
	return retried
}

// runInvocations prebuilds `lym` when the tier asks and a test the run executes may use it.
// It then runs each invocation of run in order under ctx, stopping at the first non-zero exit.
// For a command holding {test_targets} it first narrows each invocation's targets.
// It keeps the packages holding a test of the tier.
// An invocation left with no package runs no command, and a line on request.stderr says so.
// While a command runs, the process cap watches every process carrying env's run marker, when env sets one,
// and the memory guard watches its test binaries, when the request turns it on.
// A tier with memory_mb runs each command under that cap, and a command that cannot be started under it is a refusal.
// A tier with `results: trx` opens each invocation's {args} with vstestResultsArgs over a fresh temp directory.
// It refuses a call whose own args hold a results directory or a trx logger, since the gate owns those words.
// Once the command ends, the counter reads its whole output.
// The results files there then replace the counts and failures its console output gave, and the directory is removed.
// After each passing invocation it hands notice the line of noTestNotice, when that line is not empty.
// It returns that exit code, the commands it rendered, what the guards stopped,
// and a refusal when a command could not be run at all.
func runInvocations(ctx context.Context, run tierRun, counter counter, env []string, request testRequest, notice func(string)) (exit int, commands []string, stops runStops, refusal string) {
	if len(run.invocations) == 0 {
		return 0, nil, runStops{}, ""
	}
	if refusal := resultsArgsRefusal(run.number, run.tier, run.args); refusal != "" {
		return 0, nil, runStops{}, refusal
	}
	if needsPrebuiltLym(run, request.root) {
		bin, cleanup, err := prebuildLym(ctx, request.goBinary, request.root, run.tier.Parallel, request.prebuiltStore, request.pipeWaitDelay, nil)
		if err != nil {
			if ctx.Err() != nil {
				return 1, nil, runStops{}, ""
			}
			return 0, nil, runStops{}, fmt.Sprintf("gate test: cannot build lym for the tier: %v; way forward: fix the build, then re-run the same command", err)
		}
		defer cleanup()
		if bin != "" {
			env = append(slices.Clone(env), gateslot.PrebuiltLymEnv+"="+bin)
		}
	}

	shellName, shellFlag := verifyrun.ShellFor(runtime.GOOS)
	marker := runMarker(env)
	capFor := request.processCapFor
	if capFor == nil {
		capFor = newProcessCap
	}
	guardFor := request.memoryGuardFor
	if guardFor == nil {
		guardFor = newMemoryGuard
	}
	for _, invocation := range run.invocations {
		dir := resolveRunDir(request.root, invocation.dir)
		testTargets := invocation.targets
		if strings.Contains(run.tier.Command, gateslot.TestTargetsPlaceholder) {
			testTargets = tierTestTargets(ctx, request.goBinary, dir, run, invocation.targets, env)
			if len(testTargets) == 0 {
				printLine(request.stderr, fmt.Sprintf("gate test: no package of %s in %s holds a %s test, so the command of %s does not run there", strings.Join(invocation.targets, " "), dir, tierName(run.number), tierName(run.number)))
				continue
			}
		}
		args, resultsDir := run.args, ""
		// A summary or a failure block the last invocation left open ends here, with or without a results file.
		vstest, _ := counter.(*vstestCounter)
		if vstest != nil {
			vstest.startInvocation()
		}
		if run.tier.Results == gateslot.ResultsTRX {
			if dir, err := lymtemp.MkdirTemp("gate-vstest-"); err != nil {
				logger.Warn("gate test: could not make the vstest results directory; the console counts stand", "tier", tierName(run.number), "cause", err)
			} else {
				resultsDir = dir
				args = append(vstestResultsArgs(dir), run.args...)
			}
		}
		command, err := renderTierCommand(run.number, run.tier, invocation.targets, testTargets, invocation.terms, args)
		if err != nil {
			removeResultsDir(resultsDir)
			return 0, commands, runStops{}, err.Error()
		}
		commands = append(commands, command)
		discoveredBefore := counter.Counts().discovered

		cmd := exec.CommandContext(ctx, shellName, shellFlag, command)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdout = counter
		cmd.Stderr = counter
		logger.Info("gate test: spawning tier command", "tier", tierName(run.number), "dir", dir, "command", command)
		watchCtx, stopWatching := context.WithCancel(ctx)
		var watchdogDone sync.WaitGroup
		capKilled, runErr := runTiedChild(cmd, request.pipeWaitDelay, run.tier.MemoryLimit(), func(pid int) {
			if marker != "" && request.samplePeriod > 0 {
				watchdogDone.Add(1)
				go func() {
					defer watchdogDone.Done()
					stops.processCap = capFor(marker, request.samplePeriod).run(watchCtx)
				}()
			}
			if request.memoryGuarding {
				watchdogDone.Add(1)
				go func() {
					defer watchdogDone.Done()
					stops.guardKills = guardFor(request.memoryQuit, request.stderr).run(watchCtx, pid)
				}()
			}
			if !request.watching {
				return
			}
			watchdogDone.Add(1)
			go func() {
				defer watchdogDone.Done()
				newWatchdog(request.busyQuit, request.samplePeriod, request.stderr).run(watchCtx, pid)
			}()
		})
		stopWatching()
		watchdogDone.Wait()
		if capKilled {
			stops.memoryCap = tierMemoryCapLine(tierName(run.number), run.tier.MemoryMB)
		}
		exit = tierExitCodeOf(runErr)
		logger.Info("gate test: tier command ended", "tier", tierName(run.number), "exit", exit, "error", runErr, "memory_cap_kill", capKilled)
		if resultsDir != "" {
			// A last console line without a newline is still pending, and would count on top of the file's results.
			if err := counter.Flush(); err != nil {
				logger.Warn("gate test: could not flush the tier's output", "tier", tierName(run.number), "cause", err)
			}
			if results, read := readTRXResults(resultsDir); read {
				vstest.replaceInvocation(results)
			}
			removeResultsDir(resultsDir)
		}
		if errors.Is(runErr, proc.ErrMemoryCap) && ctx.Err() == nil {
			return 0, commands, runStops{}, tierMemoryCapRefusal(tierName(run.number), run.tier.MemoryMB, dir, runErr)
		}
		if cmd.Process == nil && ctx.Err() == nil {
			return 0, commands, runStops{}, fmt.Sprintf("gate test: could not start the shell for %s in %s: %v; way forward: make the tier's dir an existing directory and put the platform shell on PATH, then re-run the same command", tierName(run.number), dir, runErr)
		}
		if exit != 0 || ctx.Err() != nil || stops.stopped() {
			return exit, commands, stops, ""
		}
		if line := noTestNotice(run, invocation, dir, counter.Counts().discovered-discoveredBefore); line != "" {
			notice(line)
		}
	}
	return 0, commands, runStops{}, ""
}

// removeResultsDir removes a vstest invocation's results directory, doing nothing for an empty dir.
func removeResultsDir(dir string) {
	if dir == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn("gate test: could not remove the vstest results directory", "dir", dir, "cause", err)
	}
}

// noTestNotice returns the line of a passing `vstest` invocation whose output reported no test, or "" for any other.
// discovered is the number of tests the invocation's output reported, and dir is its run directory.
// A test project that sets IsTestProject false unless an MSBuild property is passed builds and exits 0 this way,
// so the line names the project, the class filter the scope gave it, and how to run its tests by hand.
// The line reads only the invocation's total, so it names no MSBuild property.
// It sees no skip inside an invocation that ran some test.
// The hand run is offered only while the hub's `gate.refuse_raw` does not list `dotnet test`.
// A run whose args carry a test filter gets the line of filteredNoTestNotice instead.
func noTestNotice(run tierRun, inv invocation, dir string, discovered int) string {
	if run.tier.Counter != counterVSTest || discovered > 0 {
		return ""
	}
	project := dir
	if len(inv.targets) > 0 {
		project = strings.Join(inv.targets, " ")
	}
	classes := ""
	if len(inv.terms) > 0 {
		classes = " under the class filter " + strings.Join(inv.terms, "|")
	}
	if holdsCallerFilter(run.args) {
		return filteredNoTestNotice(run, project, classes)
	}
	return fmt.Sprintf("gate test: %s ran no test of %s%s: dotnet test exited 0 and reported no test, as a test project does that sets IsTestProject false unless an MSBuild property is passed, so the gate's pass covers none of its tests; the gate reads only the invocation's total, so it names no property and sees no skip inside an invocation that ran some test; way forward: when those tests must run and the hub's %s does not list dotnet test, run them by hand with dotnet test <project> -p:<property>=true, the property its project file's IsTestProject condition names, and check that the run's summary line reports them; otherwise report the skipped tests to your parent", tierName(run.number), project, classes, keyName(refuseRawKey))
}

// filteredNoTestNotice returns the line of a passing `vstest` invocation of project that reported no test.
// It covers the case where the call's own words carried a test filter.
// classes is the clause naming the scope's class filter, empty when it set none.
// The filter most likely names a test of a tier the call does not run,
// so the line names the declared tiers of run.otherTiers and the call that runs the test through its own tier.
func filteredNoTestNotice(run tierRun, project, classes string) string {
	tier := tierName(run.number)
	others := "the hub declares no tier besides those this call runs"
	wayForward := "check that the filter selects a test of the project, then re-run the same command"
	if len(run.otherTiers) > 0 {
		others = fmt.Sprintf("the hub also declares %s, which this call does not run, and the test may belong to one of them", tierNames(run.otherTiers))
		wayForward = "run the test through the tier that holds it, with lym gate test --tier <N> <its package or test project> -- <args> and the same words after \"--\""
	}
	return fmt.Sprintf("gate test: %s ran no test of %s%s: dotnet test exited 0 and no test of %s matched the filter among the words after \"--\"; %s; way forward: %s",
		tier, project, classes, project, others, wayForward)
}

// tierTestTargets returns the packages of targets in the module at moduleDir that hold a test run's tier executes.
// The tiers counted are run's own and those it covers.
// A target that names no directory, such as an import path, is kept.
// It returns targets unnarrowed when their packages cannot be listed or the narrowed list passes greenCommandBudget:
// the narrowing only saves time, so a failure around it runs every target.
func tierTestTargets(ctx context.Context, goBinary, moduleDir string, run tierRun, targets, env []string) []string {
	packages, err := expandPackages(ctx, goBinary, moduleDir, run.tier, targets, env)
	if err != nil {
		logger.Warn("gate test: could not list the packages to keep those with a test of the tier; every target runs", "tier", tierName(run.number), "dir", moduleDir, "cause", err)
		return targets
	}
	tiers := append([]int{run.number}, run.tier.Covers...)
	kept := slices.DeleteFunc(packages, func(pkg string) bool {
		return strings.HasPrefix(pkg, ".") && !impactset.HoldsTierTest(filepath.Join(moduleDir, filepath.FromSlash(pkg)), tiers)
	})
	if len(strings.Join(kept, " ")) > greenCommandBudget {
		logger.Info("gate test: the packages with a test of the tier exceed the command budget; every target runs", "tier", tierName(run.number), "packages", len(kept))
		return targets
	}
	return kept
}

// filterFlags are the go test flags whose filter may match nothing in a package,
// so a run that carries one cannot tell a missing tag from an empty match.
var filterFlags = []string{"run", "skip", "list", "bench"}

// emptyTierFailure returns the failure line of a passing go-json tier run whose package ran no test.
// Such a package holds a Test function in a file that matches the tier's tag `tierN` on the host's GOOS and GOARCH.
// It returns "" when no package does.
// ran is the per-package count of tests that ran, keyed by import path.
// root is the worktree root that the invocation directories resolve against.
// A package belongs to the module, among those the invocations run in, with the longest module path that prefixes it.
// args are the call's own words after `--`.
// A filter among them exempts the run, since it may match nothing in a package.
// The run's args are not read, as the guard step appends its own `-run` filter there.
// A test the run's watch skip names does not count toward a package's tests.
// A run with no counts, such as a vstest one, has nothing to check.
func emptyTierFailure(run tierRun, root string, ran map[string]int, args []string) string {
	if len(ran) == 0 || slices.ContainsFunc(args, isFilterFlag) {
		return ""
	}
	modules := map[string]string{}
	for _, invocation := range run.invocations {
		dir := resolveRunDir(root, invocation.dir)
		if path := modulePathOf(dir); path != "" {
			modules[path] = dir
		}
	}

	tag := tierName(run.number)
	tagged := build.Default
	tagged.BuildTags = []string{tag}
	var empty []string
	for pkg, count := range ran {
		if count > 0 {
			continue
		}
		dir := packageDir(modules, pkg)
		if dir != "" && holdsTierTest(tagged, dir, run.skip) {
			empty = append(empty, pkg)
		}
	}
	if len(empty) == 0 {
		return ""
	}
	sort.Strings(empty)
	return fmt.Sprintf("gate test: %s ran no test in %s although it holds tests under the build tag %s; the tier's command lacks its tag; way forward: add -tags %s to the tier's command with \"lym config gate\" from the prime%s", tag, strings.Join(empty, ", "), tag, tag, roleClause)
}

// isFilterFlag reports whether arg is one of filterFlags.
// It accepts one or two dashes, with or without the `test.` prefix and with or without an `=` value.
func isFilterFlag(arg string) bool {
	name := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "test.")
	name, _, _ = strings.Cut(name, "=")
	return strings.HasPrefix(arg, "-") && slices.Contains(filterFlags, name)
}

// modulePathOf returns the module path that dir's go.mod declares, or "" when dir holds no readable go.mod.
func modulePathOf(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`)
		}
	}
	return ""
}

// packageDir returns the directory of the package with import path pkg, looked up in modules.
// modules maps module paths to directories, and the longest module path that prefixes pkg wins.
// It returns "" when no module holds pkg.
func packageDir(modules map[string]string, pkg string) string {
	best := ""
	for path := range modules {
		if (pkg == path || strings.HasPrefix(pkg, path+"/")) && len(path) > len(best) {
			best = path
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(modules[best], filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(pkg, best), "/")))
}

// holdsTierTest reports whether a `_test.go` file in dir matches the build tags of tagged and declares a Test function.
// A Test function named in skipped does not count.
// A file that cannot be matched or parsed is left out: a build failure already failed the run.
func holdsTierTest(tagged build.Context, dir string, skipped []string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		if matches, err := tagged.MatchFile(dir, name); err != nil || !matches {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && isTestName(fn.Name.Name) && !slices.Contains(skipped, fn.Name.Name) {
				return true
			}
		}
	}
	return false
}

// isTestName reports whether name is a Test function name go test runs:
// `Test` followed by no lowercase letter, with `TestMain` apart.
func isTestName(name string) bool {
	rest, found := strings.CutPrefix(name, "Test")
	if !found || name == "TestMain" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || !unicode.IsLower(first)
}

// guardStepRun returns the guard step of one tier group for the module at moduleDir.
// The step runs the command of the tier numbered number, with the guard packages of guards as {targets}.
// Its {args} are the call's own words followed by `-run '^(<names>)$'`.
func guardStepRun(cfg gateslot.Config, moduleDir string, number int, guards map[string][]string, request testRequest) (tierRun, error) {
	tier, err := cfg.Tier(number)
	if err != nil {
		return tierRun{}, fmt.Errorf("gate test: %w", err)
	}
	packages, names := guardPackages(guards)
	return tierRun{
		number:      number,
		tier:        tier,
		scopeForm:   scopeGuard,
		invocations: []invocation{{dir: moduleDir, targets: packages}},
		args:        append(slices.Clone(request.args), "-run", "^("+strings.Join(names, "|")+")$"),
		guardStep:   true,
	}, nil
}

// guardPackages returns the module-relative package arguments of the directories holding a guard test.
// The packages come in directory order; the names are the sorted guard test names of all of them.
func guardPackages(guards map[string][]string) (packages, names []string) {
	var dirs []string
	for dir, dirNames := range guards {
		if len(dirNames) > 0 {
			dirs = append(dirs, dir)
			names = append(names, dirNames...)
		}
	}
	sort.Strings(dirs)
	sort.Strings(names)
	for _, dir := range dirs {
		if dir == "." {
			packages = append(packages, ".")
			continue
		}
		packages = append(packages, "./"+dir)
	}
	return packages, slices.Compact(names)
}

// resolveRunDir returns dir as an absolute path: itself when absolute, else joined to root.
func resolveRunDir(root, dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(root, dir)
}

// printLine writes line and a newline to w, doing nothing for a nil w.
func printLine(w io.Writer, line string) {
	if w == nil {
		return
	}
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		logger.Warn("gate test: could not write a line to stderr", "cause", err)
	}
}
