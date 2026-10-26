// deps.go declares Deps, the seams every pawl producer is told, the fail-loud check that each one a producer uses is set, and the context checks the producers share.

package pawlengine

import (
	"context"
	"fmt"
	"time"
)

// Step is one named, idempotent step of Beam-Up.
type Step struct {
	// Name names the step in a failure.
	Name string
	// Run does the step.
	Run func() error
}

// Deps carries every told value and seam of the pawl producers.
// A producer refuses a nil seam it uses when it is built, naming the field; Watch alone may be nil.
type Deps struct {
	// Slug is the beam's slug, which names its landing and start in a notice.
	Slug string
	// Paths holds the run's land request, dobby-stop marker and await marker.
	Paths Paths
	// RunID is the pawl run's seed id, which a land request names.
	RunID string
	// SetStatus writes the beam project entry's run status naming producer as the row the run is at.
	SetStatus func(producer string) error
	// EntryProducer reads the producer the entry's run status names.
	EntryProducer func() (string, error)
	// OwnedElsewhere reports whether another machine owns the beam.
	OwnedElsewhere func() (bool, error)
	// MarkBeam marks the pair a beam; it is a no-op on a marked pair.
	MarkBeam func() error
	// SpawnPush starts the beam's push.
	SpawnPush func() error
	// RegenerateWorkspace regenerates the hub's workspace file.
	RegenerateWorkspace func() error
	// Steps are the beam steps Beam-Up runs, in order, with no config reconcile.
	Steps []Step
	// StopDobby stops the beam's dobby so it starts no new member.
	StopDobby func() error
	// StartDobby starts the beam's dobby again.
	StartDobby func() error
	// DobbyLive reports whether the beam's dobby runs.
	DobbyLive func() (bool, error)
	// Recheck returns the blockers the landing re-check finds, empty when it passes.
	Recheck func() ([]string, error)
	// WriteSnapshot writes the snapshot the perch compares against.
	WriteSnapshot func() error
	// Notify posts one line to the beam's own orch.
	Notify func(line string) error
	// Down takes the beam's dobby and orch down.
	Down func() error
	// Watch delivers a value whenever the land request's directory changes; nil leaves the timer alone.
	Watch <-chan struct{}
	// Now reads the clock.
	Now func() time.Time
	// Sleep waits d, returning the context's error when ctx ends first.
	Sleep func(ctx context.Context, d time.Duration) error
}

// seam names one field of Deps a producer uses and whether it is set.
type seam struct {
	field string
	set   bool
}

// requireSeams returns an error naming the first of seams that is unset, on behalf of producer.
func requireSeams(producer string, seams ...seam) error {
	for _, s := range seams {
		if !s.set {
			return fmt.Errorf("pawlengine: %s: Deps.%s is unset", producer, s.field)
		}
	}
	return nil
}

// pathsSeams are the Paths fields every producer that reads or writes the run's files needs.
func (d Deps) pathsSeams() []seam {
	return []seam{
		{"Paths.LandRequest", d.Paths.LandRequest != ""},
		{"Paths.DobbyStopMarker", d.Paths.DobbyStopMarker != ""},
		{"Paths.AwaitMarker", d.Paths.AwaitMarker != ""},
	}
}

// entryErr returns nil when ctx is not yet cancelled, and otherwise an error naming the producer that never started.
func entryErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("pawlengine: %s: context cancelled before run started: %w", name, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled, and otherwise an error naming the producer a cancellation ended.
// Every non-Done exit consults it first, so a cancellation is never read as a verdict.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("pawlengine: %s: context cancelled during run: %w", name, ctx.Err())
}
