// sharedbootstrap.go holds the blocks `start` and `step` share.
// Every helper here is lock-agnostic: none acquires or releases loomengine.LoomBootstrapLock, because the lock's position differs between the two calling verbs.
// `start` acquires it later than its own seed/verify/commit block and holds it across the strand work, the driver spawn, and the run-lock handshake,
// while `step` spawns no driver and runs no handshake,
// so it wraps reed Up alone and releases before calling the producer.
// ensureStatusStrand is `start`'s alone: `step` touches no strand.
// Each verb wraps its own lock window around these calls;
// no helper here may assume either window.

package loomcli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/statuspane"
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
// On success it returns the resolved parent branch, the run's effective driver, bootstrapStageNone,
// and a nil error. On failure it returns two empty strings, the stage that failed, and the error
// unwrapped.
//
// The effective driver is read from any already-recorded seed, never taken from a flag, since this command declares no driver flag of its own.
// It falls back to the driver_kind the loaded config resolves to (llm by default) only when no seed exists yet;
// an operator who wants a driver on one run pre-seeds it with "lyx shed seed self --recipe loom --driver go".
// Without this read-through, a run already seeded for the llm driver would be refused here, because WriteSeed rejects a disagreeing existing seed.
func (c *loomCLI) seedAndCommitBootstrap(slug, parentFlag string) (string, string, bootstrapStage, error) {
	// Step 1: resolve the recorded parent branch, writing the provenance record only for a legacy
	// worktree created before it existed.
	recorded, found, err := fabricengine.ReadOrigin(c.location)
	if err != nil {
		return "", "", bootstrapStageOrigin, err
	}
	parent, writeOrigin, err := resolveParentBranch(recorded, found, parentFlag)
	if err != nil {
		return "", "", bootstrapStageOrigin, err
	}
	if writeOrigin {
		// loom's envelope deliberately gains no mutation keys here: the Mutation Record Invariant
		// binds fabric verb outcomes, and loom's own result is not one, so the recorder is thrown
		// away rather than surfaced.
		originRec := fabricengine.NewMutations("")
		if err := fabricengine.WriteOrigin(originRec, c.location, slug, fabricengine.Origin{ParentBranch: parent}); err != nil {
			return "", "", bootstrapStageOrigin, err
		}
	}

	// Step 1b: write loom's own seed, the run's identity, before the status file it seeds next --
	// the status file must never exist without a seed beside it, which is the inconsistency card
	// 14's refusal exists to catch. The existing seed, if any, is read first so its recorded driver
	// survives this write unchanged -- see resolveSeedDriver. WriteSeed is already idempotent
	// against a byte-identical seed, so a re-run needs no sentinel handling of its own; a disagreeing
	// seed's refusal propagates as a returned error here, at bootstrapStageSeed. params.parent is the
	// run's recorded startup choice, not the durable truth -- fabricengine.Origin.ParentBranch
	// (written just above) stays the durable record, and resolveParentBranch's own disagreement
	// refusal already guards it.
	existingSeed, seedFound, err := shedrun.ReadSeed(c.location, shedrun.SelfRunID)
	if err != nil {
		return "", "", bootstrapStageSeed, err
	}
	seedRecipe := resolveSeedRecipe(existingSeed, seedFound)
	configDirs := fabricengine.ConfigDirs(c.location)
	shedConfig, err := sheddriver.LoadConfig(configDirs)
	if err != nil {
		return "", "", bootstrapStageSeed, err
	}
	recipeOverride, err := loomengine.RecipeOverride(configDirs, seedRecipe)
	if err != nil {
		return "", "", bootstrapStageSeed, err
	}
	driver := resolveSeedDriver(existingSeed, seedFound, sheddriver.ResolveDriverKind(shedConfig, recipeOverride))
	if err := shedrun.WriteSeed(c.location, shedrun.SelfRunID, loomSeedFor(seedRecipe, parent, driver)); err != nil {
		if errors.Is(err, shedrun.ErrDisagreeingSeed) {
			err = disagreeingSeedRefusal(shedrun.SeedFile(c.location, shedrun.SelfRunID), existingSeed, parent)
		}
		return "", "", bootstrapStageSeed, err
	}

	// Step 2: seed the status file, tolerating exactly the already-seeded sentinel so a re-run works.
	// A stat-then-seed probe here would reintroduce the exact race the seeder's single lock exists to
	// close, so ErrSeedExists is the only accepted outcome.
	// seedErr is kept rather than discarded because step 2b below needs to tell a genuine first seed
	// from an ErrSeedExists re-entry, and this is the only place that distinction is observable.
	seedErr := loomshed.Seed(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug, parent)
	if seedErr != nil && !errors.Is(seedErr, loomshed.ErrSeedExists) {
		return "", "", bootstrapStageSeed, seedErr
	}
	// An already-present status file must be THIS task's own: `lyx fabric add` run from a task
	// worktree forks the whole pair, `_lyx` task state included, and the driver would otherwise
	// silently resume the inherited task's run under the wrong slug (crucible round fable5-high-r3,
	// F-B7).
	if err := loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug); err != nil {
		return "", "", bootstrapStageOwnership, err
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
	commitPaths := bootstrapCommitPaths(c.location)
	commitRec := fabricengine.NewMutations("")
	commitMsg := fmt.Sprintf("loom: seed session bootstrap for %s", slug)
	if _, _, err := fabricengine.CommitAnchoredPaths(commitRec, c.location, commitPaths, commitMsg, fabricengine.EnvSyncOptions()); err != nil {
		return "", "", bootstrapStageCommit, err
	}

	return parent, driver, bootstrapStageNone, nil
}

