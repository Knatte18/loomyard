// land.go implements NewLand, the producer that lands a finished child run into the prime:
// it pulls the prime, runs the hub-wide deploy command, prunes the board and, when the pull's records side stopped, tells the orch.

package battenshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// deployLogFileSuffix is the fixed suffix of the file the deploy command's output goes to, joined onto the row's own name.
	deployLogFileSuffix = "-deploy.log"
	// landLockPollBase is the first pause between two attempts on a contended land lock.
	landLockPollBase = time.Second
	// landLockPollCeiling is the longest pause between two attempts on a contended land lock.
	landLockPollCeiling = 30 * time.Second
	// primeDirtyWaitBound is how long the row waits for the prime's tracked files to be clean before it gives up with Stuck.
	primeDirtyWaitBound = 10 * time.Minute
	// primeDirtyPollInterval is the pause between two reads of the prime's uncommitted tracked paths.
	primeDirtyPollInterval = 5 * time.Second
	// deployTimeout is how long the deploy command may run.
	deployTimeout = 30 * time.Minute
	// recordsStopMarkerLine is the row-notice marker line recording that the records-stop notice was posted.
	recordsStopMarkerLine = "records-stop"
)

// LandingSettings is the part of the hub-wide landing config the Land row reads.
type LandingSettings struct {
	// Deploy is the shell command run in the prime after a landing; empty runs nothing.
	Deploy string
	// DeployBranches lists the base branches a landing into which is deployed; an empty list deploys nothing.
	DeployBranches []string
}

// DeployResult is how a deploy command ended once its shell started.
type DeployResult struct {
	// ExitCode is the command's exit code.
	ExitCode int
	// TimedOut reports that the command was ended for running past its timeout.
	TimedOut bool
}

// LandDeps carries every outside effect NewLand needs, each behind a seam so this package imports none of them and every path and command is told.
type LandDeps struct {
	// ParentBranch returns the child's recorded parent branch; empty means the origin record carries none.
	ParentBranch func() (string, error)
	// PrimeBranch returns the branch the prime is on.
	PrimeBranch func() (string, error)
	// PrimeDirty returns the prime code worktree's uncommitted tracked paths.
	PrimeDirty func() ([]string, error)
	// Pull pulls the prime; a refusal, a diverged prime included, is an error carrying the reason.
	// A non-empty recordsStop with a nil error is a records side that did not pull while the code side did: the landing goes on and names it.
	Pull func(ctx context.Context) (recordsStop string, err error)
	// Landing reads the hub-wide landing config; it is read on every Call.
	Landing func() (LandingSettings, error)
	// RunDeploy runs command in the prime with its output to logPath, ending it after timeout.
	// An error means the shell could not start.
	RunDeploy func(ctx context.Context, command, logPath string, timeout time.Duration) (DeployResult, error)
	// PruneBoard prunes the done tasks off the board.
	PruneBoard func() error
	// LandLock is the hub-level lock one landing holds for its whole body, with the same non-blocking acquire shape as the prime lock.
	LandLock PrimeLock
	// PauseRequested reports a pending `lyx batten pause`; nil reads as never paused.
	PauseRequested func() (bool, error)
	// Sleep is the pause between two polls; a test replaces it to keep the waits out of real time.
	// A nil Sleep resolves to waitOrCancel.
	Sleep func(ctx context.Context, d time.Duration)
	// Now is the clock the dirty-prime bound is measured on; a nil Now resolves to time.Now.
	Now func() time.Time
	// Notices carries the seams the row's notices go through.
	Notices RowNoticeDeps
}

// landProducer lands one finished child run.
type landProducer struct {
	name       string
	slug       string
	deps       LandDeps
	primeLock  PrimeLock
	scratchDir string
	notifier   rowNotifier
}

var _ shedengine.ShedProducer = (*landProducer)(nil)

