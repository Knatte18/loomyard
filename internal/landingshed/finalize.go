// finalize.go implements the Finalize producer.
// With no parent lock held it merges the parent's tip into the task worktree and gates the merged tree.
// Under the parent lock it checks the parent has not moved since.
// It then merges the task branch into the parent pair's own worktree and records the landing for the parent's pusher.
// It reports only what the shedengine.ShedProducer seam can carry:
// a verdict, an output pointer whose Reason the engine persists, and an error.
//
// This producer's merge critical section is the parent-side merge.
//
// A merged pull request does not make this producer redundant:
// The remote service sees only one of the two sides, so the other side's branch was never in the pull request at all.
// After a remote-side merge, the visible side reports already-up-to-date while the other side merges.
//
// A landing the pull-request resolver says goes through a pull request merges the run's open pull request on GitHub.
// That merge runs under the parent lock and before the parent-side merge; finalize_pr.go holds it.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// finalizeName is the producer name Finalize's log lines and error text carry.
const finalizeName = "Finalize"

// parentMerger is the narrow seam Finalize holds the opened parent pair's handle behind.
// The seam lets this package's tests substitute a fake, and it adds no second public way to obtain the handle:
// production opens the pair only through `deps.OpenParentFabric`.
type parentMerger interface {
	// Merge merges source into the parent pair's own worktree.
	Merge(source string, opts fabriclayout.MergeOptions) (fabriclayout.MergeResult, error)
	// HeadSHA reads the parent pair's own current head commit.
	HeadSHA() (string, error)
	// MidMerge reads the merge the parent pair has parked, if any.
	MidMerge() (fabriclayout.MidMergeState, error)
	// FastForwardPair moves the parent pair to its upstream as a fast-forward only.
	FastForwardPair(opts fabricengine.SyncOptions) (fabriclayout.FastForwardResult, error)
	// CodeDirtyPaths reads the parent pair's code checkout and its tracked uncommitted paths.
	CodeDirtyPaths() (checkout string, paths []string, err error)
}

var _ parentMerger = (*fabricengine.Fabric)(nil)

// Finalize is the ShedProducer that merges the parent branch into the task worktree and gates it.
// It then lands the task branch on the parent pair's own worktree under the parent lock.
type Finalize struct {
	deps     Deps
	resolver resolver
	// parentOpener adapts deps.OpenParentFabric's concrete *fabricengine.Fabric return type onto the parentMerger seam.
	// It is called once per Call, never at construction:
	// The pair constructor it wraps stat-checks a layout that may not be wired yet.
	// Opening eagerly would therefore fail before the run's own preflight has confirmed anything is wired.
	parentOpener func() (parentMerger, error)
	// gate is the post-merge verify gate the merged tree passes through.
	// A zero value carries no command, so struct-literal tests run without a gate.
	gate verifyGate
	// premerge clears the merge-in a previous attempt left parked.
	// A zero value runs no probe, so struct-literal tests run without it.
	premerge preMerge
	// guard holds the parent lock and tests whether the parent moved.
	// A zero value takes no lock and reads no tips, so struct-literal tests run without it.
	guard parentGuard
	// lockFreed reports that the current locked pass freed the parent lock for an agent session.
	// Call then runs the pass again.
	lockFreed bool
	// lockHeld reports that this process holds the parent lock in a lock store, so freeLock has a lock to free.
	lockHeld bool
	// gatedTip is the parent tip the current pass merged and gated with no parent lock held.
	// It is empty when the pass ran no lock-free merge and gate, and the locked pass then merges and gates under the lock.
	gatedTip string
	// parentMoved reports that the current locked pass found the parent's head past gatedTip and freed the lock.
	// Call then counts a lost race and runs the passes again.
	parentMoved bool
	// parentHeadMerged is the parent pair's head read right before the parent branch was last merged into the task worktree.
	// It is the base of the landed diff.
	// It is empty when this process read none, as in a Finalize resumed past the merge.
	parentHeadMerged string
	// flaky holds each gate failure of this process that passed when run once more on the same head.
	flaky []flakyPass
	// configBeforeMerge is the read of the task's config changes made before the current call's first merge of the parent.
	// It is nil until that read.
	configBeforeMerge *configRead
}

// configRead is what one read of the task's per-worktree config changes answered.
type configRead struct {
	changes fabriclayout.ConfigChanges
	err     error
}

// flakyHeading opens the landing texts' section listing the gate failures a rerun on the same head did not repeat.
const flakyHeading = "## Flaky test failures"

var _ shedengine.ShedProducer = (*Finalize)(nil)

// NewFinalize constructs a Finalize from deps.
// It rejects a nil OpenFabric, OpenParentFabric, Lock.Store, Lock.Owner or ReadParentTips in deps:
// each gets a distinct error, so a mis-wired producer fails at construction rather than nil-panicking at call time.
// A nil parent opener is a construction error, not a silent no-op:
// This producer never creates a worktree to merge into.
// Without a working opener it could never reach the parent pair at all.
//
// It builds its resolver at construction time from the told values and stores it behind the resolver seam.
// The resolver's constructor performs no I/O,
// so a mis-wired resolver is a construction error rather than a first-call surprise.
func NewFinalize(deps Deps) (*Finalize, error) {
	if deps.OpenFabric == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.OpenFabric must not be nil")
	}
	if deps.OpenParentFabric == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.OpenParentFabric must not be nil")
	}
	if deps.DescriptionPath == "" {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.DescriptionPath must not be empty")
	}
	if deps.Lock.Store == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.Lock.Store must not be nil")
	}
	if deps.Lock.Owner == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.Lock.Owner must not be nil")
	}
	if deps.ReadParentTips == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.ReadParentTips must not be nil")
	}

	res, err := newResolver(deps, "NewFinalize")
	if err != nil {
		return nil, err
	}
	fz := assembleFinalize(deps, res, newLandingGate(deps))
	fz.premerge = newPreMerge(deps)
	fz.guard = newParentGuard(deps)
	fz.parentOpener = func() (parentMerger, error) {
		h, err := deps.OpenParentFabric()
		if err != nil {
			return nil, err
		}
		return h, nil
	}
	return fz, nil
}

// assembleFinalize builds a Finalize over deps, resolver and gate, and wires the gate to report its flaky passes to it,
// so the landing texts list their failures.
// The wiring enables the gate's rerun of a test failure and the landing texts' flaky section.
func assembleFinalize(deps Deps, resolver resolver, gate verifyGate) *Finalize {
	fz := &Finalize{deps: deps, resolver: resolver, gate: gate}
	fz.gate.onFlakyPass = func(pass flakyPass) { fz.flaky = append(fz.flaky, pass) }
	return fz
}

// landingTexts composes from summary and credit the GitHub merge's title and body and the landing commit's message.
// The body and the message carry the flaky section after summary's body.
// The section lists each gate failure of this process that passed when run once more on the same head.
// The message ends in the trailer that names the run, so the landing is found on the parent branch.
func (fz *Finalize) landingTexts(summary summaryparser.Summary, credit string) (title, body, message string) {
	summary.Body = withSection(summary.Body, fz.flakySection())
	message = fmt.Sprintf("%s\n\n%s: %s", strings.TrimRight(summary.LandingMessage(credit), "\n"), fabriclayout.LandingTrailerKey, fz.deps.Slug)
	return summary.Title, bodyWithCredit(summary.Body, credit), message
}

// flakySection renders the section that lists, each once, the gate failures a rerun on the same head did not repeat.
// It is empty when there were none.
func (fz *Finalize) flakySection() string {
	var failures []string
	for _, pass := range fz.flaky {
		for _, failure := range pass.failures {
			if !slices.Contains(failures, failure) {
				failures = append(failures, failure)
			}
		}
	}
	if len(failures) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(flakyHeading + "\n\nThe landing gate failed on these and passed when run once more on the same head, so no fixer ran for them:\n")
	for _, failure := range failures {
		fmt.Fprintf(&b, "\n- `%s`", failure)
	}
	return b.String()
}

