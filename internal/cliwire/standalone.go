// standalone.go holds StandaloneRequest and Standalone, the types of the standalone prologue.

package cliwire

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/contracts/specs"
	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// StandaloneRequest carries one standalone-mode wiring call's raw flag values, as parsed.
// ResolveStandalone makes them absolute itself, through ResolveToldDir.
//
// Each wire has already resolved these values for its own hub branch, so this resolves them again.
// That is harmless, because ResolveToldDir is idempotent on an absolute input.
// The repeat is intended: do not hoist the wires' resolution back out of this request to remove it.
type StandaloneRequest struct {
	Cwd             string
	StencilsDirFlag string
	PlanDirFlag     string
	TargetDirFlag   string
}

// Standalone is the resolved target and derived state identity of a standalone-mode run.
// It also carries every told directory the caller needs to finish building its engine stack.
//
// DefaultPlanDir is the mode's own default, carried so a caller can record it for a refusal's
// recourse text. PlanDir and PlanDirOverridden are the zero value when the module carries no Plan.
type Standalone struct {
	Target      string
	StateDir    string
	Hash8       string
	StencilsDir string
	// SpecsDir is the resolved standalone deployed-specs directory.
	// No flag sets it, since there is no --specs-dir, so it is always the derived default.
	SpecsDir          string
	PlanDir           string
	PlanDirOverridden bool
	DefaultPlanDir    string
}

// ResolveStandalone performs the whole ordered standalone prologue.
// It resolves the target and derives the state identity, then refuses nested geometry.
// It redirects the durable sink, seeds the stencils and specs directories, and resolves the plan directory.
// Every fallible step returns the zero Standalone alongside its error, and no step is best-effort.
//
// The durable sink is armed lazily, on the first Info-or-above record.
// The redirect therefore binds only if no statement before it logs.
// It cannot come first, because Derive's stateDir is what tells it where to point.
// Every statement above the redirect must stay log-free.
// No logging statement may be added below the redirect where it could be hoisted above it.
//
// The nested-geometry refusal runs after standalonestate.Derive and before the sink redirect.
// It also runs before any substrate boot.
// The stencils seed failure is a hard error here, because nothing else will ever create that directory.
// The seed runs only for the derived default.
// A curated stencil set named by --stencils-dir is never rewritten from under the operator.
// These asymmetries must not be simplified away.
//
// The specs seed runs unconditionally, outside the stencils-override guard.
// --stencils-dir overrides the stencils set only, and no flag overrides specs.
// An edited deployed spec is still protected: the reconcile policy warns on it and never overwrites it.
// Like the stencils seed, its failure is a hard error, because nothing else creates this directory.
// Otherwise a reconcile failure would surface much later as a less informative prompt-render failure.
func (m Module) ResolveStandalone(req StandaloneRequest) (Standalone, error) {
	target, err := m.resolveStandaloneTarget(req.Cwd, req.TargetDirFlag)
	if err != nil {
		return Standalone{}, err
	}

	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		return Standalone{}, err
	}

	if err := m.refuseNestedStandaloneGeometry(target, stateDir); err != nil {
		return Standalone{}, err
	}

	// The redirect must come before any statement below that can log.
	logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)

	stencilsDir := ResolveToldDir(req.Cwd, req.StencilsDirFlag)
	// A curated stencil set that is missing must refuse here, before the caller takes the run lock and boots substrate.
	// Refusing later would fail at the first prompt render.
	if err := m.RefuseUnreadableStencilsDir(stencilsDir); err != nil {
		return Standalone{}, err
	}
	if stencilsDir == "" {
		// Seed only the standalone default: a curated stencil set the operator named is never rewritten.
		stencilsDir = standalonegeom.StencilsDir(stateDir)
		if _, err := stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev(), buildinfo.IsProduction(), buildvcs.Running().Clean()), stencilstore.Source{}); err != nil {
			// Nothing else creates this directory, so a reconcile failure is a hard error here.
			// Otherwise every prompt render would fail later with a less informative message.
			return Standalone{}, fmt.Errorf("%s: seed the standalone stencils directory %s: %w", m.Name, stencilsDir, err)
		}
	}

	// This block sits outside the stencils-override guard above.
	// It logs nothing, so it does not widen the log-free obligation of the redirect above it.
	specsDir := standalonegeom.SpecsDir(stateDir)
	if _, err := stencilstore.Reconcile(specsDir, specs.Registry(), stencilstore.ModeFor(buildinfo.IsDev(), buildinfo.IsProduction(), buildvcs.Running().Clean()), stencilstore.Source{}); err != nil {
		return Standalone{}, fmt.Errorf("%s: seed the standalone specs directory %s: %w", m.Name, specsDir, err)
	}

	result := Standalone{
		Target:      target,
		StateDir:    stateDir,
		Hash8:       hash8,
		StencilsDir: stencilsDir,
		SpecsDir:    specsDir,
	}

	if m.Plan == nil {
		return result, nil
	}

	defaultPlanDir := m.Plan.DefaultPlanDir(stateDir)
	planDir, overridden := ResolvePlanDir(ResolveToldDir(req.Cwd, req.PlanDirFlag), defaultPlanDir)
	if !planDirHasContent(planDir) {
		// The recourse names the default location, never the override the operator supplied.
		recourse := planDir
		if overridden {
			recourse = defaultPlanDir
		}
		return Standalone{}, errors.New(m.Plan.MissingPlanRefusal(planDir, recourse))
	}

	result.PlanDir = planDir
	result.PlanDirOverridden = overridden
	result.DefaultPlanDir = defaultPlanDir
	return result, nil
}