// NewLand returns a shedengine.ShedProducer that lands the task worktree identified by slug.
// Under the land lock it pulls the prime, runs the deploy command and prunes the board, then posts one notice only when the pull's records side stopped.
// A failed step is Stuck naming it, and a pause seen during a wait is Stuck naming the goto that clears it; neither posts a notice, since the driver's hand-back tells the orch of a stop the stop chain did not settle.
func NewLand(name, slug string, deps LandDeps, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer {
	if deps.Sleep == nil {
		deps.Sleep = waitOrCancel
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.PauseRequested == nil {
		deps.PauseRequested = func() (bool, error) { return false, nil }
	}
	return &landProducer{
		name:       name,
		slug:       slug,
		deps:       deps,
		primeLock:  primeLock,
		scratchDir: scratchDir,
		notifier:   newRowNotifier(name, slug, scratchDir, deps.Notices),
	}
}

// stop returns the Stuck verdict for a failed step, or the cancelled-during-run error when ctx ended first.
// It records the reason and posts no notice: the orch hears of the stop only if the stop chain gives it back.
func (p *landProducer) stop(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return p.stuck(ctx, reason, reportStuck)
}

// pause returns the Stuck verdict for a pause request honoured while waiting, or the cancelled-during-run error when ctx ended first.
// It records the reason as a wait, since the operator asked for the pause.
func (p *landProducer) pause(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return p.stuck(ctx, reason, reportWait)
}

// stuck records reason through record and returns the Stuck verdict.
func (p *landProducer) stuck(ctx context.Context, reason string, record func(producer, reason, scratchDir string, attrs ...any)) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	record(p.name, reason, p.scratchDir, "slug", p.slug)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}

// failure returns the Stuck verdict for a step that failed with err.
func (p *landProducer) failure(ctx context.Context, step string, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return p.stop(ctx, fmt.Sprintf("%s failed: %s", step, err.Error()))
}

// PauseHonouredPrefix leads the reason of a Land Stuck that honours a pause request seen while it waited.
const PauseHonouredPrefix = "pause honoured"

// pausedReason is the reason of a Stuck that honours a pause request seen while waiting for what.
// Shed's blocked persist leaves the pause flag set, so the way forward clears it with a goto before the run re-enters the row.
func (p *landProducer) pausedReason(what string) string {
	return fmt.Sprintf("%s: a pause was requested while waiting for %s; batten stays blocked at %s, so run \"lyx batten goto %s --to %s\" and then \"lyx batten run %s\"",
		PauseHonouredPrefix, what, p.name, p.slug, p.name, p.slug)
}

// Call implements shedengine.ShedProducer.
// It runs, under the land lock, the prime check, the pull, the deploy, the board prune and the records-stop notice, each idempotent so a resume re-runs the row from the top.
// Every failed step, a seam read error included, is Stuck naming the step; only a cancelled context is a hard error.
func (p *landProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	release, reason, paused, err := p.acquireLandLock(ctx)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if release == nil {
		if paused {
			return p.pause(ctx, reason)
		}
		return p.stop(ctx, reason)
	}
	defer func() {
		if rerr := release(); rerr != nil {
			logger.Warn("battenshed: release land lock failed", "producer", p.name, "slug", p.slug, "path", p.deps.LandLock.Path, "error", rerr)
		}
	}()

	parent, err := p.deps.ParentBranch()
	if err != nil {
		return p.failure(ctx, "prime check", err)
	}
	if parent == "" {
		return p.stop(ctx, "prime check failed: the child's origin record carries no parent branch, so it cannot be compared with the prime's branch")
	}
	primeBranch, err := p.deps.PrimeBranch()
	if err != nil {
		return p.failure(ctx, "prime check", err)
	}

	var recordsStop string
	if parent != primeBranch {
		logger.Info("battenshed: parent branch is not the prime's branch; skipping pull and deploy", "producer", p.name, "slug", p.slug, "parent", parent, "prime", primeBranch)
	} else {
		reason, paused, err := p.waitCleanPrime(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if paused {
			return p.pause(ctx, reason)
		}
		if reason != "" {
			return p.stop(ctx, reason)
		}
		reason, recordsStop, err = p.pullAndDeploy(ctx, parent)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if reason != "" {
			return p.stop(ctx, reason)
		}
	}

	if err := p.deps.PruneBoard(); err != nil {
		return p.failure(ctx, "board prune", err)
	}

	p.notifier.recordsStopped(ctx, p.slug, recordsStop)
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// pullAndDeploy pulls the prime under the prime lock and then, outside it, runs the deploy command when the parent branch is one the landing config deploys.
// A non-empty reason is the Stuck reason of the failed step.
// A non-empty recordsStop is the records side's stop message from a pull that otherwise succeeded; the deploy runs regardless.
// A non-nil err is the cancelled-during-run diagnosis.
func (p *landProducer) pullAndDeploy(ctx context.Context, parent string) (reason, recordsStop string, err error) {
	release, lockReason, err := acquirePrimeLock(ctx, p.name, p.slug, p.primeLock)
	if err != nil {
		if ctx.Err() != nil {
			return "", "", err
		}
		return "prime lock failed: " + err.Error(), "", nil
	}
	if release == nil {
		return lockReason, "", nil
	}
	recordsStop, pullErr := p.deps.Pull(ctx)
	if rerr := release(); rerr != nil {
		logger.Warn("battenshed: release prime lock failed", "producer", p.name, "slug", p.slug, "path", p.primeLock.Path, "error", rerr)
	}
	if pullErr != nil {
		return "pull failed: " + pullErr.Error(), "", nil
	}

	settings, err := p.deps.Landing()
	if err != nil {
		return "deploy failed: read the landing config: " + err.Error(), recordsStop, nil
	}
	if settings.Deploy == "" {
		return "", recordsStop, nil
	}
	if !slices.Contains(settings.DeployBranches, parent) {
		logger.Info("battenshed: parent branch is not in deploy_branches; skipping deploy", "producer", p.name, "slug", p.slug, "parent", parent)
		return "", recordsStop, nil
	}

	logPath := filepath.Join(p.scratchDir, p.name+deployLogFileSuffix)
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		return "deploy failed: " + err.Error(), recordsStop, nil
	}
	wayForward := fmt.Sprintf("log: %s; way forward: fix the cause, then run \"lyx batten run %s\" from the prime", logPath, p.slug)
	result, err := p.deps.RunDeploy(ctx, settings.Deploy, logPath, deployTimeout)
	switch {
	case err != nil:
		return fmt.Sprintf("deploy failed: the shell could not start (%s); %s", err.Error(), wayForward), recordsStop, nil
	case result.TimedOut:
		return fmt.Sprintf("deploy failed: timed out after %s; %s", deployTimeout, wayForward), recordsStop, nil
	case result.ExitCode != 0:
		return fmt.Sprintf("deploy failed: exit code %d; %s", result.ExitCode, wayForward), recordsStop, nil
	}
	return "", recordsStop, nil
}

// acquireLandLock takes the land lock, polling a contended one from landLockPollBase doubling to landLockPollCeiling with no time limit.
// A non-nil release means the lock is held and the caller must call it exactly once.
// A non-empty stuckReason is a pause honoured between two polls, or a failure to read the pause flag or to acquire; paused tells the pause from the failures.
// A non-nil err is the cancelled-during-run diagnosis.
func (p *landProducer) acquireLandLock(ctx context.Context) (release func() error, stuckReason string, paused bool, err error) {
	lock := p.deps.LandLock
	poll := landLockPollBase
	for attempt := 0; ; attempt++ {
		rel, ok, acquireErr := lock.Acquire()
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return nil, "", false, cerr
		}
		if acquireErr != nil {
			return nil, fmt.Sprintf("land lock failed: acquire %q: %s", lock.Path, acquireErr.Error()), false, nil
		}
		if ok {
			return rel, "", false, nil
		}
		if attempt == 0 {
			logger.Info("battenshed: land lock contended, waiting", "producer", p.name, "path", lock.Path, "slug", p.slug)
		}
		pauseRequested, pauseErr := p.deps.PauseRequested()
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return nil, "", false, cerr
		}
		if pauseErr != nil {
			return nil, fmt.Sprintf("land lock failed: read the pause flag: %s", pauseErr.Error()), false, nil
		}
		if pauseRequested {
			return nil, p.pausedReason("the land lock"), true, nil
		}
		p.deps.Sleep(ctx, poll)
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return nil, "", false, cerr
		}
		poll = min(poll*2, landLockPollCeiling)
	}
}

