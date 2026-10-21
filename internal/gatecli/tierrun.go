// tierrun.go runs one tier whose gate slots are already held: its prebuild, its command per invocation, its counter, its timeout and the record of how it ended.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// tierResult is how one tier run ended.
type tierResult struct {
	// record holds what the tier run itself knows; the caller adds the worktree, slug, tier list and wait start before it writes the record.
	record gateslot.RunRecord
	// log is the run's whole output as the counter passed it on, with the timeout line when the tier timed out.
	log []byte
	// refusal is set instead of an outcome when the tier could not run at all: no process started, a prebuild failed or a command could not be rendered.
	refusal string
}

// runTier runs the tier of run in ctx with its slots held: it prebuilds `lyx` when the tier asks, then runs each invocation's command through the platform shell as a tied child, stopping at the first non-zero exit.
// The tier's timeout_min bounds the prebuild and every invocation together, counted from the call; a tier that outlives it has its process tree killed, exits 1 and prints and logs gateslot.TierTimeoutLine.
// env is the environment every command gets, before the tier's own env is added.
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

	exit, commands, refusal := runInvocations(tierCtx, run, counter, slices.Concat(env, renderTierEnv(run.tier)), request)
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
	return tierResult{record: record, log: logBuffer.Bytes()}
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
		command, err := renderTierCommand(run.tier, invocation.targets, run.args)
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

// runGuardStep runs the guard step of cfg over the guard tests guards names for the module at moduleDir, with its slots already held: one run of guard_tier's command with the guard packages as {targets}, and `-run '^(<names>)$'` after the call's own words as {args}.
func runGuardStep(ctx context.Context, out io.Writer, cfg gateslot.Config, moduleDir string, guards map[string][]string, env []string, request testRequest) tierResult {
	tier, err := cfg.Tier(cfg.GuardTier)
	if err != nil {
		return tierResult{refusal: fmt.Sprintf("gate test: guard_tier: %v", err)}
	}
	packages, names := guardPackages(guards)
	run := tierRun{
		number:      cfg.GuardTier,
		tier:        tier,
		scopeForm:   scopeGuard,
		invocations: []invocation{{dir: moduleDir, targets: packages}},
		args:        append(slices.Clone(request.args), "-run", "^("+strings.Join(names, "|")+")$"),
		guardStep:   true,
	}
	return runTier(ctx, out, run, env, request)
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
