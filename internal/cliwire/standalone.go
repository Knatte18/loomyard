// standalone.go holds StandaloneRequest and Standalone, the request/result types for the single
// ordered standalone prologue, plus ResolveStandalone itself.

package cliwire

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// StandaloneRequest carries one standalone-mode wiring call's raw, as-parsed flag values, exactly as
// they reach each CLI's own wire. ResolveStandalone makes them absolute itself via ResolveToldDir.
//
// This deliberately re-resolves values each wire has already resolved for its own hub branch --
// harmless, because ResolveToldDir is idempotent on an absolute input, and named here so a later
// reader does not "fix" the apparent double resolution by hoisting it back out.
type StandaloneRequest struct {
	Cwd             string
	StencilsDirFlag string
	PlanDirFlag     string
	TargetDirFlag   string
}

// Standalone is ResolveStandalone's result: the resolved target and derived state identity, plus
// every told directory the caller needs to finish building its own engine stack.
//
// DefaultPlanDir is the mode's own default, carried so a caller can record it for a refusal's
// recourse text. PlanDir and PlanDirOverridden are the zero value when the module carries no Plan.
type Standalone struct {
	Target            string
	StateDir          string
	Hash8             string
	StencilsDir       string
	PlanDir           string
	PlanDirOverridden bool
	DefaultPlanDir    string
}

// ResolveStandalone performs the whole ordered standalone prologue: target resolution,
// standalonestate.Derive, the nested-geometry refusal, the durable-sink redirect, the stencils
// resolve-and-seed, and plan-dir resolution with override detection. Every fallible step returns the
// zero Standalone alongside its error, and no step is best-effort.
//
// The durable sink is armed lazily on the first Info-or-above record, so the redirect below binds
// only if it runs before anything in the sequence can log; it cannot be first, because it is
// Derive's own stateDir that tells it where to point. The obligation a later editor inherits is
// therefore that every statement above the redirect stays log-free and no logging statement is added
// below it that could be hoisted above.
//
// The nested-geometry refusal must run after standalonestate.Derive and before both the sink
// redirect and any substrate boot. The stencils seed's failure is a hard error in standalone because
// nothing else will ever create that directory (unlike the root pre-run's best-effort logged seed),
// and the seed runs only for the derived default so a curated stencil set named by --stencils-dir is
// never rewritten from under the operator. These asymmetries are deliberate and must not be
// "simplified" away.
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

	// The redirect runs before the first statement below that CAN log -- see the function's own
	// doc comment for the full ordering obligation.
	logger.SetDurableSinkDirWithWorktreeRoot(standalonegeom.LogsDir(stateDir), target)

	stencilsDir := ResolveToldDir(req.Cwd, req.StencilsDirFlag)
	if stencilsDir == "" {
		// An operator who named a curated stencil set must not have it rewritten from under them --
		// seed only the standalone DEFAULT, never an explicit override.
		stencilsDir = standalonegeom.StencilsDir(stateDir)
		if _, err := stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), ""); err != nil {
			// Unlike the root pre-run's best-effort, logged-only seed pass, nothing else will ever
			// create this directory: a reconcile failure here is a hard error, since every prompt
			// render would otherwise fail later with a far less informative message.
			return Standalone{}, fmt.Errorf("%s: seed the standalone stencils directory %s: %w", m.Name, stencilsDir, err)
		}
	}

	result := Standalone{
		Target:      target,
		StateDir:    stateDir,
		Hash8:       hash8,
		StencilsDir: stencilsDir,
	}

	if m.Plan == nil {
		return result, nil
	}

	defaultPlanDir := m.Plan.DefaultPlanDir(stateDir)
	planDir, overridden := ResolvePlanDir(ResolveToldDir(req.Cwd, req.PlanDirFlag), defaultPlanDir)
	if !planDirHasContent(planDir) {
		// The recourse names the DEFAULT location, never the override the operator just supplied --
		// see MissingPlanRefusal's own doc comment for why the parameter is named recourse rather
		// than defaultPlanDir.
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

// resolveStandaloneTarget resolves standalone mode's --target-dir into the one spelling of the one
// directory every downstream consumer must agree on: cwd when targetDirFlag is empty, or
// targetDirFlag made absolute against cwd otherwise, then symlink-normalized, then lifted to the
// root of the repository it sits in.
//
// The result is always absolute, which is standalonestate.Derive's own precondition, because Derive
// normalises through EvalSymlinks+Clean and compares case-insensitively on Windows, so two spellings
// of the same directory must not produce different <state> values.
//
// Both normalizations exist because the target is an IDENTITY here, not merely a path.
// standalonestate.Derive hashes it into hash8, which names the state directory, the reed socket and
// the tmux session, and standalonegeom builds the session name's readable half from it — so two
// spellings of one repository produce two of everything. Normalize is Derive's own rule, exported by
// the package that owns the identity precisely so the CLI boundary can apply it once here rather
// than each site re-deriving it.
//
// The repository-root lift answers the other half. preflight.ResolveMode returns ModeStandalone for
// a plain repository's SUBDIRECTORY too, so a verb run from repo/ and from repo/src/ would otherwise
// derive two different hash8 values, two state directories, two reed sessions, and (for a caller
// that also resolves a relative profile or fasit path) two different resolutions of that path
// against the wrong base — silently resolving the profile's own relative target and fasit paths
// against the subdirectory rather than the repository. Where an operator stands inside a repository
// is not supposed to change which repository they are driving.
//
// A target with no repository above it is returned unchanged: standalone mode legitimately covers a
// plain directory that is no git repository at all, which ResolveMode folds into the same verdict.
//
// A told --target-dir must EXIST and be a directory, and that check is the reason this function is
// fallible. Without it the resolution silently succeeded against the wrong repository:
// standalonestate.Normalize falls back to Clean for a path that does not exist, and RepositoryRootOf
// then climbs until it finds a ".git" — so from inside /repo, a mistyped `--target-dir ./reposs`
// resolved to /repo/reposs, found no repository there, climbed, and returned /repo. The run then
// drove the repository the operator was standing in rather than the one they named, with the same
// hash8, state directory and reed session as the no-flag invocation, so nothing in the output told
// the two apart — and for a caller whose fix phase writes, that is an edit to the wrong tree
// (crucible round opus-medium-r6, R6-7). A --target-dir naming a FILE resolved the same way.
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

// planDirHasContent reports whether dir exists and contains at least one "*.md" file -- the minimal
// on-disk shape an authored plan directory carries. It never distinguishes "missing directory" from
// "empty directory" from "directory with no .md files": all three are the same usage error to a
// standalone operator, which has no bootstrap and no empty-plan fallback to fall back to.
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
