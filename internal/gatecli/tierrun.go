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
	"github.com/Knatte18/loomyard/internal/logger"
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
	// failures, causes and unparsed feed the failure report: failed tests, lines the verb states, and unparsed output.
	failures []runFailure
	causes   []string
	unparsed []string
}

// runTier runs the tier of run in ctx with its slots held.
// It prebuilds `lym` when the tier asks.
// Each invocation's command then runs through the platform shell as a tied child.
// The run stops at the first non-zero exit.
// The tier's timeout_min bounds the prebuild and every invocation together, counted from the call;
// a tier that outlives it has its process tree killed, exits 1 and prints and logs gateslot.TierTimeoutLine.
// env is the environment every command gets, before the tier's own env is added.
// When the resulting GOFLAGS carries `-trimpath`, it logs a line saying so before the first command,
// and it returns that line for the caller to print.
// A run with no invocation passes at once with nothing discovered.
func runTier(ctx context.Context, out io.Writer, run tierRun, env []string, request testRequest) tierResult {
	started := time.Now()
	timeout := time.Duration(run.tier.TimeoutMin) * request.timeoutUnit
	record := gateslot.RunRecord{Tier: run.number, ScopeForm: run.scopeForm, Slots: run.tier.Slots, GuardStep: run.guardStep, Start: started}

	var logBuffer bytes.Buffer
	counter, err := newCounter(run.tier.Counter, io.MultiWriter(out, &logBuffer))
	if err != nil {
		return tierResult{refusal: fmt.Sprintf("gate test: %v; way forward: fix the counter of %s with \"lym config gate\" from the prime%s", err, tierName(run.number), roleClause)}
	}

	tierCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tierEnv := slices.Concat(env, renderTierEnv(run.tier))
	trimpathNote := ""
	if len(run.invocations) > 0 {
		trimpathNote = trimpathLine(tierName(run.number), tierEnv)
	}
	if trimpathNote != "" {
		logBuffer.WriteString(trimpathNote + "\n")
	}
	exit, commands, refusal := runInvocations(tierCtx, run, counter, tierEnv, request)
	if refusal != "" {
		return tierResult{refusal: refusal}
	}
	if err := counter.Flush(); err != nil {
		logger.Warn("gate test: could not flush the tier's output", "tier", tierName(run.number), "cause", err)
	}

	var causes []string
	timedOut := ctx.Err() == nil && errors.Is(tierCtx.Err(), context.DeadlineExceeded) && exit != 0
	if timedOut {
		exit = 1
		line := gateslot.TierTimeoutLine(tierName(run.number), timeout)
		logBuffer.WriteString(line + "\n")
		printLine(request.stderr, line)
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
	return tierResult{record: record, log: logBuffer.Bytes(), trimpathNote: trimpathNote, failures: failures, causes: causes, unparsed: counter.Unparsed()}
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
// It returns "" for any other result.
func failedOnShortage(result tierResult) string {
	if result.refusal != "" || result.buildFailed || result.record.ExitCode == 0 || result.record.TimedOut {
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

// retryTier runs attempt, the tier named tier, and returns its result.
// It reruns the tier after delay if the run failed on a shortage, or crashed its test runner with no failing test.
// Neither judges the code under test.
// The line announcing the rerun goes to stderr and opens the rerun's log.
// Once the delay has passed, recordFirst gets the first run's result,
// so its record stays on file beside the rerun's, which is marked Rerun.
// A rerun that fails on a shortage again carries its marker in shortage,
// and a rerun that crashes after a first run that crashed carries its signal in crash and its record's RunnerCrash.
// A rerun that crashes after a first run that failed on a shortage carries no crash:
// only one of the two runs crashed.
// A ctx cancelled before or during the delay returns the first result.
func retryTier(ctx context.Context, stderr io.Writer, tier string, delay time.Duration, attempt func() tierResult, recordFirst func(tierResult)) tierResult {
	result := attempt()
	marker := failedOnShortage(result)
	signal := ""
	if marker == "" {
		signal = runnerCrash(result)
	}
	if (marker == "" && signal == "") || ctx.Err() != nil {
		return result
	}
	cause := fmt.Sprintf("failed on a resource shortage (%q in its output), an environment error a rerun clears", marker)
	if signal != "" {
		cause = fmt.Sprintf("crashed its test runner (exit %d, %s) with no failing test, which judges no code", result.record.ExitCode, signal)
		result.record.RunnerCrash = signal
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
	retried.log = append([]byte(line+"\n"), retried.log...)
	return retried
}

// runInvocations prebuilds `lym` when the tier asks.
// It then runs each invocation of run in order under ctx, stopping at the first non-zero exit.
// It returns that exit code, the commands it rendered and a refusal when a command could not be run at all.
func runInvocations(ctx context.Context, run tierRun, counter counter, env []string, request testRequest) (exit int, commands []string, refusal string) {
	if len(run.invocations) == 0 {
		return 0, nil, ""
	}
	if run.tier.PrebuiltLym {
		bin, cleanup, err := prebuildLym(ctx, request.goBinary, request.root, run.tier.Parallel, request.prebuiltStore)
		if err != nil {
			if ctx.Err() != nil {
				return 1, nil, ""
			}
			return 0, nil, fmt.Sprintf("gate test: cannot build lym for the tier: %v; way forward: fix the build, then re-run the same command", err)
		}
		defer cleanup()
		if bin != "" {
			env = append(slices.Clone(env), gateslot.PrebuiltLymEnv+"="+bin)
		}
	}

	shellName, shellFlag := verifyrun.ShellFor(runtime.GOOS)
	for _, invocation := range run.invocations {
		command, err := renderTierCommand(run.number, run.tier, invocation.targets, invocation.terms, run.args)
		if err != nil {
			return 0, commands, err.Error()
		}
		commands = append(commands, command)
		dir := resolveRunDir(request.root, invocation.dir)

		cmd := exec.CommandContext(ctx, shellName, shellFlag, command)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdout = counter
		cmd.Stderr = counter
		logger.Info("gate test: spawning tier command", "tier", tierName(run.number), "dir", dir, "command", command)
		watchCtx, stopWatching := context.WithCancel(ctx)
		var watchdogDone sync.WaitGroup
		runErr := runTiedChild(cmd, request.pipeWaitDelay, func(pid int) {
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
		exit = tierExitCodeOf(runErr)
		logger.Info("gate test: tier command ended", "tier", tierName(run.number), "exit", exit, "error", runErr)
		if cmd.Process == nil && ctx.Err() == nil {
			return 0, commands, fmt.Sprintf("gate test: could not start the shell for %s in %s: %v; way forward: make the tier's dir an existing directory and put the platform shell on PATH, then re-run the same command", tierName(run.number), dir, runErr)
		}
		if exit != 0 || ctx.Err() != nil {
			return exit, commands, ""
		}
	}
	return 0, commands, ""
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
		if dir != "" && holdsTierTest(tagged, dir) {
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
// A file that cannot be matched or parsed is left out: a build failure already failed the run.
func holdsTierTest(tagged build.Context, dir string) bool {
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
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && isTestName(fn.Name.Name) {
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
