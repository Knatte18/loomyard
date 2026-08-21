// build.go declares this package's assembly entry point, Build, which resolves each row's engine
// through the shedrecipe registry and calls the returned constructor to produce the
// []shedengine.ProducerDef that shedengine.Shed already consumes unchanged.

package shedbuild

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// Build resolves each row of r against the shedrecipe registry and calls the returned constructor
// with env, producing the []shedengine.ProducerDef that shedengine.Shed already consumes
// unchanged.
//
// Build returns an error naming r has no producers when r.Producers is empty -- defence in depth,
// since Parse already rejects that, but Build also accepts a hand-built Recipe that never went
// through Parse.
//
// Build runs no reachability, cycle, blind-gate, dangling-target, or segment analysis, and never
// calls into internal/shedcheck: a legitimately resumable graph starts mid-graph, so
// reachability-from-entry is the wrong question at build time, and shedengine's own validation
// already rejects dangling targets, duplicate names, and cross-segment OnStuck before a run
// touches anything.
//
// Build is not filesystem-free: it is a pass-through for the construction-time filesystem effects
// some registry constructors have of their own accord -- see internal/shedbuild/doc.go for the
// single site enumerating which constructor produces which effect; this package neither
// suppresses nor wraps those effects.
func Build(r Recipe, env shedrecipe.Env) ([]shedengine.ProducerDef, error) {
	if len(r.Producers) == 0 {
		return nil, fmt.Errorf("shedbuild: recipe has no producers")
	}

	defs := make([]shedengine.ProducerDef, len(r.Producers))
	for i, row := range r.Producers {
		ctor, err := shedrecipe.Lookup(row.Engine)
		if err != nil {
			return nil, fmt.Errorf("shedbuild: producer %d %q: %w", i, row.Name, err)
		}
		producer, err := ctor(row.Name, shedrecipe.Config(row.Config), env)
		if err != nil {
			return nil, fmt.Errorf("shedbuild: producer %d %q: %w", i, row.Name, err)
		}
		defs[i] = shedengine.ProducerDef{
			Name:       row.Name,
			Producer:   producer,
			OnDone:     row.OnDone,
			OnStuck:    row.OnStuck,
			Segment:    row.Segment,
			MaxBounces: row.MaxBounces,
		}
	}
	return defs, nil
}
