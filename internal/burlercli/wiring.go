// wiring.go holds burler's mode decision and the engine-stack construction for whichever mode wins.
// resolvePersistentPreRun resolves cwd, asks preflight.ResolveMode for the mode, and hands both to wire.
// The mode decision lives in wire so a test can drive it with a given preflight.Mode.
// Driving the real pre-run would reach lymcwd.Resolve and its git spawn, which breaks the Test Tier Purity Invariant.

package burlercli

import (
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
)

// burlerProgram is the program name burler signs its notices to the orch with.
const burlerProgram = "BURLER"

// wireModule holds burler's per-CLI data for internal/cliwire's shared wiring prologue.
// cliwire names no caller: no production file in that package may name webster or burler.
// The message-bearing data that varies by caller therefore lives here, with the caller.
//
// Plan is nil because burler parses no plan.
// A nil Plan makes ResolveStandalone skip plan-dir resolution entirely.
var wireModule = cliwire.Module{
	Name:             "burler",
	StateArtifacts:   "instruction files, shuttle run directories and trace logs",
	TargetRole:       "the repository it reviews",
	TargetRecourse:   "Review a target",
	HubTargetSubject: "the anchor path is already the target",
}

// wire computes hub or standalone mode from loc and mode, then builds the whole engine stack onto c:
// the module configs, the reed and claude engines, the shuttleengine.Runner, and the burlerengine.Engine.
//
// mode == preflight.ModeHub selects hub mode, because loc's paths are real.
// preflight.ModeStandalone selects standalone mode.
// It covers a plain downloaded git repository and an unresolvable cwd alike.
// preflight.ResolveMode returns a nil Location for both, and wire cannot tell them apart.
//
// There is no plan-dir flag parameter, because burler parses no plan.
//
// wire never sees the refused case: a ResolveMode error aborts the pre-run before wire is called.
// A refusal is a resolution verdict rather than a wiring choice, so mode carries only two reachable values here.
//
// cwd is the already-resolved working directory.
// wire uses it as standalone's --target-dir default and as the base for relative flag values.
// wire never resolves cwd itself.
// stencilsDirFlag and targetDirFlag are the raw values of the two persistent flags, as parsed;
// an empty string means the operator passed none.
//
// Both are made absolute here, via cliwire.ResolveToldDir.
// This is the one boundary that still knows the directory the operator typed them from.
// A relative value stored verbatim resolves against this process's cwd,
// but the pane burler spawns runs at the target (standalone) or the anchor (hub).
// Every other told path in this codebase is required absolute for the same reason.
//
// wire performs no cwd resolution and spawns no process.
// Every path it touches is supplied by the caller (loc, cwd) or read from the filesystem,
// so a test can drive it directly.
//
// wireStandalone takes no loc, because a standalone session must never read a fictional Location.
func (c *burlerCLI) wire(loc *lymcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, targetDirFlag string) error {
	stencilsDir := cliwire.ResolveToldDir(cwd, stencilsDirFlag)

	if mode == preflight.ModeHub {
		return c.wireHub(loc, stencilsDir, targetDirFlag)
	}
	return c.wireStandalone(cwd, stencilsDir, targetDirFlag)
}

