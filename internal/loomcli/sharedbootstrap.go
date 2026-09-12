// sharedbootstrap.go holds the blocks `run` and `step` share.
// Every helper here is lock-agnostic: none acquires or releases loomengine.LoomBootstrapLock,
// because the lock's position differs between the two calling verbs -- `run` acquires it later than
// its own seed/verify/commit block and holds it across the strand work, the driver spawn, and the
// run-lock handshake, while `step` spawns no driver and runs no handshake, so it wraps the strand
// block alone and releases before calling the producer. Each verb wraps its own lock window around
// these calls; no helper here may assume either window.

package loomcli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shell"
)

// bootstrapStage names the sub-step of seedAndCommitBootstrap a failure occurred at, so a caller can
// classify the failure without re-running the sub-steps: `run` deliberately ignores it and keeps
// writing its envelope exactly as before this extraction, while `step` maps it onto its own refusal-
// kind vocabulary.
type bootstrapStage int

const (
	// bootstrapStageNone is the zero value: no failure occurred, or the caller has not yet inspected
	// the returned stage.
	bootstrapStageNone bootstrapStage = iota
	// bootstrapStageOrigin means the failure occurred while reading or writing the origin record, or
	// while resolving the parent branch from it.
	bootstrapStageOrigin
	// bootstrapStageSeed means the failure occurred while seeding the status file, for a reason other
	// than the file already existing.
	bootstrapStageSeed
	// bootstrapStageOwnership means the failure occurred while verifying the seeded status file
	// belongs to this task's own slug.
	bootstrapStageOwnership
	// bootstrapStageCommit means the failure occurred while committing the seed and the origin
	// record into the fabric.
	bootstrapStageCommit
)

// seedAndCommitBootstrap resolves the parent branch, seeds the status file when absent, verifies
// seed ownership, and commits the seed and the origin record into the fabric -- today's steps 1
// through 3 from run.go's RunE, held here verbatim and in today's order so `step` can call the exact
// same sequence.
//
// On success it returns the resolved parent branch, bootstrapStageNone, and a nil error. On failure
// it returns the empty string, the stage that failed, and the error unwrapped.
func (c *loomCLI) seedAndCommitBootstrap(slug, parentFlag string) (string, bootstrapStage, error) {
	// Step 1: resolve the recorded parent branch, writing the provenance record only for a legacy
	// worktree created before it existed.
	recorded, found, err := fabricengine.ReadOrigin(c.location)
	if err != nil {
		return "", bootstrapStageOrigin, err
	}
	parent, writeOrigin, err := resolveParentBranch(recorded, found, parentFlag)
	if err != nil {
		return "", bootstrapStageOrigin, err
	}
	if writeOrigin {
		// loom's envelope deliberately gains no mutation keys here: the Mutation Record Invariant
		// binds fabric verb outcomes, and loom's own result is not one, so the recorder is thrown
		// away rather than surfaced.
		originRec := fabricengine.NewMutations("")
		if err := fabricengine.WriteOrigin(originRec, c.location, slug, fabricengine.Origin{ParentBranch: parent}); err != nil {
			return "", bootstrapStageOrigin, err
		}
	}

	// Step 2: seed the status file, tolerating exactly the already-seeded sentinel so a re-run works.
	// A stat-then-seed probe here would reintroduce the exact race the seeder's single lock exists to
	// close, so ErrSeedExists is the only accepted outcome.
	if err := loomshed.Seed(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug, parent); err != nil && !errors.Is(err, loomshed.ErrSeedExists) {
		return "", bootstrapStageSeed, err
	}
	// An already-present status file must be THIS task's own: `lyx fabric add` run from a task
	// worktree forks the whole pair, `_lyx` task state included, and the driver would otherwise
	// silently resume the inherited task's run under the wrong slug (crucible round fable5-high-r3,
	// F-B7).
	if err := loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug); err != nil {
		return "", bootstrapStageOwnership, err
	}

	// Step 3: commit the seed and the provenance record into the fabric, unconditionally on every
	// invocation -- not gated on this invocation's own writeOrigin. The origin record's path is
	// included every time for the same reason the status file's path always is: if a prior
	// invocation wrote the record to disk (step 1) but crashed before this step committed it,
	// resolveParentBranch's next read finds the record present with a matching value and reports
	// write == false, even though the record is still untracked in the fabric. Including the path
	// unconditionally makes that state self-heal on the very next call, exactly as the status file
	// already does -- and costs nothing on the ordinary path, since committing an already-clean,
	// already-tracked path is a no-op (StageAndCommit reports committed == false).
	// This must precede any producer call: the phase machine's very first precondition row scans the
	// fabric including untracked files, and neither file is on the never-tracked exclude list, so an
	// uncommitted seed or record would fail that check immediately.
	commitPaths := []string{loomengine.LoomStatusRel(), fabricengine.OriginRecordRel()}
	commitRec := fabricengine.NewMutations("")
	commitMsg := fmt.Sprintf("loom: seed session bootstrap for %s", slug)
	if _, _, err := fabricengine.CommitAnchoredPaths(commitRec, c.location, commitPaths, commitMsg, fabricengine.EnvSyncOptions()); err != nil {
		return "", bootstrapStageCommit, err
	}

	return parent, bootstrapStageNone, nil
}

