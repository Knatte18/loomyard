// coveragemap.go declares the per-package coverage map record and reads and writes it through internal/state.

package testcover

import (
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// mapFileName is the file name of a package's coverage map within its directory.
const mapFileName = "map.json"

// Dir returns the directory under the board directory boardDir that holds the coverage maps and the exit record.
// Every worktree of the hub on the machine shares it.
func Dir(boardDir string) string {
	return filepath.Join(boardDir, lymdirs.DotLymDirName, lymdirs.TestCoverDirName)
}

// Map is one package's coverage map.
// It records which blocks of the package's own files each judged test covers, and the tree it was built from.
type Map struct {
	// Commit is the commit the map was built from.
	Commit string `json:"commit"`
	// BuiltAt is when the map was built.
	BuiltAt time.Time `json:"built_at"`
	// Files maps each Go file the package directory tracks, by name, to its ContentHash.
	Files map[string]string `json:"files"`
	// Blocks is the block key table that MapTest.Blocks indexes.
	Blocks []BlockKey `json:"blocks"`
	// Tests holds one entry per judged top-level test.
	Tests []MapTest `json:"tests"`
	// BuildFailure is the error of a coverage build that failed, empty when the build succeeded.
	BuildFailure string `json:"build_failure,omitempty"`
}

// MapTest is one judged top-level test of a coverage map.
type MapTest struct {
	Name string `json:"name"`
	// Tier is the tier the test's tiergate.Require names.
	Tier int `json:"tier"`
	// FuncHash is the ContentHash of the test function's source text.
	FuncHash string `json:"func_hash"`
	// Seconds is the test's standalone duration.
	Seconds float64 `json:"seconds"`
	// Blocks are indexes into Map.Blocks of the blocks the test covers.
	Blocks []int `json:"blocks"`
}

// MapPath returns the path of the coverage map of the package pkg under the maps directory dir.
// pkg is a module-relative slash path.
func MapPath(dir, pkg string) string {
	return filepath.Join(dir, filepath.FromSlash(pkg), mapFileName)
}

// ReadMap reads the coverage map of the package pkg under the maps directory dir, and reports whether one exists.
// A key the record does not declare is ignored, never refused.
func ReadMap(dir, pkg string) (Map, bool, error) {
	path := MapPath(dir, pkg)
	return state.ReadJSON[Map](path, lock.TrackedFileLockPath(path, ".lock"))
}

// WriteMap replaces the coverage map of the package pkg under the maps directory dir with m, atomically,
// so a reader sees either the old map or the new one.
// It keeps each key of the replaced record that Map does not declare.
func WriteMap(dir, pkg string, m Map) error {
	path := MapPath(dir, pkg)
	return state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), m)
}
