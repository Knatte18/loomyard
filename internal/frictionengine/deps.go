// deps.go declares the seams Reflect drives: Shuttle over one shuttle run, and Clock over the archive timestamp.
// It also declares Deps, whose every value the caller tells, and Report, the value Reflect returns.

package frictionengine

import (
	"time"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Shuttle is the seam this package drives.
// Run spawns the reflection agent, and Attach finds one that a killed driving process left alive.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
	Attach(shuttleengine.Spec) (shuttleengine.Result, bool, error)
}

var _ Shuttle = (*shuttleengine.Runner)(nil)

// Clock is the seam the archive timestamp is read through,
// so a test can assert an exact archive directory name rather than a pattern.
type Clock interface {
	Now() time.Time
}

// realClock is the package-local Clock a nil Deps.Clock selects, wrapping time.Now.
type realClock struct{}

// Now implements Clock by returning time.Now().
func (realClock) Now() time.Time {
	return time.Now()
}

// Deps carries every value a reflection run needs.
// The caller tells all of them, and this package derives none.
type Deps struct {
	// Shuttle is the seam this package spawns the reflection agent through. Told by the caller.
	Shuttle Shuttle
	// FrictionDir is the absolute friction directory Reflect scans and archives covered files out of.
	// It is never renamed or removed.
	// Told by the caller.
	FrictionDir string
	// ArchivePrefix is the absolute prefix an archive sibling's timestamp is appended to.
	// Told by the caller.
	ArchivePrefix string
	// StencilsDir is the absolute directory the reflection prompt is read from. Told by the caller.
	StencilsDir string
	// ParentName is the name of the session the reflection agent escalates to, rendered into its parent directive.
	// Told by the caller; empty renders the no-parent variant.
	ParentName string
	// FrictionSpec is the model-spec string (see internal/modelspec) that selects the reflection agent's model.
	// Told by the caller.
	FrictionSpec string
	// Registry resolves FrictionSpec's alias, when it is one, to a concrete model.
	// Told by the caller.
	Registry modelspec.Registry
	// TaskSlug names the task the notes came from.
	// The reflection agent writes it into each issue's provenance line.
	// Told by the caller.
	TaskSlug string
	// Timeout bounds one Reflect call's wall-clock budget, attach wait and spawn together.
	// Zero defers to shuttle's own run_timeout_min for each spec, with no budget across them.
	// Told by the caller.
	Timeout time.Duration
	// Clock is the archive timestamp's seam.
	// A nil Clock selects a package-local realClock, which wraps time.Now.
	Clock Clock
}

// Report is Reflect's return value.
type Report struct {
	// Status is one of StatusSkipped, StatusReflected, or StatusFailed.
	Status string
	// ReportPath is the post-archive path of the reflection agent's report file, set only for StatusReflected.
	ReportPath string
	// NoteCount is the number of friction notes the last archive covered;
	// on a failure, the notes the failed step concerned.
	NoteCount int
}

// The three outcomes Reflect can report.
// There is no "filed" status: nothing in Go parses the agent's report file,
// so this package cannot tell whether an issue was created.
const (
	// StatusSkipped reports that the scan found no notes worth reflecting on,
	// so no reflection agent was spawned.
	StatusSkipped = "skipped"
	// StatusReflected reports that a reflection agent ran to completion and its covered files were archived.
	StatusReflected = "reflected"
	// StatusFailed reports a runtime failure below the validation line.
	// A Deps validation failure returns a non-nil error instead.
	// This status therefore covers only failures that come after validation.
	StatusFailed = "failed"
)
