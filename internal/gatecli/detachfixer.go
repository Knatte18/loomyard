// detachfixer.go runs the card-fixer phase of the detached gate runner.
// It waits on a fixer an earlier run left and hands a red verdict to a cold fixer.
// It re-runs the gate over the tree that fixer left.

package gatecli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

const (
	// fixerReportFile receives the failure report the fixer reads.
	fixerReportFile = "fixer-report.md"
	// fixerReplyFile is the reply file the fixer writes as its final action.
	fixerReplyFile = "fixer-reply.md"
)

// fixerWaitSlice is how long one wait on a fixer blocks before the runner reads its context again.
const fixerWaitSlice = time.Second

// fixerOutcome is how a wait on a fixer ended.
type fixerOutcome struct {
	status     string
	deadReason string
	reply      string
}

// note is the text appended to a re-run's stderr for this outcome: the fixer's reply, or why it ended without one.
func (o fixerOutcome) note() string {
	if o.status == gatefixer.StatusDone {
		return o.reply
	}
	return fmt.Sprintf("The cold fixer ended without a reply (%s); any change it left uncommitted is in the worktree.", o.deadReason)
}

// awaitFixer waits on rec's fixer in slices of `fixerWaitSlice` until the fixer ends.
// It returns ctx's error if ctx is cancelled first.
func awaitFixer(ctx context.Context, kit cardFixerKit, rec *gatefixer.Record) (fixerOutcome, error) {
	for {
		if err := ctx.Err(); err != nil {
			return fixerOutcome{}, err
		}
		result, err := kit.fixer.Await(rec, kit.timeout, fixerWaitSlice)
		if err != nil {
			return fixerOutcome{}, err
		}
		if result.Status != gatefixer.StatusRunning {
			return fixerOutcome{status: result.Status, deadReason: result.DeadReason, reply: result.Reply}, nil
		}
	}
}

// watchPhase watches a phase for a terminating signal and for a passed lease, which both cancel it.
// It returns the phase's context and a closer that ends the watch.
// The closer is called once and reports the abandoned exit: {128 + signal, abandoned} after a signal.
// After a passed lease it reports {1, abandoned} and writes abandonedLine to stderr.
// It reports false when neither event ended the phase.
func (d detachRun) watchPhase(ctx context.Context) (context.Context, func() (detachExit, bool)) {
	signalCtx, caught, stopSignals := catchTerminatingSignals(ctx)
	phaseCtx, cancelPhase := context.WithCancel(signalCtx)
	var leasePassed atomic.Bool
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		d.cancelOnPassedLease(phaseCtx, cancelPhase, &leasePassed)
	}()
	return phaseCtx, func() (detachExit, bool) {
		cancelledDuringPhase := phaseCtx.Err() != nil
		cancelPhase()
		<-watchDone
		stopSignals()
		switch {
		case caught.Load() != 0:
			return detachExit{Code: exitAfterSignal(caught, 1), Abandoned: true}, true
		case cancelledDuringPhase && leasePassed.Load():
			d.appendStderr(abandonedLine(verbTest))
			return detachExit{Code: 1, Abandoned: true}, true
		}
		return detachExit{}, false
	}
}

// runWithFixer runs request as runRequest does, adding the card fixer around a `test` request.
// Before the gate runs, a live fixer record in the key directory is waited on and marked terminal.
// When that record cannot be attached to, its strand is stopped instead.
// A red verdict, reached after the run released its slots, goes to a cold fixer.
// The re-run's stderr carries the fixer's reply, and the re-run's exit is the call's.
// The first run's result ends the call for a pass, an abandoned exit, or a non-zero exit with nothing on stderr.
// A refusal leaves such an exit.
// It also ends the call for a red verdict whose every failed run crashed its test runner.
// It also ends the call for a request that is not `test`, and for a request the seam turns down or errors on.
func (d detachRun) runWithFixer(ctx context.Context, request detachRequest, gateTree func() *cobra.Command, seams detachSeams) detachExit {
	if request.Verb != verbTest {
		return d.runRequest(ctx, request, gateTree)
	}
	kit, on, err := seams.cardFixer(request)
	if err != nil {
		logger.Warn("gate run-detached: the card fixer is off for this run", "dir", d.dir, "cause", err)
		on = false
	}
	store := keyFixerStore{dir: filepath.Dir(d.dir)}
	if exit, ended := d.settleLiveFixer(ctx, request, seams, kit, on, store); ended {
		return exit
	}
	first := d.runRequest(ctx, request, gateTree)
	if !on || first.Abandoned || first.Code == 0 || !d.holdsFailureReport() || d.endedOnRunnerCrash() {
		return first
	}
	return d.fixAndRerun(ctx, request, gateTree, seams, kit, store, first)
}

