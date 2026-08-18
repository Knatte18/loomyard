// wiring.go implements wire, the package-private method resolvePersistentPreRun (cli.go) delegates
// to once it has resolved cwd and told mode via preflight.ResolveMode: the mode decision itself, and
// the whole engine stack construction for whichever mode wins.
// The mode decision lives HERE, inside the extracted function, rather than upstream in
// resolvePersistentPreRun, precisely so a test can drive it with a told preflight.Mode and stay
// tier 1 -- driving the real pre-run would reach lyxcwd.Resolve and its git spawn.

package perchcli

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// wire computes hub-or-standalone mode from loc/mode -- the *lyxcwd.Location and preflight.Mode a
// preflight.ResolveMode(cwd) call already told it -- and builds the whole engine stack onto c: the
// module configs, the reed/claude engines, the shuttleengine.Runner, the nested burlerengine.Engine,
// and the perch geometry the run verb constructs its own perchengine.Engine from.
//
// mode == preflight.ModeHub selects hub mode, because the Location's paths are real.
// preflight.ModeStandalone selects standalone mode, which covers BOTH a plain downloaded git
// repository and an unresolvable cwd -- preflight.ResolveMode returns a nil Location for each, and
// the two are indistinguishable from here.
//
// There is no planDirFlag parameter here: that flag is webster-only, because perch parses no plan.
//
// wire never sees the refused case at all: a ResolveMode error aborts upstream in
// resolvePersistentPreRun, because a refusal is a resolution verdict rather than a wiring choice --
// which is why mode carries only two reachable values here.
//
// cwd is the already-resolved cwd resolvePersistentPreRun read via lyxcwd.CwdFrom -- wire needs it
// only as standalone's --target-dir default, never to resolve or re-resolve anything itself.
// stencilsDirFlag and targetDirFlag are the two persistent flags' raw, as-parsed values (empty
// string when the operator did not pass one).
//
// wireStandalone deliberately does not take loc: a standalone session must never read a fictional
// Location.
//
// wire performs no cwd resolution and spawns no process -- every path it touches is either supplied
// by the caller (loc, cwd) or a plain filesystem read (config loads, the standalone stencil seed) --
// so a test can drive it directly and stay inside the Test Tier Purity Invariant.
func (c *perchCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, targetDirFlag string) error {
	if mode == preflight.ModeHub {
		return c.wireHub(loc, stencilsDirFlag, targetDirFlag)
	}
	return c.wireStandalone(cwd, stencilsDirFlag, targetDirFlag)
}

// wireHub builds the engine stack for hub mode, reproducing today's PersistentPreRunE body
// byte-for-byte in resolved values: every module config loaded over the anchor path, hubgeom's
// geometry builders, and the reed/claude engines wired into a shuttleengine.Runner.
func (c *perchCLI) wireHub(loc *lyxcwd.Location, stencilsDirFlag, targetDirFlag string) error {
	if targetDirFlag != "" {
		return fmt.Errorf("perch: --target-dir is not honoured in hub mode: the worktree is already the target, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec")
	}

	// Every config is anchored at anchorPath, matching burlercli/shuttlecli's own resolution: the
	// worktree the operator is actually standing in, never WorktreeRoot or any fabric sibling.
	anchorPath := loc.AnchorPath()

	shuttleCfg, err := shuttleengine.LoadConfig(anchorPath, "shuttle")
	if err != nil {
		return err
	}

	reedCfg, err := reedengine.LoadConfig(anchorPath, "reed")
	if err != nil {
		return err
	}

	// Loaded ONCE here, at the same anchorPath anchor every config load above uses, and reused for
	// both perchCfg's judge_model resolution (via LoadConfigWithRegistry) and decodeProfile's
	// profile-field resolution in runCmd -- models.yaml is read exactly once per invocation.
	modelReg, err := modelspec.LoadRegistry(anchorPath)
	if err != nil {
		return err
	}

	perchCfg, err := perchengine.LoadConfigWithRegistry(anchorPath, "perch", modelReg)
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

	stencilsDir := fabricengine.StencilsDir(loc.HubPath)
	if stencilsDirFlag != "" {
		stencilsDir = stencilsDirFlag
	}

	reedGeom := hubgeom.ReedGeometry(loc)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	// The nested burler engine receives the SAME stencilsDir the run verb later passes to
	// perchengine.Engine.Run -- never a second fabricengine.StencilsDir call -- so a burler round
	// behaves the same whether driven directly or nested inside a perch block.
	c.burlerEngine = burlerengine.New(runner, hubgeom.BurlerGeometry(loc), burlerCfg, stencilsDir)
	c.runner = runner
	c.perchCfg = perchCfg
	c.modelReg = modelReg
	c.perchGeom = hubgeom.PerchGeometry(loc)
	// Anchored at perchGeom.AnchorPath, like the config loads above and like Layout.LyxDir itself:
	// the initialized _lyx (the fabric junction, mirrored at <fabric>/<RelPath>/_lyx) lives at the
	// directory lyx init ran in, which is Cwd -- not necessarily the git worktree root. Anchoring at
	// WorktreeRoot would, in a nested-initialized repo, write run dirs into an un-junctioned _lyx the
	// fabric commit's RelPath-scoped pathspec never includes, silently stranding every artifact
	// outside fabric. TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd enforces it.
	c.runDirBase = perchengine.RunsDir(c.perchGeom.AnchorPath)
	// Anchored at the same perchGeom.AnchorPath as runDirBase -- the never-tracked half of the pair:
	// run.lock, state.json.lock, and the pause flag live here instead of inside _lyx, so a block's
	// two directories can never disagree about which anchor they belong to.
	c.scratchDirBase = perchengine.ScratchDir(c.perchGeom.AnchorPath)
	c.stencilsDir = stencilsDir
	c.anchorRel = loc.AnchorRel
	// openFabric stays a closure and is not opened here: fabricengine.Open stat-checks the paired
	// sibling and would fail the pre-run in healthy-but-unwired locations.
	c.openFabric = func() (*fabricengine.Fabric, error) { return fabricengine.Open(loc) }
	c.mode = "hub"
	c.stateDir = ""
	return nil
}

