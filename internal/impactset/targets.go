// targets.go declares DeriveTargets and its pure core: the impacted packages of the working tree as a target set.

package impactset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// Targets is the impacted set of a diff from a base to the working tree.
type Targets struct {
	// Whole is set when the diff cannot be narrowed to a set, and Fallback says why.
	Whole    bool
	Fallback string
	// Base is the commit the diff started from, set whenever the base was usable.
	Base string
	// Modules are the impacted packages per module; empty with Whole, and empty when only Markdown files changed.
	Modules []ModuleTargets
	// Guards maps the directory of each package outside the set to its `//lyx:guard` tests, sorted by name, each with its file's tier.
	Guards map[string][]GuardTest
	// BinaryTests are the sorted worktree-relative directories of every package whose tests import `internal/testkit/lyxbin`, inside the set or not; empty with Whole.
	BinaryTests []string
}

// ModuleTargets is the impacted packages of one module.
type ModuleTargets struct {
	// Dir is the module directory relative to the worktree, `.` for the root module.
	Dir string
	// Packages are the module-relative package arguments, in the order the module graph lists them.
	Packages []string
}

// DeriveTargets derives the impacted set of the diff between base and the working tree in worktree, uncommitted and untracked files included.
// A fallback is reported as Whole with its reason and is no error; an error is a failure to read git or a package directory, a cancelled ctx, or a *GuardScanError for a test file the guard scan cannot accept.
// ctx bounds the derivation: it is checked before each git read, and a cancelled ctx kills a stalled `go list`.
func DeriveTargets(ctx context.Context, worktree, base string) (Targets, error) {
	if _, err := os.Stat(filepath.Join(worktree, "go.mod")); err != nil {
		return wholeTargets("", "no go.mod at the worktree root"), nil
	}
	if base == "" {
		return wholeTargets("", "no base commit to diff from"), nil
	}
	if err := ctx.Err(); err != nil {
		return Targets{}, err
	}
	resolvedBase, ancestor := AncestorBase(worktree, base)
	if !ancestor {
		return wholeTargets("", fmt.Sprintf("base %s is not an ancestor of HEAD", base)), nil
	}

	changedPaths, err := ChangedPaths(ctx, worktree, resolvedBase)
	if err != nil {
		return Targets{}, err
	}
	pkgs, err := listPackages(ctx, worktree)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Targets{}, ctxErr
		}
		return wholeTargets(resolvedBase, "go list failed: "+err.Error()), nil
	}
	graph := newModuleGraph(pkgs)
	dirs := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		dirs = append(dirs, p.Dir)
	}
	guards, err := FindGuardTests(worktree, dirs)
	if err != nil {
		return Targets{}, err
	}

	targets := targetsFor(graph, changedPaths, guards)
	targets.Base = resolvedBase
	return targets, nil
}

func wholeTargets(base, fallback string) Targets {
	return Targets{Whole: true, Fallback: fallback, Base: base}
}

// ChangedPaths returns every path that differs between base and the working tree: tracked changes, staged or not, a renamed file by both its old and new path, and untracked files that are not ignored.
// ctx is checked before each git read.
func ChangedPaths(ctx context.Context, worktree, base string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	output, err := gitexec.Run([]string{"diff", "--name-status", "-z", "--find-renames", base}, worktree)
	if err != nil {
		return nil, fmt.Errorf("impactset: list the files changed since %s: %w", base, err)
	}
	paths := parseChangedPaths(output)

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	untracked, err := gitexec.Run([]string{"ls-files", "--others", "--exclude-standard", "-z"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("impactset: list the untracked files: %w", err)
	}
	for _, untrackedPath := range strings.Split(untracked, "\x00") {
		if untrackedPath != "" {
			paths = append(paths, untrackedPath)
		}
	}
	return paths, nil
}

// targetsFor is the pure core of DeriveTargets: the impacted set of the changed paths over the graph as root-module targets, or a Whole fallback.
// guards maps a package directory to its guard tests.
func targetsFor(graph moduleGraph, changedPaths []string, guards map[string][]GuardTest) Targets {
	set, fallback := impactedFor(graph, changedPaths)
	if fallback != "" {
		return wholeTargets("", fallback)
	}

	targets := Targets{Guards: map[string][]GuardTest{}, BinaryTests: graph.addBinaryBuildingTests(map[string]bool{})}
	if len(set) > 0 {
		targets.Modules = []ModuleTargets{{Dir: ".", Packages: packageArgs(graph.packageDirs(set))}}
	}
	for dir, tests := range guards {
		if len(tests) > 0 && !set[graph.byDir[dir].ImportPath] {
			sorted := slices.Clone(tests)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
			targets.Guards[dir] = slices.Compact(sorted)
		}
	}
	return targets
}
