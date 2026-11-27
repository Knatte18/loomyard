// testmapplan.go picks the tests of a package a coverage map holds, and scans them for spawning.

package gatecli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/testcover"
)

// mappedTests returns the Test functions of the package directory pkgDir.
// It keeps those whose tiergate.Require names a mapped tier the hub declares.
func (r *testMapRun) mappedTests(pkgDir string) ([]testcover.TestFunc, error) {
	funcs, err := testcover.ScanTestFuncs(pkgDir)
	if err != nil {
		return nil, err
	}
	var mapped []testcover.TestFunc
	for _, fn := range funcs {
		if _, declared := r.tiers[fn.Tier]; fn.Tiered && declared {
			mapped = append(mapped, fn)
		}
	}
	return mapped, nil
}

// scanSpawns returns the spawn scan's verdict on each test of the package directory pkgDir.
func (r *testMapRun) scanSpawns(pkgDir string) (map[string]testcover.SpawnVerdict, error) {
	return testcover.ScanSpawns(r.modulePath, filepath.Join(r.moduleDir, filepath.FromSlash(testKitDir)), pkgDir)
}