// wireHub builds the engine stack for hub mode.
// It loads every module config over the anchor path and builds the geometry with hubgeom.
// It wires the reed and claude engines into a shuttleengine.Runner.
//
// stencilsDirOverride arrives already absolute or empty, because wire made it so.
// This function never sees a raw flag value and must not honour one.
func (c *burlerCLI) wireHub(loc *lymcwd.Location, stencilsDirOverride, targetDirFlag string) error {
	if err := wireModule.RefuseTargetDirInHubMode(targetDirFlag); err != nil {
		return err
	}

	// Both configs anchor at loc.AnchorPath(), the worktree the operator is standing in.
	// They never anchor at WorktreeRoot or at a fabric sibling.
	// hubgeom.BurlerGeometry reports that same anchor path as its WorktreeRoot.
	shuttleCfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(loc), "shuttle")
	if err != nil {
		return err
	}

	// lenses.Load degrades to the template on an absent lenses.yaml.
	// Its only errors are a read or decode failure.
	lensesCfg, err := lenses.Load(fabricengine.ConfigDirs(loc))
	if err != nil {
		return err
	}

	reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(loc), "reed")
	if err != nil {
		return err
	}

	language, err := languageconfig.Load(fabricengine.ConfigDirs(loc))
	if err != nil {
		return err
	}
	languageSkills := language.Skills

	stencilsDir := fabricengine.StencilsDir(loc.HubPath)
	if stencilsDirOverride != "" {
		// Check the told directory through the same descriptor method standalone's prologue uses,
		// so the two modes refuse the same inputs.
		// Unchecked, a mistyped --stencils-dir fails only at the first instruction render.
		// By then the run lock and substrate boot have already run.
		// Refused here, it costs nothing.
		if err := wireModule.RefuseUnreadableStencilsDir(stencilsDirOverride); err != nil {
			return err
		}
		stencilsDir = stencilsDirOverride
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
		_, err := orchcli.NotifyAsker(loc, burlerProgram, line)
		return err
	})

	// A `lym burler` run is not a tapestry run and has no friction directory in either mode.
	burlerGeom, err := hubgeom.BurlerGeometry(loc)
	if err != nil {
		return err
	}
	burlerGeom.DefaultEngine = shuttleCfg.Engine
	c.engine = burlerengine.New(seatengine.RunnerShuttle(runner), burlerGeom, lensesCfg, stencilsDir, "", languageSkills)
	c.languageSkills = languageSkills
	c.mode = "hub"
	c.stateDir = ""
	c.stencilsDir = stencilsDir
	return nil
}

// wireStandalone builds the engine stack for standalone mode.
// It runs the shared standalone prologue through wireModule.ResolveStandalone.
// It then wires the reed and claude engines into a runner built by shuttleengine.NewDetachedRunner.
// The standalone anchor, the derived state directory, lies outside the target worktree root by design.
// NewDetachedRunner is built for that layout.
//
// stencilsDirOverride arrives already absolute or empty, because wire made it so.
// This function never sees a raw flag value and must not honour one.
func (c *burlerCLI) wireStandalone(cwd, stencilsDirOverride, targetDirFlag string) error {
	res, err := wireModule.ResolveStandalone(cliwire.StandaloneRequest{
		Cwd:             cwd,
		StencilsDirFlag: stencilsDirOverride,
		TargetDirFlag:   targetDirFlag,
	})
	if err != nil {
		return err
	}

	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: res.StateDir}, "shuttle")
	if err != nil {
		return err
	}
	// lenses.Load degrades to the template on an absent lenses.yaml.
	lensesCfg, err := lenses.Load(configengine.Dirs{Anchor: res.StateDir})
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: res.StateDir}, "reed")
	if err != nil {
		return err
	}

	reedGeom := standalonegeom.ReedGeometry(res.Target, res.StateDir, res.Hash8)
	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	// The standalone anchor is the derived state directory, deliberately outside the worktree root.
	// NewRunner's containment assertion would refuse that layout, so the detached runner is used.
	engines, err := shuttleengines.New(shuttleCfg, configengine.Dirs{Anchor: res.StateDir})
	if err != nil {
		return err
	}
	runner := shuttleengine.NewDetachedRunner(reedEngine, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)
	runner.SetPause(standalonegeom.PauseGeometry(res.StateDir))

	// Standalone's reed session lives on its own derived geometry, which no CLI verb reaches: `lym reed up` is hub-only.
	// The run verb therefore boots it in-process through this seam.
	// It is assigned here and executed only by run, so wiring itself never boots a tmux server.
	//
	// The standalone reed is booted by a long-lived supervising run, which exists for the session's working lifetime.
	// Hub mode lacks that process, which is why it needs a detached daemon at all;
	// reusing the run process is the smaller mechanism.
	// Standalone computes no hub lock path and spawns no daemon.
	c.reedUp = func() error {
		_, err := reedEngine.Up()
		return err
	}

	// A `lym burler` run is not a tapestry run and has no friction directory in either mode.
	burlerGeom := standalonegeom.BurlerGeometry(res.Target, res.StateDir, res.Hash8)
	burlerGeom.DefaultEngine = shuttleCfg.Engine
	c.engine = burlerengine.New(seatengine.RunnerShuttle(runner), burlerGeom, lensesCfg, res.StencilsDir, "", c.languageSkills)
	c.mode = "standalone"
	c.stateDir = res.StateDir
	c.stencilsDir = res.StencilsDir
	return nil
}
