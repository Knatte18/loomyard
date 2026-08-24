// wiring.go implements wire, extracted from the pre-run (cli.go's resolvePersistentPreRun) so a test
// can drive it against a hand-built *lyxcwd.Location and stay tier 1.
// wire resolves no cwd and spawns no process -- every path it touches is either caller-supplied
// (location, cwd) or a plain config read anchored at location.AnchorPath(), so a test can drive it
// directly without breaching the Test Tier Purity Invariant.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
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

	reedGeom := hubgeom.ReedGeometry(location)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	claudeEngine := claudeengine.New()
	runner := shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	websterGeom := hubgeom.WebsterGeometry(location)

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
		// time -- what the Stencil Ownership Invariant requires. autonomous is the literal true,
		// unconditionally, per the autonomous-only Shared Decision.
		DiscussionSpec: func() (shuttleengine.Spec, error) {
			return loomengine.DiscussionSpec(location, websterGeom.StencilsDir, loomCfg, registry, seedSlug(location.WorktreeName), true)
		},
		// CommitDiscussion mirrors the seed commit run.go already performs, including its
		// NewMutations("") record and its EnvSyncOptions(). The pathspec is the whole discussion
		// directory deliberately, so archiveStaleOutputs' timestamped siblings are committed rather
		// than left as untracked weft dirt. A second Done over already-committed artifacts is a
		// no-op rather than an error: CommitAnchoredPaths reports committed == false for an
		// already-clean, already-tracked path, and this closure discards that result alongside the
		// sha, returning only the error.
		CommitDiscussion: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, fmt.Sprintf("loom: discussion artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		// StencilsDir, RunRoot, Burler, and Now are left zero -- only SingleLLM and Bouncer/
		// BurlerRound read StencilsDir/RunRoot, and no row in loom's recipe uses those engines yet.
		// StencilsDir in particular stays unfilled here even though DiscussionWrite is now wired:
		// the DiscussionSpec closure above captures websterGeom.StencilsDir directly rather than
		// reading it back off Env. A nil Now is legal, defaulting to time.Now inside
		// NewSingleLLMProducer.
		//
		// Landing is deliberately left unfilled here too, but for a different reason than the other
		// four: Env.Landing is assembled in drive.go, immediately before loomrecipe.New, because
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