// Call runs one Finalize iteration.
//
// Before it takes the parent lock, Finalize merges the parent's tip into the task worktree and gates the merged tree.
// That pass holds no lock, and neither does a fixer or fix review the gate calls for.
// Under the lock, a parent whose head is still the tip that pass merged finds the merge up to date and the tree verified,
// so the lock is held for the checks and the landing alone.
// A parent that moved since frees the lock, and Finalize merges the new tip and gates it again with no lock held.
// After `landing.finalize_lost_races` such lost races in one Call, Finalize merges and gates under the lock,
// so a run on a busy parent is never starved.
// A parent already holding the task's work runs no lock-free pass either.
// Either way, what lands is a tree that passed the gate on top of the parent it lands on.
//
// Finalize takes the parent lock before anything that moves the parent, and releases it on every way out.
// A paused lock wait or an unreadable lock record is a halting Stuck.
// A landing that reaches Done, a Stuck and an error all free the lock.
// Finalize runs the owner checks before its lock-free merge, and again under that lock.
// A run whose origin has ended or stopped is Stuck.
// A run or parent that another machine owns and a failed owner read halt.
// An unreachable origin is a transient error, so the driver waits it out and the landing goes on once origin answers.
// A parent already holding the task's work skips the owner checks, and an unreachable origin then proceeds.
// Finalize then opens the parent pair and syncs it.
// A pair that is missing, dirty or mid-merge is Stuck with a way forward before any merge.
// A parent that cannot be fast-forwarded to its upstream is Stuck the same way.
// Finalize then merges the parent branch into the task worktree through its resolver.
// The resolver resolves a conflict with the conflict session.
// A directory the merge-in deleted can keep files that only the deleted ignore rules ignored;
// Finalize removes those before it checks the tree clean.
// Finalize gates the merged tree whether the merge conflicted or was large.
// The `landing` gate judges the merged tree, and a failing gate runs the merge fixer in finalize mode.
// A fixer change over the caps is settled in place by the fix review and the fixer.
// Finalize never sends the run back to an earlier row.
// The lock is freed before the fixer or a fix review starts.
// Once their change is let through, Finalize runs its passes again, the lock-free pass first while lost races remain.
// It then takes the lock again at the front of the queue,
// and it runs every step under the lock again, from the pull-request step on.
// A Finalize resumed with such a change already let through starts at the front too.
// After a merge-in-required error from the parent-side merge, Finalize re-runs the merge and its gate once.
// It does so under the same lock, so a code-side parent commit that arrived in between is merged, gated and landed.
//
// Immediately before each landing attempt, Finalize re-reads the parent lock record,
// so a lock that something released lands nothing and is a halting Stuck.
// The re-read and the landing are not atomic: a release between them is caught only by the parent push's rejection.
// Every local landing leaves one commit on the parent that carries the run's landing trailer.
// The landing commit carries the change description.
// A gate failure that passed when run once more on the same head is then listed as flaky.
// Both the landing commit and the GitHub merge's body list it.
// After a blank line it carries the credit block, when the config yields one.
//
// A landing that needs GitHub never merges there while the parent branch still owes the push of an earlier landing.
// An owed push is a transient error, so the driver waits it out, and a refused one halts naming its way forward.
// A landing whose need shows without asking GitHub is checked before the lock,
// and every landing that merges on GitHub is checked again under it.
// A local landing is never checked: it lands on top of the owed one.
//
// Right after the lock, Finalize follows the pull-request resolver, which asks GitHub's rules for the parent branch.
// A no closes a recorded open pull request, drops the record and lands locally.
// A rules read that fails for a reason other than a transient or token one halts when a pull request is recorded.
// Such a failed read closes nothing.
// A yes needs the recorded pull request and halts without one.
// A yes halts on a recorded pull request closed without the operator's ending, unless the landing takes it at once.
// That landing reopens a closed pull request only after the owner checks, just before the merge.
// A closed pull request whose record carries the operator's ending is reopened and merged on GitHub like an open one,
// and a merged one lands through the already-landed path.
// An open pull request is merged on GitHub just before the parent-side merge.
// The merge takes the pushed task head as its expected head.
// Then the parent pair is fast-forwarded to that merge,
// so the parent-side merge finds the parent holding the task's work.
// A landing whose beam.pull_request is trail needs no record:
// Finalize opens the pull request itself at that point and squash-merges it with no review.
// A no marked TrueMerge never squashes the parent-side merge,
// so the parent's history keeps the task branch's merge commits.
//
// Finalize pushes nothing itself.
// It records the landing on the parent branch, whose pusher pushes it and marks the board task done, and returns Done.
// With pushes off it commits the run's records, marks the board task done itself and records no landing.
//
// Finalize is idempotent over an already-landed parent.
// Such a parent already contains the task's changes:
// an operator squash-merged the task on the remote, or an earlier Finalize landed and failed later.
// The empty squash is classified as already-up-to-date, so `Fabric.Merge` commits nothing and returns no error.
// Finalize then proceeds as for a fresh landing:
// the landing is recorded again with the parent's head, and the verdict is Done.
func (fz *Finalize) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, finalizeName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// Step 1a: parse the final-summary artifact before any commit, any catch-up merge-in, or any parent-side mutation,
	// so a missing or malformed artifact returns an error and the run never half-lands.
	// The error is never Stuck, and the run never falls back silently to an unset `MergeOptions.Message`;
	// the composed message below is the conclude-commit message.
	// The engines the task ran under and the regression check are read with it for the same reason:
	// the credit block and the check, which follows the body, are part of both landing messages.
	summary, err := summaryparser.Parse(fz.deps.DescriptionPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: parse change description: %w", finalizeName, err)
	}
	credit, err := fz.deps.completeSummary(summary)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", finalizeName, err)
	}

	// Step 1a2: reap the sandbox a hardener run left, before the lock and the merge, so the worktree's teardown meets none.
	if fz.deps.Reap != nil {
		if err := fz.deps.Reap(); err != nil {
			return halt(ctx, finalizeName, fmt.Sprintf("the sandbox reap failed: %v; way forward: run `lym harden sandbox clear` in the task worktree, then `lym loom resume`", err)).result()
		}
	}

	// Step 1b: commit the product's own status file,
	// so the pair carries no tracked modification when the merge guard below runs.
	// This must happen inside Call rather than once at bootstrap: Shed rewrites that file on every transition.
	// That includes the persist a resumed run makes immediately before calling this producer,
	// so any earlier commit is already stale by now.
	//
	// A commit failure maps to a returned error, never to Stuck:
	// a git fault is infrastructure rather than a merge precondition a human resolves by editing the branch,
	// and it is the same disposition loomshed's own commit decorators already give a failing commit seam.
	if fz.deps.CommitStatus != nil {
		if err := fz.deps.CommitStatus(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: commit status file: %w", finalizeName, err)
		}
	}

	fz.configBeforeMerge = nil

	// Step 1b1 runs with no lock held, and steps 1b2 to 7 under the parent lock.
	// Both run again each time a fixer or fix review ran with the lock freed, and each time the parent moved past step 1b1.
	// Each later pass queues at the front, and so does a run resumed after its fixer.
	front, err := fz.returningFromFixer()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	lostRaces := 0
	for ; ; front = true {
		fz.gatedTip = ""
		if lostRaces < fz.deps.Config.FinalizeLostRaces {
			if end := fz.gateUnlocked(ctx, front); end != nil {
				return end.result()
			}
		}
		if end := fz.landUnderLock(ctx, front, *summary, credit); end != nil {
			return end.result()
		}
		if fz.parentMoved {
			lostRaces++
			if lostRaces == fz.deps.Config.FinalizeLostRaces {
				logger.Info("landingshed: the parent moved past every lock-free merge and gate, so the run merges and gates under the parent lock", "producer", finalizeName, "branch", fz.deps.ParentBranch, "lostRaces", lostRaces)
			}
		}
	}
}

// gateUnlocked is step 1b1: with no lock held, it merges the parent's tip into the task worktree and gates the result.
// It runs the owner checks first, so a run this machine may not land ends before anything is merged into its worktree.
// It keeps the tip it merged as gatedTip, which the locked pass compares with the parent it lands on.
// A parent ahead of its upstream by owed landing tips alone is merged at its local tip.
// That tip is the head the locked pass lands on, so its pass wins the race unless another landing moves the parent.
// A landing known to need GitHub cannot land until origin holds the owed tips.
// Such a landing is the exception and merges the upstream tip.
// A failed read of whether it needs GitHub merges the upstream tip too.
// A need that only GitHub's repository rules set is not known there, since reading them is a GitHub call.
// A parent already holding the task's work runs no such pass and keeps no tip.
// A failed read of whether it holds the work halts as an owner check.
// When front is set, a lock taken to read the local parent tip is waited for at the front of the queue.
// It returns nil when the merged tree passed and its change is settled, and how the call ends otherwise.
func (fz *Finalize) gateUnlocked(ctx context.Context, front bool) *ending {
	held, err := fz.parentHoldsWork()
	if err != nil {
		return halt(ctx, finalizeName, ownerHoldsReadReason(err))
	}
	if held {
		return nil
	}
	// The pre-push check leaves a run at this row alone,
	// so the owners are judged before any commit the merge or a fixer makes.
	if end := fz.ownerChecks(ctx, held); end != nil {
		return end
	}
	fz.readConfigBeforeMerge()
	needsGitHub, readErr := fz.needsGitHubOnDisk()
	// The decision under the lock ends the call on a failed read, so the read's error only keeps the owed tips out here.
	takeOwedLocal := readErr == nil && !needsGitHub
	tip, lockReason, err := parentMergeTip(ctx, fz.deps, fz.guard, finalizeName, front, takeOwedLocal)
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	if lockReason != "" {
		return halt(ctx, finalizeName, lockReason)
	}
	if end := fz.mergeParent(ctx, tip); end != nil {
		return end
	}
	if end := fz.gateAndFix(ctx); end != nil {
		return end
	}
	fz.gatedTip = tip
	return nil
}

