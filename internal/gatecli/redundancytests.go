// redundancytests.go picks the tests the redundancy report step judges, from the working tree or a commit's tree.
// It takes each top-level test of a changed test file whose text the merge base lacks.
// It also hashes the Go files of a package as a tree holds them.

package gatecli

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// redundancyTree reads the files of one tree: the working tree, or the tree a commit stores.
type redundancyTree interface {
	// file returns the worktree-relative slash path rel as the tree holds it, and whether the tree holds it.
	file(rel string) ([]byte, bool, error)
	// goFiles returns the names of the Go files directly in the worktree-relative slash directory dir, sorted.
	// A directory the tree lacks holds none.
	goFiles(dir string) ([]string, error)
}

// workingTree is the working tree of the worktree at root, untracked files included.
type workingTree struct {
	root string
}

func (w workingTree) file(rel string) ([]byte, bool, error) {
	data, err := os.ReadFile(filepath.Join(w.root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (w workingTree) goFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(w.root, filepath.FromSlash(dir)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// commitTree is the tree the commit rev stores in the worktree's code checkout.
type commitTree struct {
	fabric *fabricengine.Fabric
	rev    string
}

func (c commitTree) file(rel string) ([]byte, bool, error) {
	return c.fabric.CodeFileAt(c.rev, rel)
}

func (c commitTree) goFiles(dir string) ([]string, error) {
	names, err := c.fabric.CodeDirAt(c.rev, dir)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(names, func(name string) bool { return !strings.HasSuffix(name, ".go") }), nil
}

// redundancyCandidate is one test the step's selection takes: a current top-level test of a changed test file.
type redundancyCandidate struct {
	// pkg is the module-relative slash path of the test's package, "." for the module root.
	pkg string
	// file is the worktree-relative slash path of the test's file.
	file string
	fn   testcover.TestFunc
}

// changedTests returns the tests the step takes from tree, sorted by file and then name.
// changed are the worktree-relative slash paths that differ between the merge base base and tree.
// moduleRel is the worktree-relative slash path of the Go module; a path outside a package of it is passed over.
// For each package holding a changed `_test.go` file, it reads the package's test files at base through fabric.
// It takes each top-level test of a changed file that tree holds unless a test of that name at base has the same text.
// So a test file moved within its package adds none of its unchanged tests, and a deleted test file adds none.
func changedTests(tree redundancyTree, fabric *fabricengine.Fabric, base, moduleRel string, changed []string) ([]redundancyCandidate, error) {
	filesByDir := map[string][]string{}
	for _, rel := range changed {
		if !strings.HasSuffix(rel, "_test.go") {
			continue
		}
		if dir := path.Dir(rel); isModulePackage(moduleRel, dir) {
			filesByDir[dir] = append(filesByDir[dir], rel)
		}
	}
	var taken []redundancyCandidate
	for _, dir := range slices.Sorted(maps.Keys(filesByDir)) {
		pkg := modulePackageOf(moduleRel, dir)
		var current []redundancyCandidate
		files := slices.Sorted(slices.Values(filesByDir[dir]))
		for _, file := range slices.Compact(files) {
			data, found, err := tree.file(file)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", file, err)
			}
			if !found {
				continue
			}
			funcs, err := testcover.ParseTestFuncs(file, data)
			if err != nil {
				return nil, err
			}
			for _, fn := range funcs {
				current = append(current, redundancyCandidate{pkg: pkg, file: file, fn: fn})
			}
		}
		if len(current) == 0 {
			continue
		}
		atBase, err := testHashesAt(fabric, base, dir)
		if err != nil {
			return nil, err
		}
		for _, candidate := range current {
			if hash, found := atBase[candidate.fn.Name]; !found || hash != candidate.fn.Hash {
				taken = append(taken, candidate)
			}
		}
	}
	slices.SortFunc(taken, func(a, b redundancyCandidate) int {
		return cmp.Or(strings.Compare(a.file, b.file), strings.Compare(a.fn.Name, b.fn.Name))
	})
	return taken, nil
}

// testHashesAt returns the function hash of each top-level test, by name.
// It reads the `_test.go` files directly in dir at the commit rev.
// dir is worktree-relative and slash-separated; a directory absent at rev holds none.
func testHashesAt(fabric *fabricengine.Fabric, rev, dir string) (map[string]string, error) {
	names, err := fabric.CodeDirAt(rev, dir)
	if err != nil {
		return nil, err
	}
	hashes := map[string]string{}
	for _, name := range names {
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		rel := path.Join(dir, name)
		data, found, err := fabric.CodeFileAt(rev, rel)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		funcs, err := testcover.ParseTestFuncs(rel, data)
		if err != nil {
			return nil, fmt.Errorf("at %s: %w", rev, err)
		}
		for _, fn := range funcs {
			hashes[fn.Name] = fn.Hash
		}
	}
	return hashes, nil
}

// isModulePackage reports whether the worktree-relative slash directory dir is a package of the module at moduleRel.
// It is one a package walk enters: no testdata, and no directory whose name opens with a dot or an underscore.
func isModulePackage(moduleRel, dir string) bool {
	if moduleRel != "." && dir != moduleRel && !strings.HasPrefix(dir, moduleRel+"/") {
		return false
	}
	pkg := modulePackageOf(moduleRel, dir)
	if pkg == "." {
		return true
	}
	return !slices.ContainsFunc(strings.Split(pkg, "/"), func(segment string) bool {
		return segment == "testdata" || strings.HasPrefix(segment, ".") || strings.HasPrefix(segment, "_")
	})
}

// modulePackageOf returns the module-relative slash path of the package whose worktree-relative directory is dir.
// dir lies in the module at moduleRel.
func modulePackageOf(moduleRel, dir string) string {
	switch {
	case moduleRel == ".":
		return dir
	case dir == moduleRel:
		return "."
	}
	return strings.TrimPrefix(dir, moduleRel+"/")
}

// packageDirOf returns the worktree-relative slash directory of the package pkg of the module at moduleRel.
func packageDirOf(moduleRel, pkg string) string {
	return path.Join(moduleRel, pkg)
}

// goFileHashes returns the ContentHash of each Go file, by name.
// It reads the files directly in the worktree-relative slash directory dir of tree.
func goFileHashes(tree redundancyTree, dir string) (map[string]string, error) {
	names, err := tree.goFiles(dir)
	if err != nil {
		return nil, err
	}
	hashes := map[string]string{}
	for _, name := range names {
		data, found, err := tree.file(path.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if found {
			hashes[name] = testcover.ContentHash(data)
		}
	}
	return hashes, nil
}

// testHashesOf returns the function hash of each of tests by package and then name, the form a report records them in.
func testHashesOf(tests []redundancyCandidate) map[string]map[string]string {
	hashes := map[string]map[string]string{}
	for _, test := range tests {
		if hashes[test.pkg] == nil {
			hashes[test.pkg] = map[string]string{}
		}
		hashes[test.pkg][test.fn.Name] = test.fn.Hash
	}
	return hashes
}

// goModule is the Go module the step judges, the one whose packages the coverage maps hold.
type goModule struct {
	// dir is the module root, rel its worktree-relative slash path and path its import path.
	dir, rel, path string
}

// importPath returns the import path of the module-relative package pkg.
func (m goModule) importPath(pkg string) string {
	if pkg == "." {
		return m.path
	}
	return m.path + "/" + pkg
}

// redundancyModule returns the module of the worktree at root that the coverage maps hold:
// the dir of the lowest of tier0 and tier1 that cfg declares, whose go.mod names the import path.
func redundancyModule(cfg gateslot.Config, root string) (goModule, error) {
	for _, number := range mappedTiers {
		tier, err := cfg.Tier(number)
		if err != nil {
			continue
		}
		dir := resolveRunDir(root, tier.Dir)
		rel, err := filepath.Rel(root, dir)
		modulePath := modulePathOf(dir)
		if modulePath == "" || err != nil || !filepath.IsLocal(rel) {
			return goModule{}, fmt.Errorf("%s holds no go.mod naming a module inside the worktree", dir)
		}
		return goModule{dir: dir, rel: filepath.ToSlash(rel), path: modulePath}, nil
	}
	return goModule{}, errors.New("the hub declares neither tier0 nor tier1, so no coverage map holds a test")
}
