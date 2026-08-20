// wiring.go implements wire, extracted from the pre-run (cli.go's resolvePersistentPreRun) so a test
// can drive it against a hand-built *lyxcwd.Location and stay tier 1.
// wire resolves no cwd and spawns no process -- every path it touches is either caller-supplied
// (location, cwd) or a plain config read anchored at location.AnchorPath(), so a test can drive it
// directly without breaching the Test Tier Purity Invariant.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// wire builds the whole engine stack onto c from location and cwd: every module config anchored at
// location.AnchorPath(), the reed engine and shuttle runner, the assembled websterengine.RunDeps, and
// the assembled loomshed.Deps wrapping it.
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

	c.deps = loomshed.Deps{
		StatusPath:     loomengine.LoomStatusFile(location),
		LockPath:       loomengine.LoomRunLock(location),
		StatusLockPath: loomengine.LoomStatusLock(location),
		// MaxBounces is left zero so shedengine.Shed's own default applies. "Default" here means
		// the inherited per-producer default every ProducerDef.MaxBounces of 0 falls back to
		// (which itself falls back to shedengine's internal default of ten), not a run-wide
		// total -- the budget itself is per-producer and episode-scoped, counted from the
		// persisted history rather than held in memory.
		AnchorPath:         anchorPath,
		WorktreeRoot:       location.WorktreePath(),
		DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
		SupportLogPath:     loomengine.DiscussionSupportLog(location),
		// Preflight is the adapter constructor, never the bare loomengine.Preflight function: loomshed.New
		// requires a shedengine.ShedProducer, and the adapter is what maps loomengine.Preflight's Report
		// onto that contract.
		Preflight: loomshed.NewPreflightProducer(cwd),
		// WebsterRun is left nil so shedadapters.NewWebsterProducer defaults to the production entry
		// point, websterengine.Run.
		WebsterDeps: runDeps,
	}

	c.location = location
	c.cwd = cwd
	c.cfg = loomCfg
	c.reed = reedEngine
	c.runDeps = runDeps
	return nil
}