// returningFromFixer reports whether the row's attempt record shows a fixer's change already let through.
// Such a run is coming back from its fixer, and it queues at the front.
// A record that does not decode reads as no, and the gate then halts naming it.
func (fz *Finalize) returningFromFixer() (bool, error) {
	rec, reason, err := fz.readAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName))
	if err != nil {
		return false, err
	}
	return reason == "" && rec.Judged != "", nil
}

// landUnderLock runs one pass of the steps from the parent lock to the landing.
// It takes the lock, at the front of the queue when front is set, and releases it on every way out.
// A landing that merges on GitHub ends before that merge while the parent branch owes a push.
// One known to need GitHub before the lock ends without taking it.
// summary and credit compose the GitHub merge's title and message and the landing commit's message once the gate passed.
// After a lock-free merge and gate, the pass lands only while the parent's head is gatedTip;
// a parent that moved sets parentMoved and frees the lock.
// It returns nil when the parent moved.
// It also returns nil when the pass freed the lock for a fixer or fix review whose change was let through.
// On nil the caller runs the passes again.
// Otherwise it returns how the call ends.
func (fz *Finalize) landUnderLock(ctx context.Context, front bool, summary summaryparser.Summary, credit string) *ending {
	fz.lockFreed, fz.parentMoved = false, false

	// A landing already known to need GitHub waits for the parent's push without holding the lock.
	if fz.deps.ParentPushState != nil {
		// A failed read answers false here, since the decision under the lock makes the same read and ends the call on it.
		if needsGitHub, _ := fz.needsGitHubOnDisk(); needsGitHub {
			if end := fz.parentPushBlocks(ctx); end != nil {
				return end
			}
		}
	}

	// Step 1b2: take the parent lock before anything moves the parent.
	// The lock is released on every way out of this pass.
	// A lock a dead Finalize left is taken over by the acquire:
	// the lock store grants a record whose owner process no longer lives.
	lockReason, err := fz.guard.ensure(ctx, finalizeName, front)
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	if lockReason != "" {
		return halt(ctx, finalizeName, lockReason)
	}
	fz.lockHeld = fz.guard.lock.Store != nil
	defer func() {
		fz.lockHeld = false
		fz.guard.release(finalizeName)
	}()

	// Step 1b2b: under the lock, decide how the landing relates to the run's pull request.
	// The decision follows the resolver that `PR-Review` follows.
	landing, end := fz.pullRequestLanding(ctx)
	if end != nil {
		return end
	}
	// A landing another run recorded while this one waited for the lock is seen here.
	if landing.merge {
		if end := fz.parentPushBlocks(ctx); end != nil {
			return end
		}
	}

	// Whether the parent already holds the task's work is read for the owner checks.
	// It is read again after them for the land check and the sync.
	// The owner checks' run-scope reconcile can fast-forward the task branch.
	// A failed read halts as an owner check, so a doubt never skips the checks.
	held, err := fz.parentHoldsWork()
	if err != nil {
		return halt(ctx, finalizeName, ownerHoldsReadReason(err))
	}

	// Step 1b3: judge the run against origin under the lock, and halt unless this machine owns the run and its parent.
	// A run whose Finalize already merged into the local parent here is never read as ended or owned elsewhere,
	// so a Finalize re-entered after its done mark proceeds.
	if end := fz.ownerChecks(ctx, held); end != nil {
		return end
	}
	if held, err = fz.parentHoldsWork(); err != nil {
		return halt(ctx, finalizeName, ownerHoldsReadReason(err))
	}

	// Step 1b3b: under the same lock, halt a beam's landing while a member or a forked pair remains under it.
	if end := fz.landCheck(ctx, held); end != nil {
		return end
	}

	// Step 1b4: open the parent pair, which this producer never creates, and sync it before any merge.
	parentHandle, err := fz.parentOpener()
	if err != nil {
		return stuck(ctx, finalizeName, fmt.Sprintf("no live pair for parent branch %q: %v; way forward: re-create the parent pair, then run \"lym loom resume\"", fz.deps.ParentBranch, err), "error", err)
	}
	syncReason, err := fz.syncParent(parentHandle, held)
	if end := gateEnding(ctx, finalizeName, syncReason, err); end != nil {
		return end
	}

	// Step 1b4b: after a lock-free merge and gate, land only on the parent tip that pass merged.
	// A parent that moved since frees the lock, so the new tip is merged and gated with no lock held.
	// A parent that came to hold the task's work since that pass was not raced, so it is not compared.
	if fz.gatedTip != "" && !held {
		head, err := parentHandle.HeadSHA()
		if err != nil {
			return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the head of parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err))
		}
		if head != fz.gatedTip {
			logger.Info("landingshed: the parent moved since the lock-free merge and gate, so the run frees the lock and merges and gates again", "producer", finalizeName, "branch", fz.deps.ParentBranch, "gated", fz.gatedTip, "head", head)
			fz.parentMoved = true
			return nil
		}
	}

	// Step 1c: compose the notice about the task's per-worktree config changes after the parent pair's sync,
	// so it holds the parent's current values, and before any merge under the lock.
	// It is queued only at Done.
	notice := fz.configChangeNotice()

	// Steps 2 to 6 run once, and once more after a merge-in-required error from the parent-side merge:
	// the second pass merges the parent again under the same lock,
	// so a code-side commit that arrived between the check and the parent-side merge is merged, gated and landed.
	for retried := false; ; retried = true {
		// Steps 2 and 3: merge the parent branch into the task worktree and judge the merge, then gate the merged tree.
		if end := fz.mergeAndGate(ctx, parentHandle); end != nil {
			return end
		}
		if fz.lockFreed {
			return nil
		}
		title, body, landingMessage := fz.landingTexts(summary, credit)

		// Step 2b: commit the parent pair's own run records,
		// so a sibling run's uncommitted status record does not trip the parent-side merge guard.
		if end := fz.commitParentRecords(ctx); end != nil {
			return end
		}

		// Step 5: re-check the lock immediately before the landing, so a release since the acquire lands nothing.
		if end := fz.recheckLock(ctx); end != nil {
			return end
		}

		// Step 5b: a landing through an open pull request merges it on GitHub first, once,
		// so the parent-side merge below finds the parent holding the task's work.
		// The retry never comes back here, since the GitHub merge is done.
		if landing.merge && !retried {
			if end := fz.mergeOnGitHub(ctx, parentHandle, landing, title, body); end != nil {
				return end
			}
		}

		// Step 6: the parent-side merge, this producer's own merge critical section.
		// `MergeOptions.Message` is set whether or not `Config.Squash` is true.
		// It is the conclude-commit message for both merge shapes.
		// Gating it on Squash would leave the non-squash landing commit with an unset message.
		// A parent that takes its pull request at once always squashes on GitHub,
		// so the parent-side merge squashes too, whatever landing.yaml says.
		// A true merge never squashes, since a squash would drop the merged commit from the parent's history.
		// A merge that does not squash never fast-forwards, so it leaves a commit that carries the landing message.
		mergeOpts := fabriclayout.MergeOptions{Squash: (fz.deps.Config.Squash || landing.atOnce) && !landing.trueMerge, NoFastForward: true, Message: landingMessage}
		_, mergeErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
		if mergeErr == nil {
			// The landing has happened on the local parent, and its push is handed to the parent's pusher.
			// A beam's spent pull request record is dropped once that hand-off answers Done.
			// The parent's head right after the merge goes to both landing records.
			// On a pass whose merge commits, it is the run's own landing commit.
			landed, headErr := parentHandle.HeadSHA()
			fz.recordLanding(landed, headErr)
			end := fz.pushParent(ctx, landed, headErr, notice)
			if end.err == nil && end.outcome == shedengine.Done && landing.dropBeamRecord {
				fz.dropBeamRecord()
			}
			return end
		}
		var mergeInRequired *fabricengine.ErrMergeInRequired
		if !retried && errors.As(mergeErr, &mergeInRequired) {
			continue
		}

		// Steps 6-7: a guard error carrying the dirty-worktree reason is recognized by its accessor, not a matched string.
		// The error is surfaced verbatim: never stash, never reset, never retry.
		// Any other merge error is stuck with the error surfaced.
		var guardErr *fabricengine.MergeGuardError
		if errors.As(mergeErr, &guardErr) && guardErr.WorktreeDirty() {
			return stuck(ctx, finalizeName, fz.dirtyParentReason(parentHandle, guardErr))
		}
		return stuck(ctx, finalizeName, fmt.Sprintf("parent-side merge failed: %v", mergeErr), "error", mergeErr)
	}
}

