// gomodule.go reads the Go module a hub's tiers build and the source trees of a worktree and of a commit.
// It tells a module's packages from the rest of a tree by their worktree-relative paths.

package gatecli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// moduleTiers are the tiers whose dir names the Go module, lowest first.
var moduleTiers = []int{0, 1}

// sourceTree reads the files of one tree: the working tree, or the tree a commit stores.
type sourceTree interface {
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

// inModule reports whether the worktree-relative slash directory dir is the module's root at moduleRel or lies under it.
func inModule(moduleRel, dir string) bool {
	return moduleRel == "." || dir == moduleRel || strings.HasPrefix(dir, moduleRel+"/")
}

// isModulePackage reports whether the worktree-relative slash directory dir is a package of the module at moduleRel.
// It is one a package walk enters: no testdata, and no directory whose name opens with a dot or an underscore.
func isModulePackage(moduleRel, dir string) bool {
	if !inModule(moduleRel, dir) {
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

// goModule is a Go module of a worktree.
type goModule struct {
	// dir is the module root, rel its worktree-relative slash path and path its import path.
	dir, rel, path string
}

// relativePackage returns the module-relative slash path of the package at the import path importPath.
// A package outside the module keeps its import path.
func (m goModule) relativePackage(importPath string) string {
	if importPath == m.path {
		return "."
	}
	if rel, inside := strings.CutPrefix(importPath, m.path+"/"); inside {
		return rel
	}
	return importPath
}

// goModuleOf returns the Go module of the worktree at root:
// the dir of the lowest of tier0 and tier1 that cfg declares, whose go.mod names the import path.
func goModuleOf(cfg gateslot.Config, root string) (goModule, error) {
	for _, number := range moduleTiers {
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
	return goModule{}, errors.New("the hub declares neither tier0 nor tier1, so no Go module is known")
}

// testreachModule returns module as the testreach package names it.
func testreachModule(module goModule) testreach.Module {
	return testreach.Module{Dir: module.dir, Rel: module.rel, Path: module.path}
}
