// testmaptree.go reads, for `lym gate testmap`, the files a package owns at a commit by the path-to-package rule.
// It hashes them, finds the uncommitted and untracked ones, and tells which of them changed between two commits.

package gatecli

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/testcover"
)

// packageTree is one package's tracked files at a commit:
// every file under its directory down to the next package directory or nested module.
type packageTree struct {
	// dir is the package's worktree-relative slash directory.
	dir string
	// nested holds the package and module directories below dir, as slash paths relative to it.
	nested []string
	// hashes maps each of the package's files, by slash path relative to dir, to its ContentHash.
	hashes map[string]string
}

// readPackageTree reads the package tree of the worktree-relative slash directory dir at the commit rev.
func (r *testMapRun) readPackageTree(rev, dir string) (packageTree, error) {
	files, err := r.fabric.CodeTreeAt(rev, dir)
	if err != nil {
		return packageTree{}, err
	}
	tree := packageTree{dir: dir, nested: nestedPackageDirs(files), hashes: map[string]string{}}
	for _, rel := range files {
		if !tree.ownsRelative(rel) {
			continue
		}
		data, found, err := r.fabric.CodeFileAt(rev, path.Join(dir, rel))
		if err != nil {
			return packageTree{}, err
		}
		if found {
			tree.hashes[rel] = testcover.ContentHash(data)
		}
	}
	return tree, nil
}

// nestedPackageDirs returns the directories below a package directory that hold another package or a nested module.
// They are sorted.
// files are the slash paths, relative to the package directory, of every file under it.
// A directory holds a package when it holds a Go file directly and the go command would build it:
// it lies under no testdata directory and no directory whose name opens with a dot or an underscore.
// A directory holding go.mod is a nested module wherever it lies.
// A directory whose Go files build constraints all exclude still counts as nested:
// the bound only misses files of the owner, and never adds a false one.
func nestedPackageDirs(files []string) []string {
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

// owns reports whether the package owns the worktree-relative slash path file:
// it lies under the package directory and under none of the nested directories.
func (p packageTree) owns(file string) bool {
	if p.dir == "." {
		return p.ownsRelative(file)
	}
	rel, found := strings.CutPrefix(file, p.dir+"/")
	return found && p.ownsRelative(rel)
}

// ownsRelative reports whether the package owns rel, a slash path relative to its directory.
func (p packageTree) ownsRelative(rel string) bool {
	return !slices.ContainsFunc(p.nested, func(dir string) bool { return rel == dir || strings.HasPrefix(rel, dir+"/") })
}

// otherFiles returns the hashes of the files of p but those Go files that lie directly in its directory, or nil for none.
func (p packageTree) otherFiles() map[string]string {
	other := map[string]string{}
	for rel, hash := range p.hashes {
		if path.Dir(rel) != "." || !strings.HasSuffix(rel, ".go") {
			other[rel] = hash
		}
	}
	if len(other) == 0 {
		return nil
	}
	return other
}

// dirtyOwnedFile reads the worktree's uncommitted and untracked files afresh.
// It returns the first in path order that a package of trees owns, or "" for none.
func (r *testMapRun) dirtyOwnedFile(trees map[string]packageTree) (string, error) {
	entries, err := r.fabric.Status()
	if err != nil {
		return "", fmt.Errorf("read the worktree status: %w", err)
	}
	var dirty []string
	for _, entry := range entries {
		for _, tree := range trees {
			if tree.owns(entry.Path) {
				dirty = append(dirty, entry.Path)
				break
			}
		}
	}
	if len(dirty) == 0 {
		return "", nil
	}
	return slices.Min(dirty), nil
}

// changedFiles returns the files of trees, read at the commit head, whose hash at the current HEAD differs, sorted.
// A file added or removed since counts as changed.
// It names a file of the package at importPath by its path relative to the package directory,
// and a file of any other package by its path relative to the worktree.
func (r *testMapRun) changedFiles(head, importPath string, trees map[string]packageTree) ([]string, error) {
	current, err := r.fabric.HeadSHA()
	if err != nil {
		return nil, fmt.Errorf("read HEAD: %w", err)
	}
	if current == head {
		return nil, nil
	}
	var changed []string
	for linked, before := range trees {
		after, err := r.readPackageTree(current, before.dir)
		if err != nil {
			return nil, err
		}
		for rel := range before.hashes {
			if after.hashes[rel] != before.hashes[rel] {
				changed = append(changed, changedName(linked, importPath, before.dir, rel))
			}
		}
		for rel := range after.hashes {
			if _, found := before.hashes[rel]; !found {
				changed = append(changed, changedName(linked, importPath, before.dir, rel))
			}
		}
	}
	slices.Sort(changed)
	return changed, nil
}

// changedName names the file rel of the package linked in a skip cause of the package at importPath.
// The package linked has the directory dir.
func changedName(linked, importPath, dir, rel string) string {
	if linked == importPath {
		return rel
	}
	return path.Join(dir, rel)
}