// parentPushBlocks ends a landing that merges on GitHub while the parent branch's push is not done.
// GitHub therefore never merges into a base that lacks an earlier landing.
// An owed push ends it with a git-transport transient error.
// A refused push ends it with a blocked halt naming the refusal's way forward.
// It returns nil with neither, and with no seam wired.
func (fz *Finalize) parentPushBlocks(ctx context.Context) *ending {
	if fz.deps.ParentPushState == nil {
		return nil
	}
	state, err := fz.deps.ParentPushState()
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the push state of parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err))
	}
	switch {
	case state.Refused:
		return halt(ctx, finalizeName, fmt.Sprintf("%sthe push of parent branch %q is refused, so GitHub would merge into a base that lacks its landings; way forward: %s; then, once that cleared the refusal, run `lym loom resume` in the task worktree", LandingStopPrefix, fz.deps.ParentBranch, state.WayForward))
	case state.Owed:
		return fail(ctx, finalizeName, shedengine.MarkTransient(shedengine.TransientGitTransport,
			fmt.Errorf("landingshed: %s: parent branch %q still owes the push of a landing, so GitHub would merge into a stale base; the landing's re-step goes on once origin holds it", finalizeName, fz.deps.ParentBranch)))
	}
	return nil
}

// needsGitHubOnDisk reports whether the landing needs GitHub, as read with no parent lock and no GitHub call.
// A recorded pull request needs it, and so does a trail mode.
// A review mode needs it when its origin and task branch let a pull request open.
// A failed read is returned as the error, with false.
func (fz *Finalize) needsGitHubOnDisk() (bool, error) {
	pr := fz.deps.PullRequest
	if pr.Mode == nil {
		return false, nil
	}
	_, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return false, err
	}
	if found {
		return true, nil
	}
	beam := NotABeam
	if pr.BeamRecord != nil {
		_, recorded, err := pr.BeamRecord()
		if err != nil {
			return false, err
		}
		if recorded {
			return true, nil
		}
		beam = BeamWithoutPullRequest
	}
	mode, err := pr.Mode()
	if err != nil {
		return false, err
	}
	switch mode {
	case beamsetting.PullRequestTrail:
		return true, nil
	case beamsetting.PullRequestReview:
		origin, err := pr.Origin()
		if err != nil {
			return false, err
		}
		hasCommits, err := pr.TaskHasCommits()
		if err != nil {
			return false, err
		}
		return ResolvePullRequest(mode, beam, false, origin, hasCommits).Open, nil
	}
	return false, nil
}

// parentHoldsWork reports whether the parent branch already holds the task's work; with no seam wired it does not.
func (fz *Finalize) parentHoldsWork() (bool, error) {
	if fz.deps.ParentHoldsTaskWork == nil {
		return false, nil
	}
	return fz.deps.ParentHoldsTaskWork()
}

// ownerHoldsReadReason is the owner-check halt reason of a failed read of whether the parent holds the task's work.
func ownerHoldsReadReason(err error) string {
	return OwnerHaltPrefix + fmt.Sprintf("could not read whether the parent branch already holds the task's work: %v; %s", err, ownerRerun)
}

// freeLock frees the parent lock before an agent session that works on the task worktree alone.
// This lets other runs land on the parent meanwhile.
// The run then runs its passes again and takes the lock again at the front of the queue.
// The lock-free pass runs first while lost races remain.
// A Finalize that holds no lock, as in its lock-free pass or with no lock store, frees nothing;
// it goes on in the same pass.
func (fz *Finalize) freeLock() {
	if !fz.lockHeld {
		return
	}
	fz.guard.release(finalizeName)
	fz.lockHeld, fz.lockFreed = false, true
	logger.Info("landingshed: freed the parent lock for an agent session; the run takes it again after", "producer", finalizeName, "branch", fz.deps.ParentBranch)
}

// syncParent probes the opened parent pair and, with a push, brings its local branch level with its upstream;
// it returns a Stuck reason, or "" when Finalize may go on.
// A pair mid-merge or holding tracked dirt in its code checkout is Stuck.
// That holds whether the parent is behind, equal, without an upstream, or under the push skip.
// Under the push skip, syncParent then neither fetches nor fast-forwards.
// A parent ahead of its upstream by landing tips whose push is owed reads as level, so the landing adds its own on top.
// Any other parent branch ahead of or diverged from its upstream is Stuck, since Finalize never rewrites the parent.
// A fetch that fails on transport is a transient error.
// A parent behind its upstream is fast-forwarded in its own pair.
// The fast-forward moves it to the upstream tip and touches no local commit.
// An upstream that is no longer a fast-forward of the pair is Stuck.
// A parent already holding the task's work, as held reports, is past its parent-side merge and is left as it is.
func (fz *Finalize) syncParent(parent parentMerger, held bool) (string, error) {
	branch := fz.deps.ParentBranch
	const resume = `then run "lym loom resume"`
	if held {
		return "", nil
	}

	state, err := parent.MidMerge()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the merge state of the parent pair of %q: %w", finalizeName, branch, err)
	}
	if state.Kind != fabriclayout.MidMergeNone {
		return fmt.Sprintf("a merge is in progress in the parent pair of %q; way forward: finish or abort that merge in the parent pair, %s", branch, resume), nil
	}
	checkout, dirty, err := parent.CodeDirtyPaths()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the dirty paths of the parent pair of %q: %w", finalizeName, branch, err)
	}
	if len(dirty) > 0 {
		return DirtyParentRefusal(checkout, dirty), nil
	}
	if fz.deps.PushSkipped {
		return "", nil
	}

	tips, err := fz.deps.ReadParentTips(true)
	if err != nil {
		err = fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", finalizeName, branch, err)
		if shedtransient.Class(err) == shedengine.TransientGitTransport {
			err = shedengine.MarkTransient(shedengine.TransientGitTransport, err)
		}
		return "", err
	}
	switch tips.Relation {
	case fabriclayout.ParentNoUpstream, fabriclayout.ParentEqual:
		return "", nil
	case fabriclayout.ParentAhead:
		if fz.deps.ParentAheadByOwed != nil {
			owed, err := fz.deps.ParentAheadByOwed()
			if err != nil {
				return "", fmt.Errorf("landingshed: %s: read whether parent branch %q is ahead by owed landings alone: %w", finalizeName, branch, err)
			}
			if owed {
				return "", nil
			}
		}
		return fmt.Sprintf("the local parent branch %q is ahead of its upstream; way forward: push or reconcile %q in its own pair, %s", branch, branch, resume), nil
	case fabriclayout.ParentDiverged:
		return fmt.Sprintf("the local parent branch %q has diverged from its upstream; way forward: push or reconcile %q in its own pair, %s", branch, branch, resume), nil
	case fabriclayout.ParentBehind:
	default:
		return "", fmt.Errorf("landingshed: %s: unknown relation %q of parent branch %q to its upstream", finalizeName, tips.Relation, branch)
	}

	result, err := parent.FastForwardPair(fabricengine.SyncOptions{SkipPush: fz.deps.PushSkipped})
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: fast-forward the parent pair of %q: %w", finalizeName, branch, err)
	}
	switch result.Refusal {
	case "":
		return "", nil
	case fabriclayout.FastForwardDirty:
		return fmt.Sprintf("the parent pair of %q has uncommitted changes; way forward: commit or stash them in the parent pair, %s", branch, resume), nil
	case fabriclayout.FastForwardMergeInProgress:
		return fmt.Sprintf("a merge is in progress in the parent pair of %q; way forward: finish or abort that merge in the parent pair, %s", branch, resume), nil
	case fabriclayout.FastForwardNotFastForward:
		return fmt.Sprintf("the upstream of %q is no longer a fast-forward of the parent pair; way forward: reconcile %q with its upstream in its own pair, %s", branch, branch, resume), nil
	default:
		return "", fmt.Errorf("landingshed: %s: unknown fast-forward refusal %q for parent branch %q", finalizeName, result.Refusal, branch)
	}
}

// maxListedDirtyPaths caps how many dirty paths a refusal lists before it counts the rest.
const maxListedDirtyPaths = 20

// gotoResumeWayForward is the closing step every dirty-parent way forward ends in.
const gotoResumeWayForward = `"lym loom goto --to Finalize" and "lym loom resume" in the task worktree`

