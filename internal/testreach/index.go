// index.go declares the index file of the shared coverage maps and the stamp that says whether it is current.

package testreach

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// indexFileName is the file name of the index beside the shared maps.
const indexFileName = "index.json"

// indexVersion is the shape of the index file; an index of another shape is read as absent.
const indexVersion = 2

// indexFile is the reverse table of the shared maps, from each function to the tests that reach it, as a cache file.
type indexFile struct {
	Version int `json:"version"`
	// Stamp is the directoryStamp of the maps directory the table was built from.
	Stamp string `json:"stamp"`
	// Packages holds what the reader needs of each map but its tests, by module-relative package.
	Packages map[string]packageInfo `json:"packages"`
	Reaches  []funcReaches          `json:"reaches"`
}

// funcReaches is the tests of the shared maps that reach one function.
type funcReaches struct {
	Func  testcover.FuncKey `json:"func"`
	Tests []reachEntry      `json:"tests"`
}

// packageInfo is what the reader keeps of one package's map besides its reach entries.
type packageInfo struct {
	Commit  string    `json:"commit"`
	BuiltAt time.Time `json:"built_at"`
	// Unseen holds the tests of the map that may run a function the map does not show them reaching, sorted by name.
	Unseen []UnseenTest `json:"unseen,omitempty"`
	// LinkedDigests maps each linked package's import path to the digestHashes of the file hashes the map recorded for it.
	LinkedDigests map[string]string `json:"linked_digests,omitempty"`
	// Worktree is set for a map read from the worktree's own maps; it is never stored.
	Worktree bool `json:"-"`
}

// reachEntry is one test that reaches a function, with the map it came from.
type reachEntry struct {
	// MapPackage is the module-relative package whose map holds the test.
	MapPackage string `json:"map_package"`
	// Package is the import path of the test's package.
	Package string `json:"package"`
	Test    string `json:"test"`
	// Funcs is how many functions the test's entry reaches, own and linked together.
	Funcs int `json:"funcs"`
}

// mapTable is the reverse table of a set of maps and the facts kept per package.
type mapTable struct {
	Packages map[string]packageInfo
	Reaches  map[testcover.FuncKey][]reachEntry
}

// WriteIndex writes the index of the shared coverage maps of module in sharedDir, beside the maps.
// It builds the reverse table from the maps as Open builds it, and stamps it with the directory's stamp.
// The stamp is read before the maps, so a map rewritten meanwhile makes the next Open ignore the index.
// Bound: a map rewritten at the same size within one modification-time tick keeps its stamp.
// Open can then serve the index's old pairs until the next WriteIndex.
// A failed write returns an error naming the index path; the index is a cache, so no reader depends on it.
func WriteIndex(module Module, sharedDir string) error {
	stamp, err := directoryStamp(sharedDir)
	if err != nil {
		return err
	}
	coverageMaps, err := readMapDir(sharedDir)
	if err != nil {
		return err
	}
	table := buildTable(module, coverageMaps, false)
	index := indexFile{Version: indexVersion, Stamp: stamp, Packages: table.Packages}
	for fn, tests := range table.Reaches {
		slices.SortFunc(tests, func(a, b reachEntry) int {
			return cmp.Or(strings.Compare(a.MapPackage, b.MapPackage), strings.Compare(a.Test, b.Test))
		})
		index.Reaches = append(index.Reaches, funcReaches{Func: fn, Tests: tests})
	}
	slices.SortFunc(index.Reaches, func(a, b funcReaches) int { return testcover.CompareFuncKeys(a.Func, b.Func) })
	path := indexPath(sharedDir)
	if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), index); err != nil {
		return fmt.Errorf("write the index %s: %w", path, err)
	}
	return nil
}

// indexPath returns the path of the index file beside the maps in dir.
func indexPath(dir string) string {
	return filepath.Join(dir, indexFileName)
}

// readIndex reads the index beside the maps in dir.
// It reports false for an index that is absent, does not parse or has another version;
// the index is a cache, so none of those stops the reader.
func readIndex(dir string) (indexFile, bool) {
	// The read takes no lock, since WriteIndex replaces the file atomically; a lock file would be written beside the maps.
	data, err := os.ReadFile(indexPath(dir))
	if err != nil {
		return indexFile{}, false
	}
	var index indexFile
	if err := json.Unmarshal(data, &index); err != nil || index.Version != indexVersion {
		return indexFile{}, false
	}
	return index, true
}

// directoryStamp returns a text that changes when a map file is added to or removed from the maps directory dir.
// The text also changes when a map file changes its size or modification time.
// An absent directory has the stamp of one with no map.
func directoryStamp(dir string) (string, error) {
	packages, err := mapFilePackages(dir)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(packages))
	for _, pkg := range packages {
		info, err := os.Stat(testcover.MapPath(dir, pkg))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("stamp the coverage map of %s: %w", pkg, err)
		}
		lines = append(lines, fmt.Sprintf("%s|%d|%d", pkg, info.Size(), info.ModTime().UnixNano()))
	}
	slices.Sort(lines)
	return testcover.ContentHash([]byte(strings.Join(lines, "\n"))), nil
}
