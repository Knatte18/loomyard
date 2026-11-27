// sharedbootstrap.go holds the bootstrap steps that `start` and `step` share.
// Every helper here is lock-agnostic: none acquires or releases loomengine.LoomBootstrapLock.
// Each verb wraps its own lock window around these calls, so no helper may assume either window.
// `start` holds the lock across the strand work, the driver spawn, and the run-lock handshake.
// `step` holds it around reed Up alone and releases it before it calls the producer.
// ensureStatusStrand is `start`'s alone, because `step` touches no strand.

package loomcli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

// bootstrapStage names the sub-step of seedAndCommitBootstrap where a failure occurred.
// A caller classifies the failure from it without re-running the sub-steps.
// `start` ignores the stage, and `step` maps it onto its own refusal-kind vocabulary.
type bootstrapStage int

const (
	// bootstrapStageNone is the zero value, meaning no failure occurred.
	// A caller that has not yet inspected the returned stage also sees this value.
	bootstrapStageNone bootstrapStage = iota
	// bootstrapStageOrigin means the failure occurred on the origin record:
	// reading or writing it, or resolving the parent branch from it.
	bootstrapStageOrigin
	// bootstrapStageSeed means the failure occurred while writing loom's shedrun.Seed or while seeding the status file.
	// An existing seed or status file that already agrees with this run is not a failure.
	// A seed that disagrees with an existing one fails here, as does any other write error.
	bootstrapStageSeed
	// bootstrapStageOwnership means the failure occurred while verifying the seeded status file's owner.
	// The status file must belong to this task's own slug.
	bootstrapStageOwnership
	// bootstrapStageCommit means the failure occurred while committing into the fabric.
	// It covers the seed, the status file, the origin record, and the friction directory.
	bootstrapStageCommit
)