// dirtyPathList renders the first maxListedDirtyPaths paths, Go-quoted and joined by ", ".
// It ends the list with ", and N more" when there are more paths.
// Quoting keeps a path holding a space or a comma one item, and the form holds no line break.
func dirtyPathList(paths []string) string {
	shown := paths
	if len(shown) > maxListedDirtyPaths {
		shown = shown[:maxListedDirtyPaths]
	}
	quoted := make([]string, len(shown))
	for i, p := range shown {
		quoted[i] = strconv.Quote(p)
	}
	list := strings.Join(quoted, ", ")
	if more := len(paths) - len(shown); more > 0 {
		list += fmt.Sprintf(", and %d more", more)
	}
	return list
}

// dirtyParentSubject names the parent's code checkout and the tracked paths dirty in it.
func dirtyParentSubject(checkout string, paths []string) string {
	return fmt.Sprintf("the parent's code checkout %s has uncommitted tracked paths: %s", checkout, dirtyPathList(paths))
}

// DirtyParentRefusal words the refusal for a parent whose code checkout holds uncommitted tracked paths.
// It names the checkout, the paths (capped and quoted), and the way forward when the dirt is the only obstacle.
func DirtyParentRefusal(checkout string, paths []string) string {
	return fmt.Sprintf("%s; way forward: commit or discard them in %s with plain git, then %s",
		dirtyParentSubject(checkout, paths), checkout, gotoResumeWayForward)
}

// dirtyParentReason words the Stuck reason for a parent-side merge that the dirty guard refused.
// The reason takes one of four forms, set by the other reasons the guard error carries:
// a merge already in progress, a detached HEAD, a branch not synced to its upstream, or dirt alone.
func (fz *Finalize) dirtyParentReason(parentHandle parentMerger, guardErr *fabricengine.MergeGuardError) string {
	checkout := guardErr.Checkout
	text := guardErr.Error()
	switch {
	case guardErr.AlreadyInProgress():
		reason := fmt.Sprintf("%s; the parent's code checkout is %s", text, checkout)
		wayForward := gotoResumeWayForward
		state, err := parentHandle.MidMerge()
		switch {
		case err != nil:
			logger.Warn("landingshed: read the parent pair's parked merge failed", "producer", finalizeName, "cause", err)
			wayForward = fz.parkedMergeWayForward(checkout)
		case state.Kind == fabriclayout.MidMergeParked:
			reason += fmt.Sprintf("; the parked fabric merge is a %s of %s in the parent pair", state.Verb, state.Source)
			wayForward = fz.parkedMergeWayForward(checkout)
		}
		return reason + "; way forward: " + wayForward
	case guardErr.DetachedHead():
		return fmt.Sprintf("%s: %s; way forward: check out the parent branch %q in %s with plain git, and when the checkout refuses to carry the changes, set them aside first with plain git and restore them on the branch after it; then commit them on the parent branch or discard them in %s with plain git; then %s",
			text, dirtyParentSubject(checkout, guardErr.DirtyPaths), fz.deps.ParentBranch, checkout, checkout, gotoResumeWayForward)
	case guardErr.NotSynced():
		return fmt.Sprintf("%s: %s; way forward: commit them on the parent branch or discard them in %s with plain git; then push or reconcile the parent branch %q in its own pair; then %s",
			text, dirtyParentSubject(checkout, guardErr.DirtyPaths), checkout, fz.deps.ParentBranch, gotoResumeWayForward)
	default:
		return text + ": " + DirtyParentRefusal(checkout, guardErr.DirtyPaths)
	}
}

// parkedMergeWayForward words the way forward out of a fabric merge parked in the parent pair.
// "--abort" hard-resets the parent's code checkout,
// and a plain git commit or stash there would take the staged merge with it,
// so the operator saves a copy of their own change outside the checkout first.
func (fz *Finalize) parkedMergeWayForward(checkout string) string {
	return fmt.Sprintf(`the party that started that merge, or the orch or the operator when that party is gone, concludes it with "lym fabric merge --continue" or discards it with "lym fabric merge --abort" in the parent pair, then %s; "--abort" hard-resets the parent's code checkout to the merge's start, so the operator first saves a copy of any change of their own in %s outside it, since a plain git commit or stash there would take the staged merge with it; a session lym refuses the verb from reports status: FAILED and the orch runs it`,
		gotoResumeWayForward, checkout)
}

// readConfigBeforeMerge reads the task's per-worktree config changes, once per call.
// A merge of the parent moves the base the changes are measured from, so the read precedes the call's first one.
func (fz *Finalize) readConfigBeforeMerge() {
	if fz.configBeforeMerge != nil || fz.deps.ConfigChanges == nil {
		return
	}
	changes, err := fz.deps.ConfigChanges()
	fz.configBeforeMerge = &configRead{changes: changes, err: err}
}

