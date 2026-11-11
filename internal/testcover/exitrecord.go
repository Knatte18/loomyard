// exitrecord.go declares the record a coverage map upkeep pass writes as it exits.
// It reads and writes the record through internal/state.

package testcover

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// ExitRecord is what one coverage map upkeep pass leaves as it exits.
// Its process ID tells a reader the pass it waited on from any other.
type ExitRecord struct {
	PID int `json:"pid"`
	// PlannedHead is the prime's HEAD the pass planned and measured at, empty when it ended before reading it.
	PlannedHead string `json:"planned_head,omitempty"`
	// Queued counts the queued work left when the pass ended:
	// the packages its package cap left unmeasured, and those whose map build lock another call held.
	Queued int `json:"queued"`
	// Plan counts the plan's verdicts.
	Plan PlanCounts `json:"plan"`
	// SelfCheckMismatches counts the kept entries a build at the planned HEAD could not place, so the pass re-measured them.
	SelfCheckMismatches int `json:"self_check_mismatches"`
	// AuditMismatches counts the entries the closing audit round re-measured to a different result.
	AuditMismatches int `json:"audit_mismatches"`
	// Packages holds one line per package the pass built and ran tests for, in order.
	Packages []PackageMeasure `json:"packages,omitempty"`
}

// PlanCounts are an upkeep plan's verdict counts over the maps it planned.
type PlanCounts struct {
	// Current counts the maps with no affected test and no miss.
	Current int `json:"current"`
	// Affected counts the maps with affected tests.
	Affected int `json:"affected"`
	// Misses counts the maps the selector missed for, by reason.
	Misses map[MissReason]int `json:"misses,omitempty"`
}

// PackageMeasure is the time one package's measurement took.
type PackageMeasure struct {
	Package string `json:"package"`
	// BuildSeconds is the coverage build's duration.
	BuildSeconds float64 `json:"build_seconds"`
	// RunSeconds is the duration of the package's test runs together.
	RunSeconds float64 `json:"run_seconds"`
	// Tests counts the test runs.
	Tests int `json:"tests"`
}

// ExitRecordPath returns the path of the exit record under the maps directory dir.
func ExitRecordPath(dir string) string {
	return filepath.Join(dir, lymdirs.TestMapExitFileName)
}

// WriteExitRecord replaces the exit record under the maps directory dir with r, atomically.
// It keeps each key of the replaced record that ExitRecord does not declare.
func WriteExitRecord(dir string, r ExitRecord) error {
	path := ExitRecordPath(dir)
	return state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), r)
}

// ReadExitRecord reads the exit record under the maps directory dir, and reports whether one exists.
// A key the record does not declare is ignored, never refused, so a record that still holds the retired `unbuilt` reads.
func ReadExitRecord(dir string) (ExitRecord, bool, error) {
	path := ExitRecordPath(dir)
	return state.ReadJSON[ExitRecord](path, lock.TrackedFileLockPath(path, ".lock"))
}