// seedAndCommitBootstrap resolves the parent branch and writes loom's own shedrun.Seed.
// It seeds the status file when absent and verifies that its seed belongs to this task.
// It then commits the seed, the status file, the origin record, and the friction directory into the fabric.
// The friction directory is committed only when it holds tracked or untracked content.
// Both `start` and `step` call it, so they run the same sequence of steps.
//
// On success it returns the resolved parent branch, the run's effective driver, bootstrapStageNone,
// and a nil error.
// On failure it returns two empty strings, the stage that failed, and the error unwrapped.
//
// The effective driver is read from any already-recorded seed.
// It is never taken from a flag, since the verbs declare no driver flag of their own.
// It falls back to the driver_kind the loaded config resolves to (llm by default) only when no seed exists yet.
// An operator who wants a driver on one run pre-seeds it with "lym shed seed self --recipe tapestry --driver go".
// Without this read-through, a run already seeded for the llm driver would be refused here.
// WriteSeed rejects a disagreeing existing seed.
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
		// The Mutation Record Invariant covers fabric verb outcomes, not loom's own result.
		// The recorder is therefore discarded rather than surfaced in the envelope.
		originRec := fabricengine.NewMutations("")
		if err := fabricengine.WriteOrigin(originRec, c.location, slug, fabricengine.Origin{ParentBranch: parent}); err != nil {
			return "", "", bootstrapStageOrigin, err
		}
	}

	// Step 1b: write loom's own seed, the run's identity, before the status file it seeds next.
	// The status file must never exist without a seed beside it.
	// The existing seed, if any, is read first so that its recorded driver survives this write unchanged.
	// WriteSeed is already idempotent against a byte-identical seed, so a re-run needs no sentinel handling of its own.
	// A disagreeing seed's refusal propagates as a returned error here, at bootstrapStageSeed.
	// The parent in this seed is the run's recorded startup choice, not the durable record.
	// fabricengine.Origin.ParentBranch, written above, stays the durable record.
	// resolveParentBranch refuses a disagreeing value.
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
	driver := resolveSeedDriver(existingSeed, seedFound, sheddriver.ResolveDriverKind(shedConfig, sheddriver.Override{}))
	if err := shedrun.WriteSeed(c.location, shedrun.SelfRunID, loomSeedFor(seedRecipe, parent, driver)); err != nil {
		if errors.Is(err, shedrun.ErrDisagreeingSeed) {
			err = disagreeingSeedRefusal(shedrun.SeedFile(c.location, shedrun.SelfRunID), existingSeed, parent)
		}
		return "", "", bootstrapStageSeed, err
	}

	// Step 2: seed the status file, tolerating exactly the already-seeded sentinel so a re-run works.
	// A stat-then-seed probe would reintroduce the race that the seeder's single lock closes.
	// ErrSeedExists is therefore the only accepted outcome.
	// seedErr is kept because step 2b needs to tell a first seed from an ErrSeedExists re-entry.
	// Only here is that distinction visible.
	seedErr := loomshed.Seed(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug, parent)
	if seedErr != nil && !errors.Is(seedErr, loomshed.ErrSeedExists) {
		return "", "", bootstrapStageSeed, seedErr
	}
	// An already-present status file must belong to this task.
	// `lym fabric add` run from a task worktree forks the whole pair, `_lym` task state included,
	// and the driver would otherwise silently resume the inherited task's run under the wrong slug.
	if err := loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, slug); err != nil {
		return "", "", bootstrapStageOwnership, err
	}

	// Step 2b handles the once-per-task friction directory.
	// It runs after the ownership check, because the first-seed branch deletes the directory,
	// and a status file that belongs to another task must never have its friction notes cleared by this worktree.
	// Both `start` and `step` reach this step.
	// `start` spawns `run`, which ensures the directory itself.
	// `step` spawns no driver, so without this step it would leave the directory uncreated for its runs.
	ensureFrictionDirAfterSeed(c.frictionDir, seedErr)

	// Step 2c writes the run's identity record for its cost report on the first seed only.
	// A re-entry keeps the record already there.
	wroteRunUsage := seedErr == nil && c.writeRunUsage(seedRecipe, slug)

	// Step 3: commit the seed and the provenance record into the fabric on every invocation.
	// The commit does not depend on whether this invocation wrote the origin record.
	// The origin record's path is always included.
	// A prior invocation may have written it to disk and crashed before committing it.
	// On the next call, resolveParentBranch finds the record present and reports write == false while it is untracked.
	// The status file and the seed are included on every call for the same reason, since WriteSeed is idempotent.
	// Committing a clean path is a no-op (StageAndCommit reports committed == false), so the ordinary path pays nothing.
	// The commit must precede any producer call.
	// The phase machine's first precondition row scans the fabric, including untracked files.
	// An uncommitted path fails that check.
	// None of these three paths is on the scan's never-tracked exclude list.
	// The friction directory is committed whenever it holds tracked or untracked content.
	// A task worktree can branch from a fabric that carries committed notes, such as a beam pawl run's halt note.
	// The first-seed clear in ensureFrictionDirAfterSeed deletes those notes;
	// left uncommitted, that deletion would fail the precondition scan.
	// Probing on every invocation self-heals a clear whose commit crashed.
	// A resume's notes stay on disk and are committed here too.
	// The probe is skipped under SkipGit, where the commit is a no-op and the fabric's git may be unusable.
	syncOptions := fabricengine.EnvSyncOptions()
	withFriction := false
	if c.frictionDir != "" && !syncOptions.SkipGit {
		withFriction, err = fabricengine.AnchoredPathHasContent(c.location, loomengine.LoomFrictionDirRel())
		if err != nil {
			return "", "", bootstrapStageCommit, err
		}
	}
	commitPaths := bootstrapCommitPaths(c.location, withFriction)
	if wroteRunUsage {
		// The record is committed with the seed so the run's first precondition scan finds no untracked file;
		// the status commits carry the usage directory after it.
		commitPaths = append(commitPaths, shedrun.UsageDirRel())
	}
	commitRec := fabricengine.NewMutations("")
	commitMsg := fmt.Sprintf("loom: seed session bootstrap for %s", slug)
	if _, _, err := fabricengine.CommitAnchoredPaths(commitRec, c.location, commitPaths, commitMsg, syncOptions); err != nil {
		return "", "", bootstrapStageCommit, err
	}

	return parent, driver, bootstrapStageNone, nil
}

