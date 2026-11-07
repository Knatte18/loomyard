// routing.go declares RoutingOf and RoutingFrom, the projections of a recipe's routing that build no engines.

package shedbuild

import "github.com/Knatte18/loomyard/internal/shedengine"

// RoutingOf parses recipe and projects its graph into a shedengine.Routing.
//
// RoutingOf parses recipe with Parse and calls no registry entry,
// so it needs no shedrecipe.Env and constructs no engine.
// That lets a status verb compute recipe progress without building a Shed.
//
// A recipe holding a block row is refused: expand it with Expand, then project the result with RoutingFrom.
//
// The returned Routing.MaxBounces is 0: the shed-level default travels in ShedPaths, and the arming module fills it.
//
// RoutingOf returns Parse's error unwrapped; its callers add their own package prefix.
func RoutingOf(recipe []byte) (shedengine.Routing, error) {
	parsed, err := Parse(recipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	if err := refuseUnexpandedBlockRow(parsed.Producers); err != nil {
		return shedengine.Routing{}, err
	}

	return RoutingFrom(parsed), nil
}

// RoutingFrom projects the graph of an already-expanded recipe into a shedengine.Routing with the same Entry.
// Each row becomes a shedengine.ProducerDef with a nil Producer, and its Block is the row's BlockPath.
//
// RoutingFrom calls no registry entry and parses nothing, so it needs no shedrecipe.Env and constructs no engine.
// The returned Routing.MaxBounces is 0: the shed-level default travels in ShedPaths, and the arming module fills it.
func RoutingFrom(r Recipe) shedengine.Routing {
	producers := make([]shedengine.ProducerDef, len(r.Producers))
	for i, row := range r.Producers {
		producers[i] = shedengine.ProducerDef{
			Name:       row.Name,
			OnDone:     row.OnDone,
			OnStuck:    row.OnStuck,
			OnReroute:  row.OnReroute,
			Segment:    row.Segment,
			MaxBounces: row.MaxBounces,
			SkipTo:     row.SkipTo,
			Block:      row.BlockPath,
		}
	}

	return shedengine.Routing{Entry: r.Entry, Producers: producers}
}
