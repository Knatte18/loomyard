// coveragemap.go declares the per-package coverage map record and reads and writes it through internal/state.

package testcover

import (
	"path"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// mapFileName is the file name of a package's coverage map within its directory.
const mapFileName = "map.json"

// MapFormatVersion is the map shape a MapFormat marker names:
// The shape holds the own package's blocks and other files.
// It holds each linked package's covered functions and file hashes.
// It also holds each test's spawn verdict.
const MapFormatVersion = 2

// Dir returns the directory under the board directory boardDir that holds the coverage maps and the exit record.
// Every worktree of the hub on the machine shares it.
func Dir(boardDir string) string {
	return filepath.Join(boardDir, lymdirs.DotLymDirName, lymdirs.TestCoverDirName)
}

// Map is one package's coverage map.
// It records which blocks of the package's own files each judged test covers.
// It also records which functions of the other in-module packages its test binary links each test covers.
// It names the tree it was built from too.
type Map struct {
	// Commit is the commit the map was built from.
	Commit string `json:"commit"`
	// BuiltAt is when the map was built.
	BuiltAt time.Time `json:"built_at"`
	// Files maps each Go file the package directory tracks, by name, to its ContentHash.
	Files map[string]string `json:"files"`
	// OtherFiles maps each other tracked file of the package, by slash path relative to its directory, to its ContentHash.
	// They are the files under the directory down to the next package directory or nested module.
	// The Go files Files holds are left out; testdata is included, Go files under it among them.
	OtherFiles map[string]string `json:"other_files,omitempty"`
	// Linked maps the import path of each other in-module package the test binary links to the ContentHash of its files.
	// The files are keyed by slash path relative to that package's directory:
	// every tracked file under the directory down to the next package directory or nested module.
	Linked map[string]map[string]string `json:"linked,omitempty"`
	// Blocks is the block key table that MapTest.Blocks indexes.
	Blocks []BlockKey `json:"blocks"`
	// Funcs is the table of linked packages' functions that MapTest.Funcs indexes, sorted by CompareFuncKeys.
	Funcs []FuncKey `json:"funcs,omitempty"`
	// Tests holds one entry per judged top-level test.
	Tests []MapTest `json:"tests"`
	// BuildFailure is the error of a coverage build that failed, empty when the build succeeded.
	BuildFailure string `json:"build_failure,omitempty"`
	// Format marks a map of the MapFormatVersion shape; it is nil in a map no writer of that shape wrote.
	// Its Commit and BuiltAt equal the map's own unless a writer that does not know it rewrote the map since.
	Format *MapFormat `json:"format,omitempty"`
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
	// Blocks are indexes into Map.Blocks of the blocks of the package's own files the test covers.
	Blocks []int `json:"blocks"`
	// Funcs are indexes into Map.Funcs of the linked packages' functions the test covers.
	Funcs []int `json:"funcs,omitempty"`
	// Spawn is the static spawn scan's verdict on the test.
	// An entry whose verdict marks the test out of process or unresolved records no passing measurement.
	Spawn SpawnVerdict `json:"spawn"`
	// Outcome is why the test's run alone recorded no coverage, empty for a passing measurement.
	Outcome TestOutcome `json:"outcome,omitempty"`
}

// TestOutcome is why a coverage map's entry for a test holds no coverage: its run alone did not pass covering something.
type TestOutcome string

// The outcomes an entry records instead of a passing measurement.
const (
	OutcomeFailed     TestOutcome = "failed"
	OutcomeTimedOut   TestOutcome = "timed out"
	OutcomeSkipped    TestOutcome = "skipped"
	OutcomeNoCoverage TestOutcome = "covered nothing"
)

// MapFormat is a coverage map's format marker: the shape the map holds and the build that wrote it.
type MapFormat struct {
	// Version is the MapFormatVersion of the writer.
	Version int `json:"version"`
	// Commit and BuiltAt are the map's own, as the writer set them.
	Commit  string    `json:"commit"`
	BuiltAt time.Time `json:"built_at"`
	// Environment is what besides the module's files shaped the build.
	Environment EnvironmentStamp `json:"environment"`
}

// EnvironmentStamp records what besides the module's files shapes the coverage a map records:
// the Go toolchain, the module's requirements and the test environment the hub configures.
type EnvironmentStamp struct {
	GoVersion string `json:"go_version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	// GoModHash and GoSumHash are the ContentHash of the module's go.mod and go.sum at the map's commit.
	// Each is empty for a file the commit does not hold.
	GoModHash string `json:"go_mod_hash"`
	GoSumHash string `json:"go_sum_hash"`
	// TiersDigest digests the mapped tiers' commands and env as configured, before the `{parallel}` substitution.
	TiersDigest string `json:"tiers_digest"`
	// HubEnvDigest digests the hub's env.yaml variables the build passes its children.
	HubEnvDigest string `json:"hub_env_digest"`
}

// StampModuleFiles returns stamp with its GoModHash and GoSumHash set from the go.mod and go.sum of the module.
// moduleRel is the module's worktree-relative slash directory.
// read reads a file by worktree-relative slash path from the tree the stamp describes, found false for an absent one;
// an absent file leaves its hash empty.
// The error is read's.
func StampModuleFiles(stamp EnvironmentStamp, moduleRel string, read func(rel string) (data []byte, found bool, err error)) (EnvironmentStamp, error) {
	for _, file := range []struct {
		name string
		hash *string
	}{{"go.mod", &stamp.GoModHash}, {"go.sum", &stamp.GoSumHash}} {
		data, found, err := read(path.Join(moduleRel, file.name))
		if err != nil {
			return EnvironmentStamp{}, err
		}
		*file.hash = ""
		if found {
			*file.hash = ContentHash(data)
		}
	}
	return stamp, nil
}

// stampDifferences returns the JSON names of the fields in which the stamps a and b differ, in field order.
func stampDifferences(a, b EnvironmentStamp) []string {
	var differing []string
	for _, field := range []struct{ name, a, b string }{
		{"go_version", a.GoVersion, b.GoVersion},
		{"goos", a.GOOS, b.GOOS},
		{"goarch", a.GOARCH, b.GOARCH},
		{"go_mod_hash", a.GoModHash, b.GoModHash},
		{"go_sum_hash", a.GoSumHash, b.GoSumHash},
		{"tiers_digest", a.TiersDigest, b.TiersDigest},
		{"hub_env_digest", a.HubEnvDigest, b.HubEnvDigest},
	} {
		if field.a != field.b {
			differing = append(differing, field.name)
		}
	}
	return differing
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
