// dependents.go widens a named set of packages to every package of its module whose build or tests reach one of them.

package impactset

import (
	"context"
	"fmt"
	"path"
	"slices"
)

// DependentPackages returns packages and every package of the module at moduleDir whose build or tests reach one of them.
// packages and the result are module-relative `go` package arguments, such as `./internal/x`, or `.` for the module root.
// The result keeps packages first, in their order and cleaned spelling, then the added packages sorted by directory.
// A package the module does not hold yet stays in the result and adds nothing.
// A package that a plan card has still to create is one such.
// The graph is built under the tier tags `tier0` through `tier3`, test imports included,
// and the `cmd/lym` main package reaches every package whose tests import `internal/testkit/lymbin`.
// A failed `go list` run is an error naming moduleDir; ctx bounds the run, so a stalled `go list` is killed when it ends.
func DependentPackages(ctx context.Context, moduleDir string, packages []string) ([]string, error) {
	pkgs, err := listPackages(ctx, moduleDir)
	if err != nil {
		return nil, fmt.Errorf("impactset: list the packages of %s: %w", moduleDir, err)
	}
	return dependentsFor(newModuleGraph(pkgs), packages), nil
}

// dependentsFor is the pure core of DependentPackages over graph.
func dependentsFor(graph moduleGraph, packages []string) []string {
	seeds := map[string]bool{}
	dirs := make([]string, len(packages))
	for i, arg := range packages {
		dirs[i] = path.Clean(arg)
		if p, ok := graph.byDir[dirs[i]]; ok {
			seeds[p.ImportPath] = true
		}
	}
	result := packageArgs(dirs)
	for _, arg := range packageArgs(graph.packageDirs(graph.impactedSet(seeds))) {
		if !slices.Contains(result, arg) {
			result = append(result, arg)
		}
	}
	return result
}