// writeRunUsage writes the run's identity record into its usage directory and reports whether it wrote one.
// A failed read of the board entry or the run profiles is logged at Warn,
// and the record is written with what was read;
// a failed write is logged at Warn and reports false.
func (c *loomCLI) writeRunUsage(recipe, slug string) bool {
	var entry boardengine.Task
	found := false
	board, err := boardengine.OpenHub(c.location.HubPath)
	if err == nil {
		entry, found, err = board.GetTask(slug)
	}
	if err != nil {
		logger.Warn("loom: could not read the run's board entry for its usage record; recording it without the entry", "slug", slug, "error", err)
		entry, found = boardengine.Task{}, false
	}
	defaultProfile := ""
	profiles, err := runprofile.Load(fabricengine.ConfigDirs(c.location))
	if err == nil {
		defaultProfile, err = profiles.Default(recipe)
	}
	if err != nil {
		logger.Warn("loom: could not read the run profiles for its usage record; recording it without the default profile", "recipe", recipe, "error", err)
		defaultProfile = ""
	}
	if err := shedrun.WriteRunUsage(shedrun.UsageDir(c.location), runUsageRecord(recipe, entry, found, defaultProfile)); err != nil {
		logger.Warn("loom: could not write the run's usage record", "slug", slug, "error", err)
		return false
	}
	return true
}

// runUsageRecord builds the run's identity record for its cost report.
// The profile is the entry's own, else defaultProfile, the one the recipe takes when an entry names none.
// stored_record is true when the entry is flagged discussed, so the run starts from a stored record.
// An absent entry records an empty profile and no stored record.
func runUsageRecord(recipe string, entry boardengine.Task, found bool, defaultProfile string) shedrun.RunUsageRecord {
	record := shedrun.RunUsageRecord{Recipe: recipe}
	if !found {
		return record
	}
	record.Profile = entry.Profile
	if record.Profile == "" {
		record.Profile = defaultProfile
	}
	record.StoredRecord = entry.Discussed
	return record
}

// disagreeingSeedRefusal renders the refusal for an existing seed at seedPath.
// The seed's recipe, driver or params differ from what the bootstrap writes for the recorded parent.
// It wraps shedrun.ErrDisagreeingSeed.
// Its way forward clears the one file and re-seeds with the existing recipe, driver and recorded parent.
// Following it therefore cannot loop on the same refusal.
func disagreeingSeedRefusal(seedPath string, existing shedrun.Seed, parent string) error {
	return fmt.Errorf("%w: loom: the seed at %s records recipe %q, driver %q and params %v, which disagree with the recorded parent branch %q; way forward: rm %s, then lym shed seed self --recipe %s --driver %s --param parent=%s",
		shedrun.ErrDisagreeingSeed, seedPath, existing.Recipe, existing.Driver, existing.Params, parent, seedPath, existing.Recipe, existing.Driver, parent)
}

// resolveSeedDriver returns the driver a run records when it is seeded.
// It is the already-recorded seed's own Driver when one exists.
// It is kind, the driver_kind the loaded config resolves to (llm by default), only when no seed has been written yet.
// It is a pure function, so a Tier 1 test can pin its cases with no real fabric behind it.
//
// A recorded driver always wins over kind: editing the config after seeding has no effect on that run.
func resolveSeedDriver(existing shedrun.Seed, found bool, kind string) string {
	if !found {
		return kind
	}
	return existing.Driver
}

// resolveSeedRecipe returns the recipe a run records when it is seeded.
// It is the already-recorded seed's own Recipe when one exists, and shedrun.RecipeTapestry otherwise.
// It only reads the recipe, so an existing seed keeps the recipe it recorded when it was created.
func resolveSeedRecipe(existing shedrun.Seed, found bool) string {
	if !found {
		return shedrun.RecipeTapestry
	}
	return existing.Recipe
}

