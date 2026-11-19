// skew.go refuses to render a board copy that a newer lym binary wrote, since the running binary may not understand it.
// A long-running process would otherwise fail mid-run with a bare template error.

package stencilstore

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/buildvcs"
)

// shipped is the registry of the stencils embedded in the running binary, set at package init by RegisterShipped.
// It is only compared against, never rendered from.
// A nil shipped turns the refusal off.
var shipped Registry

// shippedOrdering is the caller's ordering of a recorded writer against the running binary, set by RegisterOrdering.
// A nil pointer means no ordering is registered.
var shippedOrdering atomic.Pointer[func(recorded Writer) Ordering]

// runningWriter is the running binary as a board copy's writer, read once per process.
var runningWriter = sync.OnceValue(func() Writer {
	id := buildvcs.Running()
	return Writer{Revision: id.Revision, Time: id.Time}
})

// NewerBoardCopyError reports that a stencil's board copy was written by a lym binary newer than the running one.
// The running binary's renderer may not understand that copy's text, so the read is refused and the board is left alone.
type NewerBoardCopyError struct {
	Stencil   string
	WrittenBy string
	Running   string
}

// Error names the stencil, the writer's revision, the running revision, and the way forward.
func (e *NewerBoardCopyError) Error() string {
	return fmt.Sprintf("stencilstore: stencil %q was written by lym build %s, which is newer than the running build %s, and this process may not render it; way forward: restart the run's driver on the current lym binary, and update that binary first if it is older than the board", e.Stencil, e.WrittenBy, e.Running)
}

// RegisterShipped names the registry of the stencils embedded in the running binary.
// From then on, a read of a board copy that a newer binary wrote is a *NewerBoardCopyError.
// A copy whose body equals its shipped default is not refused.
// The registry is only compared against; no read returns its bytes.
// The owner of the registry calls it once, from its package init; a second call panics.
func RegisterShipped(registry Registry) {
	if shipped != nil {
		panic("stencilstore: RegisterShipped called a second time")
	}
	shipped = registry
}

// RegisterOrdering names how to order a recorded writer against the running binary, normally by build ancestry.
// A recorded writer counts as newer when older answers RecordedNotOlder and its revision differs from the running one.
// With no ordering registered, or after a nil older, commit time decides.
// Then a recorded writer counts as newer only when its commit time is after the running binary's.
// A writer or running binary with no commit time is never newer.
func RegisterOrdering(older func(recorded Writer) Ordering) {
	if older == nil {
		shippedOrdering.Store(nil)
		return
	}
	shippedOrdering.Store(&older)
}

// refuseNewerBoardCopy returns a *NewerBoardCopyError when a binary newer than the running one wrote onDisk.
// onDisk is the board copy of the stencil name.
// A copy whose body equals the stencil's shipped default is never refused.
// An edited copy is refused the same as an untouched one; neither is written or replaced.
// Every other copy, including any copy of a stencil the binary does not ship, returns nil.
func refuseNewerBoardCopy(name string, onDisk []byte) error {
	if shipped == nil {
		return nil
	}
	def, known := shipped.Default(name)
	if !known {
		return nil
	}
	// The body comparison runs first: it is cheap, and the registered ordering may run git.
	if BodyHash(onDisk) == BodyHash(def) {
		return nil
	}
	running := runningWriter()
	recorded := ParseWriter(onDisk)
	if !writtenByNewerBinary(running, recorded) {
		return nil
	}
	return &NewerBoardCopyError{Stencil: name, WrittenBy: recorded.Revision, Running: running.Revision}
}

// writtenByNewerBinary reports whether recorded names a build newer than running.
// A recorded writer with no revision, or running's own revision, is never newer.
func writtenByNewerBinary(running, recorded Writer) bool {
	if recorded.Revision == "" || recorded.Revision == running.Revision {
		return false
	}
	if older := shippedOrdering.Load(); older != nil {
		return (*older)(recorded) == RecordedNotOlder
	}
	return !running.Time.IsZero() && recorded.Time.After(running.Time)
}