// disagreeingSeedRefusal renders the refusal for an existing seed at seedPath whose recipe, driver or params differ from the seed the bootstrap writes for the recorded parent.
// It wraps shedrun.ErrDisagreeingSeed and ends in a way forward that clears the one file and re-seeds with the existing recipe and driver and the recorded parent, so following it cannot loop on the same refusal.
func disagreeingSeedRefusal(seedPath string, existing shedrun.Seed, parent string) error {
	return fmt.Errorf("%w: loom: the seed at %s records recipe %q, driver %q and params %v, which disagree with the recorded parent branch %q; way forward: rm %s, then lyx shed seed self --recipe %s --driver %s --param parent=%s",
		shedrun.ErrDisagreeingSeed, seedPath, existing.Recipe, existing.Driver, existing.Params, parent, seedPath, existing.Recipe, existing.Driver, parent)
}

// resolveSeedDriver reads the effective driver seedAndCommitBootstrap's step 1b must write.
// It is the already-recorded seed's own Driver when one exists, and kind -- the driver_kind the loaded config resolves to, llm by default -- only when no seed has been written yet.
// It is a pure function, factored out so a Tier 1 test can pin its cases with no real fabric behind it.
//
// A recorded driver always wins over kind: editing the config after seeding has no effect on that run.
func resolveSeedDriver(existing shedrun.Seed, found bool, kind string) string {
	if !found {
		return kind
	}
	return existing.Driver
}

// resolveSeedRecipe reads the recipe seedAndCommitBootstrap's step 1b must write: the already-recorded seed's own Recipe when one exists, shedrun.RecipeLoom otherwise.
// It is the read, not a choice, so `lyx loom start` never writes a recipe over an existing seed;
// the recipe of a run is set only by the seeding that created the seed.
func resolveSeedRecipe(existing shedrun.Seed, found bool) string {
	if !found {
		return shedrun.RecipeLoom
	}
	return existing.Recipe
}

// loomSeedFor builds the shedrun.Seed seedAndCommitBootstrap's step 1b writes.
// The recipe and the driver to record are told by the caller and never chosen here, see resolveSeedRecipe and resolveSeedDriver.
// The run's recorded startup choice of parent branch is the seed's sole param.
// It is a pure function, factored out of seedAndCommitBootstrap so its shape is directly testable
// without driving the whole bootstrap sequence -- WriteSeed itself needs no real fabric, but
// seedAndCommitBootstrap's own step 1 (fabricengine.ReadOrigin) does, which would otherwise put
// this value's shape out of a Tier 1 test's reach.
//
// parent is the parent branch; the seed records no parent agent, which is resolved from fabric's origin record at use.
func loomSeedFor(recipe, parent, driver string) shedrun.Seed {
	return shedrun.Seed{
		Recipe: recipe,
		Driver: driver,
		Params: map[string]string{"parent": parent},
	}
}

