// wiring.go implements wire, the package-private method resolvePersistentPreRun (cli.go) delegates
// to once it has resolved cwd and told mode via preflight.ResolveMode: the mode decision itself, and
// the whole engine stack construction for whichever mode wins.
// The mode decision lives HERE, inside the extracted function, rather than upstream in
// resolvePersistentPreRun, precisely so a test can drive it with a told preflight.Mode and stay
// tier 1 -- driving the real pre-run would reach lyxcwd.Resolve and its git spawn.

package webstercli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// wireModule carries webster's own per-CLI variance for internal/cliwire's shared wiring
// prologue -- the message-bearing data cliwire needs but does not itself declare, since no
// production file in that package may name webster or burler. cliwire carries the shared
// implementation; the data that varies by caller lives here, with the caller, exactly the way
// internal/shedrecipe's constructors live in that package while the rows that vary live outside
// it.
var wireModule = cliwire.Module{
	Name:             "webster",
	StateArtifacts:   "state, locks, rendered prompts and trace logs",
	TargetRole:       "the repository it drives",
	TargetRecourse:   "Drive a target",
	HubTargetSubject: "the worktree is already the target",
	Plan: &cliwire.PlanRules{
		DefaultPlanDir: planparser.PlanDir,
		MissingPlanRefusal: func(planDir, recourse string) string {
			return fmt.Sprintf("webster: standalone plan directory %s does not exist or contains no plan files -- there is no bootstrap and no empty-plan fallback. Place an authored plan at %s, which is what `run` requires (Master's own in-pane verbs are flagless and resolve that default); --plan-dir points the bracket and read-only verbs at a plan elsewhere, but `run` refuses it", planDir, recourse)
		},
	},
}

// wire computes hub-or-standalone mode from loc/mode -- the *lyxcwd.Location and preflight.Mode a
// preflight.ResolveMode(cwd) call already told it -- and builds the whole engine stack onto c: module
// configs, the model registry, resolved roles, the reed/claude engines, the shuttleengine.Runner and
// its three adapted seams, the told websterengine.Geometry, the injected RefMatcher, and the lazy
// fabric opener.
//
// mode == preflight.ModeHub selects hub mode, because the Location's paths are real.
// preflight.ModeStandalone selects standalone mode, which covers BOTH a plain downloaded git
// repository and an unresolvable cwd -- preflight.ResolveMode returns a nil Location for each, and
// the two are indistinguishable from here.
//
// wire never sees the refused case at all: a ResolveMode error aborts upstream in
// resolvePersistentPreRun, because a refusal is a resolution verdict rather than a wiring choice --
// which is why mode carries only two reachable values here and no third Mode value exists for
// refusal.
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
// as standalone's --target-dir default and as the base every relative flag value is made absolute
// against, never to resolve or re-resolve anything itself.
// stencilsDirFlag, planDirFlag, and targetDirFlag are the three persistent flags' raw, as-parsed
// values (empty string when the operator did not pass one).
//
// All three are made absolute HERE, at the one boundary that still knows which working directory the
// operator typed them from, via cliwire.ResolveToldDir. A relative value stored verbatim is not a
// smaller version of an absolute one: this process resolves it against ITS cwd while the pane webster
// spawns runs at the target (standalone) or the anchor (hub), so one string named two different
// directories, and the standalone default-vs-override comparison below could never match a relative
// spelling of the default. Every other told path in this codebase is required absolute for exactly
// that reason.
//
// wire performs no cwd resolution and spawns no process -- every path it touches is either supplied by
// the caller (loc, cwd) or a plain filesystem read (config loads, the standalone stencil seed) -- so a
// test can drive it directly and stay inside the Test Tier Purity Invariant.
func (c *websterCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, planDirFlag, targetDirFlag string) error {
	stencilsDir := cliwire.ResolveToldDir(cwd, stencilsDirFlag)
	planDir := cliwire.ResolveToldDir(cwd, planDirFlag)

	if mode == preflight.ModeHub {
		return c.wireHub(loc, stencilsDir, planDir, targetDirFlag)
	}
	return c.wireStandalone(cwd, stencilsDir, planDir, targetDirFlag)
}

