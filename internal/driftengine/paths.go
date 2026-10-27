// paths.go — the drift module's durable and ephemeral paths, declared once.

package driftengine

import (
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// driftDirName is the directory segment both the durable and the ephemeral drift directory carry.
const driftDirName = "drift"

// Paths holds every path the drift module reads or writes, joined onto one anchor root.
// The durable files lie under _lym/drift and are fabric-synced; the ephemeral files lie under .lym/drift and are never tracked.
type Paths struct {
	// Scope is the durable scope record.
	Scope string
	// BaselineReport is the Drift-Baseline agent's report.
	BaselineReport string
	// VerifyReport is the Drift-Verify agent's report.
	VerifyReport string
	// LogsDir holds the log of every capture, compare and child run.
	LogsDir string
	// Capture is the ephemeral capture record.
	Capture string
	// CaptureLock is the lock a running capture holds.
	CaptureLock string
	// CompareMarker is the lock a running compare holds.
	CompareMarker string
	// BaselineDir is the directory the capture writes its profile files to.
	BaselineDir string
	// SideDir holds the numbered directories of side captures.
	SideDir string
	// SkipFile is the file naming one skipped test per line.
	SkipFile string
	// Runs is the run counter of the Drift-Verify agent session.
	Runs string
}

// NewPaths returns the drift paths joined onto the told anchor root.
func NewPaths(anchorRoot string) Paths {
	durable := filepath.Join(anchorRoot, lymdirs.LymDirName, driftDirName)
	ephemeral := filepath.Join(anchorRoot, lymdirs.DotLymDirName, driftDirName)
	return Paths{
		Scope:          filepath.Join(durable, "scope.json"),
		BaselineReport: filepath.Join(durable, "baseline.md"),
		VerifyReport:   filepath.Join(durable, "verify-report.md"),
		LogsDir:        filepath.Join(durable, "logs"),
		Capture:        filepath.Join(ephemeral, "capture.json"),
		CaptureLock:    filepath.Join(ephemeral, "capture.lock"),
		CompareMarker:  filepath.Join(ephemeral, "compare.lock"),
		BaselineDir:    filepath.Join(ephemeral, "baseline"),
		SideDir:        filepath.Join(ephemeral, "side"),
		SkipFile:       filepath.Join(ephemeral, "skip.txt"),
		Runs:           filepath.Join(ephemeral, "runs.json"),
	}
}

// DirRel returns the durable drift directory relative to the anchor root, `_lym/drift`, always forward-slashed, for a commit pathspec.
func DirRel() string {
	return path.Join(lymdirs.LymDirName, driftDirName)
}
