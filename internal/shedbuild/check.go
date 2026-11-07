// check.go declares this package's authoring-time helper, Check.
// Check forwards a Recipe's Entry and Terminals, with a built producer list, to internal/shedcheck.Check.

package shedbuild

import (
	"github.com/Knatte18/loomyard/internal/shedcheck"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// Check forwards producers, r.Entry and r.Terminals to shedcheck.Check in that argument order;
// it returns its findings unaltered.
//
// Nothing in production calls Check;
// it serves a caller's own test suite at authoring time, before a producer list reaches shedengine.Shed.
// It exists so a caller need not re-derive shedcheck.Check's argument order.
// It also gives Recipe's Entry and Terminals a consumer in this package.
//
// A resumed run starts mid-graph from a persisted current_producer;
// reachability from the entry is the wrong question for a production build path.
func Check(r Recipe, producers []shedengine.ProducerDef) []shedcheck.Finding {
	return shedcheck.Check(producers, r.Entry, r.Terminals)
}
