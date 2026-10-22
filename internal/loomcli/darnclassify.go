// darnclassify.go classifies a failed darn verify as owned or not owned by the task, against the impacted set of the task's diff.
// It answers the route a not-owned failure takes: back to Merge-In rather than Darn-Write.

package loomcli

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// darnFailedWayForward is the way-forward clause of a darn verify failure the task owns: the route back to Darn-Write.
const darnFailedWayForward = `run "lyx loom goto --to Darn-Write", then "lyx loom resume", in the task worktree; the Darn-Write spawn renders the Publish failure record and its gate runs the failing tests`

// notOwnedWayForward is the way-forward clause of a darn verify failure outside the task's impacted set.
const notOwnedWayForward = `fix the failure on the parent branch if it fails there too, then run "lyx loom goto --to Merge-In", then "lyx loom resume", in the task worktree, which merges a moved parent and re-runs the failing gate; or, if the task did cause it, run "lyx loom goto --to Darn-Write", then "lyx loom resume", in the task worktree, where a moved parent is merged in first`

// impactedSet is the impacted set of a diff spelled in import paths.
type impactedSet struct {
	// packages are the impacted packages.
	packages map[string]bool
	// guards maps each guard package to the names of its `//lyx:guard` tests.
	guards map[string][]string
	// binary are the packages whose tests import `internal/testkit/lyxbin`.
	binary map[string]bool
}

// importPathSet maps targets onto import paths, reading each module's path from its go.mod under worktree.
// Guards and BinaryTests are directories of the root module.
// A go.mod that is missing, unreadable or names no module is the returned error.
func importPathSet(worktree string, targets impactset.Targets) (impactedSet, error) {
	set := impactedSet{packages: map[string]bool{}, guards: map[string][]string{}, binary: map[string]bool{}}
	for _, module := range targets.Modules {
		modulePath, err := readModulePath(worktree, module.Dir)
		if err != nil {
			return impactedSet{}, err
		}
		for _, pkg := range module.Packages {
			set.packages[joinImportPath(modulePath, pkg)] = true
		}
	}
	if len(targets.Guards) == 0 && len(targets.BinaryTests) == 0 {
		return set, nil
	}
	rootPath, err := readModulePath(worktree, ".")
	if err != nil {
		return impactedSet{}, err
	}
	for dir, names := range targets.Guards {
		set.guards[joinImportPath(rootPath, dir)] = names
	}
	for _, dir := range targets.BinaryTests {
		set.binary[joinImportPath(rootPath, dir)] = true
	}
	return set, nil
}

// readModulePath returns the module path declared by the go.mod in moduleDir, relative to worktree.
func readModulePath(worktree, moduleDir string) (string, error) {
	goMod := filepath.Join(worktree, filepath.FromSlash(moduleDir), "go.mod")
	data, err := os.ReadFile(goMod)
	if err != nil {
		return "", fmt.Errorf("read module path: %w", err)
	}
	modulePath := modfile.ModulePath(data)
	if modulePath == "" {
		return "", fmt.Errorf("read module path: %s declares no module", goMod)
	}
	return modulePath, nil
}

// joinImportPath spells the module-relative package argument or directory dir as an import path under modulePath.
func joinImportPath(modulePath, dir string) string {
	dir = strings.TrimPrefix(path.Clean(dir), "./")
	if dir == "." {
		return modulePath
	}
	return modulePath + "/" + dir
}

