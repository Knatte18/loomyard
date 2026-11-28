// mapread.go reads the coverage maps of one maps directory, passing over a map no reader can use.

package testreach

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// rebuildHint is the way forward the reader names for a coverage map it cannot use.
const rebuildHint = "rebuild it with `lym gate test --map` from the prime"

// ReadPackageMap reads the coverage map of the package pkg, a module-relative slash path, from the maps directory dir.
// It reports false for a package with no map.
// It also reports false for a map that holds only a build failure.
// It reports false as well for a map whose tests name a block or function outside its tables.
// A map file that does not parse is an error naming the package and the way forward.
func ReadPackageMap(dir, pkg string) (testcover.Map, bool, error) {
	m, found, err := testcover.ReadMap(dir, pkg)
	if err != nil {
		return testcover.Map{}, false, fmt.Errorf("read the coverage map of %s: %w; %s", pkg, err, rebuildHint)
	}
	if !found || m.BuildFailure != "" || !indexesInTables(m) {
		return testcover.Map{}, false, nil
	}
	return m, true, nil
}

// indexesInTables reports whether every block and function index of m's tests lies inside m's tables.
func indexesInTables(m testcover.Map) bool {
	for _, test := range m.Tests {
		for _, i := range test.Blocks {
			if i < 0 || i >= len(m.Blocks) {
				return false
			}
		}
		for _, i := range test.Funcs {
			if i < 0 || i >= len(m.Funcs) {
				return false
			}
		}
	}
	return true
}

// mapFilePackages returns the module-relative slash path of each package that has a map file under dir, sorted.
// An absent directory holds none.
func mapFilePackages(dir string) ([]string, error) {
	var packages []string
	mapFileName := filepath.Base(testcover.MapPath(".", "."))
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() || entry.Name() != mapFileName {
			return nil
		}
		rel, err := filepath.Rel(dir, filepath.Dir(path))
		if err != nil {
			return err
		}
		packages = append(packages, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list the coverage maps under %s: %w", dir, err)
	}
	slices.Sort(packages)
	return packages, nil
}

// readMapDir reads every usable map under the maps directory dir, by module-relative package.
// A map that does not parse is logged and left out, so one corrupt map never stops the others from answering.
func readMapDir(dir string) (map[string]testcover.Map, error) {
	packages, err := mapFilePackages(dir)
	if err != nil {
		return nil, err
	}
	maps := map[string]testcover.Map{}
	for _, pkg := range packages {
		m, found, err := ReadPackageMap(dir, pkg)
		if err != nil {
			logger.Warn("coverage map skipped", "package", pkg, "error", err)
			continue
		}
		if found {
			maps[pkg] = m
		}
	}
	return maps, nil
}
