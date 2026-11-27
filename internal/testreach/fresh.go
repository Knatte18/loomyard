// fresh.go compares a coverage map's recorded file hashes with the files on disk.

package testreach

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/testcover"
)

// Fresh reports whether the hashes the map m of the package pkg records match the worktree's files under module.Dir.
// pkg is a module-relative slash path.
// The map's Files and OtherFiles are compared with the package's files.
// Each Linked package's hashes are compared with that package's files.
// Each file is hashed by testcover.ContentHash.
// A file on one side alone makes the map stale.
// It reads the disk only.
func Fresh(module Module, pkg string, m testcover.Map) (bool, error) {
	onDisk, err := diskPackageHashes(module, pkg)
	if err != nil {
		return false, err
	}
	recorded := make(map[string]string, len(m.Files)+len(m.OtherFiles))
	maps.Copy(recorded, m.Files)
	maps.Copy(recorded, m.OtherFiles)
	if !maps.Equal(onDisk, recorded) {
		return false, nil
	}
	for linked, hashes := range m.Linked {
		rel, inModule := relativePackage(module, linked)
		if !inModule {
			return false, nil
		}
		linkedOnDisk, err := diskPackageHashes(module, rel)
		if err != nil {
			return false, err
		}
		if !maps.Equal(linkedOnDisk, hashes) {
			return false, nil
		}
	}
	return true, nil
}

// importPathOf returns the import path of the package at the module-relative slash path pkg.
func importPathOf(module Module, pkg string) string {
	if pkg == "." {
		return module.Path
	}
	return module.Path + "/" + pkg
}

// relativePackage returns the module-relative slash path of the package at importPath, false for one outside the module.
func relativePackage(module Module, importPath string) (string, bool) {
	if importPath == module.Path {
		return ".", true
	}
	return strings.CutPrefix(importPath, module.Path+"/")
}

// diskPackageHashes returns the ContentHash of each file that the package at the module-relative slash path pkg owns.
// The result is keyed by slash path relative to the package directory.
// The package owns every file under its directory down to the next package directory or nested module.
// An absent directory owns none.
func diskPackageHashes(module Module, pkg string) (map[string]string, error) {
	dir := filepath.Join(module.Dir, filepath.FromSlash(pkg))
	var files []string
	err := filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list the files of %s: %w", pkg, err)
	}
	nested := testcover.NestedPackageDirs(files)
	hashes := map[string]string{}
	for _, rel := range files {
		if slices.ContainsFunc(nested, func(nestedDir string) bool { return strings.HasPrefix(rel, nestedDir+"/") }) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("read %s of %s: %w", path.Base(rel), pkg, err)
		}
		hashes[rel] = testcover.ContentHash(data)
	}
	return hashes, nil
}

// digestHashes returns one digest of a file-hash table, independent of the table's order.
func digestHashes(hashes map[string]string) string {
	var lines strings.Builder
	for _, name := range slices.Sorted(maps.Keys(hashes)) {
		fmt.Fprintf(&lines, "%s=%s\n", name, hashes[name])
	}
	return testcover.ContentHash([]byte(lines.String()))
}