// ownedFailure reports whether the task owns a failed verify, over the identities the log yields and the tests the failure record names.
// When it does not, it also returns the sorted distinct import paths of the failing packages, all outside the impacted set.
// Any doubt counts as owned: no recorded test, an opaque identity, or a failure in an impacted package or in a guard test.
func ownedFailure(failures []websterengine.VerifyFailure, tests []verifytree.FailedTest, set impactedSet) (bool, []string) {
	outside := map[string]bool{}
	for _, failure := range failures {
		switch failure.Kind {
		case websterengine.FailureKindOpaque:
			return true, nil
		case websterengine.FailureKindPackage:
			if set.packages[failure.Package] {
				return true, nil
			}
			outside[failure.Package] = true
		default:
			testName := strings.TrimPrefix(failure.ID, failure.Package+".")
			if set.owns(failure.Package, testName) {
				return true, nil
			}
			outside[failure.Package] = true
		}
	}
	if len(tests) == 0 {
		return true, nil
	}
	for _, test := range tests {
		if set.owns(test.Package, test.Test) {
			return true, nil
		}
		outside[test.Package] = true
	}
	packages := make([]string, 0, len(outside))
	for pkg := range outside {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	return false, packages
}

// owns reports whether a failing test of pkg lies in the impacted set or is a guard test of its package.
// A guard matches by its top-level test name, the part of testPath before the first slash.
func (s impactedSet) owns(pkg, testPath string) bool {
	if s.packages[pkg] {
		return true
	}
	topLevel, _, _ := strings.Cut(testPath, "/")
	return slices.Contains(s.guards[pkg], topLevel)
}

// notOwnedRoute is the route of a failure whose failing packages, outside, lie outside the impacted set: back to Merge-In.
// The lead names the packages before the merge clause, so a notice that keeps only the first characters of the reason still carries them.
func notOwnedRoute(outside []string, set impactedSet) landingshed.FailureRoute {
	wayForward := notOwnedWayForward
	var driving []string
	for _, pkg := range outside {
		if set.binary[pkg] {
			driving = append(driving, pkg)
		}
	}
	if len(driving) > 0 {
		wayForward += fmt.Sprintf("; %s drive the lyx binary in their tests and may reach the task's change, so when the test does not fail on the parent branch, the Darn-Write goto is the way forward", strings.Join(driving, ", "))
	}
	return landingshed.FailureRoute{
		Target:     loomshed.NameMergeIn,
		Lead:       fmt.Sprintf(" outside the task's impacted set, in %s,", strings.Join(outside, ", ")),
		WayForward: wayForward,
	}
}

// darnFailureRoute classifies the failed darn verify that failure records and answers its route.
// The route is Darn-Write unless the log's failing packages all lie outside the impacted set of the diff from the merge base.
// Only a failure the import graph shows unreachable from the diff skips a Darn-Write trip.
// An unreadable log, a merge base or derivation error, a Whole answer and an unmappable set all count as owned, the first four logged at Warn.
func darnFailureRoute(
	ctx context.Context,
	failure verifytree.PublishFailure,
	worktree string,
	mergeBase func() (string, error),
	derive func(ctx context.Context, worktree, base string) (impactset.Targets, error),
) landingshed.FailureRoute {
	owned := landingshed.FailureRoute{Target: loomshed.NameDarnWrite, WayForward: darnFailedWayForward}
	log, err := os.ReadFile(failure.LogPath)
	if err != nil {
		logger.Warn("loom: darn failure classified as owned: could not read the verify log", "path", failure.LogPath, "error", err)
		return owned
	}
	base, err := mergeBase()
	if err != nil {
		logger.Warn("loom: darn failure classified as owned: could not read the merge base", "error", err)
		return owned
	}
	targets, err := derive(ctx, worktree, base)
	if err != nil {
		logger.Warn("loom: darn failure classified as owned: could not derive the impacted set", "error", err)
		return owned
	}
	if targets.Whole {
		return owned
	}
	set, err := importPathSet(worktree, targets)
	if err != nil {
		logger.Warn("loom: darn failure classified as owned: could not map the impacted set to import paths", "error", err)
		return owned
	}
	if isOwned, outside := ownedFailure(websterengine.ParseVerifyFailures(string(log)), failure.Tests, set); !isOwned {
		return notOwnedRoute(outside, set)
	}
	return owned
}