// holdsFailureReport reports whether the run's stderr file holds text, which a red gate verdict writes and a refusal, whose envelope goes to stdout, does not.
func (d detachRun) holdsFailureReport() bool {
	info, err := os.Stat(d.path(stderrFile))
	return err == nil && info.Size() > 0
}

// endedOnRunnerCrash reports whether the run's stderr file holds the line a call closes with when every failed run crashed its test runner, a failure no change to the code mends.
func (d detachRun) endedOnRunnerCrash() bool {
	report, err := os.ReadFile(d.path(stderrFile))
	if err != nil {
		return false
	}
	_, crashed := gateslot.FindRunnerCrash(string(report))
	return crashed
}

// settleLiveFixer waits on the live fixer record of an earlier run before the gate runs, under `phaseFixing`.
// It stops the record's strand when the record cannot be attached to: the card fixer is off or the wait failed.
// It marks the record dead only once that stop succeeded.
// A failed stop ends the call with exit 1 and the way forward, and leaves the record live.
// It reports true with the exit to write when the call ends here.
func (d detachRun) settleLiveFixer(ctx context.Context, request detachRequest, seams detachSeams, kit cardFixerKit, on bool, store keyFixerStore) (detachExit, bool) {
	live, err := store.Live()
	if err != nil {
		logger.Warn("gate run-detached: could not read the fixer record, so the gate runs without waiting on a fixer", "dir", d.dir, "cause", err)
		return detachExit{}, false
	}
	if live == nil {
		return detachExit{}, false
	}
	d.writePhase(runPhase{State: phaseFixing})
	waitCtx, closeWatch := d.watchPhase(ctx)
	var outcome fixerOutcome
	var waitErr error
	if on {
		outcome, waitErr = awaitFixer(waitCtx, kit, live)
	}
	if exit, abandoned := closeWatch(); abandoned {
		return exit, true
	}
	if on && waitErr == nil {
		if err := store.markTerminal(outcome.status, outcome.deadReason); err != nil {
			logger.Warn("gate run-detached: could not mark the fixer record terminal", "dir", d.dir, "cause", err)
		}
		d.writePhase(runPhase{State: phaseStarting})
		return detachExit{}, false
	}

	stop, err := seams.fixerStopper(request)
	if err == nil {
		err = stop(live.StrandGUID)
	}
	if err == nil {
		err = store.markTerminal(gatefixer.StatusDead, gatefixer.DeadReasonDied)
	}
	if err != nil {
		d.appendStderr(fmt.Sprintf("gate test: the cold fixer strand %s of an earlier run could not be stopped and recorded dead: %v; way forward: run the same gate call again, which retries the stop\n", live.StrandGUID, err))
		return detachExit{Code: 1}, true
	}
	d.writePhase(runPhase{State: phaseStarting})
	return detachExit{}, false
}

// fixAndRerun hands a red verdict to a cold fixer, then runs request again over the tree the fixer left.
// The phase is `phaseFixing` from before the spawn until the re-run starts.
// The report the fixer reads is the first run's stderr.
// A launch that cannot be built or started ends with the first run's exit and a stderr line saying no fixer ran.
// A passed lease or a terminating signal during the phase stops the fixer and ends the call abandoned.
// It marks the fixer's record dead only once the stop succeeded.
func (d detachRun) fixAndRerun(ctx context.Context, request detachRequest, gateTree func() *cobra.Command, seams detachSeams, kit cardFixerKit, store keyFixerStore, first detachExit) detachExit {
	d.writePhase(runPhase{State: phaseFixing})
	reportPath, replyPath := d.path(fixerReportFile), d.path(fixerReplyFile)
	report, err := os.ReadFile(d.path(stderrFile))
	if err == nil {
		err = fsx.AtomicWriteBytes(reportPath, report)
	}
	if err != nil {
		return d.endUnfixed(first, "the failure report could not be written", err)
	}
	spec, err := cardFixerSpec(kit, request, reportPath, replyPath)
	if err != nil {
		return d.endUnfixed(first, "the fixer's prompt could not be built", err)
	}

	waitCtx, closeWatch := d.watchPhase(ctx)
	rec, _, spawnErr := kit.fixer.SpawnOrAttach(store, spec)
	var outcome fixerOutcome
	var waitErr error
	if spawnErr == nil {
		outcome, waitErr = awaitFixer(waitCtx, kit, rec)
	}
	exit, abandoned := closeWatch()
	switch {
	case spawnErr != nil && abandoned:
		return exit
	case spawnErr != nil:
		return d.endUnfixed(first, "the cold fixer could not be started", spawnErr)
	case abandoned:
		d.stopFixer(kit, store, rec)
		return exit
	case waitErr != nil:
		if err := kit.fixer.Stop(rec); err != nil {
			d.appendStderr(fmt.Sprintf("gate test: the cold fixer strand %s could not be waited on (%v) nor stopped (%v); way forward: run the same gate call again, which retries the stop\n", rec.StrandGUID, waitErr, err))
			return detachExit{Code: 1}
		}
		outcome = fixerOutcome{status: gatefixer.StatusDead, deadReason: gatefixer.DeadReasonDied}
	}

	d.rewriteTreeIdentity(seams.treeIdentity(kit.worktreeRoot))
	d.writePhase(runPhase{State: phaseStarting})
	rerun := d.runRequest(ctx, request, gateTree)
	// Marked only once the re-run has a result over the fixer's finished tree;
	// a restart during the re-run still finds the record live.
	if err := store.markTerminal(outcome.status, outcome.deadReason); err != nil {
		logger.Warn("gate run-detached: could not mark the fixer record terminal", "dir", d.dir, "cause", err)
	}
	if !rerun.Abandoned {
		d.appendStderr("\n## Cold fixer reply\n\n" + outcome.note() + "\n")
	}
	return rerun
}