// wireHub builds the engine stack for hub mode: every module config and the model registry loaded
// over the anchor path, hubgeom's geometry builders, a real fabricengine.RefScanner, and a lazy
// fabricengine.Open closure.
//
// stencilsDir and planDir arrive already absolute (or empty), made so by wire -- this function never
// sees a raw flag value and must never start honouring one.
func (c *websterCLI) wireHub(loc *lyxcwd.Location, stencilsDir, planDir, targetDirFlag string) error {
	if err := wireModule.RefuseTargetDirInHubMode(targetDirFlag); err != nil {
		return err
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
	if stencilsDir != "" {
		// The same boundary stat standalone's prologue applies, through the same descriptor method,
		// so the two modes can never drift on what a told stencils directory must be: a typo'd
		// --stencils-dir refused here costs nothing, while unchecked it failed only at the first
		// prompt render, after the run lock and substrate boot (crucible round fable-high-r7, F2).
		if err := wireModule.RefuseUnreadableStencilsDir(stencilsDir); err != nil {
			return err
		}
		geom.StencilsDir = stencilsDir
	}
	if planDir != "" {
		// Hub mode records a moved plan directory for exactly the reason standalone does: Master's own
		// in-pane verb invocations are typed flagless from the stencil in BOTH modes, so they resolve
		// the hub default and would refuse against a plan they cannot see. Every other verb keeps
		// honoring the override -- see the planDirOverridden field's own doc for the failure this
		// closes on the hub path.
		//
		// The comparison is against geom.PlanDir -- the value hubgeom.WebsterGeometry actually built --
		// and deliberately not wireModule.Plan.DefaultPlanDir(anchorPath), even though the two are the
		// same string today; comparing against the geometry's own field is what keeps the override
		// check correct if internal/hubgeom ever changes how it computes PlanDir.
		resolvedPlanDir, overridden := cliwire.ResolvePlanDir(planDir, geom.PlanDir)
		if overridden {
			c.planDirOverridden = true
			c.planDirDefault = geom.PlanDir
		}
		geom.PlanDir = resolvedPlanDir
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

// wireStandalone builds the engine stack for standalone mode by calling wireModule.ResolveStandalone
// -- internal/cliwire's single ordered standalone prologue -- and composing webster's own engines onto
// the result: a pinned websterengine.NeverMatches RefMatcher, a shuttleengine.NewDetachedRunner-
// constructed runner -- standalone's anchor (the derived state directory) is deliberately outside its
// worktree root (the target), which NewRunner's containment assertion would refuse -- and a nil fabric
// opener.
//
// stencilsDir and planDir arrive already absolute (or empty), made so by wire via
// cliwire.ResolveToldDir. Re-resolving them again inside ResolveStandalone is idempotent on an
// already-absolute value, and is deliberate rather than an oversight to hoist out: see
// cliwire.ResolveStandalone's own doc comment for the prologue's ordering obligation, which this
// function's body no longer needs to restate.
func (c *websterCLI) wireStandalone(cwd, stencilsDir, planDir, targetDirFlag string) error {
	res, err := wireModule.ResolveStandalone(cliwire.StandaloneRequest{
		Cwd:             cwd,
		StencilsDirFlag: stencilsDir,
		PlanDirFlag:     planDir,
		TargetDirFlag:   targetDirFlag,
	})
	if err != nil {
		return err
	}

	geom := standalonegeom.WebsterGeometry(res.Target, res.StateDir)
	reedGeom := standalonegeom.ReedGeometry(res.Target, res.StateDir, res.Hash8)
	geom.StencilsDir = res.StencilsDir
	geom.PlanDir = res.PlanDir
	if res.PlanDirOverridden {
		c.planDirOverridden = true
		c.planDirDefault = res.DefaultPlanDir
	}

	shuttleCfg, err := shuttleengine.LoadConfig(res.StateDir, "shuttle")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(res.StateDir, "reed")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(res.StateDir, "webster")
	if err != nil {
		return err
	}
	activeBatcher, err := batcher.Active(res.StateDir)
	if err != nil {
		return err
	}
	registry, err := modelspec.LoadRegistry(res.StateDir)
	if err != nil {
		return err
	}
	roles, err := websterengine.ResolveRoles(websterCfg, registry)
	if err != nil {
		return err
	}

	reedEngine := reedengine.New(reedCfg, reedGeom)
	claudeEngine := claudeengine.New()
	runner := shuttleengine.NewDetachedRunner(reedEngine, claudeEngine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)

	// Standalone's reed session lives on its own derived geometry, which no CLI verb can reach —
	// `lyx reed up` is hub-only — so the verbs that need one boot it in-process through this seam
	// (see the field's own doc comment). Assigned here, executed by the two verbs that spawn an
	// agent themselves, run and recover-batch: wiring runs for EVERY verb, and validate, status,
	// pause and await-batch must boot no tmux server at all.
	c.reedUp = func() error {
		_, err := reedEngine.Up()
		return err
	}

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
