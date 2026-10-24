// tierrun.go runs one tier whose gate slots are already held: its prebuild, its command per invocation, its counter, its timeout and the record of how it ended.

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
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// tierResult is how one tier run ended.
type tierResult struct {
	// record holds what the tier run itself knows, the per-package launch list of a go-json tier included; the caller adds the worktree, slug, tier list and wait start before it writes the record.
	record gateslot.RunRecord
	// log is the run's whole output as the counter passed it on, with the timeout line when the tier timed out.
	log []byte
	// refusal is set instead of an outcome when the tier could not run at all: no process started, a prebuild failed or a command could not be rendered.
	refusal string
	// trimpathNote is the line saying the tier built with `-trimpath`, empty when it did not or ran no command.
	trimpathNote string
	// buildFailed is set instead of an outcome when the build step that precedes the tier failed or timed out, so the tier ran no command and is reported blocked.
	buildFailed bool
}

// runTier runs the tier of run in ctx with its slots held: it prebuilds `lyx` when the tier asks, then runs each invocation's command through the platform shell as a tied child, stopping at the first non-zero exit.
// The tier's timeout_min bounds the prebuild and every invocation together, counted from the call; a tier that outlives it has its process tree killed, exits 1 and prints and logs gateslot.TierTimeoutLine.
// env is the environment every command gets, before the tier's own env is added.
// When the resulting GOFLAGS carries `-trimpath`, it logs a line saying so before the first command and returns the line for the caller to print.
// A run with no invocation passes at once with nothing discovered.
func runTier(ctx context.Context, out io.Writer, run tierRun, env []string, request testRequest) tierResult {
	started := time.Now()
	timeout := time.Duration(run.tier.TimeoutMin) * request.timeoutUnit
	record := gateslot.RunRecord{Tier: run.number, ScopeForm: run.scopeForm, Slots: run.tier.Slots, GuardStep: run.guardStep, Start: started}

	var logBuffer bytes.Buffer
	counter, err := newCounter(run.tier.Counter, io.MultiWriter(out, &logBuffer))
	if err != nil {
		return tierResult{refusal: fmt.Sprintf("gate test: %v; way forward: fix the counter of %s with \"lyx config gate\" from the prime%s", err, tierName(run.number), roleClause)}
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

	timedOut := ctx.Err() == nil && errors.Is(tierCtx.Err(), context.DeadlineExceeded) && exit != 0
	if timedOut {
		exit = 1
		line := gateslot.TierTimeoutLine(tierName(run.number), timeout)
		logBuffer.WriteString(line + "\n")
		printLine(request.stderr, line)
	}

	if exit == 0 {
		if line := emptyTierFailure(run, request.root, counter.TestsRun(), request.args); line != "" {
			exit = 1
			logBuffer.WriteString(line + "\n")
			printLine(request.stderr, line)
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
	return tierResult{record: record, log: logBuffer.Bytes(), trimpathNote: trimpathNote}
}

// runInvocations prebuilds `lyx` when the tier asks, then runs each invocation of run in order under ctx, stopping at the first non-zero exit.
// It returns that exit code, the commands it rendered and a refusal when a command could not be run at all.
func runInvocations(ctx context.Context, run tierRun, counter counter, env []string, request testRequest) (exit int, commands []string, refusal string) {
	if len(run.invocations) == 0 {
		return 0, nil, ""
	}
	if run.tier.PrebuiltLyx {
		bin, cleanup, err := prebuildLyx(ctx, request.goBinary, request.root, run.tier.Parallel, request.prebuiltStore)
		if err != nil {
			if ctx.Err() != nil {
				return 1, nil, ""
			}
			return 0, nil, fmt.Sprintf("gate test: cannot build lyx for the tier: %v; way forward: fix the build, then re-run the same command", err)
		}
		defer cleanup()
		if bin != "" {
			env = append(slices.Clone(env), gateslot.PrebuiltLyxEnv+"="+bin)
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
		runErr := runTiedChild(cmd, func(pid int) {
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
		exit = exitCodeOf(runErr)
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

// filterFlags are the go test flags whose filter may match nothing in a package, so a run that carries one cannot tell a missing tag from an empty match.
var filterFlags = []string{"run", "skip", "list", "bench"}

// emptyTierFailure returns the failure line of a passing go-json tier run in which a package ran no test although it holds a Test function in a file whose build constraint holds under the tier's tag, `tierN`, on the host's GOOS and GOARCH; it returns "" when no package does.
// ran is the per-package count of tests that ran, keyed by import path, with root the worktree root that the run's invocation directories resolve against.
// A package belongs to the module, among those the invocations run in, with the longest module path that prefixes it.
// args are the call's own words after `--`: a filter among them exempts the run, since it may match nothing in a package; the run's args are not read, as the guard step appends its own `-run` filter there.
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
	return fmt.Sprintf("gate test: %s ran no test in %s although it holds tests under the build tag %s; the tier's command lacks its tag; way forward: add -tags %s to the tier's command with \"lyx config gate\" from the prime%s", tag, strings.Join(empty, ", "), tag, tag, roleClause)
}

// isFilterFlag reports whether arg is one of filterFlags, spelled with one or two dashes, with or without the `test.` prefix and with or without an `=` value.
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

// packageDir returns the directory of the package with import path pkg inside the module of modules, a map from module path to directory, whose path is the longest prefix of pkg; it returns "" when no module holds pkg.
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

// isTestName reports whether name is a Test function name go test runs: `Test` followed by no lowercase letter, `TestMain` apart.
func isTestName(name string) bool {
	rest, found := strings.CutPrefix(name, "Test")
	if !found || name == "TestMain" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || !unicode.IsLower(first)
}

// guardStepRun returns the guard step of one tier group over the guard tests guards names for the module at moduleDir: one run of the command of the tier numbered number with the guard packages as {targets}, and `-run '^(<names>)$'` after the call's own words as {args}.
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

// guardPackages returns the module-relative package arguments of the directories that hold a guard test, in directory order, and the sorted guard test names of them all.
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
