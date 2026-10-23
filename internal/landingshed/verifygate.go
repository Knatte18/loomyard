// verifygate.go declares the clean-tree and post-merge landing and publish gate checks both landing producers run around their parent merge-in:
// a thin adapter over internal/verifytree, which owns the clean-tree check, the verified-tree record and the run.
// A dirty tree or a failing verify is reported as a Stuck reason, so nothing leaves the worktree on a tree that is not committed and verified.

package landingshed

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// dirtyPathsShown caps how many dirty paths a Stuck reason names.
const dirtyPathsShown = 10

// verifyGate holds the told landing, publish and base command closures, the told gate tier lookup, the told failed-verify way-forward clause, the told wait-mark callback, the verify paths and the two verifytree seams.
// The dirty, verify and now fields are in-package seams tests replace with fakes,
// so unit tests never spawn git or a shell and the mark's start time is fixed.
// A zero dirty seam skips the clean-tree check, a nil command skips the landing verify, a nil publishCommand skips the publish verify, a nil baseCommand names no base command and a nil waitMark marks nothing.
type verifyGate struct {
	// command renders the `landing` gate's call, publishCommand the `publish` gate's call, and baseCommand the call whose record entry every pass keeps.
	command        func() (string, error)
	publishCommand func() (string, error)
	baseCommand    func() (string, error)
	// tiers lists the tier numbers of a named gate; recordFailure reads it for a log that names no failing tier.
	tiers            func(gate string) ([]int, error)
	failedWayForward string
	failureTarget    string
	// classify, when set, chooses the failure record's target and the reason's way forward per failure, in place of failedWayForward and failureTarget.
	classify func(ctx context.Context, failure verifytree.PublishFailure) FailureRoute
	waitMark func(label string, start time.Time) error
	// hold is the told pause hold both verifies run under; nil holds nothing.
	hold   *verifytree.PauseHold
	paths  verifytree.Paths
	dirty  func(worktree string) ([]string, error)
	verify func(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error)
	now    func() time.Time
	// recorder is set only by Publish: nil leaves a failed verify unrecorded.
	recorder *failureRecorder
}

// FailureRoute is a classifier's answer for one failed verify: where the failure record sends the run and what the Stuck reason says about it.
type FailureRoute struct {
	// Target is the row written into the failure record's target.
	Target string
	// WayForward is the whole clause the Stuck reason ends with after `way forward: `.
	WayForward string
	// Lead is text the reason puts between `<gate> failed` and ` after merging parent branch`; empty for a failure the task owns.
	Lead string
}

// failureRecorder holds what the gate needs to write the Publish failure record.
// mergeCommit is set by Publish after its merge-in and read when a verify fails.
type failureRecorder struct {
	failingTests func(log string) []verifytree.FailedTest
	// traceFailure traces the log to the plan; nil traces nothing.
	traceFailure func(log, head string) (verifytree.FailureTrace, error)
	head         func() (string, error)
	mergeCommit  string
}

// recordFailure writes the Publish failure record for a verify of the given kind, run by the producer row row, that ended in result.
// The kind is the name of the gate that failed, and the record's tier is the last failing tier the log's summary lines name;
// a log without one, such as a refusal before the first tier, takes the highest tier of the gate's list.
// A result that is not a failed run, a timeout and a shell that could not start write none, since none of them names a failing test.
// The record's target and the returned route come from the told classifier, called with the record as written except its target, and from the told target and way-forward clause when there is none.
// It returns the route and whether it wrote the record: a write failure is logged, reported as false and changes nothing else, and a result that writes no record returns the zero route.
func (g verifyGate) recordFailure(ctx context.Context, kind verifytree.FailureKind, row string, result verifytree.Result) (FailureRoute, bool) {
	if g.recorder == nil || result.Status != verifytree.StatusFailed || result.TimedOut || result.ExitCode < 0 {
		return FailureRoute{}, false
	}
	failure := verifytree.PublishFailure{
		Kind:        kind,
		Row:         row,
		LogPath:     result.Log,
		MergeCommit: g.recorder.mergeCommit,
		WrittenAt:   g.now().UTC().Format(time.RFC3339),
	}
	if g.recorder.head != nil {
		head, err := g.recorder.head()
		if err != nil {
			logger.Warn("landingshed: could not read HEAD for the publish failure record", "cause", err)
		}
		failure.Head = head
	}
	var log string
	if content, err := os.ReadFile(result.Log); err == nil {
		log = string(content)
		if g.recorder.failingTests != nil {
			failure.Tests = g.recorder.failingTests(log)
		}
	}
	if g.recorder.traceFailure != nil && log != "" && failure.Head != "" {
		trace, err := g.recorder.traceFailure(log, failure.Head)
		if err != nil {
			logger.Warn("landingshed: could not trace the failed verify to the plan", "kind", kind, "cause", err)
		} else {
			failure.Cards = trace.Cards
			failure.LaterRange = trace.LaterRange
			failure.Untraced = trace.Untraced
		}
	}
	if tier, found := gateslot.FailedTier(log); found {
		failure.Tier = tier
	} else {
		list, err := g.tiers(string(kind))
		if err != nil || len(list) == 0 {
			logger.Warn("landingshed: could not read the gate's tiers for the publish failure record", "gate", kind, "tiers", len(list), "cause", err)
			return FailureRoute{}, false
		}
		failure.Tier = slices.Max(list)
	}
	route := FailureRoute{Target: g.failureTarget, WayForward: g.failedWayForward}
	if g.classify != nil {
		route = g.classify(ctx, failure)
	}
	failure.Target = route.Target
	if err := verifytree.WritePublishFailure(g.paths, failure); err != nil {
		logger.Warn("landingshed: could not write the publish failure record", "kind", kind, "cause", err)
		return FailureRoute{}, false
	}
	return route, true
}

