// producer.go defines the ShedProducer seam Shed drives once per iteration -- the minimal contract
// a producer must satisfy so Shed can call, resume, and route it uniformly regardless of what the
// producer does internally.

package shedengine

import "context"

// Outcome is one producer call's verdict.
type Outcome string

// The two legal Outcome values. A ShedProducer must return exactly one of these;
// any other value is an engine-level failure, not a third verdict.
const (
	Done  Outcome = "done"
	Stuck Outcome = "stuck"
)

// OutputPointer names a producer's artifact for a human to read.
// Shed never introspects Path's contents, never validates it, and never stats it to make a
// control-flow decision -- step 4 is an unconditional re-call.
type OutputPointer struct {
	Path string // "" = no artifact (gate or terminal producer)
}

// ShedProducer is the seam Shed drives once per iteration: Call runs one producer to a verdict
// and reports it, an optional output pointer, and a hard error.
// Two obligations Shed cannot enforce mechanically bind every implementation: return exactly
// Done or Stuck, and surface context cancellation as a non-nil error, never as Stuck.
type ShedProducer interface {
	Call(ctx context.Context) (Outcome, OutputPointer, error)
}

// ProducerDef is one entry in Shed's flat, ordered producer list: the seam plus the routing and
// budget config that governs it, entirely independent of the entry's position in the list.
type ProducerDef struct {
	// Name is the identity current_producer names on disk; it must be non-empty.
	Name string
	// Producer is the seam this entry drives; it must be non-nil.
	Producer ShedProducer
	// OnStuck is what makes a bounce-back a per-producer config value rather than a hardcoded
	// branch in the loop: "" escalates to a human, else Stuck bounces back to the Name it names.
	OnStuck string
	// OnDone is the sole router for a Done verdict, forward or backward through the list. The
	// empty value is load-bearing and silent, exactly as OnStuck: "" already is: an omitted
	// OnDone ends the whole Shed run quietly rather than failing loud, while a non-empty OnDone
	// jumps to the Name it names regardless of this entry's list position.
	OnDone string
	// Segment is a grouping label; "" means standalone. Its only mechanical effect is
	// validate()'s rule that a non-empty OnStuck must name a target sharing this producer's
	// Segment -- it does not scope the bounce budget, and it has no other effect anywhere else.
	Segment string
	// MaxBounces is this producer's own episode Stuck budget. 0 means "inherit Shed.MaxBounces",
	// never "no bounces allowed".
	MaxBounces int
}