// resolveStandaloneTarget resolves standalone mode's --target-dir to one spelling of one directory.
// Every downstream consumer must agree on that directory.
// The target is cwd when targetDirFlag is empty, and otherwise targetDirFlag made absolute against cwd.
// It is then symlink-normalized and lifted to the root of the repository it sits in.
//
// The result is always absolute, which is a precondition of standalonestate.Derive.
// Derive normalises through EvalSymlinks and Clean and compares case-insensitively on Windows.
// Two spellings of the same directory must therefore not produce different <state> values.
//
// Both normalizations exist because the target is an identity here, not merely a path.
// standalonestate.Derive hashes it into hash8.
// hash8 names the state directory, the reed socket and the tmux session.
// standalonegeom builds the readable half of the session name from it.
// Two spellings of one repository would therefore produce two of everything.
// Normalize is the rule Derive applies, exported by the package that owns the identity.
// The CLI boundary applies it once here, rather than each site re-deriving it.
//
// The repository-root lift covers the other half.
// preflight.ResolveMode returns ModeStandalone for a plain repository's subdirectory too.
// Without the lift, a verb run from repo/ and one from repo/src/ would derive two hash8 values:
// two state directories and two reed sessions.
// A caller that also resolves a relative profile or fasit path would then resolve it against the subdirectory.
// Where an operator stands inside a repository must not change which repository they drive.
//
// A target with no repository above it is returned unchanged.
// Standalone mode legitimately covers a plain directory with no git repository.
// ResolveMode folds that case into the same verdict.
//
// A told --target-dir must exist and be a directory; that check is why this function can fail.
// A missing path must not resolve silently.
// Normalize falls back to Clean for it, and RepositoryRootOf climbs to the enclosing repository.
// The run would then drive that repository instead of the one the operator named.
// A --target-dir naming a file resolves the same way, so the directory check refuses it too.
//
// cwd itself is never stat'd: it is where the process already is.
func (m Module) resolveStandaloneTarget(cwd, targetDirFlag string) (string, error) {
	told := cwd
	if targetDirFlag != "" {
		told = ResolveToldDir(cwd, targetDirFlag)
		info, err := os.Stat(told)
		if err != nil {
			return "", fmt.Errorf("%s: --target-dir %s (resolved to %s) cannot be read: %w -- a target that is not there is not an empty target, it silently resolves to whichever repository encloses it", m.Name, targetDirFlag, told, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s: --target-dir %s (resolved to %s) is not a directory -- the standalone target is a repository to drive, and a file resolves to whichever repository encloses it", m.Name, targetDirFlag, told)
		}
	}
	return RepositoryRootOf(standalonestate.Normalize(told)), nil
}

// planDirHasContent reports whether dir exists and holds at least one "*.md" file.
// That file is the minimal on-disk shape an authored plan directory carries.
// It reports false for a missing directory, an empty one, and one without .md files alike:
// a standalone operator gets the same usage error for each.
// Standalone mode has no bootstrap and no empty-plan fallback.
func planDirHasContent(dir string) bool {
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