// configChangeNotice composes the one-line notice about the task's per-worktree config changes, key by key.
// It returns "" when there is nothing to report or no seam is wired.
// The changes and their base are those read before the call's first merge of the parent.
// The parent values are read again here, and a file that no longer differs from the parent is not named.
// Per changed file and its module it names each key whose tip value differs from both the fork point and the parent.
// Each named key comes with its base, tip and parent values;
// a file that changes no key, such as a comments-only edit, and a key the parent already holds are not named.
// beam.autonomy is never named either:
// it is the worktree's own per-task setting, so a child's value is never meant for its parent.
// Finalize never carries those files to the parent,
// so the notice tells the orchestrator to re-apply each change meant for the parent.
// A read or parse failure is logged and reported in the notice instead of stopping the landing.
func (fz *Finalize) configChangeNotice() string {
	if fz.deps.ConfigChanges == nil {
		return ""
	}
	fz.readConfigBeforeMerge()
	changes, err := fz.configBeforeMerge.changes, fz.configBeforeMerge.err
	var current fabriclayout.ConfigChanges
	if err == nil {
		current, err = fz.deps.ConfigChanges()
	}
	if err != nil {
		logger.Warn("landingshed: read config changes failed", "producer", finalizeName, "cause", err)
		line := fmt.Sprintf("loom: the config changes of task branch %q could not be read: %v; diff its config files against the parent branch %q by hand",
			fz.deps.TaskBranch, err, fz.deps.ParentBranch)
		return strings.Join(strings.Fields(line), " ")
	}

	var keyed, unread []string
	for _, file := range changes.Files {
		now, differs := current.Contents[file]
		if !differs {
			continue
		}
		contents := changes.Contents[file]
		contents.Parent = now.Parent
		module := strings.TrimSuffix(path.Base(file), path.Ext(file))
		keys, err := configengine.ChangedKeys(contents.Base, contents.Tip, contents.Parent)
		if err != nil {
			logger.Warn("landingshed: compare config keys failed", "producer", finalizeName, "file", file, "cause", err)
			unread = append(unread, fmt.Sprintf("%s (module %s): %v", file, module, err))
			continue
		}
		var named []string
		for _, key := range keys {
			if module == beamsetting.Module && key.Key == beamsetting.AutonomyKey {
				continue
			}
			named = append(named, fmt.Sprintf("%s.%s: base %s, tip %s, parent %s", module, key.Key, key.Base, key.Tip, key.Parent))
		}
		if len(named) > 0 {
			keyed = append(keyed, fmt.Sprintf("%s (module %s): %s", file, module, strings.Join(named, "; ")))
		}
	}

	var parts []string
	if len(keyed) > 0 {
		parts = append(parts, fmt.Sprintf("task branch %q changed per-worktree config keys since it forked from %q: %s; landing does not carry them to the parent branch, so re-apply each change meant for the parent on the parent's copy of the same file",
			fz.deps.TaskBranch, fz.deps.ParentBranch, strings.Join(keyed, "; ")))
	}
	if len(unread) > 0 {
		parts = append(parts, fmt.Sprintf("the config files of task branch %q changed since it forked from %q could not be compared key by key: %s; diff them against the parent branch by hand",
			fz.deps.TaskBranch, fz.deps.ParentBranch, strings.Join(unread, "; ")))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(strings.Fields("loom: "+strings.Join(parts, "; also, ")), " ")
}

// markTaskDone writes the task's board entry done for a landing with pushes off, which no later push follows.
// A failed write is returned, so the driver re-steps the landing and the write is made again.
func (fz *Finalize) markTaskDone() error {
	if fz.deps.MarkTaskDone == nil {
		return nil
	}
	if err := fz.deps.MarkTaskDone(); err != nil {
		return fmt.Errorf("landingshed: %s: mark the board task done: %w", finalizeName, err)
	}
	return nil
}

// pushRecords commits the run's status file and pushes nothing.
// It returns nil when the landing may go on.
func (fz *Finalize) pushRecords() *ending {
	if fz.deps.CommitStatus != nil {
		if err := fz.deps.CommitStatus(); err != nil {
			return &ending{err: fmt.Errorf("landingshed: %s: commit status file: %w", finalizeName, err)}
		}
	}
	return nil
}

// recordLanding hands commit, the parent's head right after the parent-side merge, to `Deps.RecordLanding`.
// On a pass whose merge commits, it is the run's own landing commit.
// With it goes whether the landed diff is empty.
// It then hands the landed diff's size to `Deps.RecordLandedSize`.
// A non-nil headErr is the failed read of that commit.
// It is logged and records nothing; a failed record is logged too, and neither changes the verdict.
func (fz *Finalize) recordLanding(commit string, headErr error) {
	if fz.deps.RecordLanding == nil && fz.deps.RecordLandedSize == nil {
		return
	}
	if headErr != nil {
		logger.Warn("landingshed: read the landing commit failed", "producer", finalizeName, "cause", headErr)
		return
	}
	if fz.deps.RecordLanding != nil {
		if err := fz.deps.RecordLanding(commit, fz.landedDiffEmpty(commit)); err != nil {
			logger.Warn("landingshed: record the landing commit failed", "producer", finalizeName, "cause", err)
		}
	}
	fz.recordLandedSize(commit)
}

// landedDiffEmpty reports whether the landing changed no code path against the parent head merged into the task worktree.
// Such a landing put none of the run's work on the parent branch, which is logged at Warn.
// It reports false when no such head is held or the path read fails, which is logged too.
func (fz *Finalize) landedDiffEmpty(landing string) bool {
	if fz.parentHeadMerged == "" || fz.deps.ChangedPaths == nil {
		return false
	}
	paths, err := fz.deps.ChangedPaths(fz.parentHeadMerged, landing)
	if err != nil {
		logger.Warn("landingshed: read the landed diff's paths failed, so the landing is recorded as not empty", "producer", finalizeName, "cause", err)
		return false
	}
	if len(paths) > 0 {
		return false
	}
	logger.Warn("landingshed: the landing changed no code path on the parent branch", "producer", finalizeName, "landing", landing)
	return true
}

// recordLandedSize hands `Deps.RecordLandedSize` the parent head merged into the task worktree and the landing commit.
// A Finalize that holds no such head records nothing, which is logged at Info;
// a failed record is logged at Warn and never changes the verdict.
func (fz *Finalize) recordLandedSize(landing string) {
	if fz.deps.RecordLandedSize == nil {
		return
	}
	if fz.parentHeadMerged == "" {
		logger.Info("landingshed: no merged parent head was read in this process, so the landed size is not recorded", "producer", finalizeName)
		return
	}
	if err := fz.deps.RecordLandedSize(fz.parentHeadMerged, landing); err != nil {
		logger.Warn("landingshed: record the landed size failed", "producer", finalizeName, "cause", err)
	}
}

// keepParentHead reads the parent pair's head before the merge of the parent branch brings it into the task worktree.
// It keeps that head as the base of the landed diff.
// A failed read is logged at Warn and keeps no head, so the landed size goes unrecorded.
func (fz *Finalize) keepParentHead(parentHandle parentMerger) {
	head, err := parentHandle.HeadSHA()
	if err != nil {
		logger.Warn("landingshed: read the parent head before the merge-in failed", "producer", finalizeName, "cause", err)
		head = ""
	}
	fz.parentHeadMerged = head
}

// pushParent hands the push of the parent branch the merge just landed on to that parent's pusher, and reports Done,
// so no failed push fails a landing.
// It first reads whether another machine owns the run now, and one that does takes the run over and halts.
// It records landed as the landing's tip through `Deps.LandLocally`.
// landed is the parent's head right after the parent-side merge.
// On a pass whose merge commits, that head is the run's own landing commit.
// A non-nil headErr is the failed read of landed, which fails the call only when that seam is reached.
// It writes no board `done`, since that write is made once origin holds the landing.
// With a nil `Deps.LandLocally` it records no landing and commits the run's records.
// Under `Deps.PushSkipped` it records no landing, commits the run's records and writes the board `done` itself.
// The hand-off is the same whether the landing went through a pull request merged on GitHub or not.
// On its Done return only, a non-empty notice is queued through `Deps.Notify`;
// a failure there is a logged warning that never changes the verdict.
func (fz *Finalize) pushParent(ctx context.Context, landed string, headErr error, notice string) *ending {
	if end := fz.handedOverStop(ctx); end != nil {
		return end
	}
	if fz.deps.PushSkipped || fz.deps.LandLocally == nil {
		if end := fz.pushRecords(); end != nil {
			return end
		}
		if fz.deps.PushSkipped {
			if err := fz.markTaskDone(); err != nil {
				return &ending{err: err}
			}
		}
	} else {
		if headErr != nil {
			return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the head of parent branch %q: %w", finalizeName, fz.deps.ParentBranch, headErr))
		}
		if err := fz.deps.LandLocally(landed); err != nil {
			return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: record the landing on parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err))
		}
	}
	// The landing is done, so a failed removal of the attempt record changes no verdict.
	if err := RemoveFixerAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName)); err != nil {
		logger.Warn("landingshed: remove the fixer attempt record failed", "producer", finalizeName, "cause", err)
	}
	if notice != "" && fz.deps.Notify != nil {
		if err := fz.deps.Notify(notice); err != nil {
			logger.Warn("landingshed: queue config-change notice failed", "producer", finalizeName, "cause", err)
		}
	}
	return done()
}

// recheckLock re-reads the parent lock record immediately before a landing attempt.
// It returns nil when the record still names this run, so the landing proceeds.
// Otherwise it ends the call:
// a halting Stuck for a release or an unreadable record, an error for an infrastructure fault.
func (fz *Finalize) recheckLock(ctx context.Context) *ending {
	reason, err := fz.guard.stillHeld(finalizeName)
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	if reason != "" {
		return halt(ctx, finalizeName, reason)
	}
	return nil
}

// mergeAndGate merges the parent branch into the task worktree, judges the merge and gates the merged tree.
// It returns nil when the merged tree passed and the caller goes on to the parent-side merge.
// Every pass is committed and verified before the parent-side merge.
// That covers the first pass and the retry after the parent moved again.
// A halt, a gate failure or a dirty tree ends the call before the parent-side merge, the board update and the push.
// Right before the merge it keeps the parent pair's head, the base of the landed diff.
func (fz *Finalize) mergeAndGate(ctx context.Context, parentHandle parentMerger) *ending {
	fz.keepParentHead(parentHandle)
	if end := fz.mergeParent(ctx, ""); end != nil {
		return end
	}
	return fz.gateAndFix(ctx)
}

// gateAndFix runs the `landing` gate over the merged tree and, on a failure, the merge fixer in finalize mode.
// The gate skips a tree it already verified,
// so a run that went right holds the lock for one merge, a skipped gate and the landing.
// A passing gate removes the landing failure record and a failing one writes it.
// The fixer and the fix review run with the parent lock freed.
//
// A failing gate spawns the fixer while the entry has attempts left under landing.finalize_fix_attempts.
// The attempts are read from the attempt record under the row's entry.
// A gate whose result is a runner crash or a memory pressure stop judged no code;
// it halts with the gate's LandingStopPrefix reason, spawning no fixer and spending no attempt.
// A pass after the lock was taken again spends from the same attempts,
// so a second failed verify never grants a fresh cap.
// The fixer's whole change is judged against the size and kind caps from the entry's Baseline.
// That is the HEAD before the fixer's first turn, so turns that each stay under the cap but sum past it are over it.
// An entry has no attempts left when its Spent reaches the cap, whatever the record's Halt notes.
// Such an entry spawns no fixer.
// A failing gate then ends it as a spent cap, and a passing one judges its change since Baseline.
// A passing gate with a fix review's findings pending, and a turn left, spawns the fixer on those findings.
// A spent cap keeps the fixer's commits and halts with LandingStopPrefix, noting `spent` in the attempt record's Halt.
// A resume after a halt at the fix review round cap lands the change when the operator accepted it.
// The acceptance counts only at the task HEAD the resume finds;
// otherwise the resume runs the fix reviews afresh over the change as it then stands.
// An escalate report halts with EscalateStopPrefix when Deps.EscalateJudgedFirst is set;
// otherwise it halts with LandingStopPrefix.
// An unusable turn and an attempt record that does not decode each halt with LandingStopPrefix;
// a failed spawn is an error that spends no attempt.
// It returns nil when the tree passed and its change is settled, and the caller proceeds.
func (fz *Finalize) gateAndFix(ctx context.Context) *ending {
	attemptsPath := FixerAttemptsPath(fz.deps.RecordsDir, finalizeName)
	rec, reason, err := fz.readAttempts(attemptsPath)
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	if reason != "" {
		return halt(ctx, finalizeName, reason)
	}
	if rec.Halt == fixerHaltReviewCapped {
		rec.Halt = ""
		if rec.AcceptedHead == "" {
			rec.ReviewRounds, rec.FixReview, rec.Findings = 0, "", nil
		}
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return fail(ctx, finalizeName, err)
		}
	}
	if rec, err = fz.rebaseAfterParentMerge(rec); err != nil {
		return fail(ctx, finalizeName, err)
	}

	gateReason, unjudged, err := fz.gate.check(ctx, finalizeName, fz.deps.ParentBranch)
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	if unjudged {
		return halt(ctx, finalizeName, gateReason)
	}
	passed := gateReason == ""
	if passed {
		fz.gate.clearFailure()
		reason, err = fz.gate.clean(finalizeName, "after the verify")
		if end := gateEnding(ctx, finalizeName, reason, err); end != nil {
			return end
		}
	}

	spent := rec.Spent >= fz.deps.Config.FinalizeFixAttempts
	switch {
	case passed && (rec.FixReview == "" || spent):
		return fz.judgeFixerChange(ctx, rec)
	case spent:
		return fz.endSpent(ctx, rec, fz.failedGateLog(gateReason))
	}
	return fz.runFixer(ctx, rec)
}

