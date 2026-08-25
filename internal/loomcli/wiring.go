// wiring.go implements wire, extracted from the pre-run (cli.go's resolvePersistentPreRun) so a test
// can drive it against a hand-built *lyxcwd.Location and stay tier 1.
// wire resolves no cwd and spawns no process -- every path it touches is either caller-supplied
// (location, cwd) or a plain config read anchored at location.AnchorPath(), so a test can drive it
// directly without breaching the Test Tier Purity Invariant.

package loomcli

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// wire builds the whole engine stack onto c from location and cwd: every module config anchored at
// location.AnchorPath(), the reed engine and shuttle runner, the assembled websterengine.RunDeps, and
// the assembled shedrecipe.Env/loomrecipe.ShedPaths pair wrapping it.
func (c *loomCLI) wire(location *lyxcwd.Location, cwd string) error {
	anchorPath := location.AnchorPath()

	loomCfg, err := loomengine.LoadConfig(anchorPath, "loom")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(anchorPath, "reed")
	if err != nil {
		return err
	}
	shuttleCfg, err := shuttleengine.LoadConfig(anchorPath, "shuttle")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(anchorPath, "webster")
	if err != nil {
		return err
	}
	landingCfg, err := landingshed.LoadConfig(anchorPath, "landing")
	if err != nil {
		return err
	}
	// burlerengine.LoadConfig takes one argument, not the (baseDir, module) shape every other
	// loader above takes: it is an optional-file loader, so an absent burler.yaml yields a zero
	// Config and a nil error rather than a load failure.
	burlerCfg, err := burlerengine.LoadConfig(anchorPath)
	if err != nil {
		return err
	}
	registry, err := modelspec.LoadRegistry(anchorPath)
	if err != nil {
		return err
	}
	roles, err := websterengine.ResolveRoles(websterCfg, registry)
	if err != nil {
		return err
	}
	activeBatcher, err := batcher.Active(anchorPath)
	if err != nil {
		return err
	}
	reviewSettings, err := loomengine.ResolveReview(loomCfg, registry)
	if err != nil {
		return err
	}

	reedGeom := hubgeom.ReedGeometry(location)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	claudeEngine := claudeengine.New()
	runner := shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	websterGeom := hubgeom.WebsterGeometry(location)

	// BurlerGeometry, not WebsterGeometry: BurlerGeometry fills WorktreeRoot from
	// location.WorktreePath(), while WebsterGeometry fills its own from location.AnchorPath() for a
	// reason specific to webster's CLI call sites (see webstergeom.go's doc comment). That reasoning
	// does not carry over to Burler, so the two geometries are not interchangeable here.
	burlerEngine := burlerengine.New(runner, hubgeom.BurlerGeometry(location), burlerCfg, websterGeom.StencilsDir)

	runDeps := websterengine.RunDeps{
		Starter:    runnerMasterStarter{runner: runner},
		Reed:       reedEngine,
		Engine:     claudeEngine,
		ShuttleCfg: shuttleCfg,
		Roles:      roles,
		Config:     websterCfg,
		Batcher:    activeBatcher,
		Geom:       websterGeom,
		// The reference matcher is pinned to a real fabricengine.NewRefScanner(location), built
		// eagerly because that constructor only compiles a regexp and cannot fail. It must never be
		// the never-matching stand-in: that stand-in is permitted only in standalone, where there is
		// no wired fabric for the guard to protect, and loom is hub-only.
		RefMatcher: fabricengine.NewRefScanner(location),
		// The bisector opener stays a lazy closure over fabricengine.Open(location) and must not be
		// opened here: opening stat-checks the paired sibling, and this pre-run must not fail
		// "status"/"pause" against a healthy-but-unwired location.
		OpenBisector: func() (websterengine.FabricBisector, error) {
			return fabricengine.Open(location)
		},
	}

	statusPath := loomengine.LoomStatusFile(location)
	statusLockPath := loomengine.LoomStatusLock(location)

	c.env = shedrecipe.Env{
		Cwd:                cwd,
		AnchorPath:         anchorPath,
		WorktreeRoot:       location.WorktreePath(),
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
		DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
		SupportLogPath:     loomengine.DiscussionSupportLog(location),
		WebsterDeps:        runDeps,
		// WebsterRun is set explicitly to websterengine.Run, per the
		// env-webster-run-is-filled-explicitly Shared Decision: websterEntry errors on a nil
		// WebsterRun, unlike loomshed.Deps.WebsterRun, which shedadapters.NewWebsterProducer
		// defaulted when left nil.
		WebsterRun: websterengine.Run,
		// Shuttle is runner, already built above: *shuttleengine.Runner already satisfies
		// shedadapters.Shuttle, and row 3 (Discussion-Write) reads it now.
		Shuttle: runner,
		// DiscussionSpec is evaluated per Call, not resolved here, so the stencil is read at call
		// time -- what the Stencil Ownership Invariant requires. autonomous is now
		// !loomCfg.DiscussionInteractive, read fresh on every wire() call. Nothing compares it
		// against the mode a live run was started with: the resume decision is made purely on
		// live-agent evidence, so flipping the key between a crash and a resume is permitted and
		// benign -- it means only that the next spawn is interviewed differently.
		DiscussionSpec: func() (shuttleengine.Spec, error) {
			return loomengine.DiscussionSpec(location, websterGeom.StencilsDir, loomCfg, registry, seedSlug(location.WorktreeName), !loomCfg.DiscussionInteractive)
		},
		// CommitDiscussion mirrors the seed commit run.go already performs, including its
		// NewMutations("") record and its EnvSyncOptions(). The pathspec is the whole discussion
		// directory deliberately, so archiveStaleOutputs' timestamped siblings are committed rather
		// than left as untracked dirt. A second Done over already-committed artifacts is a
		// no-op rather than an error: CommitAnchoredPaths reports committed == false for an
		// already-clean, already-tracked path, and this closure discards that result alongside the
		// sha, returning only the error -- and this idempotence now covers two callers rather than
		// one, since the Discussion-Bouncer row's approved settle reaches this same closure through
		// the row's commit_seam: discussion config key.
		CommitDiscussion: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, fmt.Sprintf("loom: discussion artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// PlanSpec is evaluated per Call, not resolved here, so the stencil is read at call time --
		// what the Stencil Ownership Invariant requires. Unlike DiscussionSpec beside it, PlanSpec
		// takes no autonomous argument: the Plan producer is autonomous by design and hard-codes
		// Interactive: false internally.
		PlanSpec: func() (shuttleengine.Spec, error) {
			return loomengine.PlanSpec(location, websterGeom.StencilsDir, loomCfg, registry)
		},
		// CommitPlan mirrors CommitDiscussion above: it keeps the working tree clean for the rows
		// that follow, makes the artifact durable across a crash or a resume, and sweeps the
		// decorator's archive subdirectory into git rather than leaving it as untracked dirt. A
		// second Done over already-committed artifacts is a no-op rather than an error:
		// CommitAnchoredPaths reports committed == false for an already-clean, already-tracked path,
		// and this closure discards that result alongside the sha, returning only the error -- and
		// this idempotence now covers two callers rather than one, since the Plan-Bouncer row's
		// approved settle reaches this same closure through the row's commit_seam: plan config key.
		// The commit message is deliberately shared between the two callers: it names the artifact
		// set rather than the producer that last touched it, so a Plan-Write commit and a
		// Plan-Bouncer commit read identically. This commit fires before Plan-Validate has judged
		// the plan, and that is intentional and matches the discussion precedent: the commit keeps
		// the artifact durable, it does not certify it. The pathspec is the whole plan directory via
		// planparser.PlanDirRel(), never a hand-built filepath.Join naming the _lyx literal, which
		// the Lyxdirs Single-Declarer Invariant forbids in production path-construction context.
		CommitPlan: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{planparser.PlanDirRel()}, fmt.Sprintf("loom: plan artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// StencilsDir, RunRoot, Burler, and Now are filled for both review segments --
		// Discussion-Bouncer/Discussion-Burler and Plan-Bouncer/Plan-Burler alike. StencilsDir is
		// websterGeom.StencilsDir -- the same value the DiscussionSpec and PlanSpec closures above
		// already capture directly -- so this is one value read from one place, not a second copy
		// that could drift from theirs. Now is filled explicitly with time.Now rather than left nil,
		// even though nil defaults to time.Now inside the underlying constructors, because the
		// Bouncer's archive-filename collision suffix is the one place a test wants to inject a
		// clock.
		StencilsDir: websterGeom.StencilsDir,
		RunRoot:     loomengine.LoomReviewsDir(location),
		Burler:      burlerEngine,
		Now:         time.Now,

		ReviewModel:   reviewSettings.Model,
		ReviewEffort:  reviewSettings.Effort,
		ReviewVersion: reviewSettings.Version,
		ReviewTimeout: reviewSettings.Timeout,

		// Landing is deliberately left unfilled here, for a different reason than the four above:
		// Env.Landing is assembled in drive.go, immediately before loomrecipe.New, because
		// NewPublish/NewFinalize both open their fabric pair eagerly at construction, and wire()
		// runs for every verb including "status"/"pause" -- the same OpenBisector hazard the
		// comment above already guards against. See landingDeps (landingdeps.go) and the
		// env-landing-filled-in-drive-not-wire design decision.
	}

	// c.shedPaths carries the four told values shedengine.Shed itself reads and no shedrecipe.Env
	// registry entry reads. StatusPath and StatusLockPath are deliberately told twice, once here and
	// once above in c.env -- that duplication is inherent to the split between loomrecipe.New's two
	// argument types and must not be collapsed; loomrecipe.New errors if the two copies disagree.
	// Each pair is filled from the single statusPath/statusLockPath evaluation above rather than a
	// second loomengine accessor call, so the two copies cannot drift here.
	c.shedPaths = loomrecipe.ShedPaths{
		StatusPath:     statusPath,
		LockPath:       loomengine.LoomRunLock(location),
		StatusLockPath: statusLockPath,
		// MaxBounces is left zero so shedengine.Shed's own default applies. "Default" here means
		// the inherited per-producer default every ProducerDef.MaxBounces of 0 falls back to
		// (which itself falls back to shedengine's internal default of ten), not a run-wide
		// total -- the budget itself is per-producer and episode-scoped, counted from the
		// persisted history rather than held in memory.
	}

	c.location = location
	c.cwd = cwd
	c.cfg = loomCfg
	c.reed = reedEngine
	c.runDeps = runDeps
	c.registry = registry
	c.runner = runner
	c.landingCfg = landingCfg
	return nil
}
