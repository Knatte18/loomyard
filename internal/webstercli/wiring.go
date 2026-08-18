// wiring.go implements wire, the package-private method resolvePersistentPreRun (cli.go) delegates
// to once it has resolved cwd and the preflight.HubPresent probe: the mode decision itself, and the
// whole engine stack construction for whichever mode wins.
// The mode decision lives HERE, inside the extracted function, rather than upstream in
// resolvePersistentPreRun, precisely so a test can drive it with a told HubPresent result and stay
// tier 1 -- driving the real pre-run would reach lyxcwd.Resolve and its git spawn.

package webstercli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// wire computes hub-or-standalone mode from loc/hubPresent -- the *lyxcwd.Location and boolean
// preflight.HubPresent(cwd) already returned -- and builds the whole engine stack onto c: module
// configs, the model registry, resolved roles, the reed/claude engines, the shuttleengine.Runner and
// its three adapted seams, the told websterengine.Geometry, the injected RefMatcher, and the lazy
// fabric opener.
//
// hubPresent true selects hub mode, because the Location's paths are real. hubPresent false selects
// standalone mode, which covers BOTH a plain downloaded git repository and an unresolvable cwd --
// preflight.HubPresent returns a nil Location for each, and the two are indistinguishable from here.
//
// preflight.Wired is deliberately never consulted: Wired is a per-worktree question ("is fabric wired
// for the worktree at this exact cwd"), not a mode-selection one, and it is false in three healthy hub
// locations that run webster verbs today -- an ordinary worktree at <hub>/_board, an unpaired sibling,
// and a worktree whose paired sibling has been removed. Keying mode selection on Wired would either
// refuse those three (no fabric-sync path exists for standalone to fall back to `run`'s pre-flight
// against) or, worse, relocate a live hub's state into the standalone state directory. HubPresent asks
// the honest question instead: does a hub-level directory exist for this write to target.
//
// cwd is the already-resolved cwd resolvePersistentPreRun read via lyxcwd.CwdFrom -- wire needs it
// only as standalone's --target-dir default, never to resolve or re-resolve anything itself.
// stencilsDirFlag, planDirFlag, and targetDirFlag are the three persistent flags' raw, as-parsed
// values (empty string when the operator did not pass one).
//
// wire performs no cwd resolution and spawns no process -- every path it touches is either supplied by
// the caller (loc, cwd) or a plain filesystem read (config loads, the standalone stencil seed) -- so a
// test can drive it directly and stay inside the Test Tier Purity Invariant.
func (c *websterCLI) wire(loc *lyxcwd.Location, hubPresent bool, cwd, stencilsDirFlag, planDirFlag, targetDirFlag string) error {
	if hubPresent {
		return c.wireHub(loc, stencilsDirFlag, planDirFlag, targetDirFlag)
	}
	return c.wireStandalone(cwd, stencilsDirFlag, planDirFlag, targetDirFlag)
}

// wireHub builds the engine stack for hub mode: every module config and the model registry loaded
// over the anchor path, hubgeom's geometry builders, a real fabricengine.RefScanner, and a lazy
// fabricengine.Open closure.
func (c *websterCLI) wireHub(loc *lyxcwd.Location, stencilsDirFlag, planDirFlag, targetDirFlag string) error {
	if targetDirFlag != "" {
		return fmt.Errorf("webster: --target-dir is not honoured in hub mode: the worktree is already the target, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec")
	}

	anchorPath := loc.AnchorPath()

	shuttleCfg, err := shuttleengine.LoadConfig(anchorPath, "shuttle")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(anchorPath, "reed")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(anchorPath, "webster")
	if err != nil {
		return err
	}
	activeBatcher, err := batcher.Active(anchorPath)
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

	geom := hubgeom.WebsterGeometry(loc)
	if stencilsDirFlag != "" {
		geom.StencilsDir = stencilsDirFlag
	}
	if planDirFlag != "" {
		geom.PlanDir = planDirFlag
	}

	reedGeom := hubgeom.ReedGeometry(loc)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	claudeEngine := claudeengine.New()
	runner := shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	c.setRunner(runner, claudeEngine, reedEngine)
	c.shuttleCfg = shuttleCfg
	c.cfg = websterCfg
	c.roles = roles
	c.geom = geom
	c.anchorRel = loc.AnchorRel
	// The matcher is built eagerly because NewRefScanner only compiles a regexp and cannot fail. The
	// fabric handle stays a closure and must NOT be opened here: fabricengine.Open stat-checks the
	// paired sibling and would fail this pre-run in the three healthy-but-unwired locations that run
	// validate and status today, neither of which ever reaches the integration bisect.
	c.refMatcher = fabricengine.NewRefScanner(loc)
	c.openFabric = func() (*fabricengine.Fabric, error) { return fabricengine.Open(loc) }
	c.batcher = activeBatcher
	return nil
}