// bootstrapCommitPaths returns the three anchor-relative paths seedAndCommitBootstrap's step 3
// commits unconditionally: the status file, the seed, and the origin record. It is a pure
// function, factored out for the same reason loomSeedFor is -- so a Tier 1 test can pin the
// pathspec's exact shape without a real fabric behind it.
func bootstrapCommitPaths(location *lyxcwd.Location) []string {
	return []string{shedrun.StatusRel(location, shedrun.SelfRunID), shedrun.SeedRel(location, shedrun.SelfRunID), fabricengine.OriginRecordRel()}
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

// ensureStatusStrand is `start`'s status-strand work: it keeps, replaces or adds the worktree's status strand.
// The caller has already brought reed up,
// and only `start` calls it.
// It runs after the bootstrap-lock acquisition and ends before the run-lock probe.
//
// It returns the first error encountered, unwrapped, and nil on success. It must not acquire or
// release the bootstrap lock and must not call bootstrapLock.Release() -- the caller owns that
// entirely, since the lock's position differs between the two calling verbs (see this file's own
// header comment).
func (c *loomCLI) ensureStatusStrand() error {
	statusResult, err := c.reed.Status()
	if err != nil {
		return err
	}
	build, buildErr := statuspane.CurrentBuild()
	if buildErr != nil {
		// Without an identity nothing can be proven current, so a live strand is replaced.
		logger.Warn("loom: could not identify the running lyx build; a live status strand will be replaced", "cause", buildErr)
	}
	sidecarPath := statusSidecarPath(c.location)
	strandAction, staleGUID := resolveStatusStrandAction(statusResult.Strands, statuspane.ReadSidecar(sidecarPath), build)
	if strandAction == statusStrandKeep {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	addSpec := statusStrandAddSpec(statusStrandCmd(shell.ForGOOS(), exe))
	var strand reedengine.Strand
	if strandAction == statusStrandReplace {
		// A replace goes through ReplaceStrand so the status strand keeps its top slot. A failure is
		// not fatal to the bootstrap: it costs the operator the status pane for this run, not the
		// run itself.
		strand, err = c.reed.ReplaceStrand(staleGUID, addSpec)
		if err != nil {
			logger.Warn("loom: could not replace the status strand; the status pane will be missing this run", "guid", staleGUID, "cause", err)
			return nil
		}
	} else {
		strand, err = c.reed.AddStrand(addSpec)
		if err != nil {
			return err
		}
	}
	if buildErr == nil {
		if err := statuspane.WriteSidecar(sidecarPath, strand.GUID, build); err != nil {
			logger.Warn("loom: could not record the status strand's build; the next start will replace it", "cause", err)
		}
	}
	return nil
}

// wireLanding assembles Env.Landing: it opens the fabric, reads the landing branches and the origin URL, builds the push closure and tells landingDeps the hub's language skills.
// An origin URL that cannot be read passes through as empty.
func (c *loomCLI) wireLanding() error {
	handle, err := fabricengine.Open(c.location)
	if err != nil {
		return err
	}
	taskBranch, parentBranch, err := landingBranches(handle, c.location)
	if err != nil {
		return err
	}
	originURL, err := handle.OriginURL()
	if err != nil {
		// scalar-read-errors-refuse-or-defer-by-consumer: only Publish reads OriginURL, and only
		// when a pull request is actually required, so an unusable origin URL passes through as an
		// empty string rather than refusing this build itself.
		originURL = ""
	}
	runStart, err := seedRunStart(c.location)
	if err != nil {
		return fmt.Errorf("%w; way forward: rm %s, then lyx shed seed self --recipe loom --driver go --param parent=%s; a re-seed made after Publish opened the pull request moves the run's start past it, so PR-Gate, PR-Align and Finalize find no pull request and Publish stops on it as an older open one: restore the original started_at in the seed, or close that pull request and resume with lyx loom start", err, shedrun.SeedFile(c.location, shedrun.SelfRunID), parentBranch)
	}
	syncOpts := fabricengine.EnvSyncOptions()
	pushBranch := func() error {
		_, err := handle.PushBranch(syncOpts)
		return err
	}
	c.env.Landing = landingDeps(
		c.location,
		c.recipe,
		c.runDeps.Geom,
		taskBranch,
		originURL,
		parentBranch,
		syncOpts.SkipPush,
		pushBranch,
		c.registry,
		c.runner,
		c.landingCfg,
		c.parentName,
		c.languageSkills,
		driverWaitMark(c.reed.Status, c.reed.SetWaitMark),
		conflictSessionStopper(c.reed.Status, c.runner.StopStrand),
		c.verifySource(),
		parentLockDeps(c.location, c.runID, c.shedPaths.StatusPath, c.shedPaths.StatusLockPath),
		runStart,
	)
	return nil
}

// buildLoomShed builds the *shedengine.Shed run.go's RunE runs.
// It assembles Env.Landing through wireLanding, then constructs the shed via loomrecipe.New.
//
// It returns the constructed shed and a nil error, or a nil shed and the first error encountered,
// unwrapped.
func (c *loomCLI) buildLoomShed() (*shedengine.Shed, error) {
	if err := c.wireLanding(); err != nil {
		return nil, err
	}

	// A loop's step child adopts its trace id from LYX_TRACE_ID, so this tie names the step the loop runs and the watchdog can stop that step's runs.
	c.runner.SetStepTie(shuttleengine.StepTie{StepsDir: shedrun.StepsDir(c.location, c.runID), TraceID: logger.TraceID()})
	return c.newShed()
}
