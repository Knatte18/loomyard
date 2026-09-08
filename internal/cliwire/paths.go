// paths.go holds cliwire's pure path helpers: the ones that produce no operator-facing message and
// therefore need no Module descriptor to drive them.

package cliwire

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// gitDirName is the per-repository git administrative entry repositoryRootOf looks for, present as a
// directory in an ordinary clone and as a file in a linked worktree.
// It is a plain literal rather than an internal/lyxdirs constant: lyxdirs is the single declarer of
// loomyard's own "_lyx" and ".lyx" directories and has never owned git's.
const gitDirName = ".git"

// ResolveToldDir makes one told directory flag absolute against cwd: the empty string stays empty
// (the operator passed no flag, and each mode computes its own default), an absolute value is
// cleaned, and a relative one is joined onto cwd.
//
// Every path this module hands downstream must be absolute. A relative flag value does not fail, it
// silently means two different directories: the CLI process resolves it against ITS working
// directory while the pane webster or burler spawns runs at the standalone target or the hub anchor,
// and the standalone default-vs-override check is a path equality a relative spelling can never
// satisfy. Resolving happens once, at the wiring boundary, because that is the last point that still
// knows which working directory the operator typed the flag from — the same reason
// resolveStandaloneTarget has always done it for --target-dir.
//
// PRECONDITION, enforced only by convention rather than asserted here (crucible round
// sonnet-xhigh-r8, CW-4): cwd must be non-empty and absolute whenever flagValue is a non-empty
// relative value. filepath.Join silently drops an empty path element rather than failing, so an
// empty cwd would return flagValue unresolved (still relative), silently violating this function's
// own documented "every path this module hands downstream must be absolute" postcondition. Every
// production call site supplies cwd from an already-resolved lyxcwd.CwdFrom(ctx) or the standalone
// prologue's own told cwd, neither of which is ever empty, so this precondition is not
// live-exploitable today.
func ResolveToldDir(cwd, flagValue string) string {
	if flagValue == "" {
		return ""
	}
	if filepath.IsAbs(flagValue) {
		return filepath.Clean(flagValue)
	}
	return filepath.Join(cwd, flagValue)
}

// pathContains reports whether inner is outer itself or a descendant of it, computed the way
// shuttleengine's own told-path assertions compute it -- filepath.Rel plus a ".." prefix test -- so
// the CLI-boundary refusal and the constructor assertion it front-runs agree about what "nested"
// means. Both paths are already absolute and cleaned by their producers.
func pathContains(outer, inner string) bool {
	// filepath.Rel is case-SENSITIVE, while Windows paths are not, so LOCALAPPDATA and a target that
	// differ only in case (C:\\Users\\X vs c:\\users\\x — one directory) read as disjoint. Folded here
	// on Windows alone, matching lyxcwd.samePath's rule exactly. Not reachable from this project's
	// Linux hosts and therefore never driven live; it is a mechanical mirror of an already-stated
	// rule, not a verified behaviour.
	if runtime.GOOS == "windows" {
		outer, inner = strings.ToLower(outer), strings.ToLower(inner)
	}
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// NormalizeForContainment returns the spelling of path that a containment or equality test must use:
// standalonestate.Normalize applied to the deepest ANCESTOR of path that exists on disk, with the
// not-yet-created remainder rejoined.
//
// Plain Normalize is not enough here. It falls back to Clean whenever filepath.EvalSymlinks fails,
// and EvalSymlinks fails when ANY component is missing — which the derived state directory's own
// leaf (<stateHome>/lyx/<hash8>) routinely is on a first run. The target, meanwhile, has already been
// through Normalize with every symlink resolved. Comparing a resolved string against an unresolved
// one made refuseNestedStandaloneGeometry — and shuttleengine's own validateDetachedToldPaths, which
// compares the same two strings — both answer "disjoint" for a state home that reaches inside the
// target through a symlink, and lyx then wrote its state tree, run locks and trace logs into the
// operator's checkout: exactly the outcome the guard exists to prevent (crucible round
// opus-medium-r6, R6-15).
func NormalizeForContainment(path string) string {
	existing := filepath.Clean(path)
	var missing []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return filepath.Clean(path)
		}
		missing = append([]string{filepath.Base(existing)}, missing...)
		existing = parent
	}
	return filepath.Join(append([]string{standalonestate.Normalize(existing)}, missing...)...)
}

