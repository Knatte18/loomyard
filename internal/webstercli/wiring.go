// wiring.go builds webster's engine stack once the pre-run has resolved the mode.
// wire picks hub or standalone mode from the Location and Mode that preflight.ResolveMode reported.
// It also builds the whole stack for whichever mode wins.
// The mode decision lives in wire, not in the pre-run,
// so a test can drive it with a given preflight.Mode without reaching lymcwd.Resolve or its git spawn.

package webstercli

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// websterProgram is the program name webster signs its notices to the orch with.
const websterProgram = "WEBSTER"

// wireModule holds the per-CLI values for internal/cliwire's shared wiring prologue.
// cliwire carries the shared implementation.
// No production file in it names a caller, so the values that vary by caller live here, next to the caller.
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

// wire selects hub or standalone mode and builds the whole engine stack onto c for it.
// It takes loc and mode as preflight.ResolveMode reported them for cwd.
//
// mode == preflight.ModeHub selects hub mode, because the Location's paths are real.
// preflight.ModeStandalone selects standalone mode, for both a plain downloaded git repository and an unresolvable cwd.
// preflight.ResolveMode returns a nil Location for both, so wire cannot tell them apart.
//
// A ResolveMode refusal never reaches wire: it is a resolution verdict, not a wiring choice.
// So mode has only two reachable values here, and no Mode value exists for the refusal.
//
// preflight.Wired is never consulted: it asks whether fabric is wired for this worktree.
// That is a per-worktree question, not a mode-selection one.
// It is false in three healthy hub locations that run webster verbs.
// They are an ordinary worktree, an unpaired sibling, and a worktree whose paired sibling was removed.
// Keying mode on Wired would refuse all three, or relocate a live hub's state into the standalone state directory.
// HubPresent asks the right question instead: whether the hub-level directory this write targets exists.
//
// cwd is the already-resolved working directory, the base every relative flag value is made absolute against.
// It is also the default --target-dir in standalone mode.
// stencilsDirFlag, planDirFlag, and targetDirFlag are the raw flag values as parsed, empty when none was passed.
//
// The stencils and plan directories are made absolute here, by cliwire.ResolveToldDir.
// This is the last boundary that still knows the directory the operator typed them from.
// A relative value resolves against this process's cwd,
// but the pane webster spawns runs at the target or the anchor, so one string would name two directories,
// and the default-versus-override comparison in standalone mode could never match a relative spelling of the default.
// Every other told path in the codebase is required to be absolute for the same reason.
//
// wire resolves no cwd and spawns no process.
// Every path it touches comes from the caller or is a plain filesystem read,
// so a test can drive it directly and stay inside the Test Tier Purity Invariant.
func (c *websterCLI) wire(loc *lymcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, planDirFlag, targetDirFlag string) error {
	stencilsDir := cliwire.ResolveToldDir(cwd, stencilsDirFlag)
	planDir := cliwire.ResolveToldDir(cwd, planDirFlag)

	if mode == preflight.ModeHub {
		return c.wireHub(loc, stencilsDir, planDir, targetDirFlag)
	}
	return c.wireStandalone(cwd, stencilsDir, planDir, targetDirFlag)
}