// wireStandalone builds the engine stack for standalone mode: the already-absolute --target-dir
// (defaulted to cwd when unset), standalonestate.Derive over it -- the only place Derive is ever
// called -- standalonegeom's geometry builders over the derived state directory, every module config
// and the model registry loaded over the same state directory, a pinned websterengine.NeverMatches
// RefMatcher, and a nil fabric opener.
func (c *websterCLI) wireStandalone(cwd, stencilsDirFlag, planDirFlag, targetDirFlag string) error {
	target, err := resolveStandaloneTarget(cwd, targetDirFlag)
	if err != nil {
		return err
	}

	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		return err
	}

	geom := standalonegeom.WebsterGeometry(target, stateDir)
	reedGeom := standalonegeom.ReedGeometry(target, stateDir, hash8)

	if stencilsDirFlag != "" {
		// An operator who named a curated stencil set must not have it rewritten from under them --
		// seed only the standalone DEFAULT, never an explicit override.
		geom.StencilsDir = stencilsDirFlag
	} else if _, err := stencilstore.Reconcile(geom.StencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), ""); err != nil {
		// Unlike the root pre-run's best-effort, logged-only seed pass, nothing else will ever
		// create this directory: a reconcile failure here is a hard error, since every prompt render
		// would otherwise fail later with a far less informative message.
		return fmt.Errorf("webster: seed the standalone stencils directory %s: %w", geom.StencilsDir, err)
	}

	if planDirFlag != "" {
		geom.PlanDir = planDirFlag
	}
	if !standalonePlanDirHasContent(geom.PlanDir) {
		return fmt.Errorf("webster: standalone plan directory %s does not exist or contains no plan files -- there is no bootstrap and no empty-plan fallback; pass --plan-dir to point at an authored plan", geom.PlanDir)
	}

	shuttleCfg, err := shuttleengine.LoadConfig(stateDir, "shuttle")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(stateDir, "reed")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(stateDir, "webster")
	if err != nil {
		return err
	}
	activeBatcher, err := batcher.Active(stateDir)
	if err != nil {
		return err
	}
	registry, err := modelspec.LoadRegistry(stateDir)
	if err != nil {
		return err
	}
	roles, err := websterengine.ResolveRoles(websterCfg, registry)
	if err != nil {
		return err
	}

	reedEngine := reedengine.New(reedCfg, reedGeom)
	claudeEngine := claudeengine.New()
	runner := shuttleengine.NewRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	c.setRunner(runner, claudeEngine, reedEngine)
	c.shuttleCfg = shuttleCfg
	c.cfg = websterCfg
	c.roles = roles
	c.geom = geom
	c.anchorRel = ""
	// standalone has no fabric repo by construction: the matcher never sees a fabric reference
	// because there is nothing to reference, and the opener is left nil rather than a closure that
	// would panic or stat-fail if ever called.
	c.refMatcher = websterengine.NeverMatches{}
	c.openFabric = nil
	c.batcher = activeBatcher
	return nil
}

// setRunner stores runner and its three adapted seams (starter, injector, masterStarter) plus the
// claude/reed engines onto c, shared by both wireHub and wireStandalone so the adaptation is named
// once.
func (c *websterCLI) setRunner(runner *shuttleengine.Runner, claudeEngine shuttleengine.Engine, reedEngine shuttleengine.ReedOps) {
	c.runner = runner
	c.starter = runner
	c.injector = runner
	c.masterStarter = runnerMasterStarter{runner: runner}
	c.engine = claudeEngine
	c.reed = reedEngine
}

// resolveStandaloneTarget resolves standalone mode's --target-dir: cwd when targetDirFlag is empty,
// or targetDirFlag resolved to an absolute path (relative to cwd) otherwise. The result is always
// absolute, which is standalonestate.Derive's own precondition.
func resolveStandaloneTarget(cwd, targetDirFlag string) (string, error) {
	if targetDirFlag == "" {
		return cwd, nil
	}
	if filepath.IsAbs(targetDirFlag) {
		return filepath.Clean(targetDirFlag), nil
	}
	return filepath.Join(cwd, targetDirFlag), nil
}

// standalonePlanDirHasContent reports whether dir exists and contains at least one "*.md" file --
// the minimal on-disk shape an authored plan directory carries. It never distinguishes "missing
// directory" from "empty directory" from "directory with no .md files": all three are the same usage
// error to a standalone operator, which has no bootstrap and no empty-plan fallback to fall back to.
func standalonePlanDirHasContent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			return true
		}
	}
	return false
}
