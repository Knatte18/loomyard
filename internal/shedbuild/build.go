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
// Build takes the expanded recipe: a row carrying Block while its engine is not shedrecipe.EngineBlock is a block row that was never expanded, and Build refuses it first, ahead of the loom contract check.
// A row whose engine is shedrecipe.EngineBlock takes the entry gate of the shedrecipe block gate check its BlockGate names, and a BlockGate naming no check is refused.
// Every definition carries its row's SkipTo, and its row's BlockPath as Block.
//
// A recipe declaring `contract: loom` is checked next by checkLoomContract, before any registry lookup, and refused when it breaks the loom contract.
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
	if err := refuseUnexpandedBlockRow(r.Producers); err != nil {
		return nil, err
	}
	if r.Contract == loomContract {
		if err := checkLoomContract(r); err != nil {
			return nil, fmt.Errorf("shedbuild: %w", err)
		}
	}

	defs := make([]shedengine.ProducerDef, len(r.Producers))
	for i, row := range r.Producers {
		gate, err := rowEntryGate(row, env)
		if err != nil {
			return nil, fmt.Errorf("shedbuild: producer %d %q: %w", i, row.Name, err)
		}
		ctor, err := shedrecipe.Lookup(row.Engine)
		if err != nil {
			return nil, fmt.Errorf("shedbuild: producer %d %q: %w", i, row.Name, err)
		}
		producer, err := ctor(shedrecipe.RowIdentity{Name: row.Name, Segment: row.Segment}, shedrecipe.Config(row.Config), env)
		if err != nil {
			return nil, fmt.Errorf("shedbuild: producer %d %q: %w", i, row.Name, err)
		}
		defs[i] = shedengine.ProducerDef{
			Name:       row.Name,
			Producer:   producer,
			OnDone:     row.OnDone,
			OnStuck:    row.OnStuck,
			OnReroute:  row.OnReroute,
			Segment:    row.Segment,
			MaxBounces: row.MaxBounces,
			EntryGate:  gate,
			SkipTo:     row.SkipTo,
			Block:      row.BlockPath,
		}
	}
	return defs, nil
}

// rowEntryGate builds the entry gate of row: the block gate check its BlockGate names for a block row, and its entry_gate checks for any other row, nil for a row with none.
func rowEntryGate(row Row, env shedrecipe.Env) (shedengine.EntryGate, error) {
	if row.Engine == shedrecipe.EngineBlock {
		check, err := shedrecipe.LookupBlockGate(row.BlockGate)
		if err != nil {
			return nil, err
		}
		return check(env)
	}
	checks, err := parseEntryGate(row)
	if err != nil {
		return nil, err
	}
	return entryGateOf(row, checks, env)
}

// refuseUnexpandedBlockRow returns the refusal for the first row of rows that names a block while its engine is not shedrecipe.EngineBlock, nil when there is none.
// Such a row was parsed and never expanded, so no producer can be built for it.
func refuseUnexpandedBlockRow(rows []Row) error {
	for i, row := range rows {
		if row.Block != "" && row.Engine != shedrecipe.EngineBlock {
			return fmt.Errorf("shedbuild: producer %d %q: block row is not expanded; way forward: expand the recipe with shedbuild.Expand before building it", i, row.Name)
		}
	}
	return nil
}