// wireHub builds the engine stack for hub mode.
// It loads the module configs and the model registry from the anchor path.
// It builds the geometry with hubgeom, and it wires a real fabricengine.RefScanner and a lazy fabricengine.Open closure.
//
// stencilsDir and planDir arrive already absolute or empty, because wire made them so.
// wireHub never sees a raw flag value, and it must never start honouring one.
func (c *websterCLI) wireHub(loc *lymcwd.Location, stencilsDir, planDir, targetDirFlag string) error {
	if err := wireModule.RefuseTargetDirInHubMode(targetDirFlag); err != nil {
		return err
	}

	// The friction directory is resolved tolerantly here, not in loomcli.
	// That lets the per-batch implementer fork and the cold recovery strand receive the directive under `lym loom start`.
	// Master runs `lym webster begin-batch <NN>` and `lym webster recover-batch <NN>` as separate processes,
	// so their prompts are composed here.
	// See the webstercli-resolves-the-friction-directory-in-hub-mode Shared Decision.
	// A load error or an empty Friction key leaves it empty.
	// A `lym webster` verb must never fail over an unrelated module's config, since the value is optional bookkeeping.
	c.frictionDir = ""
	if loomCfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(loc), "loom"); err != nil {
		logger.Warn("webstercli: could not load loom config; Tier 2 friction notes are disabled for this invocation", "error", err)
	} else if loomCfg.Friction != "" {
		c.frictionDir = loomengine.LoomFrictionDir(loc)
	}

	shuttleCfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(loc), "shuttle")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(loc), "reed")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(fabricengine.ConfigDirs(loc), "webster")
	if err != nil {
		return configLoadError("webster.yaml", err)
	}
	activeBatcher, err := batcher.Active(fabricengine.ConfigDirs(loc))
	if err != nil {
		return configLoadError("batcher.yaml", err)
	}
	language, err := languageconfig.Load(fabricengine.ConfigDirs(loc))
	if err != nil {
		return configLoadError("language.yaml", err)
	}
	languageSkills := language.Skills
	registry, err := modelspec.LoadRegistry(fabricengine.ConfigDirs(loc))
	if err != nil {
		return err
	}
	roles, err := websterengine.ResolveRoles(websterCfg, registry)
	if err != nil {
		return err
	}

	geom := hubgeom.WebsterGeometry(loc)
	geom.Index = planglyph.NewSlottedIndex(fabricengine.NewReferenceRule(), geom.GateSlots, geom.GateWaitDir)
	gateTiers, err := gateTiersValue(geom.GateConfig)
	if err != nil {
		return err
	}
	if stencilsDir != "" {
		// Standalone's prologue runs the same check through the same descriptor method,
		// so the two modes agree on what a told stencils directory must be.
		// Refusing a mistyped --stencils-dir here costs nothing.
		// Unchecked, it fails only at the first prompt render, after the run lock and substrate boot.
		if err := wireModule.RefuseUnreadableStencilsDir(stencilsDir); err != nil {
			return err
		}
		geom.StencilsDir = stencilsDir
	}
	if planDir != "" {
		// Hub mode records a moved plan directory for the same reason standalone does.
		// Master types its in-pane verbs flagless from the stencil in both modes, so they resolve the hub default.
		// Against a plan they cannot see, they would refuse; every other verb keeps honoring the override.
		//
		// The comparison is against geom.PlanDir, the value hubgeom.WebsterGeometry built, not wireModule.Plan.DefaultPlanDir.
		// Reading the geometry's own field keeps the override check correct if hubgeom ever changes how it computes PlanDir.
		resolvedPlanDir, overridden := cliwire.ResolvePlanDir(planDir, geom.PlanDir)
		if overridden {
			c.planDirOverridden = true
			c.planDirDefault = geom.PlanDir
		}
		geom.PlanDir = resolvedPlanDir
	}

	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		return err
	}
	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	engines, err := shuttleengines.New(shuttleCfg, fabricengine.ConfigDirs(loc))
	if err != nil {
		return err
	}
	runner := shuttleengine.NewRunner(reedEngine, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	pauseGeom, err := hubgeom.PauseGeometry(loc)
	if err != nil {
		return err
	}
	runner.SetPause(pauseGeom)
	runner.SetNotifier(func(line string) error {
		_, err := orchcli.NotifyAsker(loc, websterProgram, line)
		return err
	})

	c.setRunner(runner, engines, reedEngine)
	c.shuttleCfg = shuttleCfg
	c.cfg = websterCfg
	c.roles = roles
	c.instructionFiles = websterengine.MasterInstructionFiles(engines, roles[websterengine.RoleMaster])
	c.pause = pauseGeom
	c.masterStrand = envMasterStrand(reedEngine.ResolveStrandGUID)
	geom.VerifyHold = verifytree.NewPauseHold(pauseGeom, nil)
	c.geom = geom
	c.anchorRel = loc.AnchorRel
	// The matcher is built eagerly because NewRefScanner only compiles a regexp and cannot fail.
	// The fabric handle stays a closure and must not be opened here.
	// fabricengine.Open stat-checks the paired sibling, and a healthy location can lack one.
	// Opening it here would fail the verbs that never commit to the fabric before they run.
	c.refMatcher = fabricengine.NewRefScanner(loc)
	c.openFabric = func() (*fabricengine.Fabric, error) { return fabricengine.Open(loc) }
	c.parentBranch = func() (string, error) {
		origin, found, err := fabricengine.ReadOrigin(loc)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("the pair has no fabric origin record")
		}
		return origin.ParentBranch, nil
	}
	c.runProcDir = fabricengine.RunProcDir(loc.HubPath)
	c.pairSlugRead = func() string { return originPairSlug(loc) }
	c.batcher = activeBatcher
	c.languageSkills = languageSkills
	c.gateTiers = gateTiers
	c.renameTools = language.RenameTools
	return nil
}