// RepositoryRootOf returns the NEAREST repository root at or above dir -- the closest ancestor (dir
// itself included) carrying a ".git" entry -- or dir unchanged when no ancestor has one.
//
// Nearest, never topmost: a submodule and a nested repository are each their own repository, and a
// walk that kept climbing past the first ".git" would silently re-target a standalone run at the
// superproject that contains it.
//
// It walks the filesystem rather than asking git, and that is deliberate on two counts. It keeps
// wire free of process spawns, which is what lets this module's whole wiring truth table be driven
// from untagged tests under the Test Tier Purity Invariant. And it is not a cwd query: dir arrives
// already resolved and already absolute, so internal/lyxcwd remains the sole owner of turning a
// working directory into a Location, per the Cwd Resolution Invariant — this only lifts an
// already-resolved path to the root of the tree it lives in.
//
// os.Lstat rather than os.Stat, and no directory-vs-file test: a linked worktree records ".git" as a
// FILE, and a repository reached through a symlink is still a repository.
//
// PRECONDITION, enforced only by convention rather than asserted here (crucible round
// sonnet-xhigh-r8, CW-4): dir must be non-empty and absolute. filepath.Join silently drops an empty
// path element rather than failing, so an empty dir would search from this PROCESS's own cwd rather
// than from any path the caller actually meant — a landmine rather than a clean failure. The sole
// production call site (resolveStandaloneTarget) can never pass one: its own dir is always derived
// from cwd, which is refused upstream by lyxcwd.CwdFrom before wire is ever reached, so this
// precondition is not live-exploitable today.
func RepositoryRootOf(dir string) string {
	for candidate := dir; ; {
		if _, err := os.Lstat(filepath.Join(candidate, gitDirName)); err == nil {
			return candidate
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return dir
		}
		candidate = parent
	}
}

// SamePlanDir reports whether a told --plan-dir names the same directory as the mode's own default.
// Both arguments are absolute by construction -- the flag value through ResolveToldDir, the default
// through hubgeom/standalonegeom -- so this is a path equality, which is what makes a "." or
// trailing-separator spelling of the default recognized as the default rather than as an override.
//
// It compares through NormalizeForContainment rather than filepath.Clean alone: a plain Clean
// equality reported an OVERRIDE for a --plan-dir naming the very same directory through a symlinked
// state home or HOME, and `run` then refused a plan that was in fact at the default location
// (crucible round opus-medium-r6, R6-15).
func SamePlanDir(planDir, defaultPlanDir string) bool {
	return NormalizeForContainment(planDir) == NormalizeForContainment(defaultPlanDir)
}

// ResolvePlanDir resolves a told --plan-dir against defaultPlanDir, the mode's own default: an empty
// toldPlanDir returns (defaultPlanDir, false), and a non-empty one returns
// (toldPlanDir, !SamePlanDir(toldPlanDir, defaultPlanDir)) -- the told spelling always wins as the
// resolved directory, and overridden is true only when the told value names a directory different
// from the default.
//
// It is a package function rather than a Module method because it is infallible, produces no
// operator-facing message, and reads no descriptor field. A told value naming the default location
// through a ".", trailing-separator, or symlinked spelling is recognised as the default rather than
// as an override, since the comparison runs through SamePlanDir rather than a raw string equality.
func ResolvePlanDir(toldPlanDir, defaultPlanDir string) (planDir string, overridden bool) {
	if toldPlanDir == "" {
		return defaultPlanDir, false
	}
	return toldPlanDir, !SamePlanDir(toldPlanDir, defaultPlanDir)
}