// clearFailure removes the Publish failure record of an earlier failed verify.
// It does nothing for a gate with no recorder, and a removal failure is logged.
func (g verifyGate) clearFailure() {
	if g.recorder == nil {
		return
	}
	if err := verifytree.RemovePublishFailure(g.paths); err != nil {
		logger.Warn("landingshed: could not remove the publish failure record", "cause", err)
	}
}

// newVerifyGate copies the gate's told values from deps and wires the real verifytree functions.
func newVerifyGate(deps Deps) verifyGate {
	return verifyGate{
		command:          deps.VerifyCommand,
		publishCommand:   deps.PublishCommand,
		baseCommand:      deps.BaseCommand,
		tiers:            deps.GateTiers,
		failedWayForward: deps.VerifyFailedWayForward,
		failureTarget:    deps.VerifyFailureTarget,
		classify:         deps.ClassifyVerifyFailure,
		waitMark:         deps.VerifyWaitMark,
		hold:             deps.VerifyHold,
		paths:            verifytree.NewPaths(deps.WorktreeRoot, deps.VerifyDir),
		dirty:            verifytree.DirtyPaths,
		now:              time.Now,
		verify: func(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error) {
			return verifytree.Verify(ctx, p, site, command)
		},
	}
}

// markWait sets the driver strand's pane mark to label, started now, and clears it when label is empty.
// A failure is logged and changes nothing else: the mark is display only.
func (g verifyGate) markWait(label string) {
	if g.waitMark == nil {
		return
	}
	var start time.Time
	if label != "" {
		start = g.now()
	}
	if err := g.waitMark(label, start); err != nil {
		logger.Warn("landingshed: could not set the verify wait mark", "label", label, "cause", err)
	}
}

// clean checks that the task worktree has no uncommitted changes at point, a phrase such as "before the merge-in".
//
// It returns a non-empty Stuck reason naming the dirty paths, a non-nil error when git cannot report the status,
// and ("", nil) when the tree is clean.
func (g verifyGate) clean(producer, point string) (string, error) {
	if g.dirty == nil {
		return "", nil
	}
	paths, err := g.dirty(g.paths.Worktree)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: check the worktree is clean %s: %w", producer, point, err)
	}
	if len(paths) == 0 {
		return "", nil
	}
	return dirtyReason(point, paths), nil
}

// dirtyReason is the Stuck reason for a dirty tree found at point.
func dirtyReason(point string, paths []string) string {
	shown := paths
	if len(shown) > dirtyPathsShown {
		shown = shown[:dirtyPathsShown]
	}
	list := strings.Join(shown, ", ")
	if more := len(paths) - len(shown); more > 0 {
		list = fmt.Sprintf("%s and %d more", list, more)
	}
	return fmt.Sprintf("the task worktree has uncommitted changes %s: %s; commit or remove them on the task branch, then resume", point, list)
}

// readBase returns the base command, the one whose record entry every pass of this gate keeps.
// A gate told no base command names none.
func (g verifyGate) readBase(producer string) (string, error) {
	if g.baseCommand == nil {
		return "", nil
	}
	base, err := g.baseCommand()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read base command: %w", producer, err)
	}
	return base, nil
}

// check runs the `landing` gate's call for producer after a merge of parentBranch.
// verifytree skips the run when its record already names the tree and the command,
// so the verify runs exactly when the tree differs from the last verified one:
// after a resolved conflict, after new parent commits and after a crash between a merge and its verify.
// The base command is the site's BaseCommand, so a pass keeps the entry the next round diffs from.
//
// It returns a non-empty Stuck reason naming the `landing` gate when the call fails or the tree is dirty,
// a non-nil error for an infrastructure fault or a cancellation, and ("", nil) when the producer may proceed.
func (g verifyGate) check(ctx context.Context, producer, parentBranch string) (string, error) {
	if g.command == nil {
		return "", nil
	}

	command, err := g.command()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read landing command: %w", producer, err)
	}
	if command == "" {
		logger.Warn("landingshed: post-merge verify skipped because no verify command is configured", "producer", producer, "parentBranch", parentBranch)
		return "", nil
	}
	base, err := g.readBase(producer)
	if err != nil {
		return "", err
	}

	g.markWait("")
	g.markWait("verify " + producer)
	result, err := g.verify(ctx, g.paths, verifytree.Site{Label: producer, BaseCommand: base, Hold: g.hold}, command)
	g.markWait("")
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: %w", producer, err)
	}
	route, recorded := g.recordFailure(ctx, verifytree.FailureKindLanding, producer, result)
	return g.verdict(result, "landing gate", producer, parentBranch, route, recorded)
}

