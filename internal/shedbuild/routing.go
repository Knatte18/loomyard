// routing.go declares RoutingOf, the projection of a recipe's routing that builds no engines.

package shedbuild

import "github.com/Knatte18/loomyard/internal/shedengine"

// RoutingOf parses recipe and projects its graph into a shedengine.Routing: Entry, plus each row's
// Name, OnDone, OnStuck, Segment and MaxBounces as a shedengine.ProducerDef with a nil Producer.
//
// RoutingOf parses through Parse, the sole recipe parser, and calls no registry entry, so it needs
// no shedrecipe.Env and constructs no engine. That is what lets a status verb, which never builds
// a Shed, compute recipe progress.
//
// The returned Routing.MaxBounces is 0: the shed-level default travels in ShedPaths, and the
// arming module fills it.
//
// RoutingOf returns Parse's error unwrapped, as NewShed does; its callers add their own package
// prefix.
func RoutingOf(recipe []byte) (shedengine.Routing, error) {
	parsed, err := Parse(recipe)
	if err != nil {
		return shedengine.Routing{}, err
	}

	producers := make([]shedengine.ProducerDef, len(parsed.Producers))
	for i, row := range parsed.Producers {
		producers[i] = shedengine.ProducerDef{
			Name:       row.Name,
			OnDone:     row.OnDone,
			OnStuck:    row.OnStuck,
			Segment:    row.Segment,
			MaxBounces: row.MaxBounces,
		}
	}

	return shedengine.Routing{Entry: parsed.Entry, Producers: producers}, nil
}