// rebaseAfterParentMerge moves the entry's Baseline to the task head.
// It does so when this pass's merge brought parent commits onto the head the fixer's change was last let through at.
// This keeps the parent's commits from counting as the fixer's change,
// and a fixer change after the merge is judged from that head.
// Any other record comes back as it is.
func (fz *Finalize) rebaseAfterParentMerge(rec FixerAttempts) (FixerAttempts, error) {
	if rec.Judged == "" {
		return rec, nil
	}
	merge, found, err := ReadMergeRecord(fz.deps.MergeRecordPath)
	if err != nil {
		return rec, fmt.Errorf("landingshed: %s: read the merge record: %w", finalizeName, err)
	}
	if !found || merge.UpToDate || merge.TaskHead != rec.Judged {
		return rec, nil
	}
	head, err := fz.deps.TaskHead()
	if err != nil {
		return rec, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err)
	}
	rec.Baseline, rec.Judged = head, head
	return rec, WriteFixerAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName), rec)
}

// readAttempts reads the fixer attempt record at path for the row's current entry, entry 0 with no row-entry read.
// A record that does not decode comes back as a halting reason naming the file and how to remove it.
func (fz *Finalize) readAttempts(path string) (FixerAttempts, string, error) {
	entry := 0
	if fz.deps.RowEntry != nil {
		var err error
		if entry, err = fz.deps.RowEntry(finalizeName); err != nil {
			return FixerAttempts{}, "", fmt.Errorf("landingshed: %s: read the row entry: %w", finalizeName, err)
		}
	}
	return readRowAttempts(finalizeName, path, entry)
}

// runFixer runs the merge fixer in finalize mode on the attempts the entry has left.
// It maps how the run ended onto the call's ending.
// The first spawn writes the HEAD before its first turn as the record's Baseline, which every resume of the entry keeps.
// A fixer spawned on a fix review's findings reads that review's report, which the record names.
// A fixed end clears the pending findings and is judged against the caps;
// a spent end ends as a spent cap, and every other end halts with its reason.
func (fz *Finalize) runFixer(ctx context.Context, rec FixerAttempts) *ending {
	fz.freeLock()
	mergeRecord, _, err := ReadMergeRecord(fz.deps.MergeRecordPath)
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the merge record the fixer reads: %w", finalizeName, err))
	}
	if rec.Baseline == "" {
		head, err := fz.deps.TaskHead()
		if err != nil {
			return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
		}
		rec.Baseline = head
	}
	outcome, updated, err := newFixerRun(fz.deps, finalizeName, fz.gate).run(ctx, FixerModeFinalize, rec, fz.deps.Config.FinalizeFixAttempts, [2]string{"", mergeRecord.MergedTip})
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	switch outcome.end {
	case fixerEndFixed:
		fz.gate.clearFailure()
		if updated.FixReview != "" {
			updated.FixReview = ""
			if err := WriteFixerAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName), updated); err != nil {
				return fail(ctx, finalizeName, err)
			}
		}
		return fz.judgeFixerChange(ctx, updated)
	case fixerEndSpent:
		return fz.endSpent(ctx, updated, outcome.failureLog)
	}
	return halt(ctx, finalizeName, outcome.haltReason(finalizeName, fz.deps.EscalateJudgedFirst))
}

// failedGateLog names the log of the landing gate's failure.
// It is the failure record's log path when one was written, and the gate's reason as one line otherwise.
func (fz *Finalize) failedGateLog(gateReason string) string {
	if failure, found, err := verifytree.ReadLandingFailure(fz.gate.paths); err == nil && found {
		return failure.LogPath
	}
	return singleLine(gateReason)
}

// judgeFixerChange judges the fixer's change, from the record's Baseline to HEAD, against the size and kind caps.
// It returns nil when the change is let through and the landing goes on.
// An entry whose fixer never ran has no Baseline and no change, and it goes on.
// An entry whose Baseline is HEAD has no change, and it goes on with HEAD kept as Judged.
// A change within the caps goes on the same way.
// So does a change a fix review approved at this HEAD, and one the operator accepted at this HEAD.
// A change over the caps goes to the fix review, which reads only the fixer's commits,
// and each review spends one of landing.finalize_fix_review_rounds:
// an approval lets the landing go on, and the attempt record keeps the HEAD it approved,
// so a later pass at that HEAD lands without a second review;
// a rejection hands its findings to the fixer, on a fresh turn cap, and the fixer's fixed change is judged again;
// a review that gives no verdict runs again;
// a review that changed the task branch halts with LandingStopPrefix;
// and a review that does not approve on the last round halts for the operator with its open findings.
func (fz *Finalize) judgeFixerChange(ctx context.Context, rec FixerAttempts) *ending {
	if rec.Baseline == "" {
		return nil
	}
	head, err := fz.deps.TaskHead()
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
	}
	attemptsPath := FixerAttemptsPath(fz.deps.RecordsDir, finalizeName)
	letThrough := func(rec FixerAttempts) *ending {
		rec.Judged = head
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return fail(ctx, finalizeName, err)
		}
		return nil
	}
	if rec.Baseline == head {
		return letThrough(rec)
	}
	change, err := fz.deps.LineChange(rec.Baseline, head)
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the fixer's change: %w", finalizeName, err))
	}
	capReason := FixCap{MaxLines: fz.deps.Config.FinalizeFixMaxLines, IsTestFile: fz.deps.IsTestFile}.Exceeded(change)
	if capReason == "" || rec.Reviewed == head || rec.AcceptedHead == head {
		return letThrough(rec)
	}
	// An acceptance covers only the HEAD it was given at, so a change past it starts the fix reviews afresh.
	if rec.AcceptedHead != "" {
		rec.AcceptedHead, rec.ReviewRounds, rec.Findings = "", 0, nil
	}

	fz.freeLock()
	for {
		review, err := runFixReview(fz.deps, fz.gate, finalizeName, rec.Baseline, head, capReason)
		if err != nil {
			return fail(ctx, finalizeName, err)
		}
		rec.ReviewRounds++
		rec.FixReview = ""
		switch review.end {
		case fixReviewEndApproved:
			rec.Reviewed = head
			return letThrough(rec)
		case fixReviewEndChanged:
			if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
				return fail(ctx, finalizeName, err)
			}
			return halt(ctx, finalizeName, fmt.Sprintf("%sthe fix review session of %s changed the task branch: %s; session: %s; way forward: restore the task branch to the fixer's commits %s..%s with a clean tree, then `lym loom resume`", LandingStopPrefix, finalizeName, review.cause, review.session, rec.Baseline, head))
		}
		if rec.ReviewRounds >= fz.deps.Config.FinalizeFixReviewRounds {
			return fz.haltReviewCapped(ctx, rec, head, capReason, review)
		}
		if review.end == fixReviewEndRejected {
			rec.FixReview, rec.Spent = review.reportPath, 0
			if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
				return fail(ctx, finalizeName, err)
			}
			return fz.runFixer(ctx, rec)
		}
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return fail(ctx, finalizeName, err)
		}
	}
}

// haltReviewCapped halts the entry for the operator when its fix reviews run out of rounds without an approval.
// It keeps the fixer's commits.
// The attempt record notes the halt and the findings the last review left open.
// An acceptance writes those findings into the change description.
func (fz *Finalize) haltReviewCapped(ctx context.Context, rec FixerAttempts, head, capReason string, review fixReviewOutcome) *ending {
	rec.Halt, rec.Findings, rec.AcceptedHead = fixerHaltReviewCapped, review.openFindings(), ""
	if err := WriteFixerAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName), rec); err != nil {
		return fail(ctx, finalizeName, err)
	}
	return halt(ctx, finalizeName, fmt.Sprintf("%sthe fix review of %s did not approve the fixer's change over the caps (%s) in %d rounds: %s; fixer commits: %s..%s; way forward: run `lym loom circling accept %s` to land the change as it stands with the open findings in the change description, or fix the findings on the task branch; then `lym loom resume`",
		LandingStopPrefix, finalizeName, capReason, rec.ReviewRounds, review.verdictSummary(), rec.Baseline, head, fz.deps.Slug))
}