// checkPublishVerify runs the `publish` gate's call for producer after the landing call passed.
// An empty call, which an empty `publish` list renders, runs nothing and logs nothing.
// The base command is the site's BaseCommand, so the pass keeps the landing entry of the record.
//
// It returns a non-empty Stuck reason naming the `publish` gate when the call fails or the tree is dirty.
// It returns a non-nil error for an infrastructure fault or a cancellation, and ("", nil) when the producer may proceed.
func (g verifyGate) checkPublishVerify(ctx context.Context, producer, parentBranch string) (string, error) {
	if g.publishCommand == nil {
		return "", nil
	}
	publishCommand, err := g.publishCommand()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read publish command: %w", producer, err)
	}
	if publishCommand == "" {
		return "", nil
	}
	base, err := g.readBase(producer)
	if err != nil {
		return "", err
	}

	result, err := g.verify(ctx, g.paths, verifytree.Site{Label: producer, BaseCommand: base, Hold: g.hold}, publishCommand)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: %w", producer, err)
	}
	route, recorded := g.recordFailure(ctx, verifytree.FailureKindPublish, producer, result)
	return g.verdict(result, "publish gate", producer, parentBranch, route, recorded)
}

// watchdogClause returns the clause a failed verify's reason carries when the log at logPath holds the CPU watchdog's line, and empty for an unreadable log or one without it.
// The clause names the window, says the goroutine stacks are in the log, and gives the way forward for a package that is CPU-bound but healthy.
func watchdogClause(logPath string) string {
	log, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	window, quit := gateslot.FindWatchdogQuit(string(log))
	if !quit {
		return ""
	}
	return fmt.Sprintf("; the gate test's CPU watchdog quit a test binary that kept a CPU core busy for %s, with its goroutine stacks in the log; if the package is CPU-bound but healthy, %s, then resume", window, gateslot.BusyQuitWayForward)
}

// verdict maps a verify result to the producer's decision, naming the gate whose call ran as what.
// It returns a non-empty Stuck reason for a dirty tree or a failed command, an error for an unknown status, and ("", nil) when the producer may proceed.
// recorded says the failure record was written; a failed command with an exit code then ends with route's way-forward clause, and otherwise, or with an empty clause, asks to fix forward on the task branch.
// The route's lead follows the gate's name in that reason.
// After a CPU watchdog quit the reason names the window and the busy_quit_sec way forward before that clause.
func (g verifyGate) verdict(result verifytree.Result, what, producer, parentBranch string, route FailureRoute, recorded bool) (string, error) {
	switch result.Status {
	case verifytree.StatusPassed:
		logger.Info("landingshed: post-merge "+what+" passed", "producer", producer, "parentBranch", parentBranch)
	case verifytree.StatusSkipped:
		logger.Info("landingshed: post-merge "+what+" skipped because the tree is already verified", "producer", producer, "parentBranch", parentBranch, "tree", result.Tree)
	case verifytree.StatusDirty:
		return dirtyReason("when the "+what+" was about to run", result.Dirty), nil
	case verifytree.StatusFailed:
		if result.TimedOut {
			return fmt.Sprintf("%s timed out after merging parent branch %q: %s; output: %s; fix the hanging test on the task branch, or, for a tier that is slow but healthy, rewrite the tier whole from the prime with `lyx config gate --set 'tiers.tier<N>={...}'`, carrying every key of the tier and the raised timeout_min, since a dotted key under the tier writes an entry the loader refuses; a session that is not the prime reports status: FAILED and the orch runs it; then resume", what, parentBranch, result.Detail, result.Log), nil
		}
		if result.ExitCode < 0 {
			return fmt.Sprintf("%s could not start after merging parent branch %q: %s; output: %s", what, parentBranch, result.Detail, result.Log), nil
		}
		watchdog := watchdogClause(result.Log)
		if recorded && route.WayForward != "" {
			return fmt.Sprintf("%s failed%s after merging parent branch %q (exit code %d); output: %s%s; way forward: %s", what, route.Lead, parentBranch, result.ExitCode, result.Log, watchdog, route.WayForward), nil
		}
		return fmt.Sprintf("%s failed after merging parent branch %q (exit code %d); output: %s%s; fix forward on the task branch, then resume", what, parentBranch, result.ExitCode, result.Log, watchdog), nil
	default:
		return "", fmt.Errorf("landingshed: %s: unknown %s status %q", producer, what, result.Status)
	}
	return "", nil
}
