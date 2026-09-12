// wiring.go implements wire, the package-private method resolvePersistentPreRun (cli.go) delegates
// to once it has resolved cwd and told mode via preflight.ResolveMode: the mode decision itself, and
// the whole engine stack construction for whichever mode wins.
// The mode decision lives HERE, inside the extracted function, rather than upstream in
// resolvePersistentPreRun, precisely so a test can drive it with a told preflight.Mode and stay
// tier 1 -- driving the real pre-run would reach lyxcwd.Resolve and its git spawn.

package burlercli

import (
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
)

// wireModule carries burler's own per-CLI variance for internal/cliwire's shared wiring prologue --
// the message-bearing data cliwire needs but does not itself declare, since no production file in
// that package may name webster or burler. cliwire carries the shared implementation; the data that
// varies by caller lives here, with the caller, exactly the way internal/shedrecipe's constructors
// live in that package while the rows that vary live outside it.
//
// Plan is nil because burler parses no plan -- a nil Plan is what makes ResolveStandalone skip
// plan-dir resolution entirely rather than each caller writing its own branch.
var wireModule = cliwire.Module{
	Name:             "burler",
	StateArtifacts:   "instruction files, shuttle run directories and trace logs",
	TargetRole:       "the repository it reviews",
	TargetRecourse:   "Review a target",
	HubTargetSubject: "the anchor path is already the target",
}

// wire computes hub-or-standalone mode from loc/mode -- the *lyxcwd.Location and preflight.Mode a
// preflight.ResolveMode(cwd) call already told it -- and builds the whole engine stack onto c: the
// module configs, the reed/claude engines, the shuttleengine.Runner, and the told burlerengine.Engine.
//
// mode == preflight.ModeHub selects hub mode, because the Location's paths are real.
// preflight.ModeStandalone selects standalone mode, which covers BOTH a plain downloaded git
// repository and an unresolvable cwd -- preflight.ResolveMode returns a nil Location for each, and
// the two are indistinguishable from here.
//
// There is no planDirFlag parameter here: that flag is webster-only, because burler parses no plan.
//
// wire never sees the refused case at all: a ResolveMode error aborts upstream in
// resolvePersistentPreRun, because a refusal is a resolution verdict rather than a wiring choice --
// which is why mode carries only two reachable values here.
//
// cwd is the already-resolved cwd resolvePersistentPreRun read via lyxcwd.CwdFrom -- wire needs it
// as standalone's --target-dir default and as the base every relative flag value is made absolute
// against, never to resolve or re-resolve anything itself.
// stencilsDirFlag and targetDirFlag are the two persistent flags' raw, as-parsed values (empty
// string when the operator did not pass one).
//
// Both are made absolute HERE, at the one boundary that still knows which working directory the
// operator typed them from, via cliwire.ResolveToldDir. A relative value stored verbatim is not a
// smaller version of an absolute one: this process resolves it against ITS cwd while the pane burler
// spawns runs at the target (standalone) or the anchor (hub), so one string named two different
// directories. Every other told path in this codebase is required absolute for exactly that reason.
//
// wireStandalone deliberately does not take loc: a standalone session must never read a fictional
// Location.
//
// wire performs no cwd resolution and spawns no process -- every path it touches is either supplied
// by the caller (loc, cwd) or a plain filesystem read (config loads, the standalone stencil seed) --
// so a test can drive it directly and stay inside the Test Tier Purity Invariant.
func (c *burlerCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, targetDirFlag string) error {
	stencilsDir := cliwire.ResolveToldDir(cwd, stencilsDirFlag)

	if mode == preflight.ModeHub {
		return c.wireHub(loc, stencilsDir, targetDirFlag)
	}
	return c.wireStandalone(cwd, stencilsDir, targetDirFlag)
}