// endSpent ends an entry whose attempts are spent over a gate that still fails, naming the failed gate's log.
// It keeps the fixer's commits, notes `spent` in the attempt record's Halt and halts with LandingStopPrefix.
func (fz *Finalize) endSpent(ctx context.Context, rec FixerAttempts, log string) *ending {
	head, err := fz.deps.TaskHead()
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
	}
	commits := "none"
	if rec.Baseline != "" {
		commits = rec.Baseline + ".." + head
	}
	rec.Halt = fixerHaltSpent
	if err := WriteFixerAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName), rec); err != nil {
		return fail(ctx, finalizeName, err)
	}
	return halt(ctx, finalizeName, fmt.Sprintf("%sthe fixer spent every attempt of %s and the landing gate still fails; log: %s; fixer commits: %s; way forward: fix the failure on the task branch, then `lym loom resume`", LandingStopPrefix, finalizeName, log, commits))
}

// mergeParent merges the parent branch into the task worktree, and judges the merge.
// A non-empty tip is the parent commit merged in place of the local parent branch's tip.
// It clears a parked merge-in, checks the tree clean and merges.
// The merge goes through the resolver, which resolves a conflict with the conflict session.
// The task head is kept in the base file before the merge,
// and the ignored leftovers measured from it are removed before each clean check,
// so a pass stopped after the merge leaves the next one its base.
//
// The merge is large when a change the parent made touches a file the task changed.
// The parent's changes are those since the merged tip the merge record names.
// A merge record that cannot be read is logged and read as absent.
// The parent side is then measured from the task's merge base.
// A parent already holding the task's work is never large.
// The merge record is written with the new merged tip, the aligned tip as it was, and the overlap.
// A large merge goes on to the gate like any other merge.
// It returns nil when the merge settled and the step goes on to the gate.
func (fz *Finalize) mergeParent(ctx context.Context, tip string) *ending {
	reason, err := fz.premerge.clear(finalizeName, fz.deps.ParentBranch)
	if end := gateEnding(ctx, finalizeName, reason, err); end != nil {
		return end
	}
	if end := fz.removeLeftovers(ctx); end != nil {
		return end
	}
	reason, err = fz.gate.clean(finalizeName, "before the merge-in")
	if end := gateEnding(ctx, finalizeName, reason, err); end != nil {
		return end
	}

	before, err := readMergeBefore(fz.deps, finalizeName)
	if err != nil {
		return &ending{err: err}
	}
	if err := fz.keepMergeInBase(before.taskHead); err != nil {
		return &ending{err: err}
	}
	merge, err := fz.resolver.Resolve(ctx, fz.deps.ParentBranch, tip)
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err))
	}
	if merge.Outcome == mergeresolve.OutcomeStuck {
		return stuck(ctx, finalizeName, merge.Reason)
	}
	if end := fz.removeLeftovers(ctx); end != nil {
		return end
	}
	reason, err = fz.gate.clean(finalizeName, "after the merge-in")
	if end := gateEnding(ctx, finalizeName, reason, err); end != nil {
		return end
	}

	newTip, overlap, err := fz.judgeParentMerge(before, merge.AlreadyUpToDate, tip)
	if err != nil {
		return &ending{err: err}
	}
	mergeCommit := ""
	if !merge.AlreadyUpToDate {
		if mergeCommit, err = fz.deps.TaskHead(); err != nil {
			return &ending{err: fmt.Errorf("landingshed: %s: read the merge commit: %w", finalizeName, err)}
		}
	}
	if fz.gate.recorder != nil {
		fz.gate.recorder.mergeCommit = mergeCommit
	}
	record := MergeRecord{
		ParentBranch: fz.deps.ParentBranch,
		MergedTip:    newTip,
		AlignedTip:   before.earlier.AlignedTip,
		TaskBase:     before.taskBase,
		TaskHead:     before.taskHead,
		MergeCommit:  mergeCommit,
		UpToDate:     merge.AlreadyUpToDate,
		Overlap:      overlap,
	}
	if err := WriteMergeRecord(fz.deps.MergeRecordPath, record); err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: %w", finalizeName, err)}
	}
	return nil
}

// keepMergeInBase writes head, the task head before a merge-in, to the base file.
// A file that already stands is kept, since an earlier pass wrote it before the merge this one resumes.
func (fz *Finalize) keepMergeInBase(head string) error {
	if fz.deps.MergeInBasePath == "" || fz.deps.RemoveLeftovers == nil {
		return nil
	}
	_, err := os.Stat(fz.deps.MergeInBasePath)
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("landingshed: %s: read the merge-in base file: %w", finalizeName, err)
	}
	if err := writeBytesAtomic(fz.deps.MergeInBasePath, "merge-in-base", []byte(head+"\n")); err != nil {
		return fmt.Errorf("landingshed: %s: %w", finalizeName, err)
	}
	return nil
}

// removeLeftovers removes the ignored files left in the directories deleted since the task head the base file holds.
// It then removes the base file.
// It does nothing while no base file stands.
// It returns nil when the caller goes on to its clean check.
func (fz *Finalize) removeLeftovers(ctx context.Context) *ending {
	if fz.deps.MergeInBasePath == "" || fz.deps.RemoveLeftovers == nil {
		return nil
	}
	base, err := os.ReadFile(fz.deps.MergeInBasePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the merge-in base file: %w", finalizeName, err))
	}
	removed, err := fz.deps.RemoveLeftovers(strings.TrimSpace(string(base)))
	for _, file := range removed {
		logger.Info("landingshed: removed an ignored leftover of a directory the merge-in deleted", "producer", finalizeName, "path", file)
	}
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: remove the leftovers of the merge-in: %w", finalizeName, err))
	}
	if err := os.Remove(fz.deps.MergeInBasePath); err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: remove the merge-in base file: %w", finalizeName, err))
	}
	return nil
}

// judgeParentMerge takes the parent tip merged: tip when it is set, and the local parent branch's tip otherwise.
// It reports the overlap between the paths the parent changed and the paths the task changed.
// The parent's changes are measured from the earlier merge record's merged tip, or the task's merge base without one.
// An up-to-date merge, a parent already holding the task's work, and an unchanged parent tip all report no overlap.
// A missing path read or merge-base read also reports no overlap.
func (fz *Finalize) judgeParentMerge(before mergeBefore, upToDate bool, tip string) (string, Overlap, error) {
	newTip := tip
	if newTip == "" {
		tips, err := fz.deps.ReadParentTips(false)
		if err != nil {
			return "", Overlap{}, fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err)
		}
		newTip = tips.Local
	}
	parentFrom := before.earlier.MergedTip
	if parentFrom == "" {
		parentFrom = before.taskBase
	}
	if upToDate || fz.deps.ChangedPaths == nil || before.taskBase == "" || parentFrom == "" || parentFrom == newTip {
		return newTip, Overlap{}, nil
	}
	held, err := fz.parentHoldsWork()
	if err != nil {
		return "", Overlap{}, fmt.Errorf("landingshed: %s: read whether the parent branch already holds the task's work: %w", finalizeName, err)
	}
	if held {
		return newTip, Overlap{}, nil
	}
	parentPaths, err := fz.deps.ChangedPaths(parentFrom, newTip)
	if err != nil {
		return "", Overlap{}, fmt.Errorf("landingshed: %s: list the paths the parent changed: %w", finalizeName, err)
	}
	taskPaths, err := fz.deps.ChangedPaths(before.taskBase, before.taskHead)
	if err != nil {
		return "", Overlap{}, fmt.Errorf("landingshed: %s: list the paths the task changed: %w", finalizeName, err)
	}
	return newTip, computeOverlap(parentPaths, taskPaths), nil
}

// commitParentRecords commits the parent pair's run records, and is a no-op when no commit function is wired.
// It returns nil when the records are committed;
// a failed commit ends the call with a Stuck verdict naming the parent pair's run records.
func (fz *Finalize) commitParentRecords(ctx context.Context) *ending {
	if fz.deps.CommitParentRecords == nil {
		return nil
	}
	err := fz.deps.CommitParentRecords()
	if err == nil {
		return nil
	}
	reason := fmt.Sprintf("the parent pair's run records could not be committed: %v; way forward: run \"lym fabric commit\" in the parent pair, then \"lym loom resume\"", err)
	return stuck(ctx, finalizeName, reason, "error", err)
}
