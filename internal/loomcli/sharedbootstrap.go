// sharedbootstrap.go holds the blocks `start` and `step` share.
// Every helper here is lock-agnostic: none acquires or releases loomengine.LoomBootstrapLock,
// because the lock's position differs between the two calling verbs -- `start` acquires it later than
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
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shell"
)

// bootstrapStage names the sub-step of seedAndCommitBootstrap a failure occurred at, so a caller can
// classify the failure without re-running the sub-steps: `start` deliberately ignores it and keeps
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
	// bootstrapStageSeed means the failure occurred while writing loom's own shedrun.Seed, or while
	// seeding the status file, for a reason other than either already existing with an agreeing
	// value.
	bootstrapStageSeed
	// bootstrapStageOwnership means the failure occurred while verifying the seeded status file
	// belongs to this task's own slug.
	bootstrapStageOwnership
	// bootstrapStageCommit means the failure occurred while committing the seed and the origin
	// record into the fabric.
	bootstrapStageCommit
)

// seedAndCommitBootstrap resolves the parent branch, writes loom's own shedrun.Seed, seeds the
// status file when absent, verifies seed ownership, and commits the seed, the status file, and the
// origin record into the fabric -- today's steps 1 through 3 from start.go's RunE, held here
// verbatim and in today's order so `step` can call the exact same sequence.
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

	// Step 1b: write loom's own seed, the run's identity, before the status file it seeds next --
	// the status file must never exist without a seed beside it, which is the inconsistency card
	// 14's refusal exists to catch. WriteSeed is already idempotent against a byte-identical seed,
	// so a re-run needs no sentinel handling of its own; a disagreeing seed's refusal propagates as
	// a returned error here, at bootstrapStageSeed. params.parent is the run's recorded startup
	// choice, not the durable truth -- fabricengine.Origin.ParentBranch (written just above) stays
	// the durable record, and resolveParentBranch's own disagreement refusal already guards it.
	if err := shedrun.WriteSeed(c.location, shedrun.SelfRunID, loomSeedFor(parent)); err != nil {
		return "", bootstrapStageSeed, err
	}

	// Step 2: seed the status file, tolerating exactly the already-seeded sentinel so a re-run works.
	// A stat-then-seed probe here would reintroduce the exact race the seeder's single lock exists to
	// close, so ErrSeedExists is the only accepted outcome.
	// seedErr is kept rather than discarded because step 2b below needs to tell a genuine first seed
	// from an ErrSeedExists re-entry, and this is the only place that distinction is observable.
	seedErr := loomshed.Seed(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug, parent)
	if seedErr != nil && !errors.Is(seedErr, loomshed.ErrSeedExists) {
		return "", bootstrapStageSeed, seedErr
	}
	// An already-present status file must be THIS task's own: `lyx fabric add` run from a task
	// worktree forks the whole pair, `_lyx` task state included, and the driver would otherwise
	// silently resume the inherited task's run under the wrong slug (crucible round fable5-high-r3,
	// F-B7).
	if err := loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug); err != nil {
		return "", bootstrapStageOwnership, err
	}

	// Step 2b: Tier 2's once-per-task friction directory handling, positioned after the ownership
	// check on purpose -- the first-seed branch DELETES the directory, and a status file belonging to
	// another task must never have its friction notes cleared by this worktree.
	// Both calling verbs reach this, which is the point: `start` spawns `run`, which ensures the
	// directory itself, but `step` spawns no driver at all and would otherwise leave every
	// step-driven run composing note paths into a directory nothing had created.
	ensureFrictionDirAfterSeed(c.frictionDir, seedErr)

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
	// fabric including untracked files, and none of the three is on the never-tracked exclude list,
	// so an uncommitted seed, status file, or record would fail that check immediately. The
	// shedrun.SeedRel entry is included unconditionally for the same self-healing reason the origin
	// record's own comment already gives: WriteSeed above is idempotent, so a prior invocation that
	// wrote the seed to disk but crashed before this step committed it self-heals on the very next
	// call.
	commitPaths := bootstrapCommitPaths()
	commitRec := fabricengine.NewMutations("")
	commitMsg := fmt.Sprintf("loom: seed session bootstrap for %s", slug)
	if _, _, err := fabricengine.CommitAnchoredPaths(commitRec, c.location, commitPaths, commitMsg, fabricengine.EnvSyncOptions()); err != nil {
		return "", bootstrapStageCommit, err
	}

	return parent, bootstrapStageNone, nil
}

// loomSeedFor builds the shedrun.Seed seedAndCommitBootstrap's step 1b writes: loom's fixed
// recipe/driver pair, and the run's recorded startup choice of parent branch as its sole param.
// It is a pure function, factored out of seedAndCommitBootstrap so its shape is directly testable
// without driving the whole bootstrap sequence -- WriteSeed itself needs no real fabric, but
// seedAndCommitBootstrap's own step 1 (fabricengine.ReadOrigin) does, which would otherwise put
// this value's shape out of a Tier 1 test's reach.
func loomSeedFor(parent string) shedrun.Seed {
	return shedrun.Seed{
		Recipe: shedrun.RecipeLoom,
		Driver: shedrun.DriverGo,
		Params: map[string]string{"parent": parent},
	}
}

// bootstrapCommitPaths returns the three anchor-relative paths seedAndCommitBootstrap's step 3
// commits unconditionally: the status file, the seed, and the origin record. It is a pure
// function, factored out for the same reason loomSeedFor is -- so a Tier 1 test can pin the
// pathspec's exact shape without a real fabric behind it.
func bootstrapCommitPaths() []string {
	return []string{shedrun.StatusRel(shedrun.SelfRunID), shedrun.SeedRel(shedrun.SelfRunID), fabricengine.OriginRecordRel()}
}

// ensureFrictionDirAfterSeed performs the once-per-task clear-and-create split immediately after
// loomshed.Seed: on a genuine first seed (seedErr is nil), the friction directory is cleared before
// being recreated, since a fresh task has no notes worth preserving; on an ErrSeedExists re-entry
// (any other seedErr value), the directory is left untouched and only ensured to exist, since a
// resume's notes are exactly the ones most worth reading. Both operations are skipped entirely when
// frictionDir is empty, which is how Tier 2's off state travels. A failed os.RemoveAll logs at Warn
// and never fails this call; a failed friction.EnsureDir is handled entirely inside that function,
// which never returns an error either.
//
// It lives here, beside its one caller, rather than in start.go where it was first written: it was
// never called from there at all. The clear-on-first-seed behaviour this function documents did not
// ship, and its four unit tests were green over an orphan -- so a worktree whose friction directory
// still held notes from an earlier task, or from an earlier run that never reached a reflection
// trigger, fed every one of them to the NEXT task's reflection agent as that task's own friction.
// Reproduced live in crucible round 1 against a real hub.
func ensureFrictionDirAfterSeed(frictionDir string, seedErr error) {
	if frictionDir == "" {
		return
	}
	if seedErr == nil {
		if err := os.RemoveAll(frictionDir); err != nil {
			logger.Warn("loom: failed to clear the friction directory on first seed; continuing", "dir", frictionDir, "error", err)
		}
	}
	friction.EnsureDir(frictionDir)
}

// ensureStatusStrand ensures the worktree's tmux session is up and its status strand exists --
// today's step-4 strand work from start.go's RunE, held here verbatim and in today's order, starting
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

// buildLoomShed builds the *shedengine.Shed run.go's RunE runs -- today's shed-construction
// block from run.go, held here verbatim and in today's order: open the fabric, read the current
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