// wireStandalone builds the engine stack for standalone mode: the already-absolute --target-dir
// (defaulted to cwd when unset), standalonestate.Derive over it -- the only place Derive is ever
// called in this package -- standalonegeom's geometry builders over the derived state directory,
// every module config loaded over the same state directory, and the reed/claude engines wired into a
// shuttleengine.Runner exactly as the hub branch does.
//
// Two asymmetries are worth calling out, since a reader will otherwise try to "simplify" them away.
// First, an explicitly-told stencilsDirFlag is read and never written in either mode -- that is what
// makes the read-only characterisation literally true and protects a curated stencil set. Second, the
// derived default's Reconcile failure is a hard pre-run error rather than the root pre-run's
// best-effort logged seed, because nothing else will ever create this directory and a silent failure
// would otherwise resurface much later as an opaque prompt-render error. The empty fourth Reconcile
// argument is the "no source tree here" value that keeps the port-back drift warning silent --
// standalone genuinely has no contracts/stencils source tree beside it.
func (c *perchCLI) wireStandalone(cwd, stencilsDirFlag, targetDirFlag string) error {
	target, err := resolveStandaloneTarget(cwd, targetDirFlag)
	if err != nil {
		return err
	}

	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		return err
	}

	var stencilsDir string
	if stencilsDirFlag != "" {
		stencilsDir = stencilsDirFlag
	} else {
		stencilsDir = standalonegeom.StencilsDir(stateDir)
		if _, err := stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), ""); err != nil {
			return fmt.Errorf("perch: seed the standalone stencils directory %s: %w", stencilsDir, err)
		}
	}

	shuttleCfg, err := shuttleengine.LoadConfig(stateDir, "shuttle")
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(stateDir, "reed")
	if err != nil {
		return err
	}

	modelReg, err := modelspec.LoadRegistry(stateDir)
	if err != nil {
		return err
	}

	perchCfg, err := perchengine.LoadConfigWithRegistry(stateDir, "perch", modelReg)
	if err != nil {
		return err
	}

	burlerCfg, err := burlerengine.LoadConfig(stateDir)
	if err != nil {
		return err
	}

	reedGeom := standalonegeom.ReedGeometry(target, stateDir, hash8)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	// The nested burler's geometry is byte-identical to what burlercli builds for the same target, so
	// a burler round behaves the same whether driven directly or through perch.
	c.burlerEngine = burlerengine.New(runner, standalonegeom.BurlerGeometry(target, stateDir), burlerCfg, stencilsDir)
	c.runner = runner
	c.perchCfg = perchCfg
	c.modelReg = modelReg
	c.perchGeom = standalonegeom.PerchGeometry(target, stateDir)
	// c.runDirBase and c.scratchDirBase are computed from c.perchGeom.AnchorPath by the same two
	// expressions the hub branch uses, so <state> plays the hub's role for both.
	c.runDirBase = perchengine.RunsDir(c.perchGeom.AnchorPath)
	c.scratchDirBase = perchengine.ScratchDir(c.perchGeom.AnchorPath)
	c.stencilsDir = stencilsDir
	c.anchorRel = ""
	// openFabric is nil rather than a closure: standalone has no fabric repo by construction -- not a
	// broken one, an absent one -- so nil is the honest representation, and a closure that would
	// stat-fail if called is not.
	c.openFabric = nil
	c.mode = "standalone"
	c.stateDir = stateDir
	return nil
}

// resolveStandaloneTarget resolves standalone mode's --target-dir: cwd when targetDirFlag is empty,
// or targetDirFlag resolved to an absolute path (relative to cwd) otherwise.
// The result is always absolute, which is standalonestate.Derive's own precondition, because Derive
// normalises through EvalSymlinks+Clean and compares case-insensitively on Windows, so two spellings
// of the same directory must not produce different <state> values.
//
// Three near-identical five-line functions across three CLI packages (webstercli, burlercli,
// perchcli) is the deliberate call over a shared package that would exist only to hold one of them;
// revisit if a fourth standalone CLI appears.
func resolveStandaloneTarget(cwd, targetDirFlag string) (string, error) {
	if targetDirFlag == "" {
		return cwd, nil
	}
	if filepath.IsAbs(targetDirFlag) {
		return filepath.Clean(targetDirFlag), nil
	}
	return filepath.Join(cwd, targetDirFlag), nil
}
