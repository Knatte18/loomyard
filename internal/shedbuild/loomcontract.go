// loomcontract.go declares the loom contract check Build runs for a recipe that declares contract: loom.

package shedbuild

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// loomContract is the value of a recipe's contract key that declares it a loom.
const loomContract = "loom"

// checkLoomContract refuses a recipe whose rows, edges or keys break the loom contract.
// The contract is at most one record row and at most one seed row, and at least one of the two.
// A recipe holding both needs the seed row after the record row on the on_done path from the entry.
// Every later clause reads the seed row as the record row when the recipe holds no record row.
// A recipe that lands a change also needs one build row after the record row on the on_done path from the entry.
// Such a recipe also needs a Finalize row followed only by terminals, and a set verify_source.
// A recipe with neither a build row nor a Finalize row is the record-only form:
// it writes and reviews the record and lands nothing, so the record row is its whole contract.
// It reads row engines, edges and the recipe's keys only;
// the engine sets it checks are declared in shedrecipe, so a new loom recipe needs no change here.
// A build row that carries a segment sits in that segment's review ring.
// It then needs a Bouncer row in the segment,
// and an on_done or on_stuck walk from that Bouncer through the segment's rows to it.
// The order clause then reads the build row's place as the Bouncer's.
// The Bouncer is where the ring lies on the path.
// Each refusal names the broken clause and its way forward.
// The first broken clause in that order is the one returned.
func checkLoomContract(r Recipe) error {
	recordRow, err := recordSourceRow(r)
	if err != nil {
		return err
	}
	if recordOnly(r) {
		return nil
	}
	buildRow, err := onlyRowOf(r, shedrecipe.LoomBuildEngines, "build", "Implementer, DarnWrite, BeamAwait, SamplerSession or HardenFix")
	if err != nil {
		return err
	}
	pathRow := buildRow
	if buildRow.Segment != "" {
		if pathRow, err = segmentBouncerReaching(r, buildRow); err != nil {
			return err
		}
	}

	path := onDonePath(r, r.Entry)
	recordAt := slices.Index(path, recordRow.Name)
	buildAt := slices.Index(path, pathRow.Name)
	if recordAt < 0 || buildAt < 0 || buildAt < recordAt {
		return fmt.Errorf("the build row %q does not lie after the record row %q on the on_done path from the entry; way forward: place the build row after the record row on the on_done path from the entry", buildRow.Name, recordRow.Name)
	}

	if err := checkFinalizeEndsInTerminals(r); err != nil {
		return err
	}

	if r.VerifySource == "" {
		return errors.New("the recipe sets no verify_source; way forward: set verify_source to plan or tree_scope")
	}
	return nil
}

// recordSourceRow returns the row that names the source of r's decision record:
// its record row when it holds one, and its seed row otherwise.
// It refuses several record rows, several seed rows, neither,
// and a seed row that does not follow the record row on the on_done path from the entry.
func recordSourceRow(r Recipe) (Row, error) {
	records := rowsOf(r, shedrecipe.LoomRecordEngines)
	seeds := rowsOf(r, []string{shedrecipe.LoomSeedEngine})
	switch {
	case len(records) > 1:
		return Row{}, fmt.Errorf("the recipe holds %d record rows, more than one; way forward: keep at most one record row, engine DiscussionWrite or RecordWrite", len(records))
	case len(seeds) > 1:
		return Row{}, fmt.Errorf("the recipe holds %d seed rows, more than one; way forward: keep at most one seed row, engine %s", len(seeds), shedrecipe.LoomSeedEngine)
	case len(records) == 0 && len(seeds) == 0:
		return Row{}, fmt.Errorf("the recipe holds no record row and no seed row; way forward: give the recipe one record row, engine DiscussionWrite or RecordWrite, or one seed row, engine %s", shedrecipe.LoomSeedEngine)
	case len(records) == 0:
		return seeds[0], nil
	case len(seeds) == 0:
		return records[0], nil
	}

	path := onDonePath(r, r.Entry)
	recordAt := slices.Index(path, records[0].Name)
	seedAt := slices.Index(path, seeds[0].Name)
	if recordAt < 0 || seedAt < recordAt {
		return Row{}, fmt.Errorf("the seed row %q does not lie after the record row %q on the on_done path from the entry; way forward: place the seed row after the record row on the on_done path from the entry", seeds[0].Name, records[0].Name)
	}
	return records[0], nil
}

