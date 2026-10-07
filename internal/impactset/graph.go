// graph.go reads the module's package graph from `go list` and answers the reachability questions the impacted set needs.

package impactset

import (
	"path/filepath"
	"sort"
	"strings"
)

const (
	// lyxbinDir is the module-relative directory of the package whose import marks a test that builds the `lyx` binary.
	lyxbinDir = "internal/testkit/lyxbin"
	// commandDir is the module-relative directory of the `lyx` command.
	commandDir = "cmd/lyx"
)

// goListFormat is the `go list -f` template parseGoList reads.
// Imports are joined by a semicolon, the field separator is a tab.
const goListFormat = `{{.ImportPath}}{{"\t"}}{{.Dir}}{{"\t"}}{{join .Imports ";"}}{{"\t"}}{{join .TestImports ";"}}{{"\t"}}{{join .XTestImports ";"}}`

// pkg is one package of the module.
type pkg struct {
	ImportPath string
	// Dir is the slash-separated directory relative to the worktree, `.` for the root.
	Dir string
	// Imports are the import paths of the package's own files, under every tag.
	Imports []string
	// TestImports are the import paths of its in-package and external test files, under every tag.
	TestImports []string
}

// parseGoList reads `go list -f goListFormat` output into packages, resolving each absolute Dir against rootDir.
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

// closure returns the import paths reachable from the roots through non-test imports, the roots included, restricted to module packages.
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
// p itself, its transitive non-test dependencies, or the dependencies of its own test imports.
func (g moduleGraph) reaches(p pkg, changed map[string]bool) bool {
	roots := append([]string{p.ImportPath}, p.TestImports...)
	for importPath := range g.closure(roots) {
		if changed[importPath] {
			return true
		}
	}
	return false
}

// impactedSet returns the import paths of the changed packages and every package that reaches one of them.
// When a changed package is a dependency of the `cmd/lyx` command, it also returns every package whose tests import the binary builder.
func (g moduleGraph) impactedSet(changed map[string]bool) map[string]bool {
	set := map[string]bool{}
	for _, p := range g.byImportPath {
		if g.reaches(p, changed) {
			set[p.ImportPath] = true
		}
	}
	if command, ok := g.byDir[commandDir]; ok {
		for importPath := range g.closure([]string{command.ImportPath}) {
			if changed[importPath] {
				g.addBinaryBuildingTests(set)
				break
			}
		}
	}
	return set
}

// addBinaryBuildingTests adds every package whose test files import the binary builder.
func (g moduleGraph) addBinaryBuildingTests(set map[string]bool) {
	lyxbin, ok := g.byDir[lyxbinDir]
	if !ok {
		return
	}
	for _, p := range g.byImportPath {
		for _, imported := range p.TestImports {
			if imported == lyxbin.ImportPath {
				set[p.ImportPath] = true
			}
		}
	}
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