// gateTiersValue reads the gate config through read once and returns its rendered tier and gate-list lines.
// An absent gate.yaml renders `none`.
// Any other load error is returned as is, since it already names `lym config gate`.
func gateTiersValue(read func() (gateslot.Config, error)) (string, error) {
	cfg, err := read()
	if errors.Is(err, gateslot.ErrConfigAbsent) {
		return "none", nil
	}
	if err != nil {
		return "", err
	}
	return cfg.TiersValue(), nil
}

// wireStandalone builds the engine stack for standalone mode from the shared standalone prologue.
// It composes webster's own engines onto that stack.
// They are a never-matching RefMatcher, a detached runner, and a nil fabric opener.
// The detached runner exists because the standalone anchor, the derived state directory, lies outside the worktree root.
// The ordinary runner's containment check would refuse that.
//
// stencilsDir and planDir arrive already absolute (or empty), made so by the caller.
// The standalone prologue resolves them again, which is idempotent on an absolute value.
// That repeat is deliberate, not an oversight to hoist out.
func (c *websterCLI) wireStandalone(cwd, stencilsDir, planDir, targetDirFlag string) error {
	// c.frictionDir is left at its zero value: a standalone webster run is not a tapestry run and has no
	// friction directory.
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
	geom.Index = planglyph.NewSlottedIndex(fabricengine.NewReferenceRule(), geom.GateSlots, geom.GateWaitDir)
	reedGeom := standalonegeom.ReedGeometry(res.Target, res.StateDir, res.Hash8)
	geom.StencilsDir = res.StencilsDir
	geom.PlanDir = res.PlanDir
	gateTiers, err := gateTiersValue(geom.GateConfig)
	if err != nil {
		return err
	}
	if res.PlanDirOverridden {
		c.planDirOverridden = true
		c.planDirDefault = res.DefaultPlanDir
	}

	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: res.StateDir}, "shuttle")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: res.StateDir}, "reed")
	if err != nil {
		return err
	}
	websterCfg, err := websterengine.LoadConfig(configengine.Dirs{Anchor: res.StateDir}, "webster")
	if err != nil {
		return configLoadError("webster.yaml", err)
	}
	activeBatcher, err := batcher.Active(configengine.Dirs{Anchor: res.StateDir})
	if err != nil {
		return configLoadError("batcher.yaml", err)
	}
	registry, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: res.StateDir})
	if err != nil {
		return err
	}
	roles, err := websterengine.ResolveRoles(websterCfg, registry)
	if err != nil {
		return err
	}

	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	engines, err := shuttleengines.New(shuttleCfg, configengine.Dirs{Anchor: res.StateDir})
	if err != nil {
		return err
	}
	runner := shuttleengine.NewDetachedRunner(reedEngine, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)
	pauseGeom := standalonegeom.PauseGeometry(res.StateDir)
	runner.SetPause(pauseGeom)

	// Standalone's reed session lives on its own derived geometry, which no CLI verb can reach.
	// `lym reed up` is hub-only, so the verbs that need a session boot it in-process through `reedUp`.
	// Wiring assigns it for every verb.
	// Only the verbs that spawn an agent call it, since the read-only verbs must boot no tmux server.
	//
	// A long-lived supervising run owns that boot for the session's working lifetime.
	// Hub mode needs a detached daemon because it lacks that supervising process.
	// Reusing the in-process boot here is the smaller mechanism, not a special case.
	// Standalone computes no hub lock path and spawns no daemon.
	c.reedUp = func() error {
		_, err := reedEngine.Up()
		return err
	}

	c.setRunner(runner, engines, reedEngine)
	c.shuttleCfg = shuttleCfg
	c.cfg = websterCfg
	c.roles = roles
	c.instructionFiles = websterengine.MasterInstructionFiles(engines, roles[websterengine.RoleMaster])
	c.pause = pauseGeom
	c.masterStrand = envMasterStrand(reedEngine.ResolveStrandGUID)
	geom.VerifyHold = verifytree.NewPauseHold(pauseGeom, nil)
	c.geom = geom
	c.anchorRel = ""
	// standalone has no fabric repo by construction: the matcher never sees a fabric reference
	// because there is nothing to reference, and the opener is left nil rather than a closure that
	// would panic or stat-fail if ever called.
	c.refMatcher = websterengine.NeverMatches{}
	c.openFabric = nil
	c.parentBranch = nil
	c.batcher = activeBatcher
	c.gateTiers = gateTiers
	return nil
}

