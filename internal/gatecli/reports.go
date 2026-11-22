// reports.go judges the report steps a `--gate` key's reports list and writes their reports.
// The steps may judge in the background beside the call's last tier; the call writes their reports after its lints.

package gatecli

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"sync"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
)

// reportSteps names the report steps a gate key's reports list.
type reportSteps struct {
	reach, redundancy, mutation bool
}

// reportStepsOf returns the report steps reports lists.
func reportStepsOf(reports []string) reportSteps {
	return reportSteps{
		reach:      slices.Contains(reports, gateslot.ReportReach),
		redundancy: slices.Contains(reports, gateslot.ReportRedundancy),
		mutation:   slices.Contains(reports, gateslot.ReportMutation),
	}
}

// any reports whether the key lists a report step.
func (r reportSteps) any() bool {
	return r.reach || r.redundancy || r.mutation
}

// lastTierStart returns the index of the first run of plan's highest tier, the run the report steps start beside.
// It returns -1 for an empty plan.
func lastTierStart(plan []tierRun) int {
	start := -1
	for i, run := range plan {
		if start < 0 || run.number > plan[start].number {
			start = i
		}
	}
	return start
}

// judgedReports are one call's report steps with their reports, judged and not yet written.
// Each step prints into a buffer of its own while it judges, and the memory guard over the steps into another,
// so the lines reach the call's stderr in step order once the reports are written.
type judgedReports struct {
	steps reportSteps

	reach       reachStep
	reachReport ReachReport
	reachLines  bytes.Buffer

	redundancy       redundancyStep
	redundancyReport RedundancyReport
	redundancyKept   bool
	redundancyLines  bytes.Buffer

	mutation       mutationStep
	mutationReport MutationReport
	mutationLines  bytes.Buffer

	// memoryQuit and guarding are the call's memory guard limit and whether the guard applies.
	memoryQuit int64
	guarding   bool
	// tierCommands are the call's running tier commands, whose test binaries the steps' guard leaves to the tier's own.
	tierCommands *runningCommands
	guardLines   bytes.Buffer
}

// newJudgedReports returns the report steps of the call e runs, unjudged.
// It reads e on the calling goroutine, so the steps may then judge on another while e's tiers run.
func (e *execution) newJudgedReports(steps reportSteps) *judgedReports {
	j := &judgedReports{
		steps: steps, reach: e.reachStep(), redundancy: e.redundancyStep(), mutation: e.mutationStep(),
		memoryQuit: e.request.memoryQuit, guarding: e.request.memoryGuarding, tierCommands: e.request.commands,
	}
	j.reach.stderr, j.redundancy.stderr, j.mutation.reach.stderr = &j.reachLines, &j.redundancyLines, &j.mutationLines
	return j
}

// guard returns the memory guard over the steps, printing its kills to the guard lines.
// It leaves a test binary below a running tier command to the tier's own guard.
func (j *judgedReports) guard() memoryGuard {
	guard := newMemoryGuard(j.memoryQuit, &j.guardLines)
	guard.foreign = j.tierCommands.holds
	return guard
}

// judge judges each listed step under ctx, reach first and mutation last, under one memory guard.
// The guard leaves alone a test binary below a running tier command, which the tier's own guard watches.
// red tells a call one of whose runs failed, and inHub a call inside a hub; either way the reports then judge nothing.
func (j *judgedReports) judge(ctx context.Context, red, inHub bool) {
	stopGuard, guardKilled := startMemoryGuard(ctx, j.guard(), os.Getpid(), j.guarding)
	defer stopGuard()
	if j.steps.reach {
		j.reach.ctx, j.reach.guardKilled = ctx, guardKilled
		j.reachReport = j.reach.judgeReport(red, inHub)
	}
	if j.steps.redundancy {
		j.redundancy.ctx, j.redundancy.guardKilled = ctx, guardKilled
		j.redundancyReport, j.redundancyKept = j.redundancy.judgeReport(red, inHub)
	}
	if j.steps.mutation {
		j.mutation.reach.ctx, j.mutation.reach.guardKilled = ctx, guardKilled
		j.mutationReport = j.mutation.judgeReport(red, inHub)
	}
}

// write writes each judged report in step order and prints to stderr the step's lines followed by its report's.
// The memory guard's lines print first.
func (j *judgedReports) write(stderr io.Writer) {
	flushLines(stderr, &j.guardLines)
	if j.steps.reach {
		flushLines(stderr, &j.reachLines)
		j.reach.stderr = stderr
		j.reach.writeReport(j.reachReport)
	}
	if j.steps.redundancy {
		flushLines(stderr, &j.redundancyLines)
		j.redundancy.stderr = stderr
		j.redundancy.writeReport(j.redundancyReport, j.redundancyKept)
	}
	if j.steps.mutation {
		flushLines(stderr, &j.mutationLines)
		j.mutation.reach.stderr = stderr
		j.mutation.writeReport(j.mutationReport)
	}
}

// flushLines writes the lines held in lines to w, and nothing when w is nil.
func flushLines(w io.Writer, lines *bytes.Buffer) {
	if w == nil {
		return
	}
	if _, err := lines.WriteTo(w); err != nil {
		logger.Warn("gate test: could not write the report steps' lines to stderr", "cause", err)
	}
}

// reportChain is one call's report steps judging in the background, as a green call's, beside the call's last tier.
type reportChain struct {
	cancel context.CancelFunc
	done   chan struct{}
	judged *judgedReports
}

// startReports starts judging the report steps of the call e runs in the background, under a context ctx parents.
func (e *execution) startReports(ctx context.Context, steps reportSteps, inHub bool) *reportChain {
	chainCtx, cancel := context.WithCancel(ctx)
	chain := &reportChain{cancel: cancel, done: make(chan struct{}), judged: e.newJudgedReports(steps)}
	go func() {
		defer close(chain.done)
		chain.judged.judge(chainCtx, false, inHub)
	}()
	return chain
}

// wait returns the judged reports once the steps end.
func (c *reportChain) wait() *judgedReports {
	<-c.done
	c.cancel()
	return c.judged
}

// stop cancels the steps and returns once they end; their reports are dropped.
// It may be called again, and after wait.
func (c *reportChain) stop() {
	c.cancel()
	<-c.done
}

// runningCommands is the set of tier command processes a call is running, safe for concurrent use.
// A nil set records nothing and holds no pid.
type runningCommands struct {
	mu   sync.Mutex
	pids map[int]bool
}

// add records the tier command with pid as running.
func (c *runningCommands) add(pid int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pids == nil {
		c.pids = map[int]bool{}
	}
	c.pids[pid] = true
}

// remove records the tier command with pid as ended.
func (c *runningCommands) remove(pid int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pids, pid)
}

// holds reports whether the tier command with pid is running.
func (c *runningCommands) holds(pid int) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pids[pid]
}
