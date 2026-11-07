// graph.go reads the module's package graph from `go list` and answers the reachability questions the impacted set needs.

package impactset

import (
	"path/filepath"
	"sort"
	"strings"
)

const (
	// lymbinDir is the module-relative directory of the package whose import marks a test that builds the `lym` binary.
	lymbinDir = "internal/testkit/lymbin"
	// commandDir is the module-relative directory of the `lym` command.
	commandDir = "cmd/lym"
)

// goListFormat is the `go list -f` template for one line per package.
// Fields are tab-separated, and the imports within one field are joined by a semicolon.
const goListFormat = `{{.ImportPath}}{{"\t"}}{{.Dir}}{{"\t"}}{{join .Imports ";"}}{{"\t"}}{{join .TestImports ";"}}{{"\t"}}{{join .XTestImports ";"}}`

// pkg is one package of the module, as `go list` reports it.
type pkg struct {
	ImportPath string
	// Dir is the slash-separated directory relative to the worktree, `.` for the root.
	Dir string
	// Imports are the import paths of the package's own files, under every tag.
	Imports []string
	// TestImports are the import paths of its in-package and external test files, under every tag.
	TestImports []string
}

// parseGoList reads `go list -f goListFormat` output into packages, making each absolute Dir relative to rootDir.
// A package whose directory lies outside rootDir is skipped.
func parseGoList(output, rootDir string) []pkg {
	var pkgs []pkg
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			continue
		}
		relative, err := filepath.Rel(rootDir, fields[1])
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		dir := filepath.ToSlash(relative)
		pkgs = append(pkgs, pkg{
			ImportPath:  fields[0],
			Dir:         dir,
			Imports:     splitImports(fields[2]),
			TestImports: append(splitImports(fields[3]), splitImports(fields[4])...),
		})
	}
	return pkgs
}

func splitImports(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, ";")
}

// moduleGraph indexes packages by import path and directory.
type moduleGraph struct {
	byImportPath map[string]pkg
	byDir        map[string]pkg
}

func newModuleGraph(pkgs []pkg) moduleGraph {
	g := moduleGraph{byImportPath: map[string]pkg{}, byDir: map[string]pkg{}}
	for _, p := range pkgs {
		g.byImportPath[p.ImportPath] = p
		g.byDir[p.Dir] = p
	}
	return g
}

// closure returns the import paths of the module packages reachable from the roots through non-test imports.
// The roots themselves are included when they are module packages.
func (g moduleGraph) closure(roots []string) map[string]bool {
	seen := map[string]bool{}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		importPath := queue[0]
		queue = queue[1:]
		p, ok := g.byImportPath[importPath]
		if !ok || seen[importPath] {
			continue
		}
		seen[importPath] = true
		queue = append(queue, p.Imports...)
	}
	return seen
}

// reaches reports whether package p's build or test reaches a package in changed:
// p itself, its transitive non-test dependencies, or its test imports and their dependencies.
func (g moduleGraph) reaches(p pkg, changed map[string]bool) bool {
	roots := append([]string{p.ImportPath}, p.TestImports...)
	for importPath := range g.closure(roots) {
		if changed[importPath] {
			return true
		}
	}
	return false
}

// impactedSet returns the import paths of the changed packages and of every package that reaches one of them.
// The `cmd/lym` main package is imported by no test,
// so a change to it also adds every package whose test files import `internal/testkit/lymbin`.
func (g moduleGraph) impactedSet(changed map[string]bool) map[string]bool {
	set := map[string]bool{}
	for _, p := range g.byImportPath {
		if g.reaches(p, changed) {
			set[p.ImportPath] = true
		}
	}
	if command, ok := g.byDir[commandDir]; ok && changed[command.ImportPath] {
		g.addBinaryBuildingTests(set)
	}
	return set
}

// addBinaryBuildingTests adds every package whose test files import `internal/testkit/lymbin` to set.
// It returns the sorted module-relative directories of those packages.
// The result is empty when the module has no `internal/testkit/lymbin` package.
func (g moduleGraph) addBinaryBuildingTests(set map[string]bool) []string {
	lymbin, ok := g.byDir[lymbinDir]
	if !ok {
		return nil
	}
	var dirs []string
	for _, p := range g.byImportPath {
		for _, imported := range p.TestImports {
			if imported == lymbin.ImportPath {
				set[p.ImportPath] = true
				dirs = append(dirs, p.Dir)
				break
			}
		}
	}
	sort.Strings(dirs)
	return dirs
}

// packageDirs returns the directories of the import paths, sorted.
func (g moduleGraph) packageDirs(importPaths map[string]bool) []string {
	var dirs []string
	for importPath := range importPaths {
		dirs = append(dirs, g.byImportPath[importPath].Dir)
	}
	sort.Strings(dirs)
	return dirs
}
