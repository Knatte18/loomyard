// build.go declares this package's assembly entry point, Build.
// It resolves each row's engine through the shedrecipe registry;
// it calls the constructor it returns to produce the []shedengine.ProducerDef that shedengine.Shed consumes.

package shedbuild

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// Build resolves each row of r against the shedrecipe registry and calls the returned constructor with env.
// It produces the []shedengine.ProducerDef that shedengine.Shed consumes as is.
//
// Build returns an error when r has no producers, because it also accepts a Recipe that was not parsed.
//
// Build takes the expanded recipe.
// A row carrying Block whose engine is not shedrecipe.EngineBlock was never expanded;
// Build refuses it ahead of the loom contract check.
// A row whose engine is shedrecipe.EngineBlock takes the entry gate of the block gate check its BlockGate names;
// a BlockGate naming no check is refused.
// Every definition carries its row's SkipTo, and its row's BlockPath as Block.
//
// A recipe declaring `contract: loom` is checked for the loom contract before any registry lookup;
// it is refused when it breaks the contract.
//
// Build runs no reachability, cycle, blind-gate, dangling-target or segment analysis;
// it never calls internal/shedcheck.
// A legitimately resumable graph starts mid-graph, so reachability from the entry is the wrong question at build time.
// shedengine's own validation rejects dangling targets, duplicate names and cross-segment OnStuck before a run starts.
//
// Build is not filesystem-free: some registry constructors reach disk at construction time.
// Build passes those effects through without suppressing or wrapping them.
// internal/shedbuild/doc.go lists which constructor produces which effect.
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

// rowEntryGate builds the entry gate of row.
// A block row takes the block gate check its BlockGate names; any other row takes its entry_gate checks.
// A row with no entry_gate checks has a nil gate.
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

// refuseUnexpandedBlockRow returns the refusal for the first row that names a block but is not an EngineBlock row.
// It returns nil when no row does.
// Such a row was parsed and never expanded, so no producer can be built for it.
func refuseUnexpandedBlockRow(rows []Row) error {
	for i, row := range rows {
		if row.Block != "" && row.Engine != shedrecipe.EngineBlock {
			return fmt.Errorf("shedbuild: producer %d %q: block row is not expanded; way forward: expand the recipe with shedbuild.Expand before building it", i, row.Name)
		}
	}
	return nil
}
