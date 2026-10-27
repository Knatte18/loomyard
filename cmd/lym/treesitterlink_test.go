// treesitterlink_test.go declares the pure half of the tree-sitter link guard.
// That is the parse of `go list` output, the shortest import chain from a test binary to tree-sitter, and the comparison against the allowed list.
// treesitterlink_integration_test.go runs `go list` and feeds this half.
//
// Linking tree-sitter costs every test binary that does so a slower link, so only the packages that call the code index for real may do it.
// A package outside the list fails with its chain.
// An entry whose binary no longer links tree-sitter fails as stale, so the list tracks the tree in both directions.
// It carries no build tag, so the `tier1` guard and the `tier0` test that read it both compile it.

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	// treeSitterImportPath is the package whose presence in a test binary's dependencies is the guarded link.
	treeSitterImportPath = "github.com/tree-sitter/go-tree-sitter"
	// treeSitterModulePrefix is the module path every guarded package starts with.
	treeSitterModulePrefix = "github.com/Knatte18/loomyard/"
)

// goListImportsFormat is the `go list -f` template parseGoListImports reads.
// Imports are joined by a semicolon because a variant's import path, `pkg [pkg.test]`, contains a space.
const goListImportsFormat = `{{.ImportPath}}{{"\t"}}{{join .Imports ";"}}`

// parseGoListImports reads `go list -f goListImportsFormat` output into a map from a package's import path to its imports.
// Import paths keep their `[pkg.test]` variant suffix, since that is how the imports of one variant name another.
func parseGoListImports(output string) map[string][]string {
	deps := map[string][]string{}
	for _, line := range strings.Split(output, "\n") {
		importPath, imports, _ := strings.Cut(line, "\t")
		if importPath == "" {
			continue
		}
		if imports == "" {
			deps[importPath] = nil
			continue
		}
		deps[importPath] = strings.Split(imports, ";")
	}
	return deps
}

// importChainToTreeSitter returns the shortest import chain from the package named from to tree-sitter, from first, or nil when it is unreachable.
// Variant suffixes are kept, because a name without one may be another node.
func importChainToTreeSitter(deps map[string][]string, from string) []string {
	parent := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node == treeSitterImportPath {
			var chain []string
			for n := node; n != ""; n = parent[n] {
				chain = append([]string{n}, chain...)
			}
			return chain
		}
		for _, imported := range deps[node] {
			if _, seen := parent[imported]; !seen {
				parent[imported] = node
				queue = append(queue, imported)
			}
		}
	}
	return nil
}

// treeSitterLinkedTestPackages maps the module-relative path of every package whose test binary links tree-sitter to that binary's import chain.
// A test binary is the `<pkg>.test` node, which imports the package's variants under test and its external test package.
func treeSitterLinkedTestPackages(deps map[string][]string) map[string][]string {
	linked := map[string][]string{}
	for importPath := range deps {
		pkg, ok := strings.CutSuffix(importPath, ".test")
		if !ok || !strings.HasPrefix(pkg, treeSitterModulePrefix) {
			continue
		}
		if chain := importChainToTreeSitter(deps, importPath); chain != nil {
			linked[strings.TrimPrefix(pkg, treeSitterModulePrefix)] = chain
		}
	}
	return linked
}

// formatImportChain renders a chain as `a -> b -> c`, dropping each node's `[pkg.test]` variant suffix.
func formatImportChain(chain []string) string {
	names := make([]string, len(chain))
	for i, node := range chain {
		names[i], _, _ = strings.Cut(node, " [")
	}
	return strings.Join(names, " -> ")
}

// treeSitterLinkFindings compares the packages whose test binaries link tree-sitter against allowed.
// It reports each linked package outside the list with its chain, and each entry whose package does not link it, both sorted.
func treeSitterLinkFindings(linked map[string][]string, allowed []scankit.Entry) []string {
	allowlist := scankit.NewAllowlist(allowed)
	var findings []string
	for pkg, chain := range linked {
		if allowlist.Allowed(pkg) {
			continue
		}
		findings = append(findings, fmt.Sprintf(
			"%s: its test binary links tree-sitter through %s; cut the import at its seam, or allow the package in treeSitterAllowedLinks with the remaining link stated",
			pkg, formatImportChain(chain)))
	}
	sort.Strings(findings)
	for _, key := range allowlist.Stale() {
		findings = append(findings, fmt.Sprintf("%s: allowed in treeSitterAllowedLinks, but its test binary no longer links tree-sitter; remove the entry", key))
	}
	return findings
}
