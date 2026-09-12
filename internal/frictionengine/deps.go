// deps.go declares this package's told-value surface: the two narrow seams Reflect drives (Shuttle
// over one shuttle run, Clock over the archive timestamp), the Deps struct every value in which is
// told by the caller and derived by nobody here, and the Report Reflect returns.

package frictionengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Shuttle is the one-method seam this package drives to spawn the reflection agent, the same shape
// mergeresolve.Shuttle already uses over shuttleengine.Runner.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// The compile-time assertion that *shuttleengine.Runner satisfies Shuttle.
var _ Shuttle = (*shuttleengine.Runner)(nil)

// Clock is the archive timestamp's seam, so a test can assert an exact archive directory name rather
// than a pattern.
type Clock interface {
	Now() time.Time
}

// realClock is the package-local Clock a nil Deps.Clock selects, wrapping time.Now.
type realClock struct{}

// Now implements Clock by returning time.Now().
func (realClock) Now() time.Time {
	return time.Now()
}

// Deps carries every value Reflect needs, all of it told by the caller and derived by nobody in this
// package.
type Deps struct {
	// Shuttle is the seam this package spawns the reflection agent through. Told by the caller.
	Shuttle Shuttle
	// FrictionDir is the absolute friction directory Reflect scans, archives, and recreates. Told by
	// the caller.
	FrictionDir string
	// ArchivePrefix is the absolute prefix an archive sibling's timestamp is appended to. Told by the
	// caller.
	ArchivePrefix string
	// StencilsDir is the absolute directory the reflection prompt is read from. Told by the caller.
	StencilsDir string
	// FrictionSpec is the model-spec string (see internal/modelspec) selecting the reflection agent's
	// model. Told by the caller.
	FrictionSpec string
	// Registry resolves FrictionSpec's alias, when it is one, to a concrete model. Told by the
	// caller.
	Registry modelspec.Registry
	// Timeout bounds the reflection session's wall-clock deadline. Told by the caller.
	Timeout time.Duration
	// Clock is the archive timestamp's seam. A nil Clock selects a package-local realClock wrapping
	// time.Now.
	Clock Clock
}

// Report is Reflect's return value.
type Report struct {
	// Status is one of StatusSkipped, StatusReflected, or StatusFailed.
	Status string
	// ReportPath is the reflection agent's own report file, populated only for StatusReflected, and
	// carrying the post-archive path.
	ReportPath string
	// NoteCount is the number of friction notes the scan found.
	NoteCount int
}

// The three outcomes Reflect can report. There is deliberately no "filed" status: nothing in Go
// parses the agent's report file, so whether an issue was actually created is not something this
// package can honestly assert.
const (
	// StatusSkipped reports that the scan found no notes worth reflecting on, so no agent was
	// spawned.
	StatusSkipped = "skipped"
	// StatusReflected reports that the reflection agent ran to completion and the friction directory
	// was archived.
	StatusReflected = "reflected"
	// StatusFailed reports a runtime failure: a Deps validation failure never reaches this status
	// (that returns a non-nil error instead), so StatusFailed is reserved for a failure below the
	// validation line.
	StatusFailed = "failed"
)
