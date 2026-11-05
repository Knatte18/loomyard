// paths.go — the drift module's durable and ephemeral paths, declared once.

package driftengine

import (
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// driftDirName is the directory segment both the durable and the ephemeral drift directory carry.
const driftDirName = "drift"

// The names of the durable files the records guard, the seal and the judge read.
const (
	scopeFileName     = "scope.json"
	judgedFileName    = "judged.json"
	expectedFileName  = "expected.yaml"
	judgementFileName = "judgement.yaml"
)

// Paths holds every path the drift module reads or writes, joined onto one anchor root.
// The durable files lie under _lym/drift and are fabric-synced; the ephemeral files lie under .lym/drift and are never tracked.
type Paths struct {
	// Scope is the durable scope record.
	Scope string
	// Judged is the durable record of the judge's stored rulings; it is guarded together with Scope.
	Judged string
	// Expected is the expectation the expect agent writes, which Drift-Expect seals.
	Expected string
	// Judgement is the file the judge writes its rulings to, outside the records guard; Drift-Verify removes it once the rulings are stored.
	Judgement string
	// LogsDir holds the log of every capture, compare and child run.
	LogsDir string
	// Capture is the ephemeral capture record.
	Capture string
	// CaptureLock is the lock a running capture holds.
	CaptureLock string
	// CompareMarker is the lock a running compare holds.
	CompareMarker string
	// Listing is the ephemeral identity listing of the baseline's cases, which Drift-Expect writes for the expect agent to read.
	Listing string
	// JudgeRequest is the ephemeral request Drift-Verify writes before each judge spawn.
	JudgeRequest string
	// BaselineDir is the directory the capture writes its profile files to.
	BaselineDir string
	// CurrentDir holds the directory and log of every current capture a check ran.
	CurrentDir string
}

// NewPaths returns the drift paths joined onto the told anchor root.
func NewPaths(anchorRoot string) Paths {
	durable := filepath.Join(anchorRoot, lymdirs.LymDirName, driftDirName)
	ephemeral := filepath.Join(anchorRoot, lymdirs.DotLymDirName, driftDirName)
	return Paths{
		Scope:         filepath.Join(durable, scopeFileName),
		Judged:        filepath.Join(durable, judgedFileName),
		Expected:      filepath.Join(durable, expectedFileName),
		Judgement:     filepath.Join(durable, judgementFileName),
		LogsDir:       filepath.Join(durable, "logs"),
		Capture:       filepath.Join(ephemeral, "capture.json"),
		CaptureLock:   filepath.Join(ephemeral, "capture.lock"),
		CompareMarker: filepath.Join(ephemeral, "compare.lock"),
		Listing:       filepath.Join(ephemeral, "listing.yaml"),
		JudgeRequest:  filepath.Join(ephemeral, "judge-request.json"),
		BaselineDir:   filepath.Join(ephemeral, "baseline"),
		CurrentDir:    filepath.Join(ephemeral, "current"),
	}
}

// DirRel returns the durable drift directory relative to the anchor root, `_lym/drift`, always forward-slashed, for a commit pathspec.
func DirRel() string {
	return path.Join(lymdirs.LymDirName, driftDirName)
}

// durableRel returns the anchor-relative, forward-slashed path of the durable drift file called name.
func durableRel(name string) string {
	return path.Join(DirRel(), name)
}
