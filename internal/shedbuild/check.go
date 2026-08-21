// check.go declares this package's authoring-time helper, Check, a thin forward from a Recipe's
// told Entry and Terminals plus an already-built producer list into internal/shedcheck.Check.

package shedbuild

import (
	"github.com/Knatte18/loomyard/internal/shedcheck"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// Check forwards producers, r.Entry, and r.Terminals to shedcheck.Check in that argument order and
// returns its findings unaltered.
//
// Nothing in production calls Check: it exists for a caller's own test suite, at authoring time,
// before an assembled producer list ever reaches a shedengine.Shed, and it exists at all only so a
// caller need not re-derive shedcheck.Check's argument order and so Recipe's Entry and Terminals
// fields have a documented consumer in the package that declares them.
//
// Build deliberately does not call Check, for the same reason internal/shedcheck/doc.go gives for
// keeping it out of every production constructor: a legitimately resumed run starts mid-graph from
// a persisted current_producer, so reachability-from-entry is the wrong question for a production
// build path to ask.
func Check(r Recipe, producers []shedengine.ProducerDef) []shedcheck.Finding {
	return shedcheck.Check(producers, r.Entry, r.Terminals)
}
