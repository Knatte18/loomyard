// packagedirs.go tells which directories below a package directory hold another package or a nested module.
// The files under them belong to that package and not the one above.

package testcover

import (
	"maps"
	"path"
	"slices"
	"strings"
)

// NestedPackageDirs returns the directories below a package directory that hold another package or a nested module.
// They are sorted.
// files are the slash paths, relative to the package directory, of every file under it.
// A directory holds a package when it holds a Go file directly and the go command would build it:
// it lies under no testdata directory and no directory whose name opens with a dot or an underscore.
// A directory holding go.mod is a nested module wherever it lies.
// A directory whose Go files build constraints all exclude still counts as nested:
// the bound only misses files of the owner, and never adds a false one.
func NestedPackageDirs(files []string) []string {
	nested := map[string]struct{}{}
	for _, rel := range files {
		dir, name := path.Dir(rel), path.Base(rel)
		if dir == "." {
			continue
		}
		if name == "go.mod" || (strings.HasSuffix(name, ".go") && buildableDir(dir)) {
			nested[dir] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(nested))
}

// buildableDir reports whether the go command builds a package in the slash directory dir:
// no element of it is testdata or opens with a dot or an underscore.
func buildableDir(dir string) bool {
	for _, element := range strings.Split(dir, "/") {
		if element == "testdata" || strings.HasPrefix(element, ".") || strings.HasPrefix(element, "_") {
			return false
		}
	}
	return true
}