// wireHub builds the engine stack for hub mode, reproducing today's PersistentPreRunE body
// byte-for-byte in resolved values: every module config loaded over the anchor path, hubgeom's
// geometry builders, and the reed/claude engines wired into a shuttleengine.Runner.
//
// stencilsDirOverride arrives already absolute (or empty), made so by wire -- this function never
// sees a raw flag value and must never start honouring one.
func (c *burlerCLI) wireHub(loc *lyxcwd.Location, stencilsDirOverride, targetDirFlag string) error {
	if err := wireModule.RefuseTargetDirInHubMode(targetDirFlag); err != nil {
		return err
	}

	// Both configs anchor at loc.AnchorPath() -- the worktree the operator is actually standing in,
	// never WorktreeRoot or any fabric sibling. hubgeom.BurlerGeometry now tells burlerengine that
	// same anchor path as its own WorktreeRoot, so the geometry beside this wiring agrees with it.
	anchorPath := loc.AnchorPath()

	shuttleCfg, err := shuttleengine.LoadConfig(anchorPath, "shuttle")
	if err != nil {
		return err
	}

	// burlerengine.LoadConfig's only error today is a read/decode failure -- an absent burler.yaml is
	// not an error, it decodes to the zero Config (clustering then fails later, at fan resolution,
	// with a message naming `lyx config reconcile`).
	burlerCfg, err := burlerengine.LoadConfig(anchorPath)
	if err != nil {
		return err
	}

	reedCfg, err := reedengine.LoadConfig(anchorPath, "reed")
	if err != nil {
		return err
	}

	stencilsDir := fabricengine.StencilsDir(loc.HubPath)
	if stencilsDirOverride != "" {
		// The same boundary stat standalone's prologue applies, through the same descriptor method,
		// so the two modes can never drift on what a told stencils directory must be: a typo'd
		// --stencils-dir refused here costs nothing, while unchecked it failed only at the first
		// instruction render, after the run lock and substrate boot (crucible round fable-high-r7, F2).
		if err := wireModule.RefuseUnreadableStencilsDir(stencilsDirOverride); err != nil {
			return err
		}
		stencilsDir = stencilsDirOverride
	}

	reedGeom := hubgeom.ReedGeometry(loc)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	// A `lyx burler` run is not a loom run and has no friction directory in either mode.
	c.engine = burlerengine.New(runner, hubgeom.BurlerGeometry(loc), burlerCfg, stencilsDir, "")
	c.mode = "hub"
	c.stateDir = ""
	c.stencilsDir = stencilsDir
	return nil
}

// wireStandalone builds the engine stack for standalone mode by calling wireModule.ResolveStandalone
// -- internal/cliwire's single ordered standalone prologue -- and composing burler's own engine onto
// the result: the reed/claude engines wired into a shuttleengine.NewDetachedRunner-constructed
// runner, since standalone's anchor (the derived state directory) is deliberately outside its
// worktree root (the target), which NewRunner's containment assertion would refuse.
//
// See cliwire.ResolveStandalone's own doc comment for the prologue's ordering obligation and its two
// asymmetries (an explicitly-told stencilsDirFlag is read and never written in either mode; the
// derived default's Reconcile failure is a hard pre-run error), which this function's body no longer
// needs to restate.
//
// stencilsDirOverride arrives already absolute (or empty), made so by wire -- this function never
// sees a raw flag value and must never start honouring one.
func (c *burlerCLI) wireStandalone(cwd, stencilsDirOverride, targetDirFlag string) error {
	res, err := wireModule.ResolveStandalone(cliwire.StandaloneRequest{
		Cwd:             cwd,
		StencilsDirFlag: stencilsDirOverride,
		TargetDirFlag:   targetDirFlag,
	})
	if err != nil {
		return err
	}

	shuttleCfg, err := shuttleengine.LoadConfig(res.StateDir, "shuttle")
	if err != nil {
		return err
	}
	burlerCfg, err := burlerengine.LoadConfig(res.StateDir)
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(res.StateDir, "reed")
	if err != nil {
		return err
	}

	reedGeom := standalonegeom.ReedGeometry(res.Target, res.StateDir, res.Hash8)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	// Standalone's anchor (the derived state directory) is deliberately outside its worktree root
	// (the target), which NewRunner's containment assertion would refuse -- NewDetachedRunner stays.
	runner := shuttleengine.NewDetachedRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)

	// Standalone's reed session lives on its own derived geometry, which no CLI verb can reach —
	// `lyx reed up` is hub-only — so the run verb boots it in-process through this seam (see the
	// field's own doc comment). Assigned here, executed only by run, so wiring itself never boots
	// a tmux server.
	c.reedUp = func() error {
		_, err := reedEngine.Up()
		return err
	}

	// A `lyx burler` run is not a loom run and has no friction directory in either mode.
	c.engine = burlerengine.New(runner, standalonegeom.BurlerGeometry(res.Target, res.StateDir), burlerCfg, res.StencilsDir, "")
	c.mode = "standalone"
	c.stateDir = res.StateDir
	c.stencilsDir = res.StencilsDir
	return nil
}