// rowsOf returns the rows of r whose engine is in engines, in row order.
func rowsOf(r Recipe, engines []string) []Row {
	var found []Row
	for _, row := range r.Producers {
		if slices.Contains(engines, row.Engine) {
			found = append(found, row)
		}
	}
	return found
}

// segmentBouncerEngine is the engine of the row that gates a segment's review ring.
const segmentBouncerEngine = "Bouncer"

// segmentBouncerReaching returns the Bouncer row of build's segment from which an on_done or on_stuck walk reaches build.
// The walk passes only through rows of that segment.
// It refuses a segment with no Bouncer row, and one whose Bouncer rows reach build by no edge.
func segmentBouncerReaching(r Recipe, build Row) (Row, error) {
	var bouncers []Row
	for _, row := range r.Producers {
		if row.Segment == build.Segment && row.Engine == segmentBouncerEngine {
			bouncers = append(bouncers, row)
		}
	}
	if len(bouncers) == 0 {
		return Row{}, fmt.Errorf("the build row %q sits in segment %q, which holds no Bouncer row; way forward: add the segment's Bouncer row", build.Name, build.Segment)
	}
	for _, bouncer := range bouncers {
		if ringReaches(r, bouncer, build) {
			return bouncer, nil
		}
	}
	return Row{}, fmt.Errorf("the build row %q is reached by no on_done or on_stuck edge from its segment's Bouncer %q; way forward: add the edge from a row of the ring to the build row", build.Name, bouncers[0].Name)
}

// ringReaches reports whether an on_done or on_stuck walk from start, through the rows of start's segment, meets target.
func ringReaches(r Recipe, start, target Row) bool {
	inSegment := make(map[string]Row, len(r.Producers))
	for _, row := range r.Producers {
		if row.Segment == start.Segment {
			inSegment[row.Name] = row
		}
	}
	seen := map[string]bool{start.Name: true}
	for queue := []Row{start}; len(queue) > 0; queue = queue[1:] {
		for _, next := range []string{queue[0].OnDone, queue[0].OnStuck} {
			if next == target.Name {
				return true
			}
			if row, ok := inSegment[next]; ok && !seen[next] {
				seen[next] = true
				queue = append(queue, row)
			}
		}
	}
	return false
}

// recordOnly reports whether r holds neither a build row nor a Finalize row, the loom contract's record-only form.
func recordOnly(r Recipe) bool {
	return !slices.ContainsFunc(r.Producers, func(row Row) bool {
		return slices.Contains(shedrecipe.LoomBuildEngines, row.Engine) || row.Engine == shedrecipe.LoomFinalizeEngine
	})
}

// onlyRowOf returns the one row of r whose engine is in engines.
// It refuses zero or several such rows, and its error names role and the engine choice.
func onlyRowOf(r Recipe, engines []string, role, engineChoice string) (Row, error) {
	found := rowsOf(r, engines)
	if len(found) != 1 {
		return Row{}, fmt.Errorf("the recipe holds %d %s rows, not exactly one; way forward: give the recipe exactly one %s row, engine %s", len(found), role, role, engineChoice)
	}
	return found[0], nil
}

// checkFinalizeEndsInTerminals refuses a recipe with no Finalize row, or one whose on_done path leads to a row that is not a terminal.
func checkFinalizeEndsInTerminals(r Recipe) error {
	const wayForward = "way forward: end the recipe's on_done path at Finalize followed only by terminals"
	finalizeSeen := false
	for _, row := range r.Producers {
		if row.Engine != shedrecipe.LoomFinalizeEngine {
			continue
		}
		finalizeSeen = true
		for _, name := range onDonePath(r, row.Name)[1:] {
			if !slices.Contains(r.Terminals, name) {
				return fmt.Errorf("row %q follows %q on the on_done path but is not a terminal; %s", name, row.Name, wayForward)
			}
		}
	}
	if !finalizeSeen {
		return fmt.Errorf("the recipe holds no %s row; %s", shedrecipe.LoomFinalizeEngine, wayForward)
	}
	return nil
}

// onDonePath returns the row names met by following on_done from start, start first.
// The walk stops at an empty on_done, at a name no row holds, or at a row already met.
func onDonePath(r Recipe, start string) []string {
	byName := make(map[string]Row, len(r.Producers))
	for _, row := range r.Producers {
		byName[row.Name] = row
	}

	var path []string
	for name := start; name != ""; {
		if slices.Contains(path, name) {
			break
		}
		path = append(path, name)
		row, ok := byName[name]
		if !ok {
			break
		}
		name = row.OnDone
	}
	return path
}
