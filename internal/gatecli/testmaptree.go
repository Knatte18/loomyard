// testmaptree.go reads, for `lym gate test --map`, the files a package owns at a commit by the path-to-package rule.
// It hashes them and finds the uncommitted and untracked ones.

package gatecli

import (
	"fmt"
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
	tree := packageTree{dir: dir, nested: testcover.NestedPackageDirs(files), hashes: map[string]string{}}
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
// Only the code side counts: the records side holds no file of a package.
func (r *testMapRun) dirtyOwnedFile(trees map[string]packageTree) (string, error) {
	paths, err := r.fabric.CodeWorktreeChanges()
	if err != nil {
		return "", fmt.Errorf("read the worktree status: %w", err)
	}
	var dirty []string
	for _, changed := range paths {
		for _, tree := range trees {
			if tree.owns(changed) {
				dirty = append(dirty, changed)
				break
			}
		}
	}
	if len(dirty) == 0 {
		return "", nil
	}
	return slices.Min(dirty), nil
}

// dirtyModuleFile reads the worktree's uncommitted and untracked files afresh.
// It returns the first in path order that lies under the module root, or "" for none.
// Only the code side counts: the records side holds no file of the module.
func (r *testMapRun) dirtyModuleFile() (string, error) {
	paths, err := r.fabric.CodeWorktreeChanges()
	if err != nil {
		return "", fmt.Errorf("read the worktree status: %w", err)
	}
	var dirty []string
	for _, changed := range paths {
		if _, inModule := r.moduleRelative(changed); inModule {
			dirty = append(dirty, changed)
		}
	}
	if len(dirty) == 0 {
		return "", nil
	}
	return slices.Min(dirty), nil
}
