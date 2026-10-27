// build.go runs the hub's shared build step once per `lym gate test` call inside the slots of the first tier that opts into it, and reports the tiers a failed build blocks.

package gatecli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"slices"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// scopeBuild labels the run record of the build step.
const scopeBuild = "build"

// runBuild runs the hub's build command inside the slots run's tier holds, in the build's dir, as a tied child of the platform shell, with env, the build's own env and no word the call gave after `--`.
// Its output passes to the call's stdout and into its log, and the build is bounded by its own timeout_min counted from now.
// It writes the build's run record, scope form build, for the tier it precedes, prints its summary line and marks the build started, so no later tier runs it again.
// ok is false when the tier must not run its command: the result then holds a refusal when the shell could not start, and marks the tier blocked when the build failed or timed out, recording its log path and exit code.
func (e *execution) runBuild(run tierRun, env []string) (result tierResult, ok bool) {
	e.buildDone = true
	step := e.cfg.Build
	dir := resolveRunDir(e.request.root, step.Dir)
	timeout := time.Duration(step.TimeoutMin) * e.request.timeoutUnit
	started := time.Now()

	buildCtx, cancel := context.WithTimeout(e.ctx, timeout)
	defer cancel()

	var logBuffer bytes.Buffer
	shellName, shellFlag := verifyrun.ShellFor(runtime.GOOS)
	cmd := exec.CommandContext(buildCtx, shellName, shellFlag, step.Command)
	cmd.Dir = dir
	cmd.Env = slices.Concat(env, renderBuildEnv(step))
	cmd.Stdout = io.MultiWriter(e.out, &logBuffer)
	cmd.Stderr = io.MultiWriter(e.out, &logBuffer)
	logger.Info("gate test: spawning the build", "before", tierName(run.number), "dir", dir, "command", step.Command)
	runErr := runTiedChild(cmd, nil)
	exit := exitCodeOf(runErr)
	logger.Info("gate test: the build ended", "before", tierName(run.number), "exit", exit, "error", runErr)
	if cmd.Process == nil && e.ctx.Err() == nil {
		return tierResult{refusal: fmt.Sprintf("gate test: could not start the shell for the build in %s: %v; way forward: make build.dir an existing directory and put the platform shell on PATH, then re-run the same command", dir, runErr)}, false
	}

	timedOut := e.ctx.Err() == nil && errors.Is(buildCtx.Err(), context.DeadlineExceeded) && exit != 0
	if timedOut {
		exit = 1
		line := gateslot.BuildTimeoutLine(timeout)
		logBuffer.WriteString(line + "\n")
		printLine(e.request.stderr, line)
	}

	end := time.Now()
	record := gateslot.RunRecord{
		Tier: run.number, Command: step.Command, RunDir: dir, ScopeForm: scopeBuild, Slots: run.tier.Slots,
		Start: started, End: end, WallSec: end.Sub(started).Seconds(), ExitCode: exit, TimedOut: timedOut, Outcome: "passed", Build: true,
	}
	if exit != 0 {
		record.Outcome = "failed"
	}
	logPath := e.writeRecord(record, logBuffer.Bytes(), "build")
	printLine(e.request.stderr, gateslot.BuildSummaryLine(record, logPath))
	if exit == 0 {
		return tierResult{}, true
	}
	e.buildFailure, e.buildExit = logPath, exit
	return tierResult{buildFailed: true}, false
}

// reportBlocked writes the run record of a tier that opts into the failed build and ran no command, outcome failed with the build's exit code and the build's log as blocked_by, and names the tier and the log on stderr.
// It prints no tier summary line, and returns the build's exit code with the tier marked blocked.
func (e *execution) reportBlocked(run tierRun) runEnd {
	now := time.Now()
	record := gateslot.RunRecord{
		Tier: run.number, ScopeForm: run.scopeForm, Slots: run.tier.Slots, GuardStep: run.guardStep,
		WaitStart: now, Start: now, End: now, ExitCode: e.buildExit, Outcome: "failed", BlockedBy: e.buildFailure,
	}
	line := fmt.Sprintf("gate test: %s did not run, since the build failed; see %s", tierName(run.number), e.buildFailure)
	e.writeRecord(record, []byte(line+"\n"), tierName(run.number))
	printLine(e.request.stderr, line)
	if e.request.observer.Ended != nil {
		e.request.observer.Ended(record)
	}
	return runEnd{code: e.buildExit, halt: e.ctx.Err() != nil, ran: true, blocked: true}
}

// renderBuildEnv returns the build's env as KEY=VALUE entries in key order.
func renderBuildEnv(step gateslot.BuildStep) []string {
	keys := make([]string, 0, len(step.Env))
	for key := range step.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]string, len(keys))
	for i, key := range keys {
		entries[i] = key + "=" + step.Env[key]
	}
	return entries
}