// cardFixerSpec builds the launch of the fixer of request's red call.
// It renders the card body into the shared cold frame.
// The body fills in the gate call, the card ids named by the call's arguments, and the worktree root.
// The launch has no parent.
func cardFixerSpec(kit cardFixerKit, request detachRequest, reportPath, replyPath string) (gatefixer.Spec, error) {
	cardIDs := "none"
	if ids := flagValues(request.Args, "card"); len(ids) > 0 {
		cardIDs = strings.Join(ids, ", ")
	}
	prompt, err := gatefixer.RenderPrompt(gatefixer.PromptInputs{
		StencilsDir: kit.stencilsDir,
		BodyStencil: fixerBodyStencil,
		BodyValues: map[string]string{
			"gate_call":     "lym gate test " + strings.Join(request.Args, " "),
			"card_ids":      cardIDs,
			"worktree_root": kit.worktreeRoot,
		},
		ReportPath: reportPath,
		ReplyPath:  replyPath,
		RepoRoot:   kit.worktreeRoot,
	})
	if err != nil {
		return gatefixer.Spec{}, err
	}
	return gatefixer.Spec{
		Prompt:      prompt,
		ReportPath:  reportPath,
		ReplyPath:   replyPath,
		Role:        gatefixer.RoleGateFixer,
		ModelChoice: kit.model,
		Skills:      languageconfig.AppendSkills(nil, kit.skills),
		Segment:     segmentcolor.Webster,
		Timeout:     kit.timeout,
	}, nil
}

// stopFixer stops rec's strand and marks its record dead only once the stop succeeded;
// a failed stop leaves the record live for the next run to retry.
func (d detachRun) stopFixer(kit cardFixerKit, store keyFixerStore, rec *gatefixer.Record) {
	if err := kit.fixer.Stop(rec); err != nil {
		logger.Warn("gate run-detached: could not stop the fixer strand of an abandoned run", "dir", d.dir, "strand", rec.StrandGUID, "cause", err)
		return
	}
	if err := store.markTerminal(gatefixer.StatusDead, gatefixer.DeadReasonDied); err != nil {
		logger.Warn("gate run-detached: could not mark the fixer record dead", "dir", d.dir, "cause", err)
	}
}

// endUnfixed ends a call whose fixer never ran and returns first;
// before that it writes a stderr line naming what failed and that no fixer ran.
func (d detachRun) endUnfixed(first detachExit, what string, cause error) detachExit {
	d.appendStderr(fmt.Sprintf("gate test: %s (%v); no cold fixer ran\n", what, cause))
	d.writePhase(runPhase{State: phaseRunning})
	return first
}

// rewriteTreeIdentity writes identity as the run's tree record,
// so a front joins the re-run over the tree the fixer left.
func (d detachRun) rewriteTreeIdentity(identity treeRecord) {
	data, err := json.Marshal(identity)
	if err == nil {
		err = fsx.AtomicWriteBytes(d.path(treeFile), data)
	}
	if err != nil {
		logger.Warn("gate run-detached: could not rewrite the tree identity", "dir", d.dir, "cause", err)
	}
}

// appendStderr appends text to the run's stderr file and makes sure its stdout file exists;
// it logs a failure because the text only informs the caller.
func (d detachRun) appendStderr(text string) {
	stdout, err := os.OpenFile(d.path(stdoutFile), os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		err = stdout.Close()
	}
	if err != nil {
		logger.Warn("gate run-detached: could not open the stdout file", "dir", d.dir, "cause", err)
		return
	}
	stderr, err := os.OpenFile(d.path(stderrFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logger.Warn("gate run-detached: could not open the stderr file", "dir", d.dir, "cause", err)
		return
	}
	if _, err := stderr.WriteString(text); err != nil {
		logger.Warn("gate run-detached: could not append to the stderr file", "dir", d.dir, "cause", err)
	}
	if err := stderr.Close(); err != nil {
		logger.Warn("gate run-detached: could not close the stderr file", "dir", d.dir, "cause", err)
	}
}