// configLoadRefusal is the refusal a verb's config load returns before its body runs.
// It carries the message, whether the config file's content is invalid, and the cause.
// configLoadError is the only code that constructs one.
type configLoadRefusal struct {
	message string
	invalid bool
	cause   error
}

func (r configLoadRefusal) Error() string { return r.message }

func (r configLoadRefusal) Unwrap() error { return r.cause }

// configLoadError wraps the error of loading the named config file (such as "batcher.yaml") as a configLoadRefusal.
// The refusal names the file and the way forward.
// A content failure (configengine.ErrInvalid) is invalid and names the fix.
// A retired batcher key (batcher.ErrRetiredKey) is invalid too, and its cause already ends in the reconcile way forward.
// A languageconfig.ErrUnusable cause is invalid and ends in its own way forward.
// The hub-wide language.yaml never lives under a worktree's _lym/config.
// That is why this cause gets its own way forward.
// Any other cause is a file that exists but cannot be read, which is transient.
// Both wiring prologues return it before any verb's body runs,
// so a config that does not load leaves the recorded partition and state untouched.
func configLoadError(file string, cause error) error {
	switch {
	case errors.Is(cause, languageconfig.ErrUnusable):
		return configLoadRefusal{message: fmt.Sprintf("webster: %s did not load: %v, then re-run the verb", file, cause), invalid: true, cause: cause}
	case errors.Is(cause, batcher.ErrRetiredKey):
		// The retired-key cause already ends in the reconcile way forward.
		return configLoadRefusal{message: fmt.Sprintf("webster: %s did not load: %v, then re-run the verb", file, cause), invalid: true, cause: cause}
	case errors.Is(cause, configengine.ErrInvalid):
		return configLoadRefusal{message: fmt.Sprintf("webster: %s did not load: %v; way forward: fix %s under _lym/config, then re-run the verb", file, cause, file), invalid: true, cause: cause}
	default:
		return configLoadRefusal{message: fmt.Sprintf("webster: %s could not be read: %v; way forward: transient, re-run the verb", file, cause), cause: cause}
	}
}

// setRunner stores runner on c and derives the starter and master starter from it;
// it also stores the engine set and the reed engine, so the adaptation lives in one place.
func (c *websterCLI) setRunner(runner *shuttleengine.Runner, engines shuttleengine.EngineSet, reedEngine shuttleengine.ReedOps) {
	c.runner = runner
	c.starter = runner
	c.masterStarter = runnerMasterStarter{runner: runner}
	c.engines = engines
	c.reed = reedEngine
}