// loomSeedFor builds the shedrun.Seed that records a run's recipe, driver and parent branch.
// The caller passes the recipe and the driver to record; this function never chooses them.
// The parent branch is the seed's sole param.
// It is a pure function, so a Tier 1 test can pin the seed's shape without a real fabric behind it.
//
// parent is the parent branch; the seed records no parent agent, which is resolved from fabric's origin record at use.
func loomSeedFor(recipe, parent, driver string) shedrun.Seed {
	return shedrun.Seed{
		Recipe: recipe,
		Driver: driver,
		Params: map[string]string{"parent": parent},
	}
}

// bootstrapCommitPaths returns the anchor-relative paths the bootstrap commits:
// the status file, the seed and the origin record always,
// and the friction directory when withFriction is set,
// so a first seed's clear of notes the fabric already tracked is committed with the seed.
// Git refuses a commit pathspec that matches nothing;
// so withFriction must be set only when the directory matches tracked or untracked content.
// It is a pure function, so a Tier 1 test can pin the pathspec's exact shape without a real fabric behind it.
func bootstrapCommitPaths(location *lymcwd.Location, withFriction bool) []string {
	paths := []string{shedrun.StatusRel(location, shedrun.SelfRunID), shedrun.SeedRel(location, shedrun.SelfRunID), fabricengine.OriginRecordRel()}
	if withFriction {
		paths = append(paths, loomengine.LoomFrictionDirRel())
	}
	return paths
}