// ensureStatusStrand ensures the worktree's tmux session is up and its status strand exists --
// today's step-4 strand work from run.go's RunE, held here verbatim and in today's order, starting
// after the bootstrap-lock acquisition and ending before the run-lock probe.
//
// It returns the first error encountered, unwrapped, and nil on success. It must not acquire or
// release the bootstrap lock and must not call bootstrapLock.Release() -- the caller owns that
// entirely, since the lock's position differs between the two calling verbs (see this file's own
// header comment).
func (c *loomCLI) ensureStatusStrand() error {
	if _, err := c.reed.Up(); err != nil {
		return err
	}
	statusResult, err := c.reed.Status()
	if err != nil {
		return err
	}
	strandAction, staleGUID := resolveStatusStrandAction(statusResult.Strands)
	if strandAction == statusStrandReplace {
		// A tracked-but-dead entry must be removed before adding, because reed's add has no upsert
		// semantics and would otherwise leave two strands under one display name. A removal failure
		// is not fatal to the bootstrap: it costs the operator the status pane for this run, not the
		// run itself.
		if _, err := c.reed.RemoveStrand(staleGUID, false); err != nil {
			logger.Warn("loom: could not remove a dead status strand; the status pane will be missing this run", "guid", staleGUID, "cause", err)
			strandAction = statusStrandKeep
		} else {
			strandAction = statusStrandAdd
		}
	}
	if strandAction == statusStrandAdd {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		addSpec := reedengine.AddSpec{
			NameOverride: statusStrandDisplayName,
			Cmd:          statusStrandCmd(shell.ForGOOS(), exe),
			Display: render.Display{
				Anchor:                   render.AnchorBelowParent,
				ShrinkWhenWaitingOnChild: true,
			},
		}
		if _, err := c.reed.AddStrand(addSpec); err != nil {
			return err
		}
	}
	return nil
}

// buildLoomShed builds the *shedengine.Shed drive.go's RunE runs -- today's shed-construction
// block from drive.go, held here verbatim and in today's order: open the fabric, read the current
// branch and origin URL, resolve the recorded origin and the landing parent, assemble Env.Landing,
// and construct the shed via loomrecipe.New.
//
// It returns the constructed shed and a nil error, or a nil shed and the first error encountered,
// unwrapped.
func (c *loomCLI) buildLoomShed() (*shedengine.Shed, error) {
	handle, err := fabricengine.Open(c.location)
	if err != nil {
		return nil, err
	}
	taskBranch, err := handle.CurrentBranch()
	if err != nil {
		return nil, err
	}
	originURL, err := handle.OriginURL()
	if err != nil {
		// scalar-read-errors-refuse-or-defer-by-consumer: only Publish reads OriginURL, and only
		// when a pull request is actually required, so an unusable origin URL passes through as an
		// empty string rather than refusing this build itself.
		originURL = ""
	}
	recorded, found, err := fabricengine.ReadOrigin(c.location)
	if err != nil {
		return nil, err
	}
	parentBranch, err := resolveLandingParent(recorded, found, taskBranch)
	if err != nil {
		return nil, err
	}
	syncOpts := fabricengine.EnvSyncOptions()
	pushBranch := func() error {
		_, err := handle.PushBranch(syncOpts)
		return err
	}
	c.env.Landing = landingDeps(
		c.location,
		c.runDeps.Geom,
		taskBranch,
		originURL,
		parentBranch,
		syncOpts.SkipPush,
		pushBranch,
		c.registry,
		c.runner,
		c.landingCfg,
	)

	return loomrecipe.New(c.env, c.shedPaths)
}
