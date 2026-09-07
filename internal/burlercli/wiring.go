// wiring.go implements wire, the package-private method resolvePersistentPreRun (cli.go) delegates
// to once it has resolved cwd and told mode via preflight.ResolveMode: the mode decision itself, and
// the whole engine stack construction for whichever mode wins.
// The mode decision lives HERE, inside the extracted function, rather than upstream in
// resolvePersistentPreRun, precisely so a test can drive it with a told preflight.Mode and stay
// tier 1 -- driving the real pre-run would reach lyxcwd.Resolve and its git spawn.

package burlercli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
// operator typed them from. A relative value stored verbatim is not a smaller version of an absolute
// one: this process resolves it against ITS cwd while the pane burler spawns runs at the target
// (standalone) or the anchor (hub), so one string named two different directories. Every other told
// path in this codebase is required absolute for exactly that reason.
//
// wireStandalone deliberately does not take loc: a standalone session must never read a fictional
// Location.
//
// wire performs no cwd resolution and spawns no process -- every path it touches is either supplied
// by the caller (loc, cwd) or a plain filesystem read (config loads, the standalone stencil seed) --
// so a test can drive it directly and stay inside the Test Tier Purity Invariant.
func (c *burlerCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, targetDirFlag string) error {
	stencilsDir := resolveToldDir(cwd, stencilsDirFlag)

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
	if targetDirFlag != "" {
		return fmt.Errorf("burler: --target-dir is not honoured in hub mode: the anchor path is already the target, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec")
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
		stencilsDir = stencilsDirOverride
	}

	reedGeom := hubgeom.ReedGeometry(loc)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	c.engine = burlerengine.New(runner, hubgeom.BurlerGeometry(loc), burlerCfg, stencilsDir)
	c.mode = "hub"
	c.stateDir = ""
	c.stencilsDir = stencilsDir
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
//
// Immediately after standalonestate.Derive succeeds, wireStandalone redirects the durable trace sink
// to standalonegeom.LogsDir(stateDir), which is what keeps a standalone invocation from writing trace
// files into the operator's repository -- placement matters because the sink is armed lazily on the
// first Info-or-above record, so the redirect only binds if it runs before anything else in this
// function can log. Its runner is also built via shuttleengine.NewDetachedRunner rather than
// NewRunner, since standalone's anchor (the derived state directory) is deliberately outside its
// worktree root (the target), which NewRunner's containment assertion would refuse.
//
// stencilsDirOverride arrives already absolute (or empty), made so by wire -- this function never
// sees a raw flag value and must never start honouring one.
func (c *burlerCLI) wireStandalone(cwd, stencilsDirOverride, targetDirFlag string) error {
	target, err := resolveStandaloneTarget(cwd, targetDirFlag)
	if err != nil {
		return err
	}

	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		return err
	}
	if err := refuseNestedStandaloneGeometry("burler", target, stateDir); err != nil {
		return err
	}
	logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)

	var stencilsDir string
	if stencilsDirOverride != "" {
		stencilsDir = stencilsDirOverride
	} else {
		stencilsDir = standalonegeom.StencilsDir(stateDir)
		if _, err := stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), ""); err != nil {
			return fmt.Errorf("burler: seed the standalone stencils directory %s: %w", stencilsDir, err)
		}
	}

	shuttleCfg, err := shuttleengine.LoadConfig(stateDir, "shuttle")
	if err != nil {
		return err
	}
	burlerCfg, err := burlerengine.LoadConfig(stateDir)
	if err != nil {
		return err
	}
	reedCfg, err := reedengine.LoadConfig(stateDir, "reed")
	if err != nil {
		return err
	}

	reedGeom := standalonegeom.ReedGeometry(target, stateDir, hash8)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	runner := shuttleengine.NewDetachedRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, reedGeom.PaneCwd, shuttleCfg)

	// Standalone's reed session lives on its own derived geometry, which no CLI verb can reach —
	// `lyx reed up` is hub-only — so the run verb boots it in-process through this seam (see the
	// field's own doc comment). Assigned here, executed only by run, so wiring itself never boots
	// a tmux server.
	c.reedUp = func() error {
		_, err := reedEngine.Up()
		return err
	}

	c.engine = burlerengine.New(runner, standalonegeom.BurlerGeometry(target, stateDir), burlerCfg, stencilsDir)
	c.mode = "standalone"
	c.stateDir = stateDir
	c.stencilsDir = stencilsDir
	return nil
}

// refuseNestedStandaloneGeometry refuses a standalone target and derived state directory that are
// not disjoint, naming module in every message so an operator reading a live error knows which CLI
// refused.
//
// Standalone geometry's whole premise is a state directory OUTSIDE the repository being reviewed:
// burler's instruction directory, shuttle run directories and trace logs all land under it, and none
// of them may appear inside the operator's own checkout.
// shuttleengine.NewDetachedRunner asserts the same disjointness -- but it asserts it LATE, on a
// Runner already constructed after this CLI has booted a tmux server, and its message blames "a
// subpath-anchored hub geometry handed to the wrong constructor", which is not what happened here
// and offers the operator no lever at all.
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
		return fmt.Errorf("%s: the derived state directory %s lies inside the standalone target %s: standalone mode keeps its instruction files, shuttle run directories and trace logs strictly outside the repository it reviews, so the two must be disjoint. The state home is nested under the target -- a repository rooted at your home directory is the usual cause. Point XDG_STATE_HOME (LOCALAPPDATA on Windows) at a directory outside %s and re-run", module, stateDir, target, target)
	}
	if pathContains(stateDir, target) {
		return fmt.Errorf("%s: the standalone target %s lies inside the derived state directory %s: standalone mode keeps its instruction files, shuttle run directories and trace logs strictly outside the repository it reviews, so the two must be disjoint. Review a target outside the state home, or point XDG_STATE_HOME (LOCALAPPDATA on Windows) elsewhere, and re-run", module, target, stateDir)
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
// directory while the pane burler spawns runs at the standalone target or the hub anchor. Resolving
// happens once, at the wiring boundary, because that is the last point that still knows which
// working directory the operator typed the flag from — the same reason resolveStandaloneTarget has
// always done it for --target-dir.
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
// or targetDirFlag made absolute against cwd otherwise.
// The result is always absolute, which is standalonestate.Derive's own precondition, because Derive
// normalises through EvalSymlinks+Clean and compares case-insensitively on Windows, so two spellings
// of the same directory must not produce different <state> values.
func resolveStandaloneTarget(cwd, targetDirFlag string) (string, error) {
	if targetDirFlag == "" {
		return cwd, nil
	}
	return resolveToldDir(cwd, targetDirFlag), nil
}