// waitCleanPrime waits, for at most primeDirtyWaitBound, until the prime has no uncommitted tracked paths.
// A non-empty stuckReason is a pause honoured, a failed read, or the bound spent naming the paths; paused tells the pause from the failures.
// A non-nil err is the cancelled-during-run diagnosis.
func (p *landProducer) waitCleanPrime(ctx context.Context) (stuckReason string, paused bool, err error) {
	deadline := p.deps.Now().Add(primeDirtyWaitBound)
	for {
		dirty, readErr := p.deps.PrimeDirty()
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", false, cerr
		}
		if readErr != nil {
			return fmt.Sprintf("prime check failed: read the prime's uncommitted paths: %s", readErr.Error()), false, nil
		}
		if len(dirty) == 0 {
			return "", false, nil
		}
		if !p.deps.Now().Before(deadline) {
			return fmt.Sprintf("prime check failed: the prime still has uncommitted tracked paths after %s: %s; way forward: commit or discard them, then run \"lyx batten run %s\" from the prime", primeDirtyWaitBound, strings.Join(dirty, ", "), p.slug), false, nil
		}
		pauseRequested, pauseErr := p.deps.PauseRequested()
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", false, cerr
		}
		if pauseErr != nil {
			return fmt.Sprintf("prime check failed: read the pause flag: %s", pauseErr.Error()), false, nil
		}
		if pauseRequested {
			return p.pausedReason("a clean prime"), true, nil
		}
		p.deps.Sleep(ctx, primeDirtyPollInterval)
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", false, cerr
		}
	}
}

// recordsStopped posts the one notice of a landing that succeeded, once per batten run: the landed slug and the stop of the prime's records side.
// It posts nothing when recordsStop is empty, since the landing then needs nothing from the orch.
// The marker line is written only once the notice was posted, and a notice that could not be posted is held back or dropped with a Warn, never retried.
func (n rowNotifier) recordsStopped(ctx context.Context, landed, recordsStop string) {
	if recordsStop == "" || n.deps.Notify == nil || ctx.Err() != nil {
		return
	}
	if slices.Contains(n.readMarker(), recordsStopMarkerLine) {
		return
	}
	line := fmt.Sprintf("%s %s: landed; the prime's records did not pull: %s", noticePrefix, landed, recordsStop)
	if n.send(ctx, line) {
		n.setMarkerLine(recordsStopMarkerLine, recordsStopMarkerLine)
	}
}