// ensureFrictionDirAfterSeed performs the once-per-task clear-and-create split right after loomshed.Seed.
// On a genuine first seed (seedErr is nil), the friction directory is cleared and then recreated.
// A fresh task has no notes worth preserving.
// On a loomshed.ErrSeedExists re-entry (any other seedErr value), the directory is only ensured to exist.
// A resume's notes are the ones most worth reading, and they stay on disk.
// The bootstrap's commit step still commits whatever the directory holds.
// Both operations are skipped when frictionDir is empty.
// A failed os.RemoveAll logs at Warn and never fails this call;
// a failed friction.EnsureDir is handled entirely inside that function, which never returns an error either.
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
// Only `start` calls it, after it has brought reed up.
// It runs after the bootstrap-lock acquisition and ends before the run-lock probe.
//
// It returns the first error encountered, unwrapped, and nil on success.
// The caller owns the bootstrap lock: this function must neither acquire nor release it.
// The lock's position differs between the verbs that run the bootstrap.
func (c *loomCLI) ensureStatusStrand() error {
	statusResult, err := c.reed.Status()
	if err != nil {
		return err
	}
	build, buildErr := statuspane.CurrentBuild()
	if buildErr != nil {
		// Without an identity nothing can be proven current, so a live strand is replaced.
		logger.Warn("loom: could not identify the running lym build; a live status strand will be replaced", "cause", buildErr)
	}
	sidecarPath := statusSidecarPath(c.location)
	strandAction, staleGUID := resolveStatusStrandAction(statusResult.Strands, statuspane.ReadSidecar(sidecarPath), build)
	if strandAction == statusStrandKeep {
		return nil
	}
	exe, err := selfexec.LymBinary()
	if err != nil {
		return err
	}
	addSpec := c.statusStrandSpec(exe)
	var strand reedengine.Strand
	if strandAction == statusStrandReplace {
		// A replace goes through ReplaceStrand so the status strand keeps its top slot.
		// A failure is not fatal to the bootstrap;
		// it costs the operator the status pane for this run, not the run itself.
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

// statusStrandSpec builds the spec for adding or replacing the status strand.
// The spec runs the checklist pane command of the lym binary at exe.
// Its window is titled with the worktree's console title, the recipe standing where the slug does.
func (c *loomCLI) statusStrandSpec(exe string) reedengine.AddSpec {
	return statusStrandAddSpec(statusStrandCmd(shell.ForGOOS(), exe), statusWindowTitle(c.location, c.recipe))
}

// statusWindowTitle returns the name of the status strand's window for a run of recipe in the worktree at location.
// It reads the hub's shortname and the board at the call.
// It gives the recipe the worktree's console title parts, such as `T4 [ly:P3] tapestry`.
// A part the hub or board does not give is left out, so the recipe stands alone when none is given.
// No prime name is passed, so the prime is never titled as the orch.
func statusWindowTitle(location *lymcwd.Location, recipe string) string {
	return boardengine.RecipeWindowTitle(location.HubPath, location.WorktreeName, recipe)
}

// wireLanding assembles Env.Landing.
// It opens the fabric, reads the landing branches, and passes the hub's language skills on.
// It then sets the fixer's seat runner to the seat engine wire built.
func (c *loomCLI) wireLanding() error {
	handle, err := fabricengine.Open(c.location)
	if err != nil {
		return err
	}
	taskBranch, parentBranch, err := landingBranches(handle, c.location)
	if err != nil {
		return err
	}
	c.env.Landing = landingDeps(
		c.location,
		c.recipe,
		c.runDeps.Geom,
		taskBranch,
		parentBranch,
		fabricengine.EnvSyncOptions().SkipPush,
		c.registry,
		c.runner,
		c.landingCfg,
		c.parentName,
		c.languageSkills,
		driverWaitMark(c.reed.Status, c.reed.SetWaitMark),
		conflictSessionStopper(c.reed.Status, c.runner.StopStrand),
		c.verifySource(),
		parentLockDeps(c.location, c.runID, c.shedPaths.StatusPath, c.shedPaths.StatusLockPath),
	)
	// The PR-Review session's liveness and process registration read reed's strand table.
	slug := seedSlug(c.location.WorktreeName)
	registerPane := func(pid int) error {
		return runproc.Register(fabricengine.RunProcDir(c.location.HubPath), slug, runproc.ChildRecord(pid, runproc.RoleSession, nil))
	}
	c.env.Landing.PullRequest.SessionLive = sessionLiveness(c.reed.Status)
	c.env.Landing.PullRequest.RegisterSession = sessionRegistrar(c.reed.Status, registerPane)
	// The sampler run's session is checked for liveness and registered, and its row wakes on its record's file event.
	// The building paths pass through here; status and pause never do.
	// So the watch opens only for a run that drives the row.
	if recipeHasEngine(c.recipe, samplerSessionEngine) {
		c.env.Sampler.Sessions = c.runner
		c.env.Sampler.SessionLive = sessionLiveness(c.reed.Status)
		c.env.Sampler.RegisterSession = sessionRegistrar(c.reed.Status, registerPane)
		c.env.Sampler.Watch = openRecordWatch(shedrun.SamplerRecord(c.location, c.runID))
	}
	// The fixer runs on the seat engine wire already built, so no second engine is constructed.
	c.env.Landing.FixerSeats = c.env.Seats
	// A pawl run's land check names the beam's parent worktree as where a cancel runs, read from the beam's origin record.
	if c.recipe == shedrun.RecipePawl {
		prime, err := primeOfHub(c.location)
		if err != nil {
			return err
		}
		parentWorktree, err := beamParentWorktree(prime, seedSlug(c.location.WorktreeName))
		if err != nil {
			return err
		}
		c.env.Landing.ParentWorktree = parentWorktree
	}
	return nil
}

// buildLoomShed builds the *shedengine.Shed that the run command runs.
// It assembles Env.Landing, then constructs the shed with loomrecipe.New.
//
// It returns the constructed shed and a nil error, or a nil shed and the first error encountered, unwrapped.
func (c *loomCLI) buildLoomShed() (*shedengine.Shed, error) {
	if err := c.wireLanding(); err != nil {
		return nil, err
	}

	// A loop's step child adopts its trace id from LYM_TRACE_ID.
	// This tie names the step the loop runs, so the watchdog can stop that step's runs.
	c.runner.SetStepTie(shuttleengine.StepTie{StepsDir: shedrun.StepsDir(c.location, c.runID), TraceID: logger.TraceID()})
	// Beam-Await wakes on a land request's file event, which only a running shed needs a watch for.
	// The wiring also runs for status and pause, so the watch is opened here instead.
	if c.recipe == shedrun.RecipePawl {
		c.env.Pawl.Watch = openLandRequestWatch(c.env.Pawl.Paths)
	}
	return c.newShed()
}
