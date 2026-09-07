// wiring.go implements wire, the package-private method resolvePersistentPreRun (cli.go) delegates
// to once it has resolved cwd and told mode via preflight.ResolveMode: the mode decision itself, and
// the whole engine stack construction for whichever mode wins.
// The mode decision lives HERE, inside the extracted function, rather than upstream in
// resolvePersistentPreRun, precisely so a test can drive it with a told preflight.Mode and stay
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
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

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
// operator typed them from. A relative value stored verbatim is not a smaller version of an absolute
// one: this process resolves it against ITS cwd while the pane webster spawns runs at the target
// (standalone) or the anchor (hub), so one string named two different directories, and the
// standalone default-vs-override comparison below could never match a relative spelling of the
// default. Every other told path in this codebase is required absolute for exactly that reason.
//
// wire performs no cwd resolution and spawns no process -- every path it touches is either supplied by
// the caller (loc, cwd) or a plain filesystem read (config loads, the standalone stencil seed) -- so a
// test can drive it directly and stay inside the Test Tier Purity Invariant.
func (c *websterCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, planDirFlag, targetDirFlag string) error {
	stencilsDir := resolveToldDir(cwd, stencilsDirFlag)
	planDir := resolveToldDir(cwd, planDirFlag)

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
	if stencilsDir != "" {
		geom.StencilsDir = stencilsDir
	}
	if planDir != "" {
		geom.PlanDir = planDir
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
// called -- an immediate redirect of the durable trace sink to standalonegeom.LogsDir(stateDir),
// which is what keeps a standalone invocation from writing trace files into the operator's
// repository, standalonegeom's geometry builders over the derived state directory, every module
// config and the model registry loaded over the same state directory, a pinned
// websterengine.NeverMatches RefMatcher, a shuttleengine.NewDetachedRunner-constructed runner --
// standalone's anchor (the derived state directory) is deliberately outside its worktree root (the
// target), which NewRunner's containment assertion would refuse -- and a nil fabric opener.
//
// stencilsDir and planDir arrive already absolute (or empty), made so by wire -- this function never
// sees a raw flag value and must never start honouring one, since the default-vs-override comparison
// below is a path equality and a relative spelling of the default could never satisfy it.
func (c *websterCLI) wireStandalone(cwd, stencilsDir, planDir, targetDirFlag string) error {
	target, err := resolveStandaloneTarget(cwd, targetDirFlag)
	if err != nil {
		return err
	}

	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		return err
	}
	if err := refuseNestedStandaloneGeometry("webster", target, stateDir); err != nil {
		return err
	}
	// The redirect runs before any other statement in this function because the durable sink is
	// armed lazily on the first Info-or-above record: if anything below logged first, the sink
	// would already be bound to the operator's target repository and this redirect would be too
	// late to matter.
	logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)

	geom := standalonegeom.WebsterGeometry(target, stateDir)
	reedGeom := standalonegeom.ReedGeometry(target, stateDir, hash8)

	if stencilsDir != "" {
		// An operator who named a curated stencil set must not have it rewritten from under them --
		// seed only the standalone DEFAULT, never an explicit override.
		geom.StencilsDir = stencilsDir
	} else if _, err := stencilstore.Reconcile(geom.StencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), ""); err != nil {
		// Unlike the root pre-run's best-effort, logged-only seed pass, nothing else will ever
		// create this directory: a reconcile failure here is a hard error, since every prompt render
		// would otherwise fail later with a far less informative message.
		return fmt.Errorf("webster: seed the standalone stencils directory %s: %w", geom.StencilsDir, err)
	}

	if planDir != "" {
		// Record whether the flag actually moved the plan off the standalone default: the run verb
		// refuses to spawn Master over a moved plan, because Master's own in-pane verb invocations
		// (status, begin-batch, record-batch — typed from the stencil, flagless) resolve the
		// DEFAULT plan directory and would wire-refuse against a plan they cannot see (found live
		// in crucible round fable5-high-r3, F-A3). Every other verb keeps honoring the override —
		// an operator driving bracket verbs by hand passes the flag on each call.
		//
		// Both sides of the comparison are absolute and cleaned by construction — planDir by wire's
		// own resolveToldDir, geom.PlanDir by standalonegeom — so a flag value that names the
		// default location is recognised as such however the operator spelled it.
		if planDir != filepath.Clean(geom.PlanDir) {
			c.standalonePlanDirOverridden = true
			c.standalonePlanDirDefault = geom.PlanDir
		}
		geom.PlanDir = planDir
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

// refuseNestedStandaloneGeometry refuses a standalone target and derived state directory that are
// not disjoint, naming module in every message so an operator reading a live error knows which CLI
// refused.
//
// Standalone geometry's whole premise is a state directory OUTSIDE the repository being driven:
// state.json, run locks, rendered prompts, shuttle run directories and trace logs all land under it,
// and none of them may appear inside the operator's own checkout.
// shuttleengine.NewDetachedRunner asserts the same disjointness -- but it asserts it LATE, on a
// Runner already constructed after this CLI has booted a tmux server and taken the run lock, and its
// message blames "a subpath-anchored hub geometry handed to the wrong constructor", which is not
// what happened here and offers the operator no lever at all.
//
// What actually happened is a state home nested under the target: a dotfiles repository rooted at
// the home directory, or an XDG_STATE_HOME deliberately pointed somewhere inside the checkout. That
// is an environment fact, it is fixable, and the fix is named here -- before any substrate is
// booted, which is the only point at which a refusal costs nothing to recover from.
//
// The reverse nesting (a target inside the state directory) is refused by the same guard for the
// same reason, with its own message: the lever there is the target, not the state home.
func refuseNestedStandaloneGeometry(module, target, stateDir string) error {
	if pathContains(target, stateDir) {
		return fmt.Errorf("%s: the derived state directory %s lies inside the standalone target %s: standalone mode keeps its state, locks, rendered prompts and trace logs strictly outside the repository it drives, so the two must be disjoint. The state home is nested under the target -- a repository rooted at your home directory is the usual cause. Point XDG_STATE_HOME (LOCALAPPDATA on Windows) at a directory outside %s and re-run", module, stateDir, target, target)
	}
	if pathContains(stateDir, target) {
		return fmt.Errorf("%s: the standalone target %s lies inside the derived state directory %s: standalone mode keeps its state, locks, rendered prompts and trace logs strictly outside the repository it drives, so the two must be disjoint. Drive a target outside the state home, or point XDG_STATE_HOME (LOCALAPPDATA on Windows) elsewhere, and re-run", module, target, stateDir)
	}
	return nil
}

// pathContains reports whether inner is outer itself or a descendant of it, computed the way
// shuttleengine's own told-path assertions compute it -- filepath.Rel plus a ".." prefix test -- so
// the CLI-boundary refusal and the constructor assertion it front-runs agree about what "nested"
// means. Both paths are already absolute and cleaned by their producers.
func pathContains(outer, inner string) bool {
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveToldDir makes one told directory flag absolute against cwd: the empty string stays empty
// (the operator passed no flag, and each mode computes its own default), an absolute value is
// cleaned, and a relative one is joined onto cwd.
//
// Every path this module hands downstream must be absolute. A relative flag value does not fail, it
// silently means two different directories: the CLI process resolves it against ITS working
// directory while the pane webster spawns runs at the standalone target or the hub anchor, and the
// standalone default-vs-override check is a path equality a relative spelling can never satisfy.
// Resolving happens once, at the wiring boundary, because that is the last point that still knows
// which working directory the operator typed the flag from — the same reason
// resolveStandaloneTarget has always done it for --target-dir.
func resolveToldDir(cwd, flagValue string) string {
	if flagValue == "" {
		return ""
	}
	if filepath.IsAbs(flagValue) {
		return filepath.Clean(flagValue)
	}
	return filepath.Join(cwd, flagValue)
}

// resolveStandaloneTarget resolves standalone mode's --target-dir: cwd when targetDirFlag is empty,
// or targetDirFlag made absolute against cwd otherwise. The result is always absolute, which is
// standalonestate.Derive's own precondition.
func resolveStandaloneTarget(cwd, targetDirFlag string) (string, error) {
	if targetDirFlag == "" {
		return cwd, nil
	}
	return resolveToldDir(cwd, targetDirFlag), nil
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
